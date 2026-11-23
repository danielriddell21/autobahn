package game

import (
	"math"
	"reflect"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/weather"
)

func TestPlanIsTheSameForTheSameSpec(t *testing.T) {
	for _, m := range city.Maps() {
		sp := custom.FromMap(m, 11)
		a, b := buildPlan(sp), buildPlan(sp)
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s: two plans for one spec differ", m.Name())
		}
		if len(a.Roads) == 0 || len(a.Blocks) == 0 {
			t.Errorf("%s: plan has %d roads and %d blocks", m.Name(), len(a.Roads), len(a.Blocks))
		}
	}
}

func TestPlanIsReadFromTheRealCity(t *testing.T) {
	flat := buildPlan(custom.FromMap(city.MapCity, 3))
	if flat.Hilly() || flat.Sea || flat.Winding {
		t.Errorf("the city is level, has no sea and is straight: hilly %v, sea %v, winding %v", flat.Hilly(), flat.Sea, flat.Winding)
	}
	for _, r := range flat.Roads {
		if len(r.Pts) != 2 {
			t.Fatalf("a straight street has %d points", len(r.Pts))
		}
	}

	sea := buildPlan(custom.FromMap(city.MapSeaside, 3))
	if !sea.Sea || sea.SeaZ <= sea.FrontZ {
		t.Errorf("the seatown's sea is at %v with the front at %v", sea.SeaZ, sea.FrontZ)
	}
	if sea.SeaZ < sea.Min.Z || sea.SeaZ > sea.Max.Z {
		t.Error("the sea is not in the patch")
	}
	beach := false
	for _, b := range sea.Blocks {
		beach = beach || b.Kind == planBeach
	}
	if !beach {
		t.Error("a coast with no beach")
	}

	vil := buildPlan(custom.FromMap(city.MapVillage, 3))
	if !vil.Hilly() || !vil.Winding {
		t.Errorf("the village has hills and bends: hilly %v, winding %v", vil.Hilly(), vil.Winding)
	}
	bent := false
	for _, r := range vil.Roads {
		bent = bent || len(r.Pts) > 2
	}
	if !bent {
		t.Error("no street of the village is bent")
	}
}

func TestPlanFollowsTheSliders(t *testing.T) {
	base := custom.FromMap(city.MapCity, 9)
	base.Step[custom.Roundabouts] = 0
	count := func(sp custom.Spec) (rounds, open, main int) {
		pl := buildPlan(sp)
		for _, b := range pl.Blocks {
			if b.Kind == planOpen {
				open++
			}
		}
		for _, r := range pl.Roads {
			if r.Class == city.Arterial {
				main++
			}
		}
		return len(pl.Rounds), open, main
	}
	r0, o0, m0 := count(base)

	more := base
	more.Step[custom.Roundabouts] = custom.Sliders[custom.Roundabouts].Max
	more.Step[custom.Parks] = custom.Sliders[custom.Parks].Max
	more.Step[custom.MainEvery] = custom.Sliders[custom.MainEvery].Max
	r1, o1, m1 := count(more)
	if r1 <= r0 {
		t.Errorf("roundabouts at their most: %d, against %d", r1, r0)
	}
	if o1 <= o0 {
		t.Errorf("open ground at its most: %d blocks, against %d", o1, o0)
	}
	if m1 >= m0 {
		t.Errorf("a main road every five lines gives %d arterial roads, every three %d", m1, m0)
	}

	wound := base
	wound.Step[custom.Wind] = custom.Sliders[custom.Wind].Max
	if pl := buildPlan(wound); !pl.Winding {
		t.Error("full winding did not bend the streets")
	}
	hilly := base
	hilly.Step[custom.Hills] = custom.Sliders[custom.Hills].Max
	hilly.Step[custom.HillSpan] = custom.Sliders[custom.HillSpan].Max
	if pl := buildPlan(hilly); !pl.Hilly() || pl.High-pl.Low < 1 {
		t.Errorf("hills at their highest show %.1f to %.1f m", pl.Low, pl.High)
	}
	coast := base
	coast.Coast = true
	if pl := buildPlan(coast); !pl.Sea {
		t.Error("a coast on the city put no sea in the plan")
	}
}

func TestPlanIsTintedForTheSeason(t *testing.T) {
	sp := custom.FromMap(city.MapCity, 4)
	if buildPlan(sp).Season != city.Autumn {
		t.Error("the city is in autumn")
	}
	sp.Step[custom.Season] = int(city.Winter) + 1
	if buildPlan(sp).Season != city.Winter {
		t.Error("the plan did not take the season the spec asks for")
	}
	seen := map[planInk]bool{}
	for _, s := range city.Seasons() {
		seen[planInkFor(s)] = true
	}
	if len(seen) != len(city.Seasons()) {
		t.Error("two seasons share a palette")
	}
}

func TestPlanStaysInThePatchAndIsFinite(t *testing.T) {
	for _, sp := range []custom.Spec{custom.Edge(city.MapMotorway, 2), custom.FromMap(city.MapVillage, 2)} {
		pl := buildPlan(sp)
		for _, b := range pl.Blocks {
			if len(b.Pts) < 4 {
				t.Fatalf("a block of %d points", len(b.Pts))
			}
			for _, p := range b.Pts {
				if math.IsNaN(float64(p.X)) || math.IsNaN(float64(p.Z)) || math.Abs(float64(p.X)) > 4000 || math.Abs(float64(p.Z)) > 4000 {
					t.Fatalf("a block point at %v", p)
				}
			}
		}
	}
}

func TestPlanKeyIgnoresWhatIsNotDrawn(t *testing.T) {
	sp := custom.FromMap(city.MapCity, 6)
	k := keyOf(sp)
	other := sp
	other.Weather = weather.Held(weather.Fog)
	other.Step[custom.Grip] = 0
	other.Step[custom.Traffic] = 3
	other.Step[custom.Limits] = 0
	if keyOf(other) != k {
		t.Error("weather, grip and traffic changed the key a plan is kept under")
	}
	for name, change := range map[string]func(*custom.Spec){
		"seed":   func(s *custom.Spec) { s.Seed++ },
		"base":   func(s *custom.Spec) { s.Base = city.MapVillage },
		"coast":  func(s *custom.Spec) { s.Coast = true },
		"wind":   func(s *custom.Spec) { s.Step[custom.Wind]++ },
		"season": func(s *custom.Spec) { s.Step[custom.Season] = 4 },
		"span":   func(s *custom.Spec) { s.Step[custom.MinSpan]++ },
	} {
		c := sp
		change(&c)
		if keyOf(c) == k {
			t.Errorf("a different %s keeps the same plan", name)
		}
	}
}

func TestPlanCacheBuildsOnceAndNotWhileTheSpecMoves(t *testing.T) {
	var pc planCache
	sp := custom.FromMap(city.MapCity, 1)
	first := pc.Ask(sp)
	if first == nil || pc.builds != 1 {
		t.Fatalf("the first plan is not built at once: %v, %d builds", first, pc.builds)
	}
	for range 200 {
		if pc.Ask(sp) != first {
			t.Fatal("a plan for an unchanged spec was replaced")
		}
	}
	if pc.builds != 1 {
		t.Fatalf("%d builds for an unchanged spec", pc.builds)
	}

	// Changes that are not drawn cost nothing, however often.
	w := sp
	w.Weather = weather.Held(weather.Rain)
	for range 50 {
		pc.Ask(w)
	}
	if pc.builds != 1 || pc.Stale(w) {
		t.Fatalf("weather rebuilt the plan: %d builds, stale %v", pc.builds, pc.Stale(w))
	}

	// A spec that keeps changing is not built for, and the old plan stays up.
	moving := sp
	for i := range 3 * planSettle {
		moving.Seed = uint64(100 + i)
		if pc.Ask(moving) != first {
			t.Fatal("a plan was built while the spec was still moving")
		}
	}
	if pc.builds != 1 || !pc.Stale(moving) {
		t.Fatalf("%d builds while moving, stale %v", pc.builds, pc.Stale(moving))
	}

	// Once it holds still the new one is built, once.
	var got *customPlan
	for range planSettle + 2 {
		got = pc.Ask(moving)
	}
	if got == first || got.Key != keyOf(moving) || pc.builds != 2 {
		t.Fatalf("the settled spec's plan was not built: %d builds", pc.builds)
	}
	if pc.Stale(moving) {
		t.Error("the plan on show is still stale")
	}
}

func BenchmarkCustomPlan(b *testing.B) {
	for _, m := range city.Maps() {
		sp := custom.FromMap(m, 5)
		b.Run(m.Name(), func(b *testing.B) {
			for range b.N {
				buildPlan(sp)
			}
		})
	}
}
