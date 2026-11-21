package sim

import (
	"math/rand/v2"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

func lone(st Style) *Agent {
	return &Agent{
		ID: 3, Style: st, V: NewVehicle(TrafficSpec(), mathx.V(0, 0), 0),
		rng: rand.New(rand.NewPCG(1, 2)),
	}
}

func TestWanderGrowsWithInattention(t *testing.T) {
	widest := func(attention float32) float32 {
		st := Normal()
		st.Attention = attention
		a := lone(st)
		var most float32
		for now := float32(0); now < 120; now += 0.1 {
			most = max(most, mathx.Abs(a.wander(st, now, 12)))
		}
		return most
	}
	sharp, dozy := widest(0.95), widest(0.6)
	if !(dozy > sharp*3) {
		t.Errorf("an inattentive driver wanders %.2fm and an attentive one %.2fm", dozy, sharp)
	}
	if dozy > wanderReach*0.4+0.01 {
		t.Errorf("wander of %.2fm is more than the driver's attention allows", dozy)
	}
	if lone(Normal()).wander(Normal(), 5, 1) != 0 {
		t.Error("a car crawling in a queue should not wander")
	}
}

func TestPerceptionLagsButNotForACloseCall(t *testing.T) {
	st := Normal()
	st.Attention = 1 // never looks away, so only the lag is measured
	a := lone(st)
	lead := NewVehicle(TrafficSpec(), mathx.V(40, 0), 0)
	a.perceive(st, lead, 40, 12, 12, 1.0/60)
	// The car in front brakes hard; the driver takes a moment to notice.
	if seen := a.perceive(st, lead, 40, 4, 12, 1.0/60); seen <= 4.5 {
		t.Errorf("the driver registered the car in front braking instantly: seen %.1f", seen)
	}
	// Unless it is right there.
	if seen := a.perceive(st, lead, 6, 4, 12, 1.0/60); seen != 4 {
		t.Errorf("about to hit it, the driver should see its real speed, saw %.1f", seen)
	}
}

func TestRollingStopAtTheSign(t *testing.T) {
	l := &city.Lane{ToNode: 5}
	for _, tc := range []struct {
		st   Style
		want bool
	}{{Normal(), true}, {Aggressive(), false}} {
		a := lone(tc.st)
		a.lane, a.stoppedAt = l, -1
		a.V.Vel = mathx.V(2, 0)
		if got := a.signStop(tc.st, 4); got != tc.want {
			t.Errorf("%s at 2 m/s, 4m from a stop sign: still stopping %v, want %v", tc.st.Name, got, tc.want)
		}
	}
}

func TestTemperShowsInBrakingAndFollowing(t *testing.T) {
	if !(Aggressive().brakePoint() > Cautious().brakePoint()) {
		t.Error("an aggressive driver should leave braking later than a cautious one")
	}
	a := lone(Aggressive())
	a.heldUp = 100
	if p := a.pressed(a.Style); !(p.Headway < a.Style.Headway) {
		t.Error("an aggressive driver held up past their patience should close up")
	}
	a.heldUp = 0
	if p := a.pressed(a.Style); p.Headway != a.Style.Headway {
		t.Error("nobody presses before their patience runs out")
	}
}

func TestHornForACutIn(t *testing.T) {
	st := Aggressive()
	st.Temper = 1
	a := lone(st)
	w := &World{}
	first := NewVehicle(TrafficSpec(), mathx.V(40, 0), 0)
	intruder := NewVehicle(TrafficSpec(), mathx.V(8, 0), 0)
	a.express(w, st, first, 35, 12, 12, 1.0/60)
	a.express(w, st, intruder, 3, 10, 12, 1.0/60)
	if !a.Hooting() {
		t.Error("an angry driver should sound the horn at somebody who cuts in close")
	}
	calm := lone(Cautious())
	calm.Style.Temper = 0
	calm.express(w, calm.Style, first, 35, 12, 12, 1.0/60)
	calm.express(w, calm.Style, intruder, 3, 10, 12, 1.0/60)
	if calm.Hooting() {
		t.Error("a driver with no temper should not")
	}
}

func TestHesitationAtTheLine(t *testing.T) {
	a := lone(Normal())
	a.hesitate(a.Style, true, 0, 1.0/60)
	if !a.hesitate(a.Style, false, 0, 1.0/60) {
		t.Error("a driver released from the line should take a moment to go")
	}
	for range 120 {
		a.hesitate(a.Style, false, 0, 1.0/60)
	}
	if a.hesitate(a.Style, false, 0, 1.0/60) {
		t.Error("the moment should pass")
	}
	// Rolling through without stopping is no reason to pause.
	b := lone(Normal())
	b.hesitate(b.Style, true, 6, 1.0/60)
	if b.hesitate(b.Style, false, 6, 1.0/60) {
		t.Error("a driver who never stopped should not hesitate")
	}
}

func TestCourteousDriverLetsSomebodyOut(t *testing.T) {
	w := NewWorld(Config{Seed: 7})
	// A minor approach and a major one into the same priority junction.
	var minor, major *city.Lane
	for _, l := range w.City.Lanes {
		n := w.City.Nodes[l.ToNode]
		if l.Control != city.ControlGiveWay || n.IsRoundabout() || w.City.Roads[l.Road].Ring {
			continue
		}
		for _, m := range w.City.Lanes {
			if m.ToNode == l.ToNode && m.Control == city.ControlNone && m.Road != l.Road && m.Length > 50 {
				minor, major = l, m
				break
			}
		}
		if minor != nil {
			break
		}
	}
	if minor == nil {
		t.Skip("no priority junction with a long major approach")
	}
	waiting := put(w, minor, minor.Length-2, Normal(), 0)
	waiting.yielding, waiting.waited = true, 2

	for _, tc := range []struct {
		courtesy float32
		want     bool
	}{{1, true}, {0, false}} {
		st := Normal()
		st.Courtesy = tc.courtesy
		a := put(w, major, major.Length-30, st, 3)
		d, ok := w.courtesyStop(a, st, 3, 1.0/60)
		if ok != tc.want {
			t.Errorf("courtesy %.0f: let them out %v, want %v", tc.courtesy, ok, tc.want)
		}
		if ok && mathx.Abs(d-(30-letOutRoom)) > 0.5 {
			t.Errorf("stopped %.1fm short of the line, want %.1f", d, 30-letOutRoom)
		}
		// Asked once, answered once.
		if _, again := w.courtesyStop(a, st, 3, 1.0/60); again != ok {
			t.Error("the decision should hold")
		}
		w.Agents = w.Agents[:len(w.Agents)-1]
	}
}
