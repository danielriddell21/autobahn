package game

import (
	"math"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

func TestTheCityHasNoHillsOnItsHorizon(t *testing.T) {
	for r := range ridges {
		for a := float32(0); a < 2*math.Pi; a += 0.1 {
			if h := skyline(city.MapCity, r, a); h != 0 {
				t.Fatalf("the city has a hill %.3f high at bearing %.1f", h, a)
			}
		}
	}
}

func TestTheHorizonMeetsItselfAndStaysLow(t *testing.T) {
	for _, m := range []city.Map{city.MapSeaside, city.MapVillage, city.MapMotorway} {
		for r := range ridges {
			top := float32(0)
			for a := float32(-math.Pi); a < math.Pi; a += 0.01 {
				h := skyline(m, r, a)
				if h < 0 || h > 0.16 {
					t.Fatalf("%s: ridge %d stands %.3f at bearing %.2f", m.Name(), r, h, a)
				}
				if again := skyline(m, r, a+2*math.Pi); mathx.Abs(again-h) > 1e-3 {
					t.Fatalf("%s: ridge %d is %.4f at %.2f and %.4f a turn on", m.Name(), r, h, a, again)
				}
				top = max(top, h)
			}
			if top < 0.02 {
				t.Errorf("%s: ridge %d never rises above %.3f", m.Name(), r, top)
			}
		}
	}
}

func TestThereIsNoLandOutToSea(t *testing.T) {
	for r := range ridges {
		for off := float32(-0.6); off <= 0.6; off += 0.05 {
			if h := skyline(city.MapSeaside, r, seaward+off); h != 0 {
				t.Errorf("ridge %d stands %.3f out to sea, %.2f off the line of it", r, h, off)
			}
		}
		if h := skyline(city.MapSeaside, r, seaward+math.Pi); h <= 0 {
			t.Errorf("ridge %d: no land inland of a seaside town", r)
		}
	}
}

func TestWrapAngle(t *testing.T) {
	for _, c := range [][2]float32{{0, 0}, {3, 3}, {4, 4 - 2*math.Pi}, {-4, 2*math.Pi - 4}, {7, 7 - 2*math.Pi}} {
		if got := wrapAngle(c[0]); mathx.Abs(got-c[1]) > 1e-5 {
			t.Errorf("wrapAngle(%v) = %v, want %v", c[0], got, c[1])
		}
	}
}

// A ridge is painted outside the shader, so nothing but its own arithmetic
// keeps it off the colours the camera looks for.
func TestRidgesKeepOffTheVisionColours(t *testing.T) {
	reserved := map[[3]uint8]bool{}
	for _, c := range BoxColours() {
		reserved[[3]uint8{c.R, c.G, c.B}] = true
	}
	if len(reserved) == 0 {
		t.Fatal("setup failed: no reserved colours")
	}
	lands := []rl.Color{rl.NewColor(255, 255, 255, 255), rl.NewColor(0, 0, 0, 255)}
	for _, s := range []city.Season{city.Autumn, city.Summer, city.Spring, city.Winter} {
		lands = append(lands, lookFor(s).park)
	}
	for tm := range NumTimes {
		l := lightingFor(tm)
		for _, land := range lands {
			for r := range ridges {
				c := ridgeShade(land, l, r)
				if c.R > 250 || c.G > 250 || c.B > 250 {
					t.Errorf("%v: ridge %d of %v land is %v, with a channel all but full", tm, r, land, c)
				}
				if reserved[[3]uint8{c.R, c.G, c.B}] {
					t.Errorf("%v: ridge %d of %v land is %v, a colour the camera looks for", tm, r, land, c)
				}
			}
		}
	}
}
