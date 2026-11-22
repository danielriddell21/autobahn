package main

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/weather"
	"github.com/danielriddell21/autobahn/internal/works"
)

func TestTargetsNamed(t *testing.T) {
	got, err := targetsNamed("all,custom:edge", "", 1)
	if err != nil || len(got) != len(city.Maps())+1 {
		t.Fatalf("all,custom:edge gave %d targets, %v", len(got), err)
	}
	if last := got[len(got)-1]; last.name != "custom:edge" || last.spec == nil || last.season != city.Winter {
		t.Errorf("the edge profile is %+v", last)
	}
	if _, err := targetsNamed("atlantis", "", 1); err == nil {
		t.Error("an unknown map was accepted")
	}
	if _, err := targetsNamed("city", "AB1-nonsense", 1); err == nil {
		t.Error("a bad code was accepted")
	}
}

// A code is driven from its own seed, whatever -from says, so that -seeds 1
// drives exactly the map that was shared.
func TestCustomCodeIsDrivenFromItsOwnSeed(t *testing.T) {
	spec := custom.FromMap(city.MapVillage, 77)
	spec.Weather = weather.Held(weather.Clear)
	got, err := targetsNamed("city", spec.Encode(), 5)
	if err != nil || len(got) != 1 || got[0].spec == nil {
		t.Fatalf("%v, %v", got, err)
	}
	cfg := runConfig{
		spec: got[0].spec, seedShift: got[0].shift, mode: autopilot.ModeNormal,
		seconds: 6, traffic: 10, police: 1, perception: 20, width: 420, height: 236, works: works.Off,
	}
	direct := cfg
	direct.seedShift = 0
	spec.Seed = 5
	direct.spec = &spec
	// From seed 5 with the code's shift, the first city is seed 77.
	a, b := driveOne(5, cfg), driveOne(77, direct)
	if a.distance != b.distance || a.points != b.points {
		t.Error("the code was not driven from its own seed")
	}
}

// The edge profile drives, and is not the city it is built from.
func TestEdgeProfileDrives(t *testing.T) {
	edge := custom.Edge(city.MapCity, 0)
	cfg := runConfig{
		world: city.MapCity, mode: autopilot.ModeNormal, seconds: 10, traffic: 10, police: 1,
		perception: 20, width: 420, height: 236, works: works.Off,
	}
	plain := driveOne(3, cfg)
	cfg.spec = &edge
	hard := driveOne(3, cfg)
	if hard.distance == 0 {
		t.Error("the car did not move on the edge profile")
	}
	if hard.distance == plain.distance {
		t.Error("the edge profile drove exactly as the plain city did")
	}
}
