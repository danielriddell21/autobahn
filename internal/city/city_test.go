package city_test

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

func build(seed uint64) *city.City { return city.Generate(city.DefaultParams(seed)) }

func TestGenerationIsDeterministic(t *testing.T) {
	a, b := build(42), build(42)
	if len(a.Lanes) != len(b.Lanes) || len(a.Buildings) != len(b.Buildings) {
		t.Fatalf("same seed gave different cities: %d/%d lanes, %d/%d buildings",
			len(a.Lanes), len(b.Lanes), len(a.Buildings), len(b.Buildings))
	}
	for i := range a.Lanes {
		if a.Lanes[i].A != b.Lanes[i].A || a.Lanes[i].B != b.Lanes[i].B {
			t.Fatalf("lane %d differs between runs of the same seed", i)
		}
	}
	if c := build(43); len(c.Lanes) == len(a.Lanes) && len(c.Buildings) == len(a.Buildings) {
		t.Log("different seeds produced the same shape; not fatal but suspicious")
	}
}

// Traffic keeps left, so a lane sits to the left of its road's centreline.
func TestLanesSitOnTheLeft(t *testing.T) {
	c := build(7)
	for _, r := range c.Roads {
		for _, id := range r.LanesAB {
			l := c.Lanes[id]
			// Offset of the lane centreline from the road centreline, measured
			// to the right of the direction of travel. Keeping left makes it
			// negative.
			mid := c.Nodes[r.A].Pos.Lerp(c.Nodes[r.B].Pos, 0.5)
			off := l.A.Lerp(l.B, 0.5).Sub(mid).Dot(r.Dir.Right())
			if off >= 0 {
				t.Fatalf("lane %d sits %.2fm to the right of the centreline; traffic should keep left",
					id, off)
			}
		}
	}
}

func TestEveryLaneLeadsSomewhere(t *testing.T) {
	c := build(11)
	for _, l := range c.Lanes {
		if !c.Complete(c.Nodes[l.ToNode]) {
			// A junction on the frontier: its far side has not been reached,
			// so its lanes have nowhere to go yet and that is not a fault.
			continue
		}
		if c.Nodes[l.ToNode].Degree() < 2 {
			continue // a stub at the edge of the grid
		}
		if len(l.Succ) == 0 {
			t.Fatalf("lane %d ends at a junction of degree %d with nowhere to go",
				l.ID, c.Nodes[l.ToNode].Degree())
		}
		for _, s := range l.Succ {
			// Circulating lanes legitimately continue onto the same synthetic
			// ring road; anywhere else, that would be a U-turn.
			if c.Roads[l.Road].Ring {
				continue
			}
			if c.Lanes[s.Lane].Road == l.Road {
				t.Fatalf("lane %d has a U-turn connector", l.ID)
			}
		}
	}
}

func TestSpeedLimitsAreTheBritishUrbanSet(t *testing.T) {
	want := map[city.RoadClass]float32{
		city.Local:     mathx.MPH(20),
		city.Collector: mathx.MPH(30),
		city.Arterial:  mathx.MPH(40),
	}
	for cls, w := range want {
		if got := cls.SpeedLimit(); got != w {
			t.Errorf("%v limit = %.1f m/s, want %.1f", cls.Name(), got, w)
		}
	}
}

func TestJunctionsAreControlled(t *testing.T) {
	c := build(3)
	var signals, giveWay, stops int
	for _, n := range c.Nodes {
		if n.Degree() < 3 {
			continue
		}
		switch n.Control {
		case city.ControlSignal:
			signals++
		case city.ControlGiveWay:
			giveWay++
		case city.ControlStop:
			stops++
		default:
			t.Errorf("junction %d of degree %d has no control", n.ID, n.Degree())
		}
	}
	if signals == 0 {
		t.Error("no signalised junctions were generated")
	}
	// Give way is the common British priority marking; stop signs are rarer.
	if giveWay == 0 {
		t.Error("no give way junctions were generated")
	}
	if giveWay < stops {
		t.Errorf("got %d give way and %d stop junctions; give way should dominate", giveWay, stops)
	}
}

func TestProjectFindsTheLaneUnderAPoint(t *testing.T) {
	c := build(5)
	l := c.Lanes[len(c.Lanes)/2]
	mid := l.Point(l.Length / 2)

	got := c.Project(mid)
	if !got.Valid {
		t.Fatal("Project found no lane under a point on a lane")
	}
	if got.Dist > 0.5 {
		t.Errorf("distance to the lane centreline = %.2fm, want ~0", got.Dist)
	}
	// A point offset to the right should report a positive lateral.
	off := mid.Add(l.Fwd.Right().Mul(1.2))
	if p := c.Project(off); p.Lateral <= 0 {
		t.Errorf("lateral for a point to the right = %.2f, want positive", p.Lateral)
	}
}

func TestSignsAndSignalsArePlaced(t *testing.T) {
	c := build(9)
	counts := map[city.PropKind]int{}
	for _, p := range c.Props {
		counts[p.Kind]++
	}
	for _, k := range []city.PropKind{
		city.PropTrafficLight, city.PropGiveWaySign, city.PropSpeedSign,
	} {
		if counts[k] == 0 {
			t.Errorf("no props of kind %v were placed", k)
		}
	}
}

func TestBuildingsStayOffTheCarriageway(t *testing.T) {
	c := build(13)
	for i, b := range c.Buildings {
		p := c.Project(b.Center)
		if !p.Valid {
			continue
		}
		if p.Dist < c.RoadHalfWidth(p.Lane) {
			t.Fatalf("building %d at %v sits %.2fm from a lane centreline, inside the carriageway",
				i, b.Center, p.Dist)
		}
	}
}

func TestRoundaboutsAreGenerated(t *testing.T) {
	c := build(7)
	rounds := c.Roundabouts()
	if len(rounds) == 0 {
		t.Fatal("no roundabouts were generated")
	}
	for _, n := range rounds {
		if !c.Complete(n) {
			continue // still on the frontier; not all its arms exist yet
		}
		if !n.IsRoundabout() {
			t.Errorf("node %d is listed but not marked", n.ID)
		}
		if n.RingRadius() < 8 {
			t.Errorf("roundabout %d has a ring radius of %.1fm", n.ID, n.RingRadius())
		}
		if n.Degree() < 3 {
			t.Errorf("roundabout %d has only %d arms", n.ID, n.Degree())
		}
	}
}

// The circulating carriageway has to be a closed loop, or traffic joining it
// would run out of road.
// finishedRoundabout returns a roundabout whose arms all exist. The first one
// the city lists may still be on the frontier, with no ring laid yet.
func finishedRoundabout(t *testing.T, c *city.City) *city.Node {
	t.Helper()
	for _, n := range c.Roundabouts() {
		if c.Complete(n) {
			return n
		}
	}
	t.Fatal("no finished roundabout in this city")
	return nil
}

func TestRingIsAClosedLoop(t *testing.T) {
	c := build(7)
	n := finishedRoundabout(t, c)

	var ring []*city.Lane
	for _, l := range c.Lanes {
		if c.Roads[l.Road].Ring && l.FromNode == n.ID {
			ring = append(ring, l)
		}
	}
	if len(ring) < 8 {
		t.Fatalf("ring has %d lanes, want a reasonable circle", len(ring))
	}

	start := ring[0]
	at := start
	for step := range len(ring) * 2 {
		var onward *city.Lane
		for _, s := range at.Succ {
			if c.Roads[c.Lanes[s.Lane].Road].Ring {
				onward = c.Lanes[s.Lane]
				break
			}
		}
		if onward == nil {
			t.Fatalf("ring lane %d after %d steps has no continuation", at.ID, step)
		}
		if onward == start {
			return // closed
		}
		at = onward
	}
	t.Error("following the ring never returned to the start")
}

func TestRoundaboutApproachesGiveWay(t *testing.T) {
	c := build(7)
	n := finishedRoundabout(t, c)

	var approaches int
	for _, l := range c.Lanes {
		if l.ToNode != n.ID || c.Roads[l.Road].Ring {
			continue
		}
		approaches++
		if l.Control != city.ControlGiveWay {
			t.Errorf("approach lane %d has control %v, want give way", l.ID, l.Control)
		}
		if len(l.Succ) == 0 {
			t.Errorf("approach lane %d does not join the ring", l.ID)
		}
		for _, s := range l.Succ {
			if !c.Roads[c.Lanes[s.Lane].Road].Ring {
				t.Errorf("approach lane %d leads somewhere other than the ring", l.ID)
			}
		}
	}
	if approaches == 0 {
		t.Fatal("the roundabout has no approach lanes")
	}
}

// Every arm has to be reachable, or the roundabout would be a dead end.
func TestRingLeavesByEveryArm(t *testing.T) {
	c := build(7)
	n := finishedRoundabout(t, c)

	exits := map[int]bool{}
	for _, l := range c.Lanes {
		if !c.Roads[l.Road].Ring || l.FromNode != n.ID {
			continue
		}
		for _, s := range l.Succ {
			if r := c.Roads[c.Lanes[s.Lane].Road]; !r.Ring {
				exits[r.ID] = true
			}
		}
	}
	if len(exits) != n.Degree() {
		t.Errorf("ring leaves by %d arms, want all %d", len(exits), n.Degree())
	}
}

func TestNoBuildingStandsInARoundabout(t *testing.T) {
	c := build(7)
	for _, n := range c.Roundabouts() {
		for i, b := range c.Buildings {
			if b.Center.DistTo(n.Pos) < n.RingRadius() {
				t.Errorf("building %d stands inside roundabout %d", i, n.ID)
			}
		}
	}
}
