package emergency

import (
	"sync"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// The IDs of the call-out job types.
const (
	AmbulanceID = "callout-ambulance"
	FireID      = "callout-fire"
	ResponseID  = "callout-response"
)

// Call-out tuning, in metres, seconds and metres a second.
const (
	// StopSpeed is the speed counted as at rest.
	StopSpeed float32 = 0.5
	// PatientRadius, HospitalRadius and SceneRadius are how close the car has
	// to be to each. The scene is the box a fire engine has to stop inside.
	PatientRadius  float32 = 6
	HospitalRadius float32 = 8
	SceneRadius    float32 = 10
	// HoseTime is how long a fire engine holds at its scene, and AttendTime how
	// long a response car does.
	HoseTime   float32 = 20
	AttendTime float32 = 4
	// pace is the share of the limit the clock allows for: a call is urgent.
	pace  float32 = 0.7
	slack float32 = 30
	// A straight line is shorter than the road, so the clock is given more.
	detour float32 = 1.4
	// streakStep is the extra pay for each call-out passed in a row, as a
	// percentage, and streakCap the most such calls count.
	streakStep = 10
	streakCap  = 5
)

// Run is the player's streak of call-outs: how many have been passed in a row.
// A call-out that is failed, however it fails, ends it. It is for the whole
// session, and is only ever worth anything as a bonus on the pay of the next.
var Run Streak

// Streak counts call-outs passed in a row.
type Streak struct {
	mu sync.Mutex
	n  int
}

// N is how many have been passed in a row.
func (s *Streak) N() int { s.mu.Lock(); defer s.mu.Unlock(); return s.n }

// Reset ends the streak.
func (s *Streak) Reset() { s.mu.Lock(); s.n = 0; s.mu.Unlock() }

func (s *Streak) settle(passed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if passed {
		s.n++
	} else {
		s.n = 0
	}
}

// Kinds of call-out.
type callout struct {
	id, name string
	base     int // pay before the length of the call is counted
	// stay is how long the car must be held at the last goal.
	stay float32
}

// Ambulance is a call-out that takes a patient to a hospital, Fire sends a
// fire engine to hold at the scene of a fire, and Response sends a car to
// attend a station's call.
var (
	Ambulance = callout{id: AmbulanceID, name: "Ambulance call", base: 200}
	Fire      = callout{id: FireID, name: "Fire call", base: 250, stay: HoseTime}
	Response  = callout{id: ResponseID, name: "Response call", base: 120, stay: AttendTime}
)

// All lists the call-out types, for registering.
func All() []job.Type { return []job.Type{Ambulance, Fire, Response} }

// ID names the type.
func (c callout) ID() string { return c.id }

// Name is what the player reads.
func (c callout) Name() string { return c.name }

// Rules allow no pursuit and charge for penalty points.
func (callout) Rules() job.Rules { return job.Rules{CostPerPoint: 10} }

// Settle ends or extends the streak: a passed call-out adds to it, and any
// other ending, a time-out or a police stop included, ends it.
func (callout) Settle(r job.Result) { Run.settle(r.Status == job.Passed) }

// Reward is the base pay, more the longer the call allowed, and more again for
// every call in a row before it.
func (c callout) Reward(s job.State) int {
	pay := c.base + int(s.Deadline-s.Start)*2
	return pay * (100 + streakStep*min(s.Streak, streakCap)) / 100
}

func cellOf(v float32) int {
	if v < 0 {
		return int(v/100) - 1
	}
	return int(v / 100)
}

// choose picks one of n by a hash of the call and where the player stands,
// so a few metres either way change nothing.
func choose(seed uint64, serial int, from mathx.Vec, n int) int {
	r := city.Chance(seed^uint64(serial)*0x9e3779b97f4a7c15, pickSalt, cellOf(from.X), cellOf(from.Z))
	return min(int(r*float32(n)), n-1)
}

// allow is how long a distance takes at the pace of the call on a lane.
func allow(dist float32, l *city.Lane) float32 {
	return dist / (pace * max(l.SpeedLimit, 1))
}

// Plan sends the player to the call.
func (c callout) Plan(cty *city.City, seed uint64, serial int, from mathx.Vec, now float32) (job.State, bool) {
	ix := NewIndex(cty)
	ix.Sync()
	s := job.State{Start: now, Streak: Run.N()}
	switch c.id {
	case AmbulanceID:
		t, ok := job.PickTarget(cty, seed, serial, from, 300, 700)
		if !ok {
			return job.State{}, false
		}
		h, ok := ix.Nearest(Hospital, t.Pos)
		if !ok {
			return job.State{}, false
		}
		s.Goals = []job.Goal{
			{Pos: t.Pos, Radius: PatientRadius, Stop: true},
			{Pos: h.Door, Radius: HospitalRadius, Stop: true},
		}
		leg := h.Door.DistTo(t.Pos) * detour
		s.Deadline = now + t.Route/(pace*max(t.Lane.SpeedLimit, 1)) + allow(leg, cty.Lanes[h.Lane]) + slack
	default:
		kind := FireSite
		if c.id == ResponseID {
			kind = Station
		}
		at := cands(ix, kind, from)
		if len(at) == 0 {
			if kind == FireSite {
				return job.State{}, false
			}
			// A station may not be built yet: attend somewhere on the road.
			t, ok := job.PickTarget(cty, seed, serial, from, 250, 700)
			if !ok {
				return job.State{}, false
			}
			s.Goals = []job.Goal{{Pos: t.Pos, Radius: SceneRadius, Stop: true}}
			s.Deadline = now + t.Route/(pace*max(t.Lane.SpeedLimit, 1)) + slack + c.stay
			return s, true
		}
		site := at[choose(seed, serial, from, len(at))]
		s.Goals = []job.Goal{{Pos: site.Door, Radius: SceneRadius, Stop: true}}
		s.Deadline = now + allow(site.Door.DistTo(from)*detour, cty.Lanes[site.Lane]) + slack + c.stay
	}
	return s, true
}

// cands are the sites of a kind a call may send the player to.
func cands(ix *Index, k Kind, from mathx.Vec) []Site {
	return ix.Near(k, from, 250, 1000)
}

// Step moves the job on. A goal is reached by coming to rest inside it. The
// last of a fire's or a response's is held for as long as it asks: moving,
// or leaving the scene, starts the hold again.
func (c callout) Step(s *job.State, f job.Frame) {
	if s.Next >= len(s.Goals) {
		return
	}
	g := s.Goals[s.Next]
	at := f.Pos.DistTo(g.Pos) <= g.Radius && f.Speed < StopSpeed
	last := s.Next == len(s.Goals)-1
	switch {
	case !at:
		s.Held = 0
	case !last || c.stay == 0:
		s.Next++
	case s.Held == 0:
		s.Held = f.Now
	case f.Now-s.Held >= c.stay:
		s.Next++
	}
	if s.Next >= len(s.Goals) {
		s.Status = job.Passed
	}
}

// Hosing reports whether a job is a fire engine at its scene with the hose
// out, and how far through its hold it is, from zero to one.
func Hosing(s job.State, now float32) (float32, bool) {
	if s.Type != FireID || s.Held == 0 || s.Status != job.Active {
		return 0, false
	}
	return mathx.Clamp((now-s.Held)/HoseTime, 0, 1), true
}
