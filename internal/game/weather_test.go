package game

import (
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/weather"
)

func TestNoWeatherUsesAReservedColour(t *testing.T) {
	reserved := map[[3]uint8]bool{}
	for _, c := range BoxColours() {
		reserved[[3]uint8{c.R, c.G, c.B}] = true
	}
	for _, c := range wxLook.colours() {
		if reserved[[3]uint8{c.R, c.G, c.B}] {
			t.Errorf("the weather draws in %v, which a detection box is drawn in", c)
		}
		if c.R > 240 || c.G > 240 || c.B > 240 {
			t.Errorf("the weather draws in %v, with a channel above 240", c)
		}
	}
}

func lightOf(l lighting) []float32 {
	v := func(a rl.Vector3) []float32 { return []float32{a.X, a.Y, a.Z} }
	var out []float32
	for _, a := range []rl.Vector3{l.sun, l.sky, l.ground} {
		out = append(out, v(a)...)
	}
	return out
}

func TestClearWeatherLeavesTheLightAlone(t *testing.T) {
	for _, h := range []TimeOfDay{TimeDay, TimeDusk, TimeNight} {
		l := lightingFor(h)
		if got := rewet(l, weather.At(1, city.Autumn, weather.Held(weather.Clear), 0), 0); got != l {
			t.Errorf("clear weather changed the light at %s", h)
		}
	}
}

func TestWeatherOnlyTakesLightAwayAndClosesTheView(t *testing.T) {
	for _, h := range []TimeOfDay{TimeDay, TimeDusk, TimeNight} {
		for _, s := range city.Seasons() {
			base := relight(lightingFor(h), s)
			for _, c := range weather.Conds() {
				st := weather.At(1, s, weather.Held(c), 0)
				got := rewet(base, st, 0)
				before, after := lightOf(base), lightOf(got)
				for i := range before {
					if after[i] > before[i]+1e-6 {
						t.Fatalf("%s %s %s: light %d went from %.3f to %.3f", h, s.Name(), c.Name(), i, before[i], after[i])
					}
				}
				if got.fogRange > base.fogRange {
					t.Errorf("%s in %s pushed the fog back", c.Name(), s.Name())
				}
				if c == weather.Fog && got.fogRange > 1.3*45+0.01 {
					t.Errorf("fog in %s has a view of %.0f m", s.Name(), got.fogRange)
				}
				if got.night != base.night {
					t.Errorf("%s changed how dark the night is", c.Name())
				}
			}
		}
	}
}

func TestLightningShowsAtNightAndNotByDay(t *testing.T) {
	st := weather.At(1, city.Summer, weather.Held(weather.Storm), 0)
	day := relight(lightingFor(TimeDay), city.Summer)
	if rewet(day, st, 1) != rewet(day, st, 0) {
		t.Error("lightning lit the day")
	}
	night := relight(lightingFor(TimeNight), city.Summer)
	dark, lit := rewet(night, st, 0), rewet(night, st, 1)
	if lit.skyTop.B <= dark.skyTop.B || lit.sky.X <= dark.sky.X {
		t.Error("lightning did not light the night sky")
	}
	for _, c := range []rl.Color{lit.skyTop, lit.skyLow} {
		if c.R > 240 || c.G > 240 || c.B > 240 {
			t.Errorf("a flash lit the sky to %v", c)
		}
	}
}

func TestAFlashIsBriefAndStrongestFirst(t *testing.T) {
	f := weather.Flash{At: 10, Delay: 2, Strength: 1}
	if flashLevel(f, 9.9) != 0 || flashLevel(f, 11) != 0 {
		t.Error("a flash outside its moment")
	}
	if flashLevel(f, 10.01) < 0.85 {
		t.Error("the flash begins weakly")
	}
	if flashLevel(f, 10.01) <= flashLevel(f, 10.15) {
		t.Error("a flash did not die away")
	}
	if flashLevel(weather.Flash{At: 10, Strength: 0.5}, 10.01) > 0.5 {
		t.Error("a weak strike flashed brightly")
	}
}

func TestWetAsphaltIsNeverBrighter(t *testing.T) {
	road := rl.NewColor(70, 72, 76, 255)
	for _, w := range []float32{0, 0.3, 1, 2} {
		got := wetAsphalt(road, w)
		if got.R > road.R || got.G > road.G || got.B > road.B {
			t.Errorf("road at wetness %.1f is drawn in %v", w, got)
		}
	}
	if wetAsphalt(road, 0) != road {
		t.Error("a dry road changed colour")
	}
	if wetAsphalt(road, 1).R >= road.R {
		t.Error("a wet road is not darker")
	}
}

func TestRainFallsRoundTheCameraFromItsNumberAndTheClock(t *testing.T) {
	rain := weather.At(1, city.Spring, weather.Held(weather.Rain), 0)
	storm := weather.At(1, city.Spring, weather.Held(weather.Storm), 0)
	sleet := weather.At(1, city.Winter, weather.Held(weather.Rain), 0)
	if rainCount(weather.At(1, city.Spring, weather.Held(weather.Clear), 0)) != 0 ||
		rainCount(weather.At(1, city.Spring, weather.Held(weather.Fog), 0)) != 0 {
		t.Error("rain in dry weather")
	}
	if rainCount(storm) <= rainCount(rain) || rainCount(rain) == 0 {
		t.Error("a storm should rain harder than rain")
	}
	if !sleet.Sleet || rainCount(sleet) == 0 {
		t.Error("winter rain is not sleet")
	}
	eye := mathx.V(-431.5, 977.2)
	for i := range rainCount(storm) {
		x, y, z := fallAt(i+1000, 12.5, eye, rainSpeed)
		if mathx.Abs(x-eye.X) > fallReach+0.01 || mathx.Abs(z-eye.Z) > fallReach+0.01 || y < 0 || y > fallHeight {
			t.Fatalf("raindrop %d is at %.1f, %.1f, %.1f", i, x, y, z)
		}
		x2, y2, z2 := fallAt(i+1000, 12.5, eye, rainSpeed)
		if x != x2 || y != y2 || z != z2 {
			t.Fatal("a raindrop was somewhere else when asked again")
		}
	}
}

func TestWeatherSettingDriftsByDefaultAndYieldsToTheFlag(t *testing.T) {
	s := DefaultSettings()
	if s.Weather != weather.Drift {
		t.Errorf("the default weather is %v, want drift", s.Weather)
	}
	if got := s.applyTo(DefaultOptions()); got.Weather != weather.Drift {
		t.Errorf("the stored weather did not reach the session: %v", got.Weather)
	}
	o := DefaultOptions()
	o.Weather, o.Given = weather.Held(weather.Fog), Given{"weather": true}
	if got := s.applyTo(o); got.Weather != weather.Held(weather.Fog) {
		t.Errorf("the flag was overridden by %v", got.Weather)
	}
	// A chase takes the weather from the host, not from a preference.
	if got := s.applyViewTo(DefaultOptions()); got.Weather != DefaultOptions().Weather {
		t.Error("a view setting changed the weather")
	}
}

func TestPlayOffersEveryWeatherOnce(t *testing.T) {
	ws := playWeathers()
	seen := map[weather.Setting]bool{}
	for _, w := range ws {
		seen[w] = true
	}
	if len(ws) != 5 || len(seen) != 5 || ws[0] != weather.Drift || !seen[weather.Held(weather.Storm)] {
		t.Errorf("Play offers %v, want drift then every weather held, once each", ws)
	}
}
