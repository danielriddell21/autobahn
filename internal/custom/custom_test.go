package custom_test

import (
	"math"
	"math/rand/v2"
	"path/filepath"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/weather"
)

// The slider table is what a code stores, so every slider's ends are the
// ranges city.Clamp holds, and the city preset is reproduced exactly.
func TestSlidersLieInsideTheRanges(t *testing.T) {
	for _, m := range city.Maps() {
		_, outside := custom.FromMap(m, 5).Params()
		if outside {
			t.Errorf("%s as sliders is outside the ranges", m.Name())
		}
	}
	// With every slider at either end, still inside.
	for _, hi := range []bool{false, true} {
		s := custom.FromMap(city.MapCity, 1)
		for i := range s.Step {
			if hi {
				s.Step[i] = custom.Sliders[i].Max
			} else {
				s.Step[i] = 0
			}
		}
		if p, outside := s.Params(); outside {
			t.Errorf("all sliders at their ends (high %v) is outside the ranges: %+v", hi, p)
		}
	}
}

func TestCityAsSlidersIsTheCity(t *testing.T) {
	got, _ := custom.FromMap(city.MapCity, 7).Params()
	if want := city.ParamsFor(city.MapCity, 7); got != want {
		t.Errorf("the city as sliders is\n%+v\nnot\n%+v", got, want)
	}
}

func TestParamsKeepTheBaseMap(t *testing.T) {
	for _, m := range city.Maps() {
		if p, _ := custom.FromMap(m, 3).Params(); p.Map != m || p.Seed != 3 {
			t.Errorf("%s: params are map %v seed %d", m.Name(), p.Map, p.Seed)
		}
	}
}

func TestEdgeIsTheHardestEnd(t *testing.T) {
	p, outside := custom.Edge(city.MapCity, 1).Params()
	if outside {
		t.Fatal("the edge profile is outside the ranges")
	}
	for name, got := range map[string]float64{
		"roundabouts": float64(p.Roundabouts), "wind": float64(p.Wind), "hills": float64(p.Hills),
		"minSpan": float64(p.MinSpan), "maxSpan": float64(p.MaxSpan),
	} {
		if math.Abs(got-city.RangeOf(name).Hi) > 1e-5 {
			t.Errorf("edge %s is %v, want the top of its range %v", name, got, city.RangeOf(name).Hi)
		}
	}
	if p.Hills/p.HillSpan > 8.0/850*1.00001 {
		t.Errorf("edge hills are steeper than the village's: %v over %v", p.Hills, p.HillSpan)
	}
}

func TestUnsupportedFlag(t *testing.T) {
	s := custom.FromMap(city.MapCity, 1)
	s.Step[custom.Wind] = 14 // 56 m of winding, past the top of the range
	if s.Flagged() {
		t.Error("a map that has not asked to be unsupported was flagged")
	}
	if p, _ := s.Params(); p.Wind != 40 {
		t.Errorf("not clamped: wind %v", p.Wind)
	}
	s.Unsupported = true
	if !s.Flagged() {
		t.Error("56 m of winding was not flagged")
	}
	if p, _ := s.Params(); p.Wind != 56 {
		t.Errorf("wind %v, want the 56 asked for", p.Wind)
	}
	if custom.FromMap(city.MapVillage, 1).Flagged() {
		t.Error("a preset was flagged")
	}
}

func TestApplyFillsTheConfig(t *testing.T) {
	s := custom.FromMap(city.MapCity, 11)
	s.Step[custom.Season] = int(city.Winter) + 1
	s.Step[custom.Traffic] = int(city.MapVillage) + 1
	s.Weather = weather.Held(weather.Rain)
	cfg := s.Apply(sim.Config{Traffic: 33, Police: 2})
	if cfg.Traffic != 33 || cfg.Police != 2 {
		t.Error("Apply overwrote what is not the map's")
	}
	if cfg.Map != city.MapCity || cfg.Seed != 11 || cfg.Params == nil || cfg.Params.Seed != 11 {
		t.Errorf("config %+v", cfg)
	}
	if cfg.Season == nil || *cfg.Season != city.Winter || cfg.Mix == nil || *cfg.Mix != sim.MixFor(city.MapVillage) {
		t.Error("season or mix not set")
	}
	if cfg.Weather != weather.Held(weather.Rain) || cfg.GripScale != 1 {
		t.Errorf("weather %v grip %v", cfg.Weather, cfg.GripScale)
	}
	w := sim.NewWorld(cfg)
	if w.Climate.Season != city.Winter {
		t.Error("the world is not in winter")
	}
}

// The road is never given less grip than winter's.
func TestGripNeverBelowWinter(t *testing.T) {
	s := custom.FromMap(city.MapCity, 1)
	s.Step[custom.Grip] = 0
	if got := s.Apply(sim.Config{}).GripScale; got != 0.7 {
		t.Errorf("grip scale %v on an autumn road at the bottom of the slider, want 0.7", got)
	}
	s.Step[custom.Season] = int(city.Winter) + 1
	if got := s.Apply(sim.Config{}).GripScale; got != 1 {
		t.Errorf("grip scale %v on a winter road, want 1", got)
	}
}

func TestLockedByPack(t *testing.T) {
	s := custom.FromMap(city.MapMotorway, 1)
	if err := s.Locked(pack.Access{}); err == nil {
		t.Error("a locked base map was offered")
	}
	if err := s.Locked(pack.Access{Bypass: true}); err != nil {
		t.Error(err)
	}
	c := custom.FromMap(city.MapCity, 1)
	c.Step[custom.Traffic] = int(city.MapSeaside) + 1
	if err := c.Locked(pack.Access{}); err == nil {
		t.Error("a locked pack's traffic was offered")
	}
	if err := custom.FromMap(city.MapCity, 1).Locked(pack.Access{}); err != nil {
		t.Error(err)
	}
}

func TestBanks(t *testing.T) {
	s := custom.FromMap(city.MapCity, 1)
	if custom.Banks(&s) || !custom.Banks(nil) {
		t.Error("only a game with no custom map banks")
	}
}

func TestPresetsStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), custom.PresetsFile)
	if got := custom.LoadPresets(path); len(got) != 0 {
		t.Fatalf("a missing file gave %v", got)
	}
	p := custom.Presets{}
	a := custom.Edge(city.MapVillage, 42)
	if err := p.Put("hard", a); err != nil {
		t.Fatal(err)
	}
	if err := p.Put("  ", a); err == nil {
		t.Error("a nameless map was kept")
	}
	p["broken"] = "AB1-nonsense"
	if err := custom.SavePresets(path, p); err != nil {
		t.Fatal(err)
	}
	got := custom.LoadPresets(path)
	if names := got.Names(); len(names) != 1 || names[0] != "hard" {
		t.Fatalf("loaded %v; the broken one should be dropped", names)
	}
	b, err := got.Get("hard")
	if err != nil || b != a {
		t.Errorf("round trip: %+v, %v", b, err)
	}
	if _, err := got.Get("nope"); err == nil {
		t.Error("a map nobody saved was found")
	}
}

func randomSpec(r *rand.Rand) custom.Spec {
	s := custom.Spec{
		Base: city.Maps()[r.IntN(len(city.Maps()))], Seed: r.Uint64(),
		Coast: r.IntN(2) == 0, Unsupported: r.IntN(2) == 0,
		Weather: weather.Setting(r.IntN(int(weather.Drift) + 1)),
	}
	for i := range s.Step {
		s.Step[i] = r.IntN(custom.Sliders[i].Max + 1)
		if s.Unsupported {
			s.Step[i] = r.IntN(custom.Sliders[i].Max+custom.MaxPad+8) - custom.MaxPad
		}
	}
	return s.Normalize()
}
