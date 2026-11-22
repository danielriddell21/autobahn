package game

import (
	"bytes"
	"encoding/gob"
	"testing"

	"github.com/danielriddell21/autobahn/internal/sim"
)

var (
	// hands has everything pressed, so a half that is wrongly taken shows.
	handsOn = Input{Throttle: 1, Brake: 0.5, Steer: -1, Handbrake: true, Reverse: true, Boost: true}
	// theirs is a different player's, to tell whose half is whose.
	theirs = Input{Throttle: 0.25, Brake: 0.75, Steer: 0.5}
)

func TestTheWheelSeatSteersAndTheOtherWorksThePedals(t *testing.T) {
	c := split(SeatWheel, handsOn, theirs)
	if c.Steer != -1 || !c.Handbrake {
		t.Errorf("wheel seat steered %v with handbrake %v, want its own -1 and true", c.Steer, c.Handbrake)
	}
	if c.Throttle != 0.25 || c.Brake != 0.75 || c.Reverse || c.Boost {
		t.Errorf("pedals %+v came from the wrong player", c)
	}
}

func TestThePedalSeatWorksThePedalsAndTheOtherSteers(t *testing.T) {
	c := split(SeatPedals, handsOn, theirs)
	if c.Throttle != 1 || c.Brake != 0.5 || !c.Reverse || !c.Boost {
		t.Errorf("pedal seat gave %+v, want its own pedals", c)
	}
	if c.Steer != 0.5 || c.Handbrake {
		t.Errorf("steering %v with handbrake %v came from the wrong player", c.Steer, c.Handbrake)
	}
}

func TestAMissingPlayerZeroesOnlyTheirHalf(t *testing.T) {
	driver := split(SeatWheel, handsOn, Input{})
	if driver.Steer != -1 || !driver.Handbrake {
		t.Errorf("a missing pedal player took the wheel too: %+v", driver)
	}
	if driver.Throttle != 0 || driver.Brake != 0 || driver.Reverse || driver.Boost {
		t.Errorf("a missing pedal player left pedals pressed: %+v", driver)
	}

	pedals := split(SeatPedals, handsOn, Input{})
	if pedals.Throttle != 1 || !pedals.Boost {
		t.Errorf("a missing driver took the pedals too: %+v", pedals)
	}
	if pedals.Steer != 0 || pedals.Handbrake {
		t.Errorf("a missing driver left the wheel turned: %+v", pedals)
	}
}

func TestBrakeBecomesReverseOnlyWhenStopped(t *testing.T) {
	back := Input{Brake: 1}
	if got := settle(back, 8); got != back {
		t.Errorf("braking at speed became %+v", got)
	}
	got := settle(back, 0.1)
	if !got.Reverse || got.Throttle != 1 || got.Brake != 0 {
		t.Errorf("braking when stopped gave %+v, want reverse", got)
	}
	auto := Input{Brake: 1, Auto: true}
	if got := settle(auto, 0); got != auto {
		t.Errorf("the autopilot's brake became %+v", got)
	}
}

func TestIndicatorsSurviveTheWire(t *testing.T) {
	for _, i := range []sim.Indicator{sim.IndicatorOff, sim.IndicatorLeft, sim.IndicatorRight} {
		if got := indicatorOf(int8(i)); got != i {
			t.Errorf("indicator %v came back as %v", i, got)
		}
		if got := indicatorIn(indicatorFlags(i)); got != i {
			t.Errorf("indicator %v came back through the flags as %v", i, got)
		}
	}
	if got := indicatorOf(99); got != sim.IndicatorOff {
		t.Errorf("an unknown indicator became %v", got)
	}
}

// A snapshot and an input are gobs, and the new fields have to come through
// them. An input goes sixty times a second, so it is held to a size too.
func TestTheCoopFieldsRoundTripThroughGob(t *testing.T) {
	s := Snapshot{
		Proto: Proto, Mode: ModeCoopCars, Seed: 99, Map: 2, Traffic: 40, Police: 5,
		Runner:  Pose{X: 1, Z: 2, Flags: FlagBoosting | FlagIndicateLeft},
		Agents:  []Pose{{X: 3}, {Z: 4, Flags: FlagIndicateRight}},
		Partner: 1, Weather: 3,
		Bounty: 120, Banked: 300, Stops: 1, Escapes: 2, Busting: 0.5, Cooldown: 0.25, Assisted: true,
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(s); err != nil {
		t.Fatal(err)
	}
	var got Snapshot
	if err := gob.NewDecoder(&buf).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Proto != s.Proto || got.Mode != s.Mode || got.Seed != s.Seed || got.Map != s.Map ||
		got.Traffic != s.Traffic || got.Police != s.Police || got.Partner != s.Partner ||
		got.Weather != s.Weather || got.Banked != s.Banked || got.Bounty != s.Bounty ||
		got.Stops != s.Stops || got.Escapes != s.Escapes || !got.Assisted ||
		got.Busting != s.Busting || got.Cooldown != s.Cooldown {
		t.Errorf("snapshot came back as %+v, want %+v", got, s)
	}
	if got.Runner != s.Runner || len(got.Agents) != 2 || got.Agents[1] != s.Agents[1] {
		t.Errorf("poses came back as %+v and %+v", got.Runner, got.Agents)
	}

	in := Input{Throttle: 1, Brake: 0.5, Steer: -0.75, Handbrake: true, Reverse: true, Boost: true, Indicator: 2, Auto: true}
	buf.Reset()
	if err := gob.NewEncoder(&buf).Encode(in); err != nil {
		t.Fatal(err)
	}
	var back Input
	if err := gob.NewDecoder(&buf).Decode(&back); err != nil {
		t.Fatal(err)
	}
	if back != in {
		t.Errorf("input came back as %+v, want %+v", back, in)
	}
}

// netplay keeps one encoder for the life of a connection, so what an input
// costs is what the second one costs, the type description having gone ahead.
func TestAnInputIsSmallOnTheWire(t *testing.T) {
	const budget = 48
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	full := Input{Throttle: 1, Brake: 0.5, Steer: -0.75, Handbrake: true, Reverse: true, Boost: true, Indicator: 2, Auto: true}
	if err := enc.Encode(full); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := enc.Encode(full); err != nil {
		t.Fatal(err)
	}
	if buf.Len() > budget {
		t.Errorf("an input is %d bytes on the wire, want at most %d", buf.Len(), budget)
	}
}

func TestTheModesCycleAndName(t *testing.T) {
	seen := map[string]bool{}
	m := ModeChase
	for range numNetModes {
		seen[m.String()] = true
		m = cycleNetMode(m, 1)
	}
	if m != ModeChase || len(seen) != int(numNetModes) {
		t.Errorf("stepping forward through the modes ended at %v having named %v", m, seen)
	}
	if cycleNetMode(ModeChase, -1) != ModeCoopCar {
		t.Error("stepping back from the first mode did not wrap")
	}
	if ModeChase.Coop() || !ModeCoopCars.Coop() || !ModeCoopCar.Coop() || ModeCoopCars.SharedCar() || !ModeCoopCar.SharedCar() {
		t.Error("the modes disagree about who is on a team and who shares a car")
	}
}
