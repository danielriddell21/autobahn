package race

import (
	"sync"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Drift tuning.
const (
	// DriftRun is how long a drift event lasts once the car is in the zone, in
	// seconds, and DriftZone how far from the middle of it a drift counts.
	DriftRun         float32 = 60
	DriftZone        float32 = 45
	driftScale       float32 = 10
	driftDeadband    float32 = 0.1 // radians of slip below which the car is only turning
	driftSpin        float32 = 1.2 // radians of slip beyond which it is spinning, not drifting
	driftMinSpeed    float32 = 5
	driftSilver      float32 = 1.5
	driftGold        float32 = 2
	driftSalt        uint64  = 0x38c2f6a1e95d704b
	driftBronzeBase  float32 = 1500
	driftBronzeRange float32 = 2000
)

// SlipAngle is the angle between the way a car is pointing and the way it is
// going, in the car's own frame: positive when it slides to the right of its
// nose. Cars are driven on the plane, so a hill does not skew it. A car that
// is hardly moving, or going backwards, has none that means anything.
func SlipAngle(vel mathx.Vec, heading float32) float32 {
	if vel.Len() < 1 {
		return 0
	}
	fwd := mathx.FromAngle(heading)
	long := vel.Dot(fwd)
	if long <= 0 {
		return 0
	}
	return mathx.Atan2(vel.Dot(fwd.Right()), long)
}

// SlipScore is what a moment of sliding is worth: the slip angle times the
// speed, over the time it lasted. Driving straight scores nothing, and neither
// does a spin or a crawl.
func SlipScore(slip, speed, dt float32) float32 {
	a := mathx.Abs(slip)
	if a < driftDeadband || a > driftSpin || speed < driftMinSpeed {
		return 0
	}
	return a * speed * dt * driftScale
}

// Targets are the scores that earn the bronze, silver and gold medals of a drift
// event, which is a function of the event.
func Targets(seed uint64, serial int) (bronze, silver, gold float32) {
	r := city.Chance(seed^uint64(serial)*0x9e3779b97f4a7c15, driftSalt, serial, 0)
	bronze = float32(int((driftBronzeBase+driftBronzeRange*r)/100)) * 100
	return bronze, bronze * driftSilver, bronze * driftGold
}

// DriftMedal is the medal a score earns, or "" for none.
func DriftMedal(score float32, seed uint64, serial int) string {
	b, s, g := Targets(seed, serial)
	switch {
	case score >= g:
		return "gold"
	case score >= s:
		return "silver"
	case score >= b:
		return "bronze"
	}
	return ""
}

// Drift is the drift event: score as much slip times speed as there is time for,
// inside a zone on the streets.
type Drift struct{}

// ID names the type.
func (Drift) ID() string { return DriftID }

// Name is what the player reads.
func (Drift) Name() string { return "Drift event" }

// Rules charge for penalty points and allow no pursuit.
func (Drift) Rules() job.Rules { return job.Rules{CostPerPoint: 10} }

// Plan picks a zone a few hundred metres off.
func (Drift) Plan(c *city.City, seed uint64, serial int, from mathx.Vec, now float32) (job.State, bool) {
	t, ok := job.PickTarget(c, seed, serial, from, 150, 500)
	if !ok {
		return job.State{}, false
	}
	return job.State{
		Start:    now,
		Goals:    []job.Goal{{Pos: t.Pos, Radius: DriftZone}},
		Deadline: now + t.Route/8 + DriftRun + 5,
	}, true
}

// Reward is paid by the medal, by how hard it was to earn.
func (Drift) Reward(s job.State) int {
	b, _, _ := Targets(s.Seed, s.Serial)
	unit := int(b / 10)
	switch s.Reason {
	case "gold":
		return unit * 3
	case "silver":
		return unit * 2
	case "bronze":
		return unit
	}
	return 0
}

// Sample is what the host tells a drift of the car each frame: the slip is
// worked out here from the car's own frame, so the host need not know how.
type Sample struct {
	Now  float32
	Pos  mathx.Vec
	Vel  mathx.Vec
	Yaw  float32 // the way the nose points
	Step float32 // the length of the frame, in seconds
}

var scores = scoreboard{m: map[*job.State]float32{}}

type scoreboard struct {
	mu sync.Mutex
	m  map[*job.State]float32
}

// Feed adds a frame of driving to a drift event under way. Only the time spent
// in the zone counts, and the clock of the event starts as the car enters it.
func Feed(s *job.State, m Sample) {
	if s == nil || s.Type != DriftID || s.Status != job.Active || len(s.Goals) == 0 {
		return
	}
	g := s.Goals[0]
	if m.Pos.DistTo(g.Pos) > g.Radius {
		return
	}
	if s.Held == 0 {
		s.Held = m.Now
	}
	scores.mu.Lock()
	scores.m[s] += SlipScore(SlipAngle(m.Vel, m.Yaw), m.Vel.Len(), m.Step)
	scores.mu.Unlock()
}

// Score is how much a drift event has scored so far.
func Score(s *job.State) float32 {
	scores.mu.Lock()
	defer scores.mu.Unlock()
	return scores.m[s]
}

func (b *scoreboard) forget(s *job.State) {
	b.mu.Lock()
	delete(b.m, s)
	if len(b.m) > 16 {
		for k := range b.m {
			if k.Status != job.Active {
				delete(b.m, k)
			}
		}
	}
	b.mu.Unlock()
}

// Step ends the event: at once on the gold score, and otherwise when the run is
// over, on the medal the score has earned, or failed without one. The clock
// starts when the car first enters the zone, which Feed sets as Held.
func (Drift) Step(s *job.State, f job.Frame) {
	score := Score(s)
	over := s.Held > 0 && f.Now-s.Held >= DriftRun
	med := DriftMedal(score, s.Seed, s.Serial)
	switch {
	case med == "gold" || over && med != "":
		s.Status, s.Reason = job.Passed, med
	case over:
		s.Status, s.Reason = job.Failed, "not enough drift"
	default:
		return
	}
	scores.forget(s)
}
