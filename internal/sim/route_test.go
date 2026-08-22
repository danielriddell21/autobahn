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
