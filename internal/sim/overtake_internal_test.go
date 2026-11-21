package sim

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// emptyRoad builds a world with no traffic, finds a long straight lane of the
// given class, and moves the player's car off onto the pavement out of the way.
func emptyRoad(t *testing.T, cls city.RoadClass, kerbside bool) (*World, *city.Lane) {
	t.Helper()
	return emptyRoadWhere(t, cls, kerbside, func(_ *World, l *city.Lane) bool { return l.Length >= 110 })
}

// emptyRoadWhere is emptyRoad, taking only a lane that also passes ok.
func emptyRoadWhere(t *testing.T, cls city.RoadClass, kerbside bool, ok func(*World, *city.Lane) bool) (*World, *city.Lane) {
	t.Helper()
	for _, seed := range []uint64{7, 3, 11, 19, 23} {
		w := NewWorld(Config{Seed: seed})
		var best *city.Lane
		for _, l := range w.City.Lanes {
			if l.Class != cls || w.City.Roads[l.Road].Ring || len(l.Succ) == 0 {
				continue
			}
			if kerbside != (l.Index == cls.Lanes()-1) || !ok(w, l) {
				continue
			}
			if best == nil || l.Length > best.Length {
				best = l
			}
		}
		if best == nil || best.Length < 70 {
			continue
		}
		// Park the player well off the road.
		w.Player.Place(best.Point(best.Length/2).Add(best.Fwd.Right().Mul(-40)), best.Heading)
		return w, best
	}
	t.Skip("no city in the seeds tried has a long enough lane")
	return nil, nil
}

// put places a driver of the given style on a lane, heading straight on at the
// far end, and adds it to the world.
func put(w *World, l *city.Lane, s float32, st Style, speed float32) *Agent {
	a := NewAgent(len(w.Agents), w.City, l, s, w.rng)
	a.Style = st
	a.hasWant = false
	a.next, a.hasNext = onward(l)
	a.V.Vel = l.Fwd.Mul(speed)
	w.Agents = append(w.Agents, a)
	return a
}

// crawler is a driver who will not go faster than a fraction of the limit and
// never passes anybody.
func crawler(frac float32) Style {
	st := Cautious()
	st.Name, st.SpeedBias, st.Overtake, st.KeepLeft = "crawler", frac, 0, 1
	st.PassesOncoming = false
	return st
}

func step(w *World, seconds float32, each func() bool) {
	for t := float32(0); t < seconds; t += 1.0 / 60 {
		w.Update(Controls{Brake: 1}, 1.0/60)
		if each != nil && !each() {
			return
		}
	}
}

func TestSiblingAndOncomingLanes(t *testing.T) {
	w, l := emptyRoad(t, city.Arterial, true)
	in := sibling(w.City, l, 0)
	if in == nil || in == l || in.Road != l.Road || in.Dir != l.Dir {
		t.Fatalf("the lane beside %d should be on the same road going the same way, got %+v", l.ID, in)
	}
	if sibling(w.City, l, l.Index) != l {
		t.Error("a lane's sibling with its own index should be itself")
	}
	on := oncoming(w.City, l)
	if on == nil || on.Dir == l.Dir || on.Index != 0 || on.Fwd.Dot(l.Fwd) > -0.9 {
		t.Fatalf("oncoming lane of %d is wrong: %+v", l.ID, on)
	}
}

func TestLaneClearWantsARoomyGap(t *testing.T) {
	w, l := emptyRoad(t, city.Arterial, true)
	out := sibling(w.City, l, 0)
	a := put(w, l, 40, Normal(), 12)
	if !w.laneClear(a, out, a.Style, 12) {
		t.Fatal("an empty lane should be clear")
	}

	// Somebody right alongside.
	b := put(w, out, 40, Normal(), 12)
	if w.laneClear(a, out, a.Style, 12) {
		t.Error("a lane with a car alongside is not clear")
	}

	// Somebody behind and closing fast: room for the bold, not the careful.
	b.V.Place(out.Point(40-14), out.Heading)
	b.V.Vel = out.Fwd.Mul(15)
	if w.laneClear(a, out, Cautious(), 12) {
		t.Error("a cautious driver should not pull out 14m ahead of a car closing at 3 m/s")
	}
	agg := Aggressive()
	agg.LaneGap = 0.3
	if !w.laneClear(a, out, agg, 12) {
		t.Error("a driver who cuts in should take that gap")
	}
}

func TestSlowCarIsPassedOnADualCarriageway(t *testing.T) {
	w, l := emptyRoad(t, city.Arterial, true)
	slow := put(w, l, 30, crawler(0.4), 6)
	fast := put(w, l, 8, Brisk(), 8)
	fast.Style.Patience = 1
	moved := false
	step(w, 25, func() bool {
		if fast.lane != l && fast.lane.Road == l.Road {
			moved = true
		}
		return fast.lane.Road == l.Road
	})
	if !moved {
		t.Fatal("the brisk driver never moved out to pass")
	}
	if ahead := slow.V.Pos.Sub(fast.V.Pos).Dot(l.Fwd); ahead > 0 {
		t.Errorf("the brisk driver is still %.1fm behind the crawler", ahead)
	}
	if fast.dazed > 0 || slow.dazed > 0 {
		t.Error("the pass ended in a collision")
	}
}

func TestDriverMovesBackToTheNearside(t *testing.T) {
	w, l := emptyRoad(t, city.Arterial, true)
	out := sibling(w.City, l, 0)
	a := put(w, out, 5, Normal(), 10)
	a.Style.KeepLeft = 1
	step(w, 12, func() bool { return a.lane == out })
	if a.lane != l {
		t.Fatalf("with an empty road the driver should move back to the kerbside lane, is on lane %d", a.lane.ID)
	}
}

func TestDriverHoldsTheLaneForTheirTurn(t *testing.T) {
	across := turnAcrossTraffic()
	w, l := emptyRoadWhere(t, city.Arterial, true, func(w *World, l *city.Lane) bool {
		out := sibling(w.City, l, 0)
		return out != nil && len(out.Succ) > 0 && serves(out, across) && !serves(l, across)
	})
	out := sibling(w.City, l, 0)
	a := put(w, out, 5, Aggressive(), 10)
	a.wantKind, a.hasWant = across, true
	for _, tr := range out.Succ {
		if tr.Kind == across {
			a.next = tr
		}
	}
	step(w, 10, func() bool { return !a.inTurn })
	if a.lane != out || a.next.Kind != across {
		t.Error("a driver turning across the road should stay in the lane beside the centreline")
	}
}

func TestPassOnASingleCarriageway(t *testing.T) {
	w, l := emptyRoad(t, city.Collector, true)
	slow := put(w, l, 20, crawler(0.35), 4)
	fast := put(w, l, 4, Aggressive(), 6)
	fast.Style.Patience = 0.5
	passed := false
	step(w, 25, func() bool {
		if fast.overtaking != nil {
			passed = true
		}
		return fast.lane == l
	})
	if !passed {
		t.Fatal("an aggressive driver never pulled out round a crawler on an empty road")
	}
	if ahead := slow.V.Pos.Sub(fast.V.Pos).Dot(l.Fwd); ahead > 0 {
		t.Errorf("the overtake did not get past: still %.1fm behind", ahead)
	}
	if fast.dazed > 0 || slow.dazed > 0 {
		t.Error("the overtake ended in a collision")
	}
	if mathx.Abs(fast.sidestep) > 0.5 && fast.overtaking == nil && fast.changing <= 0 {
		t.Errorf("the driver did not pull back in: sidestep %.1f", fast.sidestep)
	}
}

func TestNoPassIntoOncomingTraffic(t *testing.T) {
	w, l := emptyRoad(t, city.Collector, true)
	on := oncoming(w.City, l)
	put(w, l, 20, crawler(0.35), 4)
	fast := put(w, l, 4, Aggressive(), 6)
	fast.Style.Patience = 0.5
	// A car coming the other way, far enough off to see.
	put(w, on, max(on.Length-l.Length+20, 0)+10, Normal(), 12)
	step(w, 2.5, func() bool {
		if fast.overtaking != nil {
			t.Fatal("pulled out to pass with a car coming the other way")
		}
		return true
	})
}

func TestCarefulDriversDoNotUseTheOtherSide(t *testing.T) {
	w, l := emptyRoad(t, city.Collector, true)
	put(w, l, 20, crawler(0.35), 4)
	a := put(w, l, 4, Normal(), 6)
	a.Style.Patience = 0.5
	step(w, 10, func() bool {
		if a.overtaking != nil {
			t.Fatal("a normal driver overtook on a single carriageway")
		}
		return a.lane == l
	})
}

func TestDriverMovesOverForTheirTurn(t *testing.T) {
	across := turnAcrossTraffic()
	w, l := emptyRoadWhere(t, city.Arterial, true, func(w *World, l *city.Lane) bool {
		out := sibling(w.City, l, 0)
		return out != nil && len(out.Succ) > 0 && serves(out, across) && !serves(l, across)
	})
	a := put(w, l, 5, Normal(), 10)
	a.wantKind, a.hasWant = across, true
	// Until the junction, or held at its line.
	step(w, 30, func() bool { return !a.inTurn && a.lane.Length-a.s > turnLaneEnd })
	if a.lane.Index != 0 || a.next.Kind != across {
		t.Errorf("a driver meaning to turn across the road should have moved over for it: lane index %d, turn %v",
			a.lane.Index, a.next.Kind)
	}
}
