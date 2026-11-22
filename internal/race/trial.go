package race

import (
	"sync"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// The IDs of the race job types.
const (
	TrialID   = "race-trial"
	CircuitID = "race-circuit"
	SprintID  = "race-sprint"
	DriftID   = "race-drift"
)

// Race tuning.
const (
	// parPace is the share of the limit a gold time averages, and trafficAllow
	// the extra it allows for the traffic that stays on the road.
	parPace      float32 = 1.1
	trafficAllow float32 = 1.15
	// cornerTime is what each corner costs, in seconds.
	cornerTime float32 = 2.5
	// silverAt and bronzeAt are the medal times as shares of par.
	silverAt float32 = 1.15
	bronzeAt float32 = 1.35
	// raceSlack is how long a race is given, as a share of par, before it is
	// lost: a long way behind the slowest rival.
	raceSlack float32 = 1.6
	// circuitLaps is how many times a circuit goes round.
	circuitLaps = 3
	rivalCount  = 3
)

// Par is the gold time over a route of a length at a mean limit with a number
// of corners, in seconds.
func Par(length, limit float32, corners int) float32 {
	return length/(max(limit, 1)*parPace)*trafficAllow + float32(corners)*cornerTime
}

// Medal is the medal a time trial earns, or "" for a time too slow for any.
func Medal(elapsed, par float32) string {
	switch {
	case elapsed <= par:
		return "gold"
	case elapsed <= par*silverAt:
		return "silver"
	case elapsed <= par*bronzeAt:
		return "bronze"
	}
	return ""
}

// Ghost is the time a time trial's ghost sets, as a number only: nothing is
// drawn, it is the gold time to beat.
func Ghost(s job.State) float32 {
	e, ok := eventOf(s.Type)
	if !ok {
		return 0
	}
	return e.par(s)
}

// tierOf is how much of the pay a verdict earns, as an index into pay: the
// first place or the gold medal is zero.
func tierOf(reason string) int {
	switch reason {
	case "gold", "1st":
		return 0
	case "silver", "2nd":
		return 1
	case "bronze", "3rd":
		return 2
	}
	return 3
}

// event is a race over checkpoints: a time trial against the clock, a circuit
// round a loop, or a street race from one place to another. Circuits and street
// races have rivals.
type event struct {
	id, name  string
	laps      int // times round; 1 for anything but a circuit
	rivals    int
	maxWanted int
	slack     float32 // the clock allowed, as a share of par
	// pay is the reward per second of par, by tier. The last is for finishing
	// without a medal or a place worth anything.
	pay [4]int
	// loop is set for the events run round a loop.
	loop bool
}

// Trial, Circuit and Sprint are the three checkpoint races.
var (
	Trial   = event{id: TrialID, name: "Time trial", laps: 1, slack: bronzeAt, pay: [4]int{5, 3, 2, 0}, loop: true}
	Circuit = event{id: CircuitID, name: "Circuit race", laps: circuitLaps, rivals: rivalCount, slack: raceSlack, pay: [4]int{8, 5, 3, 0}, maxWanted: 1, loop: true}
	Sprint  = event{id: SprintID, name: "Street race", laps: 1, rivals: rivalCount, slack: raceSlack, pay: [4]int{7, 4, 2, 0}, maxWanted: 2}
)

// All lists the race job types, for registering.
func All() []job.Type { return []job.Type{Trial, Circuit, Sprint, Drift{}} }

func eventOf(id string) (event, bool) {
	for _, e := range []event{Trial, Circuit, Sprint} {
		if e.id == id {
			return e, true
		}
	}
	return event{}, false
}

// ID names the type.
func (e event) ID() string { return e.id }

// Name is what the player reads.
func (e event) Name() string { return e.name }

// Rules charge for penalty points; a street race survives a little police
// attention, which ordinary speeding earns, and the rest do not.
func (e event) Rules() job.Rules { return job.Rules{MaxWanted: e.maxWanted, CostPerPoint: 10} }

// par is the gold time of a job, from the clock it was given.
func (e event) par(s job.State) float32 { return (s.Deadline - s.Start) / e.slack }

// Reward is paid by the medal or the place, by the length of the race. A
// last place pays nothing.
func (e event) Reward(s job.State) int {
	return int(e.par(s)) * e.pay[tierOf(s.Reason)]
}

// Plan lays the race on the streets near from.
func (e event) Plan(c *city.City, seed uint64, serial int, from mathx.Vec, now float32) (job.State, bool) {
	var gates []Gate
	var length, limit float32
	var corners int
	if e.loop {
		l, ok := PlanLoop(c, seed, serial, from)
		if !ok {
			return job.State{}, false
		}
		gates, length, limit, corners = l.Gates, l.Length, l.Limit, l.Corners
		// Start at the first gate, round to it again.
		gates = append(gates[1:len(gates):len(gates)], gates[0])
		length *= float32(e.laps)
		corners *= e.laps
		gates = lapped(gates, e.laps)
	} else {
		r, ok := PlanRoute(c, seed, serial, from)
		if !ok {
			return job.State{}, false
		}
		gates, length, limit, corners = r.Gates, r.Length, r.Limit, r.Corners
	}
	s := job.State{Start: now}
	for _, g := range gates {
		s.Goals = append(s.Goals, job.Goal{Pos: g.Pos, Radius: g.Half})
	}
	s.Deadline = now + Par(length, limit, corners)*e.slack
	return s, true
}

// lapped repeats a lap's gates, so a race is the whole run of them.
func lapped(lap []Gate, laps int) []Gate {
	out := make([]Gate, 0, len(lap)*laps)
	for range laps {
		out = append(out, lap...)
	}
	return out
}

// Step advances the race: the next checkpoint is passed when the car's move
// comes within its radius, and no other counts, so the order is the race's. The
// last ends it. On a pass, Held is how long it took, and Reason the medal or
// the place.
func (e event) Step(s *job.State, f job.Frame) {
	prev, had := trace.swap(s, f.Pos)
	if !had || s.Next >= len(s.Goals) {
		return
	}
	g := s.Goals[s.Next]
	if !Reached(prev, f.Pos, g.Pos, g.Radius) {
		return
	}
	if s.Next++; s.Next < len(s.Goals) {
		return
	}
	elapsed, par := f.Now-s.Start, e.par(*s)
	s.Held, s.Status = elapsed, job.Passed
	if e.rivals == 0 {
		s.Reason = Medal(elapsed, par)
	} else {
		s.Reason = Ordinal(Place(elapsed, par, Field(s.Seed, s.Serial, e.rivals)))
	}
	trace.forget(s)
}

// Laps says which lap a job is on and how many there are. A race that is not a
// circuit has the one.
func Laps(s job.State) (lap, of int) {
	e, ok := eventOf(s.Type)
	if !ok || e.laps <= 1 || len(s.Goals) == 0 {
		return 1, 1
	}
	return min(s.Next/(len(s.Goals)/e.laps), e.laps-1) + 1, e.laps
}

// Rivals is the field of a job.
func Rivals(s job.State) []Rival {
	e, ok := eventOf(s.Type)
	if !ok {
		return nil
	}
	return Field(s.Seed, s.Serial, e.rivals)
}

// trace remembers where each job's car was a frame ago, which is what tells a
// gate crossed from a gate near. Plain job data has no room for it.
var trace = tracer{at: map[*job.State]mathx.Vec{}}

type tracer struct {
	mu sync.Mutex
	at map[*job.State]mathx.Vec
}

// swap records where the car is and returns where it was.
func (t *tracer) swap(s *job.State, p mathx.Vec) (mathx.Vec, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	prev, ok := t.at[s]
	if !ok && len(t.at) > 16 {
		for k := range t.at {
			if k.Status != job.Active {
				delete(t.at, k)
			}
		}
	}
	t.at[s] = p
	return prev, ok
}

func (t *tracer) forget(s *job.State) {
	t.mu.Lock()
	delete(t.at, s)
	t.mu.Unlock()
}
