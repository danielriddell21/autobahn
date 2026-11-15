package city_test

import (
	"math"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// On a two-lane approach the turn toward the kerb is made from the kerbside
// lane and the turn across the road from the lane beside the centre line, so
// neither has to cut across the other. Lane 0 is the one by the centre line.
func TestTurnsAreMadeFromTheRightLane(t *testing.T) {
	c := build(7)
	checked := 0
	for _, l := range c.Lanes {
		r := c.Roads[l.Road]
		if r.Ring || r.Class.Lanes() < 2 || c.Nodes[l.ToNode].IsRoundabout() {
			continue
		}
		kerb := r.Class.Lanes() - 1
		for _, turn := range l.Succ {
			switch turn.Kind {
			case city.TurnLeft:
				if l.Index != kerb {
					t.Fatalf("lane %d (index %d) turns left across the kerbside lane", l.ID, l.Index)
				}
			case city.TurnRight:
				if l.Index != 0 {
					t.Fatalf("lane %d (index %d) turns right from the kerb, across the lane beside it", l.ID, l.Index)
				}
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no two-lane approaches to check")
	}
}

// A car cannot turn inside its own length. Every connector through an
// ordinary junction has to be an arc something with a steering lock can follow.
func TestEveryTurnCanBeDriven(t *testing.T) {
	const tightest = 5.0 // metres; a road car's turning circle is a little under this
	c := build(7)
	for _, l := range c.Lanes {
		if c.Roads[l.Road].Ring || !c.Complete(c.Nodes[l.ToNode]) || c.Nodes[l.ToNode].IsRoundabout() {
			continue
		}
		for _, turn := range l.Succ {
			if turn.Kind == city.TurnStraight {
				continue
			}
			// A quarter circle of this length has this radius.
			if radius := turn.Len * 2 / math.Pi; radius < tightest {
				t.Fatalf("lane %d: a turn of radius %.1fm onto lane %d cannot be driven", l.ID, radius, turn.Lane)
			}
		}
	}
}

func TestBlockCornersAreRoundedForTheJunction(t *testing.T) {
	c := build(7)
	rounded := 0
	for _, b := range c.Blocks {
		outline := b.Outline()
		if len(outline) < 4 {
			t.Fatalf("a block has an outline of %d points", len(outline))
		}
		for _, p := range outline {
			if p.X < b.Min.X-0.01 || p.X > b.Max.X+0.01 || p.Z < b.Min.Z-0.01 || p.Z > b.Max.Z+0.01 {
				t.Fatalf("outline point %v lies outside its block %v..%v", p, b.Min, b.Max)
			}
		}
		corners := [4]mathx.Vec{b.Min, {X: b.Max.X, Z: b.Min.Z}, {X: b.Min.X, Z: b.Max.Z}, b.Max}
		for i, corner := range b.Corners {
			if corner.Radius == 0 {
				continue
			}
			rounded++
			// The square corner itself has been cut away.
			for _, p := range outline {
				if p.DistTo(corners[i]) < corner.Radius*0.25 {
					t.Fatalf("a corner rounded to %.1fm still reaches its square corner at %v", corner.Radius, corners[i])
				}
			}
		}
	}
	if rounded == 0 {
		t.Fatal("no block has a rounded corner")
	}
}

// The pavement must stop at a roundabout's outer kerb. Blocks are rectangles
// set back by the width of the streets beside them, which is nowhere near
// enough: the roundabout takes a scoop out of each one.
func TestNoPavementReachesIntoARoundabout(t *testing.T) {
	c := build(7)
	if len(c.Roundabouts()) == 0 {
		t.Fatal("this seed generated no roundabouts to check")
	}
	for _, n := range c.Roundabouts() {
		if !(n.RingInner() < n.RingRadius() && n.RingRadius() < n.RingOuter() && n.RingOuter() < n.Radius) {
			t.Errorf("node %d: island %.1f, lane %.1f, kerb %.1f and box %.1f are out of order",
				n.ID, n.RingInner(), n.RingRadius(), n.RingOuter(), n.Radius)
		}
		if n.RingInner() < 2 {
			t.Errorf("node %d: an island of %.1fm is too small to draw", n.ID, n.RingInner())
		}
		for _, b := range c.Blocks {
			for _, p := range b.Outline() {
				if d := p.DistTo(n.Pos); d < n.RingOuter()-0.05 {
					t.Fatalf("node %d: pavement at %v is %.1fm from the middle, inside the kerb at %.1f",
						n.ID, p, d, n.RingOuter())
				}
			}
		}
	}
}

// A driver stopped at the line cannot see the head above them, so every
// signalled approach has a second one across the junction showing the same.
func TestSignalsRepeatAcrossTheJunction(t *testing.T) {
	c := build(7)
	checked := 0
	for _, l := range c.Lanes {
		if l.Control != city.ControlSignal || l.Index != 0 || !c.Complete(c.Nodes[l.ToNode]) {
			continue
		}
		var near, far int
		for _, p := range c.Props {
			if p.Kind != city.PropTrafficLight || p.Node != l.ToNode || p.Heading != l.Heading {
				continue
			}
			if p.Pos.Sub(l.B).Dot(l.Fwd) < 0 {
				near++
			} else {
				far++
			}
		}
		if near != 1 || far != 1 {
			t.Fatalf("lane %d: %d heads at the line and %d beyond it, want one of each", l.ID, near, far)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no signalled approaches in this city")
	}
}

// A limit that changed with no sign to say so could not be obeyed by anybody
// driving on what they can see.
func TestEveryRoadPostsItsLimit(t *testing.T) {
	c := build(7)
	for _, l := range c.Lanes {
		if c.Roads[l.Road].Ring || l.Index != 0 || l.Length < 30 || !c.Complete(c.Nodes[l.FromNode]) {
			continue
		}
		// A road's furniture goes up with the junction at its A end.
		if !c.Complete(c.Nodes[c.Roads[l.Road].A]) {
			continue
		}
		found := false
		for _, p := range c.Props {
			if p.Kind == city.PropSpeedSign && p.Heading == l.Heading &&
				p.Limit == l.SpeedLimit && p.Pos.DistTo(l.A) < 25 {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("lane %d starts with no sign for its %.0f mph limit", l.ID, mathx.ToMPH(l.SpeedLimit))
		}
	}
}

func TestProjectAlongFindsTheLaneGoingYourWay(t *testing.T) {
	c := build(7)
	for _, r := range c.Roads {
		if r.Ring || r.Length < 60 {
			continue
		}
		with, against := c.Lanes[r.LanesAB[0]], c.Lanes[r.LanesBA[0]]
		// Stand just the wrong side of the centre line, pointing along the road.
		at := against.Point(against.Length/2).Lerp(with.Point(with.Length/2), 0.3)

		if got := c.Project(at); got.Lane != against {
			t.Fatalf("Project: nearest lane is %d, want the oncoming lane %d", got.Lane.ID, against.ID)
		}
		got := c.ProjectAlong(at, r.Dir)
		if !got.Valid || got.Lane.Fwd.Dot(r.Dir) < 0.9 {
			t.Fatalf("ProjectAlong returned a lane running %v for a car heading %v", got.Lane.Fwd, r.Dir)
		}
		return
	}
	t.Fatal("no road long enough to stand on")
}

func TestInJunctionCoversTheBoxAndNotTheRoad(t *testing.T) {
	c := build(7)
	for _, l := range c.Lanes {
		n := c.Nodes[l.ToNode]
		if c.Roads[l.Road].Ring || n.IsRoundabout() || l.Length < 40 {
			continue
		}
		if !c.InJunction(n.Pos, l) {
			t.Errorf("the middle of junction %d is not in its own box", n.ID)
		}
		if c.InJunction(l.Point(l.Length/2), l) {
			t.Errorf("half-way along lane %d counts as being in a junction", l.ID)
		}
		return
	}
	t.Fatal("no ordinary junction to check")
}

// A car going round a roundabout is on the ring, not on whichever approach
// happens to be nearest.
func TestACarOnTheRingIsFoundOnTheRing(t *testing.T) {
	c := build(7)
	n := finishedRoundabout(t, c)
	at := n.Pos.Add(mathx.FromAngle(0.4).Mul(n.RingRadius()))
	got := c.Project(at)
	if !got.Valid || !c.Roads[got.Lane.Road].Ring {
		t.Fatalf("a point on the ring projects onto lane %d, which is not part of it", got.Lane.ID)
	}
}

// Traffic joins a roundabout a little way round from the road it arrives by,
// and leaves a little way before the road it leaves by, so it merges with the
// flow instead of meeting it square. Either way the connector meets the road
// dead straight.
func TestRoundaboutConnectorsMergeWithTheFlow(t *testing.T) {
	c := build(7)
	n := finishedRoundabout(t, c)
	checked := 0
	for _, l := range c.Lanes {
		if c.Roads[l.Road].Ring {
			// A ring lane: any exit from it must arrive on its road straight.
			if l.ToNode != n.ID {
				continue
			}
			for _, turn := range l.Succ {
				if turn.Kind == city.TurnStraight {
					continue
				}
				out := c.Lanes[turn.Lane]
				if dot := c.TurnTangent(l, turn, 1).Dot(out.Fwd); dot < 0.97 {
					t.Errorf("the exit onto lane %d arrives %.0f degrees off the road",
						out.ID, math.Acos(float64(dot))*180/math.Pi)
				}
				checked++
			}
			continue
		}
		if l.ToNode != n.ID {
			continue
		}
		entry := l.Succ[0]
		if dot := c.TurnTangent(l, entry, 0).Dot(l.Fwd); dot < 0.97 {
			t.Errorf("the entry from lane %d leaves the road %.0f degrees off straight",
				l.ID, math.Acos(float64(dot))*180/math.Pi)
		}
		// The merge point is further round, in the direction of circulation,
		// than the road itself.
		road := l.B.Sub(n.Pos).Angle()
		round := float64(mathx.AngleDiff(n.EntryAngle(l), road))
		if round <= 0.1 || round > 1.2 {
			t.Errorf("lane %d joins the ring %.2f rad round from its road, want a little way downstream", l.ID, round)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("the roundabout has no connectors")
	}
}
