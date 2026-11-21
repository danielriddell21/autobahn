package city_test

import (
	"strings"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
)

func TestEveryMapHasANameThatParsesBack(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range city.Maps() {
		name := m.Name()
		if name == "" || seen[name] {
			t.Errorf("map %d is named %q, which is empty or taken", m, name)
		}
		seen[name] = true
		got, ok := city.ParseMap(strings.ToUpper(name) + " ")
		if !ok || got != m {
			t.Errorf("ParseMap(%q) = %v, %v; want %v", name, got, ok, m)
		}
	}
	if _, ok := city.ParseMap("atlantis"); ok {
		t.Error("ParseMap accepted a map that does not exist")
	}
}

func TestParamsForCarriesTheMap(t *testing.T) {
	for _, m := range city.Maps() {
		p := city.ParamsFor(m, 9)
		if p.Map != m || p.Seed != 9 {
			t.Errorf("ParamsFor(%v, 9) = map %v seed %d", m, p.Map, p.Seed)
		}
	}
	if p := city.ParamsFor(city.Map(99), 1); p.Map != city.MapCity {
		t.Errorf("an unknown map generated as %v, want the city", p.Map)
	}
}

// The first map is the city the game began with, and has to stay the city it
// was: the gate's numbers are measured on it.
func TestTheCityMapIsTheDefaultCity(t *testing.T) {
	if got, want := city.ParamsFor(city.MapCity, 5), city.DefaultParams(5); got != want {
		t.Errorf("city map params %+v differ from the defaults %+v", got, want)
	}
}
