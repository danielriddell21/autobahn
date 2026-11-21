package game

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// planOf builds enough of a map to have blocks of every kind.
func planOf(t *testing.T, m city.Map) *city.City {
	t.Helper()
	c := city.NewGrowing(city.ParamsFor(m, 7))
	c.EnsureAround(mathx.V(0, 0), 500)
	return c.Plan()
}

func TestTheCityAndTheBeachStayFlat(t *testing.T) {
	for _, b := range planOf(t, city.MapCity).Blocks {
		if _, ok := swellOf(b, city.MapCity); ok {
			t.Fatalf("a block of the city at %v rises", b.Min)
		}
	}
	beaches := 0
	for _, b := range planOf(t, city.MapSeaside).Blocks {
		if !b.Beach {
			continue
		}
		beaches++
		if _, ok := swellOf(b, city.MapSeaside); ok {
			t.Errorf("the beach at %v rises", b.Min)
		}
	}
	if beaches == 0 {
		t.Fatal("setup failed: no beach to look at")
	}
}

// Whatever is built on a block stands on its rim, and its hedge with it, so
// the rim has to be level; and a hill has to stay a hill.
func TestOpenGroundRisesOnlyInsideItsRim(t *testing.T) {
	for _, m := range []city.Map{city.MapSeaside, city.MapVillage, city.MapMotorway} {
		plan := planOf(t, m)
		risen := 0
		for _, b := range plan.Blocks {
			s, ok := swellOf(b, m)
			if !ok {
				continue
			}
			risen++
			in := float32(swellBack)
			if b.Park {
				in = swellRim
			}
			top := float32(0)
			for i := range 41 {
				for j := range 41 {
					p := mathx.V(
						mathx.Lerp(b.Min.X, b.Max.X, float32(i)/40),
						mathx.Lerp(b.Min.Z, b.Max.Z, float32(j)/40))
					h := s.at(p)
					edge := min(p.X-b.Min.X, b.Max.X-p.X, p.Z-b.Min.Z, b.Max.Z-p.Z)
					if edge <= in && h != 0 {
						t.Fatalf("%s: the rim at %v, %.1fm in, stands %.2fm up", m.Name(), p, edge, h)
					}
					if h < 0 || h > swellTop {
						t.Fatalf("%s: the ground at %v stands %.2fm up", m.Name(), p, h)
					}
					top = max(top, h)
				}
			}
			if top < 0.5 {
				t.Errorf("%s: the block at %v is said to rise and reaches %.2fm", m.Name(), b.Min, top)
			}
			for _, c := range b.Corners {
				if c.Bite > 0 && s.at(c.Centre.Lerp(b.Min.Lerp(b.Max, 0.5), 0.02)) != 0 {
					t.Errorf("%s: the ground rises inside the roundabout at %v", m.Name(), c.Centre)
				}
			}
		}
		if risen == 0 {
			t.Errorf("%s: nothing rises", m.Name())
		}
	}
}

// A slope with a step in it would show as a cliff, and tear the net.
func TestRisingGroundHasNoStepsInIt(t *testing.T) {
	plan := planOf(t, city.MapVillage)
	for _, b := range plan.Blocks {
		s, ok := swellOf(b, city.MapVillage)
		if !ok {
			continue
		}
		for i := range 400 {
			f := float32(i) / 400
			p := b.Min.Lerp(b.Max, f)
			q := b.Min.Lerp(b.Max, f+0.0005)
			if d, run := mathx.Abs(s.at(q)-s.at(p)), q.DistTo(p); d > run*1.2+1e-3 {
				t.Fatalf("the ground at %v climbs %.3fm in %.3fm", p, d, run)
			}
		}
	}
}

// A green keeps a flat top for its paths to cross on.
func TestAGreenIsLevelWhereItsPathsMeet(t *testing.T) {
	for _, m := range []city.Map{city.MapSeaside, city.MapVillage, city.MapMotorway} {
		for _, b := range planOf(t, m).Blocks {
			s, ok := swellOf(b, m)
			if !ok || !s.level {
				continue
			}
			_, c, has := parkFeature(b)
			if !has {
				continue
			}
			for _, d := range []mathx.Vec{{X: parkCircle}, {X: -parkCircle}, {Z: parkCircle}, {Z: -parkCircle}} {
				if got := s.at(c.Add(d)); got != s.at(c) {
					t.Errorf("%s: the green at %v is %.2fm at its middle and %.2fm at the edge of the circle",
						m.Name(), b.Min, s.at(c), got)
				}
			}
		}
	}
}

func TestTheRimMeetsTheNetEverywhere(t *testing.T) {
	// A square inside a square, each with points along its sides.
	ring := func(lo, hi float32, n int) []mathx.Vec {
		var out []mathx.Vec
		corners := []mathx.Vec{{X: lo, Z: lo}, {X: hi, Z: lo}, {X: hi, Z: hi}, {X: lo, Z: hi}}
		for i, p := range corners {
			q := corners[(i+1)%4]
			for k := range n {
				out = append(out, p.Lerp(q, float32(k)/float32(n)))
			}
		}
		return out
	}
	outer, inner := ring(0, 100, 9), ring(20, 80, 5)
	tris := stitch(outer, inner)
	if got, want := len(tris)/3, len(outer)+len(inner); got != want {
		t.Fatalf("%d triangles, want one for every point: %d", got, want)
	}
	var area float32
	for i := 0; i+2 < len(tris); i += 3 {
		a := tris[i+1].Sub(tris[i]).Cross(tris[i+2].Sub(tris[i])) / 2
		if a < 0 {
			t.Fatalf("triangle %d is turned over, and so lies on top of another", i/3)
		}
		area += a
	}
	if want := float32(100*100 - 60*60); mathx.Abs(area-want) > 1 {
		t.Errorf("the rim covers %.0f square metres, want %.0f", area, want)
	}
	if stitch(outer[:2], inner) != nil {
		t.Error("a line that encloses nothing was stitched")
	}
}

func TestTreesOnAHillStandOnIt(t *testing.T) {
	plan := planOf(t, city.MapVillage)
	seen := 0
	for _, b := range plan.Blocks {
		s, ok := swellOf(b, city.MapVillage)
		trees := copse(b, city.MapVillage)
		if (!ok || b.Park) && len(trees) != 0 {
			t.Fatalf("a copse on the block at %v, which has no hill behind houses", b.Min)
		}
		for _, tr := range trees {
			seen++
			if tr.at.X < s.lo.X || tr.at.X > s.hi.X || tr.at.Z < s.lo.Z || tr.at.Z > s.hi.Z {
				t.Errorf("a tree at %v is off its hill", tr.at)
			}
			if tr.lift != s.at(tr.at) {
				t.Errorf("a tree at %v stands at %.2fm on ground %.2fm up", tr.at, tr.lift, s.at(tr.at))
			}
		}
	}
	if seen == 0 {
		t.Error("no trees on any hill")
	}
}

func TestACameraHighUpLooksFromFurtherOff(t *testing.T) {
	if near, far := clipFor(4); near != clipNear || far != clipFar {
		t.Errorf("from a car the camera sees from %.2f to %.0f", near, far)
	}
	near, far := clipFor(400)
	if near < 50 || near > 300 {
		t.Errorf("from 400m up the camera starts looking %.0fm down", near)
	}
	if far < 1200 {
		t.Errorf("from 400m up the camera sees only %.0fm", far)
	}
}
