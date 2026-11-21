package city_test

import (
	"strings"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
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

func TestEveryMapHasASeason(t *testing.T) {
	names := map[string]bool{}
	for _, s := range city.Seasons() {
		if s.Name() == "" || names[s.Name()] {
			t.Errorf("season %d has no name of its own: %q", s, s.Name())
		}
		names[s.Name()] = true
	}
	for _, m := range city.Maps() {
		if !names[m.Season().Name()] {
			t.Errorf("%s is driven in a season that does not exist", m.Name())
		}
	}
	if city.MapCity.Season() != city.Autumn {
		t.Errorf("the city is driven in %s, want autumn", city.MapCity.Season().Name())
	}
}

func TestTheVillageIsLowQuietAndMostlyFields(t *testing.T) {
	grow := func(m city.Map) *city.City {
		c := city.NewGrowing(city.ParamsFor(m, 7))
		c.EnsureAround(mathx.V(0, 0), 1200)
		return c
	}
	village, town := grow(city.MapVillage), grow(city.MapCity)

	for _, b := range village.Buildings {
		if b.Height > 11 {
			t.Fatalf("a building in the village is %.1f m tall", b.Height)
		}
	}
	share := func(c *city.City, is func(city.Block) bool) float32 {
		n := 0
		for _, b := range c.Blocks {
			if is(b) {
				n++
			}
		}
		return float32(n) / float32(len(c.Blocks))
	}
	open := func(b city.Block) bool { return b.Park }
	if v, c := share(village, open), share(town, open); v < 0.3 || v <= c*2 {
		t.Errorf("%.2f of the village is open ground against %.2f of the city", v, c)
	}
	main := func(c *city.City) float32 {
		n, all := 0, 0
		for _, r := range c.Roads {
			if r.Ring {
				continue
			}
			all++
			if r.Class == city.Arterial {
				n++
			}
		}
		return float32(n) / float32(all)
	}
	if v, c := main(village), main(town); v >= c {
		t.Errorf("%.2f of the village's roads are main roads against %.2f of the city's", v, c)
	}
}

func TestTheMotorwayTownIsFastAndHasNoSideStreets(t *testing.T) {
	c := city.NewGrowing(city.ParamsFor(city.MapMotorway, 7))
	c.EnsureAround(mathx.V(0, 0), 1500)
	fast := 0
	for _, r := range c.Roads {
		if r.Ring {
			continue
		}
		if r.Class == city.Local {
			t.Fatalf("road %d in the motorway town is a side street", r.ID)
		}
		want := mathx.MPH(40)
		if r.Class == city.Arterial {
			want = mathx.MPH(50)
			fast++
		}
		for _, id := range append(append([]int{}, r.LanesAB...), r.LanesBA...) {
			if got := c.Lanes[id].SpeedLimit; mathx.Abs(got-want) > 0.01 {
				t.Fatalf("a lane on a %s road is limited to %.0f mph, want %.0f",
					r.Class.Name(), mathx.ToMPH(got), mathx.ToMPH(want))
			}
		}
	}
	if fast == 0 {
		t.Fatal("the motorway town has no fast roads")
	}
	for _, p := range c.Props {
		if p.Kind == city.PropSpeedSign && p.Limit > mathx.MPH(51) {
			t.Fatalf("a sign is posted at %.0f mph", mathx.ToMPH(p.Limit))
		}
	}
	// Everywhere else keeps the usual urban limits.
	town := city.Generate(city.DefaultParams(7))
	for _, l := range town.Lanes {
		if l.SpeedLimit > mathx.MPH(40)+0.01 {
			t.Fatalf("a lane in the city is limited to %.0f mph", mathx.ToMPH(l.SpeedLimit))
		}
	}
	if city.MapMotorway.Season() != city.Winter {
		t.Error("the motorway town should be driven in winter")
	}
}
