package job

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// fake is a job that passes when told to, by a Frame at the goal.
type fake struct{ rules Rules }

func (fake) ID() string     { return "fake" }
func (fake) Name() string   { return "Fake" }
func (f fake) Rules() Rules { return f.rules }
func (fake) Plan(_ *city.City, seed uint64, serial int, _ mathx.Vec, now float32) (State, bool) {
	return State{Goals: []Goal{{Pos: mathx.V(10, 0), Radius: 5}}, Start: now, Deadline: now + 60}, true
}

func (fake) Step(s *State, f Frame) {
	if f.Pos.DistTo(s.Goals[0].Pos) <= s.Goals[0].Radius {
		s.Status = Passed
	}
}
func (fake) Reward(State) int { return 100 }

func begin(t *testing.T, rules Rules) *Board {
	t.Helper()
	Register(fake{rules})
	b := &Board{}
	if _, err := b.Start(fake{rules}, nil, 1, 0, mathx.Vec{}, 0); err != nil {
		t.Fatal(err)
	}
	return b
}

var atGoal = Frame{Now: 5, Pos: mathx.V(10, 0)}

func TestPass(t *testing.T) {
	b := begin(t, Rules{CostPerPoint: 10})
	r, done := b.Step(atGoal)
	if !done || r.Status != Passed || r.Pay != 100 || b.Active() != nil {
		t.Fatalf("got %+v done=%v", r, done)
	}
}

func TestTimeOutFails(t *testing.T) {
	b := begin(t, Rules{})
	r, done := b.Step(Frame{Now: 61, Pos: mathx.V(10, 0)})
	if !done || r.Status != Failed || r.Pay != 0 {
		t.Fatalf("got %+v", r)
	}
}

func TestPoliceStopFails(t *testing.T) {
	b := begin(t, Rules{})
	if r, done := b.Step(Frame{Now: 1, PoliceStopped: true}); !done || r.Status != Failed {
		t.Fatalf("got %+v", r)
	}
}

func TestWantedLevelFailsUnlessAllowed(t *testing.T) {
	b := begin(t, Rules{})
	if r, done := b.Step(Frame{Now: 1, WantedLevel: 1}); !done || r.Status != Failed {
		t.Fatalf("got %+v", r)
	}
	b = begin(t, Rules{MaxWanted: 2})
	if _, done := b.Step(Frame{Now: 1, WantedLevel: 2}); done {
		t.Fatal("a pursuit the type allows ended the job")
	}
}

func TestFaultsReducePayToFloor(t *testing.T) {
	b := begin(t, Rules{CostPerPoint: 10})
	b.Step(Frame{Now: 1, Faults: []Fault{{"speeding", 3}}})
	if r, _ := b.Step(atGoal); r.Pay != 70 {
		t.Fatalf("pay %d, want 70", r.Pay)
	}
	b = begin(t, Rules{CostPerPoint: 1000})
	b.Step(Frame{Now: 1, Faults: []Fault{{"speeding", 3}}})
	if r, _ := b.Step(atGoal); r.Status != Passed || r.Pay != 0 {
		t.Fatalf("got %+v, want passed on 0", r)
	}
}

func TestMaxPointsFails(t *testing.T) {
	b := begin(t, Rules{MaxPoints: 5})
	b.Step(Frame{Now: 1, Faults: []Fault{{"a", 3}}})
	if r, done := b.Step(Frame{Now: 2, Faults: []Fault{{"b", 2}}}); !done || r.Status != Failed {
		t.Fatalf("got %+v", r)
	}
}

func TestAssistedStaysSetAndPaysNothing(t *testing.T) {
	b := begin(t, Rules{})
	b.Step(Frame{Now: 1, Assisted: true})
	if !b.Active().Assisted {
		t.Fatal("not marked")
	}
	b.Step(Frame{Now: 2}) // the player takes the wheel back
	if !b.Active().Assisted {
		t.Fatal("assisted came off")
	}
	if r, _ := b.Step(atGoal); r.Status != Passed || r.Pay != 0 || !r.Assisted {
		t.Fatalf("got %+v", r)
	}
}

func TestCancel(t *testing.T) {
	b := begin(t, Rules{})
	r, ok := b.Cancel("autopilot")
	if !ok || r.Status != Failed || r.Pay != 0 || b.Active() != nil {
		t.Fatalf("got %+v", r)
	}
	if _, ok := b.Cancel("again"); ok {
		t.Fatal("cancelled nothing")
	}
}

func TestSecondStartRefused(t *testing.T) {
	b := begin(t, Rules{})
	if _, err := b.Start(fake{}, nil, 1, 1, mathx.Vec{}, 0); !errors.Is(err, ErrBusy) {
		t.Fatalf("err %v", err)
	}
}

func TestStateJSONRoundTrip(t *testing.T) {
	in := State{
		Type: "dropoff", Seed: 9, Serial: 2, Next: 1, Start: 3, Deadline: 90, Points: 4,
		Goals: []Goal{{Pos: mathx.V(1, 2), Radius: 6, Stop: true}}, Status: Failed,
		Reason: "x", Assisted: true,
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out State
	if err := json.Unmarshal(raw, &out); err != nil || !reflect.DeepEqual(in, out) {
		t.Fatalf("got %+v, %v", out, err)
	}
}

// startPoint is the middle of a finished lane near the origin.
func startPoint(c *city.City) (mathx.Vec, bool) {
	best, found := mathx.Vec{}, false
	for _, l := range c.Lanes {
		if l.Index != 0 || c.Roads[l.Road].Ring || !usable(c, l) {
			continue
		}
		if p := l.Point(l.Length / 2); !found || p.Len() < best.Len() {
			best, found = p, true
		}
	}
	return best, found
}

func TestPlanDeterministicAndFinishedOnly(t *testing.T) {
	for _, m := range city.Maps() {
		t.Run(m.Name(), func(t *testing.T) {
			p := city.ParamsFor(m, 7)
			a := city.NewGrowing(p)
			a.EnsureAround(mathx.V(0, 0), 1400)
			from, ok := startPoint(a)
			if !ok {
				t.Fatal("no finished lane")
			}
			s1, ok := DropOff{}.Plan(a, 7, 1, from, 10)
			if !ok {
				t.Fatal("no job planned")
			}
			s2, _ := DropOff{}.Plan(a, 7, 1, from, 10)
			if !reflect.DeepEqual(s1, s2) {
				t.Fatal("same ask, different job")
			}
			// The goal sits on a lane between finished junctions.
			pr := a.Project(s1.Goals[0].Pos)
			if !pr.Valid || !usable(a, pr.Lane) || pr.Dist > 1 {
				t.Fatalf("goal not on finished lane: %+v", pr)
			}
			if s1.Deadline <= 10+20 {
				t.Fatalf("deadline %v leaves no driving time", s1.Deadline)
			}
			if _, ok := (DropOff{}).Plan(city.NewGrowing(p), 7, 1, from, 10); ok {
				t.Fatal("planned on a city with nothing built")
			}
		})
	}
}

func TestDropOffStep(t *testing.T) {
	s := State{Goals: []Goal{{Pos: mathx.V(0, 0), Radius: 6, Stop: true}}}
	DropOff{}.Step(&s, Frame{Pos: mathx.V(2, 0), Speed: 5})
	if s.Status != Active {
		t.Fatal("passed while moving")
	}
	DropOff{}.Step(&s, Frame{Pos: mathx.V(20, 0), Speed: 0})
	if s.Status != Active {
		t.Fatal("passed far away")
	}
	DropOff{}.Step(&s, Frame{Pos: mathx.V(2, 0), Speed: 0})
	if s.Status != Passed {
		t.Fatal("did not pass")
	}
}
