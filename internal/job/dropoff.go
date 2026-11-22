package job

import (
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// DropOff is the base job: drive to a point and stop within a few metres of it.
type DropOff struct{}

// DropOffID names the type.
const DropOffID = "dropoff"

const (
	dropRadius  float32 = 6
	dropMin     float32 = 400
	dropMax     float32 = 900
	dropSlack   float32 = 20  // seconds
	dropPace    float32 = 0.6 // share of the limit the clock allows for
	dropStopped float32 = 0.5 // m/s counted as at rest
)

func init() { Register(DropOff{}) }

// ID names the type.
func (DropOff) ID() string { return DropOffID }

// Name is what the player reads.
func (DropOff) Name() string { return "Drop-off" }

// Rules allow no pursuit and charge for penalty points.
func (DropOff) Rules() Rules { return Rules{CostPerPoint: 10} }

// Plan picks a place 400 to 900 m away by road.
func (DropOff) Plan(c *city.City, seed uint64, serial int, from mathx.Vec, now float32) (State, bool) {
	t, ok := PickTarget(c, seed, serial, from, dropMin, dropMax)
	if !ok {
		return State{}, false
	}
	limit := max(t.Lane.SpeedLimit, 1)
	return State{
		Goals:    []Goal{{Pos: t.Pos, Radius: dropRadius, Stop: true}},
		Start:    now,
		Deadline: now + t.Route/(dropPace*limit) + dropSlack,
	}, true
}

// Step passes the job when the car is at rest at the goal.
func (DropOff) Step(s *State, f Frame) {
	g := s.Goals[0]
	if f.Pos.DistTo(g.Pos) <= g.Radius && f.Speed < dropStopped {
		s.Next, s.Status = 1, Passed
	}
}

// Reward grows with the time the job allowed, which is its length.
func (DropOff) Reward(s State) int { return 100 + int(s.Deadline-s.Start)*2 }
