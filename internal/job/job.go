// Package job is the framework for jobs: tasks the player takes on while
// driving, with a goal, a clock and a reward.
//
// It sees the world only through the plain data in a [Frame] and the city it
// plans on, so it imports nothing of the simulation. A job is a [State], which
// is plain JSON-able data; what a kind of job does with it is a [Type], which
// packs add by calling [Register] from init. The host owns a [Board], which
// runs at most one job at a time and settles it: time-outs, penalty points,
// police stops and a helping autopilot are the framework's business, so a type
// only has to say when its goals are met.
//
// Jobs never touch the wanted level. The frame reports it, and a type may
// allow a pursuit through [Rules.MaxWanted]; otherwise one fails the job.
package job

import (
	"errors"
	"sort"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Status is how a job stands.
type Status int

// The states a job moves through, never backwards.
const (
	Active Status = iota
	Passed
	Failed
)

// String names the status.
func (s Status) String() string {
	switch s {
	case Passed:
		return "passed"
	case Failed:
		return "failed"
	}
	return "active"
}

// Goal is a place the player has to get to.
type Goal struct {
	Pos    mathx.Vec `json:"pos"`
	Radius float32   `json:"radius"`
	// Stop asks for the car to come to rest there.
	Stop bool `json:"stop"`
}

// State is a job in progress or finished. It is plain data so it can be saved
// or sent; everything a type needs to carry on is in it.
type State struct {
	Type     string  `json:"type"`
	Seed     uint64  `json:"seed"`
	Serial   int     `json:"serial"`
	Goals    []Goal  `json:"goals"`
	Next     int     `json:"next"` // the goal being driven to
	Start    float32 `json:"start"`
	Deadline float32 `json:"deadline"`
	Points   int     `json:"points"` // penalty points taken since the start
	Status   Status  `json:"status"`
	Reason   string  `json:"reason,omitempty"`
	// Assisted is set for good once the autopilot has had a hand in the job.
	Assisted bool `json:"assisted"`
}

// Fault is one rule broken while a job is under way.
type Fault struct {
	Kind   string
	Points int
}

// Frame is what the host tells the board each step.
type Frame struct {
	Now           float32 // simulation time, seconds
	Pos           mathx.Vec
	Speed         float32 // metres per second
	Faults        []Fault // infractions since the last frame
	PoliceStopped bool    // the police pulled the driver over this frame
	WantedLevel   int
	Assisted      bool // anything but the player is driving
}

// Rules are what the framework enforces for a type.
type Rules struct {
	// MaxWanted is the highest wanted level the job survives.
	MaxWanted int
	// CostPerPoint is taken from the pay for each penalty point.
	CostPerPoint int
	// MaxPoints, when above zero, fails the job once reached.
	MaxPoints int
}

// Type is a kind of job.
type Type interface {
	ID() string
	Name() string
	Rules() Rules
	// Plan makes a job for the player at from, or reports that there is
	// nowhere to send them yet. It must be a function of its arguments and
	// of the ground, never of a running random stream.
	Plan(c *city.City, seed uint64, serial int, from mathx.Vec, now float32) (State, bool)
	// Step advances the job by a frame, setting Status when it is decided.
	Step(s *State, f Frame)
	// Reward is the pay for passing, before penalties. It may be zero.
	Reward(s State) int
}

var registry = map[string]Type{}

// Register adds a type. Packs call it from init.
func Register(t Type) { registry[t.ID()] = t }

// Lookup returns the registered type with an id.
func Lookup(id string) (Type, bool) { t, ok := registry[id]; return t, ok }

// IDs lists the registered types, in order.
func IDs() []string {
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Payout is what a finished job pays: nothing unless it was passed, nothing if
// the autopilot helped, and the reward less the cost of the penalty points,
// never below zero.
func Payout(s State) int {
	if s.Assisted || s.Status != Passed {
		return 0
	}
	return Worth(s)
}

// Worth is what a job would pay if it were passed now: the reward less the
// cost of the points so far, never below zero. It ignores status and help.
func Worth(s State) int {
	t, ok := Lookup(s.Type)
	if !ok {
		return 0
	}
	return max(t.Reward(s)-t.Rules().CostPerPoint*s.Points, 0)
}

// Result is how a job ended.
type Result struct {
	Type     string
	Status   Status
	Reason   string
	Pay      int
	Assisted bool
}

// Board runs the host's job, one at a time.
type Board struct {
	active *State
}

// Active returns the job in progress, or nil.
func (b *Board) Active() *State { return b.active }

// ErrBusy is what Start says while a job is already under way.
var ErrBusy = errors.New("a job is already active")

// Start plans a job of a type and makes it the active one. It refuses while
// another is active, and when the type has nowhere to send the player.
func (b *Board) Start(t Type, c *city.City, seed uint64, serial int, from mathx.Vec, now float32) (State, error) {
	if b.active != nil {
		return State{}, ErrBusy
	}
	s, ok := t.Plan(c, seed, serial, from, now)
	if !ok {
		return State{}, errors.New("no place to send you yet")
	}
	s.Type, s.Seed, s.Serial, s.Status = t.ID(), seed, serial, Active
	b.active = &s
	return s, nil
}

// Cancel ends the active job as failed, for a reason.
func (b *Board) Cancel(reason string) (Result, bool) {
	if b.active == nil {
		return Result{}, false
	}
	return b.finish(Failed, reason), true
}

func (b *Board) finish(st Status, reason string) Result {
	s := b.active
	s.Status, s.Reason = st, reason
	b.active = nil
	return Result{Type: s.Type, Status: st, Reason: reason, Pay: Payout(*s), Assisted: s.Assisted}
}

// Step advances the active job by a frame. It reports a result on the frame the
// job ends, and nothing otherwise.
func (b *Board) Step(f Frame) (Result, bool) {
	s := b.active
	if s == nil {
		return Result{}, false
	}
	t, ok := Lookup(s.Type)
	if !ok {
		return b.finish(Failed, "unknown job"), true
	}
	r := t.Rules()
	if f.Assisted {
		s.Assisted = true
	}
	for _, ft := range f.Faults {
		s.Points += ft.Points
	}
	switch {
	case f.PoliceStopped:
		return b.finish(Failed, "stopped by the police"), true
	case f.WantedLevel > r.MaxWanted:
		return b.finish(Failed, "the police are after you"), true
	case r.MaxPoints > 0 && s.Points >= r.MaxPoints:
		return b.finish(Failed, "too many penalty points"), true
	case f.Now > s.Deadline:
		return b.finish(Failed, "out of time"), true
	}
	t.Step(s, f)
	if s.Status != Active {
		return b.finish(s.Status, s.Reason), true
	}
	return Result{}, false
}
