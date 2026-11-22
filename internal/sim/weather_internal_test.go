package sim

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/weather"
)

func TestARoadInTheWetNeedsMoreRoomToStopAndHoldsABendLess(t *testing.T) {
	w := NewWorld(Config{Seed: 7, Map: city.MapCity, Traffic: 30, Police: 3})
	dry := w.Player.Spec
	c := NewClimate(7, city.MapCity, weather.Held(weather.Storm))
	if !c.Apply(w) {
		t.Fatal("a storm changed nothing about how cars drive")
	}
	wet := w.Player.Spec
	if wet.BrakeForce >= dry.BrakeForce || wet.MaxLateral >= dry.MaxLateral {
		t.Error("the player's car has as much grip in a storm as in the dry")
	}
	// The same speed takes longer to stop in.
	v := float32(20)
	stop := func(s Spec) float32 { return v * v * s.Mass / (2 * s.BrakeForce) }
	if stop(wet) <= stop(dry) {
		t.Errorf("a stop from %.0f m/s takes %.1f m wet and %.1f m dry", v, stop(wet), stop(dry))
	}
	for _, a := range w.Agents {
		if a.V.Spec != a.Body.Spec.OnSurface(c.Grip()) {
			t.Fatalf("a %s was left on the dry road", a.Body.Kind.Name())
		}
		if a.style().Decel >= a.Style.Decel && !a.pursuing {
			t.Fatalf("the driver of a %s plans a dry stop in a storm", a.Body.Kind.Name())
		}
	}
	// Nothing to do until the weather moves.
	if c.Apply(w) {
		t.Error("cars were refitted though the grip had not moved")
	}
	// Clear again.
	back := NewClimate(7, city.MapCity, weather.Held(weather.Clear))
	back.applied = c.applied
	if !back.Apply(w) || w.Player.Spec != w.PlayerBody.Spec.OnSurface(1) {
		t.Error("the road did not dry")
	}
}

func TestWinterGripIsTheWorseOfTheSeasonAndTheWeather(t *testing.T) {
	c := NewClimate(1, city.MapMotorway, weather.Held(weather.Rain))
	if c.Season != city.Winter {
		t.Skip("the roadtown is not in winter")
	}
	if got := c.GripAt(0); got != winterGrip {
		t.Errorf("rain on snow gives %.2f, want %.2f", got, winterGrip)
	}
	fog := NewClimate(1, city.MapMotorway, weather.Held(weather.Fog))
	if got := fog.GripAt(0); got != winterGrip {
		t.Errorf("fog on snow gives %.2f", got)
	}
}

func TestGripIsQuantisedAndAppliedRarely(t *testing.T) {
	if got := QuantiseGrip(0.853); got < 0.849 || got > 0.851 {
		t.Errorf("0.853 rounds to %.3f", got)
	}
	w := NewWorld(Config{Seed: 3, Map: city.MapSeaside, Traffic: 5, Police: 0})
	c := NewClimate(3, city.MapSeaside, weather.Drift)
	changes := 0
	for tm := float32(0); tm < 1800; tm += 1.0 / 60 * 10 {
		w.Time = tm
		if c.Apply(w) {
			changes++
		}
	}
	// 30 minutes at ~6 frames a second is 10800 calls; the grip moves in steps
	// of a twentieth and only through a few spells.
	if changes > 150 {
		t.Errorf("cars were refitted %d times in half an hour", changes)
	}
}

func TestClearWorldKeepsTheSeasonsGripAndABareConfigIsClear(t *testing.T) {
	w := NewWorld(Config{Seed: 5, Map: city.MapCity, Traffic: 20, Police: 2})
	spec := w.Player.Spec
	for range 600 {
		w.Update(Controls{}, 1.0/20)
	}
	if w.Player.Spec != spec || w.Weather().Cond != weather.Clear || w.Weather().Sight != weather.Unlimited {
		t.Error("a world built without weather was not left dry")
	}
	for _, a := range w.Agents {
		if a.V.Spec != a.Body.Spec.OnSurface(SeasonGrip(city.MapCity.Season())) {
			t.Fatal("clear weather changed a traffic car")
		}
	}
}

func TestStormInAWorldLowersGripOnEveryoneAndSetPlayerBodyKeepsIt(t *testing.T) {
	cfg := Config{Seed: 5, Map: city.MapCity, Traffic: 20, Police: 2}
	dry := NewWorld(cfg)
	cfg.Weather = weather.Held(weather.Storm)
	w := NewWorld(cfg)
	if w.Weather().Cond != weather.Storm {
		t.Fatalf("held storm gives %v", w.Weather().Cond)
	}
	if w.Player.Spec.BrakeForce >= dry.Player.Spec.BrakeForce {
		t.Error("the player has as much grip in a storm")
	}
	w.Update(Controls{}, 1.0/20)
	for i, a := range w.Agents {
		if a.V.Spec.BrakeForce >= dry.Agents[i].V.Spec.BrakeForce {
			t.Fatalf("traffic car %d has as much grip in a storm", i)
		}
	}
	w.SetPlayerBody(BodyOf(Estate))
	if w.Player.Spec != BodyOf(Estate).Spec.OnSurface(w.Climate.Grip()) || w.Climate.Grip() >= 1 {
		t.Error("a car changed in a storm was fitted to the dry")
	}
}
