package city_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// fingerprint reduces a city to a canonical description of everything a driver
// could notice, keyed by position rather than by ID. Two cities that agree on
// this are the same place, whatever order they were built in.
func fingerprint(c *city.City) []string {
	var out []string
	for _, n := range c.Nodes {
		if !n.Settled() {
			continue
		}
		out = append(out, fmt.Sprintf("node %d,%d r=%.2f round=%v ctrl=%d",
			n.GridI, n.GridJ, n.Radius, n.Roundabout, n.Control))
	}
	for _, l := range c.Lanes {
		if c.Roads[l.Road].Ring {
			// Ring lanes are keyed by their own geometry; they have no grid
			// cell of their own to name.
			out = append(out, fmt.Sprintf("ring %.2f,%.2f->%.2f,%.2f succ=%d",
				l.A.X, l.A.Z, l.B.X, l.B.Z, len(l.Succ)))
			continue
		}
		out = append(out, fmt.Sprintf("lane %.2f,%.2f->%.2f,%.2f idx=%d dir=%d ctrl=%d limit=%.1f succ=%d",
			l.A.X, l.A.Z, l.B.X, l.B.Z, l.Index, l.Dir, l.Control, l.SpeedLimit, len(l.Succ)))
	}
	for _, b := range c.Buildings {
		out = append(out, fmt.Sprintf("building %.2f,%.2f %.1fx%.1f h=%.1f",
			b.Center.X, b.Center.Z, b.W, b.D, b.Height))
	}
	for _, p := range c.Props {
		out = append(out, fmt.Sprintf("prop %d at %.2f,%.2f", p.Kind, p.Pos.X, p.Pos.Z))
	}
	sort.Strings(out)
	return out
}

func diff(t *testing.T, want, got []string) {
	t.Helper()
	w := map[string]bool{}
	for _, s := range want {
		w[s] = true
	}
	g := map[string]bool{}
	for _, s := range got {
		g[s] = true
	}
	shown := 0
	for _, s := range want {
		if !g[s] && shown < 5 {
			t.Errorf("missing from the grown city: %s", s)
			shown++
		}
	}
	for _, s := range got {
		if !w[s] && shown < 10 {
			t.Errorf("the grown city invented: %s", s)
			shown++
		}
	}
}

// TestGrownCityMatchesOneBuiltAtOnce is the property the whole restructure
// exists for. A city reached a piece at a time, as somebody drives into it,
// must be indistinguishable from the same ground generated in one pass — the
// same junctions, the same lanes with the same successors and limits, the same
// buildings and signs. If it is not, driving out and back would change the
// world behind you.
func TestGrownCityMatchesOneBuiltAtOnce(t *testing.T) {
	const seed = 7
	const span = 600

	whole := city.NewGrowing(city.DefaultParams(seed))
	whole.EnsureAround(mathx.V(0, 0), span)

	// The same ground, reached in four overlapping steps from different
	// directions, in an order no single pass would ever use.
	grown := city.NewGrowing(city.DefaultParams(seed))
	for _, at := range []mathx.Vec{
		mathx.V(-300, -300), mathx.V(300, -300),
		mathx.V(-300, 300), mathx.V(300, 300),
		mathx.V(0, 0),
	} {
		grown.EnsureAround(at, span/2)
	}
	// Then the full extent, so both cover the same ground.
	grown.EnsureAround(mathx.V(0, 0), span)

	diff(t, fingerprint(whole), fingerprint(grown))
}

func TestGrowingIsIdempotent(t *testing.T) {
	c := city.NewGrowing(city.DefaultParams(3))
	c.EnsureAround(mathx.V(0, 0), 400)
	before := fingerprint(c)

	c.EnsureAround(mathx.V(0, 0), 400)
	c.EnsureAround(mathx.V(0, 0), 200)
	diff(t, before, fingerprint(c))
}

// TestStandingStillCostsNothing holds the line the growing city has to keep to
// be called every frame: a car that has not moved far enough to reach new
// ground must cost nothing at all, however far it has driven to get here.
func TestStandingStillCostsNothing(t *testing.T) {
	c := city.NewGrowing(city.DefaultParams(8))
	at := mathx.V(0, 0)
	c.EnsureAround(at, 700)
	lanes, buildings := len(c.Lanes), len(c.Buildings)

	if n := testing.AllocsPerRun(50, func() { c.EnsureAround(at, 700) }); n != 0 {
		t.Errorf("a frame that went nowhere allocated %.0f times, want 0", n)
	}
	if len(c.Lanes) != lanes || len(c.Buildings) != buildings {
		t.Errorf("the city grew by %d lanes and %d buildings while the car stood still",
			len(c.Lanes)-lanes, len(c.Buildings)-buildings)
	}
}

func TestGrowingExtendsTheWorld(t *testing.T) {
	c := city.NewGrowing(city.DefaultParams(4))
	c.EnsureAround(mathx.V(0, 0), 300)
	near := len(c.Lanes)

	c.EnsureAround(mathx.V(2000, 0), 300)
	if len(c.Lanes) <= near {
		t.Fatalf("driving two kilometres added no lanes: %d, was %d", len(c.Lanes), near)
	}
	// And the far ground is drivable, not just present.
	if p := c.Project(mathx.V(2000, 0)); !p.Valid {
		t.Error("no lane near the far position")
	}
}

func TestIdentitiesSurviveGrowth(t *testing.T) {
	// Agents hold lanes, routes hold lanes, the judge holds a lane. Growing
	// the world must not move any of them.
	c := city.NewGrowing(city.DefaultParams(5))
	c.EnsureAround(mathx.V(0, 0), 400)

	type snap struct {
		id     int
		a, b   mathx.Vec
		road   int
		lane   *city.Lane
		toNode int
	}
	var before []snap
	for _, l := range c.Lanes {
		before = append(before, snap{l.ID, l.A, l.B, l.Road, l, l.ToNode})
	}

	c.EnsureAround(mathx.V(1200, 900), 500)

	for _, was := range before {
		now := c.Lanes[was.id]
		if now != was.lane {
			t.Fatalf("lane %d is a different object after growth", was.id)
		}
		if now.A != was.a || now.B != was.b || now.Road != was.road || now.ToNode != was.toNode {
			t.Fatalf("lane %d changed: %+v", was.id, now)
		}
	}
}

func TestEveryLaneCanBeLeft(t *testing.T) {
	// A lane with no successor strands whatever is driving it. Only lanes on
	// the frontier may have none, and the frontier is beyond where anybody has
	// been sent.
	c := city.NewGrowing(city.DefaultParams(6))
	c.EnsureAround(mathx.V(0, 0), 500)

	stranded := 0
	for _, l := range c.Lanes {
		if len(l.Succ) > 0 {
			continue
		}
		if l.B.Len() < 300 {
			stranded++
			t.Errorf("lane %d ends at %v with nowhere to go", l.ID, l.B)
		}
	}
	if stranded > 0 {
		t.Logf("%d lanes well inside the grown area had no exit", stranded)
	}
}

func TestGeneratePathStillWorks(t *testing.T) {
	// The fixed-size entry point is the growing one underneath, so it has to
	// keep producing a complete, drivable city.
	c := city.Generate(city.DefaultParams(7))
	if len(c.Lanes) == 0 || len(c.Buildings) == 0 {
		t.Fatalf("Generate produced %d lanes and %d buildings", len(c.Lanes), len(c.Buildings))
	}
	if p := c.Project(mathx.V(0, 0)); !p.Valid {
		t.Error("nothing drivable at the origin")
	}
}
