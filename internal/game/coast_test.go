package game

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

func seasideBeaches(t *testing.T) []city.Block {
	t.Helper()
	c := city.NewGrowing(city.ParamsFor(city.MapSeaside, 7))
	c.EnsureAround(mathx.V(0, 0), 1200)
	var out []city.Block
	for _, b := range c.Blocks {
		if b.Beach {
			out = append(out, b)
		}
	}
	if len(out) < 6 {
		t.Fatalf("only %d stretches of beach to look at", len(out))
	}
	return out
}

func TestBeachHutsStandInARowOnTheSand(t *testing.T) {
	huts := 0
	for _, b := range seasideBeaches(t) {
		got := beachHuts(b)
		again := beachHuts(b)
		if len(got) != len(again) {
			t.Fatal("the same beach had a different row of huts the second time")
		}
		pier, piered := hasPier(b)
		for i, h := range got {
			huts++
			if h != again[i] {
				t.Fatal("the same beach had a different row of huts the second time")
			}
			if h.at.X < b.Min.X || h.at.X > b.Max.X || h.at.Z < b.Min.Z+promDepth || h.at.Z > b.Max.Z {
				t.Errorf("a hut at %v is off the sand of the beach %v..%v", h.at, b.Min, b.Max)
			}
			if piered && mathx.Abs(h.at.X-pier) < pierWidth/2 {
				t.Errorf("a hut at %v stands where the pier comes through at %.1f", h.at, pier)
			}
		}
	}
	if huts < 20 {
		t.Errorf("only %d beach huts along the whole front", huts)
	}
	if got := beachHuts(city.Block{Min: mathx.V(0, 0), Max: mathx.V(100, 100)}); len(got) != 0 {
		t.Error("a block that is not a beach has beach huts on it")
	}
}

func TestSomeBeachesHaveAPier(t *testing.T) {
	beaches := seasideBeaches(t)
	piers := 0
	for _, b := range beaches {
		x, ok := hasPier(b)
		if !ok {
			continue
		}
		piers++
		if x < b.Min.X+pierWidth || x > b.Max.X-pierWidth {
			t.Errorf("a pier leaves the beach %v..%v at %.1f", b.Min, b.Max, x)
		}
	}
	if piers == 0 || piers == len(beaches) {
		t.Errorf("%d of %d beaches have a pier", piers, len(beaches))
	}
	if _, ok := hasPier(city.Block{Min: mathx.V(0, 0), Max: mathx.V(100, 100)}); ok {
		t.Error("a block that is not a beach has a pier")
	}
}

func TestTheSeasideIsPaintedAndTheCityIsNot(t *testing.T) {
	if facadesFor(city.MapSeaside) == facadesFor(city.MapCity) {
		t.Error("the seaside is the same colours as the city")
	}
	for _, m := range city.Maps() {
		for _, c := range facadesFor(m) {
			if c.R == 255 || c.G == 255 || c.B == 255 {
				t.Errorf("%s paints a wall %v, with a channel at full", m.Name(), c)
			}
		}
	}
}
