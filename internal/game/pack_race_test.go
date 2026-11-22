package game

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/race"
	"github.com/danielriddell21/autobahn/internal/sim"
)

func TestRaceContentRegistersTheFourEvents(t *testing.T) {
	raceContent().Jobs()
	for _, id := range []string{race.TrialID, race.CircuitID, race.SprintID, race.DriftID} {
		if _, ok := job.Lookup(id); !ok {
			t.Errorf("%s is not registered", id)
		}
	}
}

func TestGateColoursAreKeptOffTheVisionSystems(t *testing.T) {
	reserved := map[[3]uint8]bool{}
	for _, c := range BoxColours() {
		reserved[[3]uint8{c.R, c.G, c.B}] = true
	}
	for _, c := range gateColours() {
		if reserved[[3]uint8{c.R, c.G, c.B}] {
			t.Errorf("%v is a colour the vision system draws boxes in", c)
		}
		if c.R == 255 || c.G == 255 || c.B == 255 || c.A != 255 {
			t.Errorf("%v has a channel at full or names a material", c)
		}
	}
}

func TestRaceCarsAreNotThePlaceholders(t *testing.T) {
	hatch := sim.PlayerBodyOf(sim.HotHatch).Spec
	for _, k := range []sim.BodyKind{sim.RaceCoupe, sim.RaceRoadster, sim.RaceProto} {
		b := sim.PlayerBodyOf(k)
		if b.Spec.EngineForce <= hatch.EngineForce || b.Spec.MaxSpeed <= hatch.MaxSpeed {
			t.Errorf("the %s is no quicker than the hot hatch", k.Name())
		}
		if blurbs[k] == "" {
			t.Errorf("the %s has no blurb", k.Name())
		}
	}
	if sim.BodyOf(sim.RaceProto).Spec.MaxSpeed <= sim.BodyOf(sim.RaceCoupe).Spec.MaxSpeed {
		t.Error("the prototype is not the fastest")
	}
}
