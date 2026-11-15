package sim_test

import (
	"math/rand/v2"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// TestCentrelineSpreadsOverARoundabout guards the lookahead against collapsing.
//
// A roundabout's circulating carriageway is cut into chords a few metres long,
// so a lookahead that crossed only one lane boundary ran out of road almost at
// once and clamped every remaining marker onto the same point. The camera then
// saw a single dot where a lane should be, and the autopilot declared the lane
// lost in the middle of the junction.
func TestCentrelineSpreadsOverARoundabout(t *testing.T) {
	c := city.Generate(city.DefaultParams(7))

	lane := approachToARoundabout(t, c)
	// Stand near the end of the approach, so the markers ahead run into the
	// ring rather than along the straight.
	at := lane.Point(lane.Length - 2)

	r := sim.NewRoute(c, rand.New(rand.NewPCG(7, 7)))
	r.Update(at, lane.Fwd)
	if r.Lane() == nil {
		t.Fatal("the route latched onto no lane")
	}

	const spacing float32 = 7
	pts := r.Centreline(4, spacing, 8)
	if len(pts) != 8 {
		t.Fatalf("got %d markers, want 8", len(pts))
	}

	// Consecutive markers are asked for 7 m apart. Around a bend they come out
	// closer than that, but nothing like the zero the collapse produced.
	var run float32
	for i := 1; i < len(pts); i++ {
		d := pts[i].DistTo(pts[i-1])
		if d < spacing*0.4 {
			t.Errorf("markers %d and %d are %.2f m apart: %v and %v", i-1, i, d, pts[i-1], pts[i])
		}
		run += d
	}
	if want := spacing * 4; run < want {
		t.Errorf("the markers cover %.1f m in total, want at least %.1f", run, want)
	}
}

// approachToARoundabout returns a lane arriving at a finished roundabout.
func approachToARoundabout(t *testing.T, c *city.City) *city.Lane {
	t.Helper()
	for _, l := range c.Lanes {
		if c.Roads[l.Road].Ring || l.Length < 10 {
			continue
		}
		n := c.Nodes[l.ToNode]
		if n.Roundabout && c.Complete(n) {
			return l
		}
	}
	t.Fatal("the city has no roundabout to approach")
	return nil
}

// TestCentrelineFollowsAStraight keeps the ordinary case honest: on an open
// road the markers come out at exactly the spacing asked for.
func TestCentrelineFollowsAStraight(t *testing.T) {
	c := city.Generate(city.DefaultParams(7))

	var lane *city.Lane
	for _, l := range c.Lanes {
		if !c.Roads[l.Road].Ring && l.Length > 80 {
			lane = l
			break
		}
	}
	if lane == nil {
		t.Skip("no long straight in this city")
	}

	r := sim.NewRoute(c, rand.New(rand.NewPCG(7, 7)))
	r.Update(lane.Point(2), lane.Fwd)

	const spacing float32 = 7
	pts := r.Centreline(4, spacing, 8)
	for i := 1; i < len(pts); i++ {
		if d := pts[i].DistTo(pts[i-1]); mathx.Abs(d-spacing) > 0.01 {
			t.Errorf("markers %d and %d are %.3f m apart, want %.1f", i-1, i, d, spacing)
		}
	}
}

// follow walks a route along its own guidance for a distance, reporting each
// lane it moves onto. It is a car that drives exactly where it is told.
func follow(t *testing.T, c *city.City, r *sim.Route, metres float32) []*city.Lane {
	t.Helper()
	lanes := []*city.Lane{r.Lane()}
	for range int(metres / 0.5) {
		pts := r.Centreline(0.5, 0.5, 2)
		if len(pts) < 2 || pts[0].DistTo(pts[1]) < 0.01 {
			break // the built city has run out
		}
		r.Update(pts[0], pts[1].Sub(pts[0]).Norm())
		if r.Lane() != lanes[len(lanes)-1] {
			lanes = append(lanes, r.Lane())
		}
	}
	return lanes
}

// TestRouteKeepsItsPlanThroughJunctions guards the route against changing its
// mind in the middle of a turn.
//
// Inside a junction the nearest lane is almost never the one being driven
// onto: it is the lane alongside the one just left, or the one coming the
// other way. A route that latched onto whatever was nearest re-planned from
// there, pointed its guidance back at a lane the car was leaving, and the
// camera lost the lane half-way round the corner.
func TestRouteKeepsItsPlanThroughJunctions(t *testing.T) {
	c := city.Generate(city.DefaultParams(7))
	for seed := range uint64(6) {
		// Somewhere in the middle of the city, with built streets all round.
		var start *city.Lane
		for _, l := range c.Lanes {
			if !c.Roads[l.Road].Ring && l.Length > 60 && l.A.Len() < 220 && l.ID%7 == int(seed) {
				start = l
				break
			}
		}
		if start == nil {
			continue
		}
		r := sim.NewRoute(c, rand.New(rand.NewPCG(seed, 9)))
		r.Update(start.Point(5), start.Fwd)

		lanes := follow(t, c, r, 700)
		if len(lanes) < 3 {
			t.Fatalf("seed %d: the route crossed only %d lanes in 700m", seed, len(lanes))
		}
		for i := 1; i < len(lanes); i++ {
			// Every lane the route moves onto has to lead out of the junction
			// the one before it led into. Anything else is a re-plan.
			if lanes[i].FromNode != lanes[i-1].ToNode {
				t.Fatalf("seed %d: the route jumped from lane %d (into junction %d) to lane %d (out of junction %d)",
					seed, lanes[i-1].ID, lanes[i-1].ToNode, lanes[i].ID, lanes[i].FromNode)
			}
		}
	}
}

// A route joins a roundabout, goes round, and leaves by another road. It does
// not circle for ever, and it does not come back out the way it went in.
func TestRouteLeavesARoundaboutByAnotherRoad(t *testing.T) {
	c := city.Generate(city.DefaultParams(7))
	// The roundabout nearest the middle of the city, so every road off it is
	// built.
	var in *city.Lane
	for _, l := range c.Lanes {
		if c.Roads[l.Road].Ring || l.Length < 30 || !c.Nodes[l.ToNode].IsRoundabout() {
			continue
		}
		if in == nil || l.B.Len() < in.B.Len() {
			in = l
		}
	}
	if in == nil {
		t.Fatal("the city has no roundabout to approach")
	}

	for seed := range uint64(8) {
		r := sim.NewRoute(c, rand.New(rand.NewPCG(seed, 3)))
		r.Update(in.Point(in.Length-20), in.Fwd)

		var out *city.Lane
		for _, l := range follow(t, c, r, 200)[1:] {
			if !c.Roads[l.Road].Ring {
				out = l
				break
			}
		}
		if out == nil {
			t.Fatalf("seed %d: still going round after 200m", seed)
		}
		if out.Road == in.Road {
			t.Errorf("seed %d: left the roundabout by the road it arrived on", seed)
		}
	}
}

// A driver who moves over a lane gets a route planned from the lane they are
// now in.
func TestRouteFollowsALaneChange(t *testing.T) {
	c := city.Generate(city.DefaultParams(7))
	for _, road := range c.Roads {
		if road.Ring || len(road.LanesAB) < 2 || road.Length < 80 {
			continue
		}
		a, b := c.Lanes[road.LanesAB[0]], c.Lanes[road.LanesAB[1]]
		r := sim.NewRoute(c, rand.New(rand.NewPCG(1, 1)))
		r.Update(a.Point(30), a.Fwd)
		if r.Lane() != a {
			t.Fatalf("the route started on lane %d, want %d", r.Lane().ID, a.ID)
		}
		r.Update(b.Point(34), b.Fwd)
		if r.Lane() != b {
			t.Errorf("after moving over, the route is on lane %d, want %d", r.Lane().ID, b.ID)
		}
		return
	}
	t.Skip("no two-lane road in this city")
}
