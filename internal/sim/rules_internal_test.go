package sim

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// signalledLane returns a lane governed by a signal, and the signals it is
// governed by.
func signalledLane(t *testing.T) (*city.Lane, *Signals) {
	t.Helper()
	c := city.Generate(city.DefaultParams(7))
	for _, l := range c.Lanes {
		if l.Control == city.ControlSignal && c.Complete(c.Nodes[l.ToNode]) {
			return l, NewSignals(c, 7)
		}
	}
	t.Fatal("no signalled lane in this city")
	return nil, nil
}

// until advances the signals until the lane shows the wanted aspect, and has
// only just started showing it.
func until(t *testing.T, sig *Signals, l *city.Lane, want SignalState) {
	t.Helper()
	const step = 0.05
	prev := sig.LaneState(l)
	for range int(fullCycle*2/step) + 1 {
		sig.Update(step)
		now := sig.LaneState(l)
		if now == want && prev != want {
			return
		}
		prev = now
	}
	t.Fatalf("the signal never changed to %v", want)
}

func TestSinceRestartsWhenTheAspectChanges(t *testing.T) {
	l, sig := signalledLane(t)
	for _, aspect := range []SignalState{SignalGreen, SignalAmber, SignalRed, SignalRedAmber} {
		until(t, sig, l, aspect)
		if s := sig.Since(l.ToNode, l.Group); s > 0.11 {
			t.Errorf("%v has just come on, but Since says %.2fs", aspect, s)
		}
		sig.Update(1)
		if sig.LaneState(l) != aspect {
			continue // an aspect shorter than a second; nothing more to check
		}
		if s := sig.Since(l.ToNode, l.Group); mathx.Abs(s-1) > 0.11 {
			t.Errorf("a second into %v, Since says %.2fs", aspect, s)
		}
	}
}

// Amber means stop, unless stopping would mean standing on the brakes. A car
// right on top of the line when it changes may carry on; one that had room to
// pull up may not.
func TestAmberIsOnlyAnOffenceIfTheCarCouldHaveStopped(t *testing.T) {
	l, sig := signalledLane(t)
	const speed = 17 // m/s, about 38 mph

	until(t, sig, l, SignalAmber)
	sig.Update(0.4)
	// 0.4s into the amber at this speed the car was under 7m from the line
	// when it came on. Nothing stops in that.
	if ranLight(sig, l, speed) {
		t.Error("crossing 0.4s into an amber at 38 mph was judged an offence")
	}

	sig.Update(2.2)
	if sig.LaneState(l) != SignalAmber {
		t.Skip("the amber is shorter than this test assumes")
	}
	// 2.6s in, the car was 44m back: it saw the amber and kept going.
	if !ranLight(sig, l, speed) {
		t.Error("crossing 2.6s into an amber at 38 mph was let off")
	}

	until(t, sig, l, SignalRed)
	if !ranLight(sig, l, 3) {
		t.Error("crossing on red was let off")
	}
	until(t, sig, l, SignalGreen)
	if ranLight(sig, l, speed) {
		t.Error("crossing on green was judged an offence")
	}
}

// Turning right across a junction, a car crosses the end of the lane coming
// the other way while pointing against it. That is not driving the wrong way.
func TestCrossingAJunctionIsNotTheWrongWay(t *testing.T) {
	c := city.Generate(city.DefaultParams(7))
	var l *city.Lane
	for _, cand := range c.Lanes {
		n := c.Nodes[cand.ToNode]
		if !c.Roads[cand.Road].Ring && !n.IsRoundabout() && c.Complete(n) && cand.Length > 40 {
			l = cand
			break
		}
	}
	if l == nil {
		t.Fatal("no ordinary junction in this city")
	}
	j := NewJudge(c)
	sig := NewSignals(c, 7)

	// Inside the box, just past the end of the lane, heading back up it.
	v := NewVehicle(CarSpec(), l.B.Add(l.Fwd.Mul(3)), l.Heading+3.14159)
	v.Vel = l.Fwd.Mul(-8)
	for i := range 180 {
		j.Update(v, sig, float32(i)/60, 1.0/60)
	}
	if j.Faults != 0 {
		t.Errorf("a car in the junction box was given %d faults: %v", j.Faults, j.Log())
	}

	// The same thing half-way along the lane is exactly what the rule is for.
	v.Pos = l.Point(l.Length / 2)
	for i := range 180 {
		j.Update(v, sig, 3+float32(i)/60, 1.0/60)
	}
	wrong := 0
	for _, in := range j.Log() {
		if in.Kind == WrongWay {
			wrong++
		}
	}
	if wrong == 0 {
		t.Error("driving against the flow in the middle of a lane went unnoticed")
	}
}

func TestGapNeededGrowsWithTheTurnAndTheDriver(t *testing.T) {
	c := city.Generate(city.DefaultParams(7))
	var short, long *city.Lane
	for _, l := range c.Lanes {
		if c.Roads[l.Road].Ring || c.Nodes[l.ToNode].IsRoundabout() {
			continue
		}
		for _, turn := range l.Succ {
			switch {
			case turn.Kind == city.TurnLeft && short == nil:
				short = l
			case turn.Kind == city.TurnRight && long == nil:
				long = l
			}
		}
	}
	if short == nil || long == nil {
		t.Fatal("the city has no left or no right turn")
	}
	agent := func(l *city.Lane, kind city.TurnKind, st Style) *Agent {
		a := &Agent{V: NewVehicle(TrafficSpec(), l.B, l.Heading), Style: st, lane: l}
		for _, turn := range l.Succ {
			if turn.Kind == kind {
				a.next, a.hasNext = turn, true
			}
		}
		return a
	}
	left := agent(short, city.TurnLeft, Normal()).gapNeeded()
	right := agent(long, city.TurnRight, Normal()).gapNeeded()
	if right <= left {
		t.Errorf("a turn across the road needs %.1fs and one toward the kerb %.1fs; the longer turn should need longer", right, left)
	}
	careful := agent(long, city.TurnRight, Cautious()).gapNeeded()
	pushy := agent(long, city.TurnRight, Aggressive()).gapNeeded()
	if careful <= pushy {
		t.Errorf("a cautious driver needs %.1fs and an aggressive one %.1fs", careful, pushy)
	}
	// However pushy, nobody takes a gap shorter than the turn itself.
	if pushy < 2.5 {
		t.Errorf("an aggressive driver would pull out across the road with %.1fs to spare", pushy)
	}
}

// TestWallsStandWhereTheBuildingsAre guards against a building's collision box
// being turned a quarter turn from the building.
//
// It was, for as long as there had been buildings: width and depth were handed
// to the box the wrong way round. On a square building nobody could tell. On a
// long thin one the box stuck out across the pavement into the road at each
// end, and left the building's own ends open to drive through.
func TestWallsStandWhereTheBuildingsAre(t *testing.T) {
	b := city.Building{Center: mathx.V(40, -10), W: 26, D: 6}
	var lo, hi mathx.Vec
	for i, c := range footprint(b).Corners() {
		if i == 0 {
			lo, hi = c, c
		}
		lo = mathx.V(min(lo.X, c.X), min(lo.Z, c.Z))
		hi = mathx.V(max(hi.X, c.X), max(hi.Z, c.Z))
	}
	want := [2]mathx.Vec{mathx.V(27, -13), mathx.V(53, -7)}
	if lo.DistTo(want[0]) > 0.01 || hi.DistTo(want[1]) > 0.01 {
		t.Errorf("a 26x6m building at (40,-10) collides over %v..%v, want %v..%v", lo, hi, want[0], want[1])
	}

	// And a car beside its long face, clear of it by three metres, touches
	// nothing — which it did when the box was the wrong way round.
	car := NewVehicle(CarSpec(), mathx.V(52, -10+3+3+1), 0)
	if _, _, hit := car.Box().Penetration(footprint(b)); hit {
		t.Error("a car three metres clear of the long face is in collision with it")
	}
}
