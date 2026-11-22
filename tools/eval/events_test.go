package main

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/night"
	"github.com/danielriddell21/autobahn/internal/works"
)

// The clock eval holds for -events on is inside the evening window, so the
// events are in force for the whole run.
func TestEveningHourIsInForce(t *testing.T) {
	if _, ph := night.Evening(eveningHour); ph != works.PhaseSolid {
		t.Errorf("hour %v is not inside the evening", eveningHour)
	}
}

// Events off is the default and changes nothing; on puts events in the world and
// still drives to the end.
func TestEventsFlagDrivesWithAndWithout(t *testing.T) {
	base := runConfig{
		world: city.MapCity, mode: autopilot.ModeNormal, seconds: 4, traffic: 10, police: 0,
		perception: 20, width: 210, height: 118, works: works.Off,
	}
	off := driveOne(2, base)
	base.events = true
	on := driveOne(2, base)
	if off.seconds < 3.9 || on.seconds < 3.9 {
		t.Errorf("runs stopped early: %v and %v seconds", off.seconds, on.seconds)
	}
}
