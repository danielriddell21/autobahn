package sim_test

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/sim"
)

func kinds(w *sim.World) []sim.BodyKind {
	var out []sim.BodyKind
	for _, a := range w.Agents {
		out = append(out, a.Body.Kind)
	}
	return out
}

// A config that names no mix drives the map's own, and one that names it gets
// the same world.
func TestNilMixIsTheMapsMix(t *testing.T) {
	base := sim.Config{Seed: 4, Map: city.MapVillage, Traffic: 40}
	mix := sim.MixFor(city.MapVillage)
	named := base
	named.Mix = &mix
	a, b := kinds(sim.NewWorld(base)), kinds(sim.NewWorld(named))
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("%d cars against %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("car %d is a %v with no mix and a %v with the map's own", i, a[i], b[i])
		}
	}
}

func TestMixOverridesTheTraffic(t *testing.T) {
	var mix sim.Mix
	mix[sim.Lorry] = 1
	w := sim.NewWorld(sim.Config{Seed: 4, Map: city.MapCity, Traffic: 30, Police: 0, Mix: &mix})
	for _, a := range w.Agents {
		if a.Body.Kind != sim.Lorry {
			t.Fatalf("a %v on a road that has only lorries", a.Body.Kind)
		}
	}
	// An empty mix is the city's rather than a crash.
	var none sim.Mix
	if w := sim.NewWorld(sim.Config{Seed: 4, Traffic: 10, Mix: &none}); len(w.Agents) == 0 {
		t.Error("no traffic from an empty mix")
	}
}

func TestSeasonAndGripScaleChangeGrip(t *testing.T) {
	grip := func(cfg sim.Config) float32 { return sim.NewWorld(cfg).Player.Spec.GripFront }
	cfg := sim.Config{Seed: 2, Map: city.MapCity, Traffic: 5}
	dry := grip(cfg)

	winter := city.Winter
	cfg.Season = &winter
	snow := grip(cfg)
	if snow >= dry {
		t.Errorf("a winter override left grip at %v, dry is %v", snow, dry)
	}
	if want := sim.SeasonGrip(city.Winter); snow != dry*want {
		t.Errorf("winter grip %v, want %v", snow, dry*want)
	}

	cfg.GripScale = 0.8
	if scaled := grip(cfg); scaled >= snow {
		t.Errorf("scaling the grip down left it at %v from %v", scaled, snow)
	}

	// And it reaches the traffic as well as the player.
	w := sim.NewWorld(cfg)
	if a := w.Agents[0]; a.V.Spec != a.Body.Spec.OnSurface(sim.SeasonGrip(city.Winter)*0.8) {
		t.Fatal("traffic is not on the road the config asked for")
	}

	// The motorway is a winter map; summer over it takes the snow off.
	summer := city.Summer
	if got := grip(sim.Config{Seed: 2, Map: city.MapMotorway, Season: &summer}); got != dry {
		t.Errorf("a summer motorway has grip %v, want %v", got, dry)
	}
}

// Params replaces the preset's, and the seed stays the config's.
func TestParamsOverrideTheMapsOwn(t *testing.T) {
	p := city.ParamsFor(city.MapCity, 0)
	p.Wind = 20
	w := sim.NewWorld(sim.Config{Seed: 9, Map: city.MapCity, Params: &p})
	if w.City.Params().Wind != 20 || w.City.Params().Seed != 9 {
		t.Errorf("the city was built from %+v", w.City.Params())
	}
}
