package game

import (
	"testing"

	crucihud "github.com/danielriddell21/crucible/hud"

	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/taxi"
)

// The taxi pack hands over its job types, registered once and found by the job
// framework, and the index of pick-up places for the ranks.
func TestTaxiPackRegistersItsJobs(t *testing.T) {
	var c packContent
	for _, r := range registered {
		if r.ID == pack.Taxi {
			c = r
		}
	}
	if c.Jobs == nil || c.Dressing == nil {
		t.Fatalf("the taxi pack hands over jobs %v and dressing %v", c.Jobs != nil, c.Dressing != nil)
	}
	c.Jobs()
	for _, id := range []string{taxi.FareID, taxi.RiskyID, taxi.CourierID} {
		ty, ok := job.Lookup(id)
		if !ok {
			t.Errorf("job %q is not registered", id)
			continue
		}
		if ty.Name() == "" {
			t.Errorf("job %q has no name", id)
		}
	}
	g := &Game{world: sim.NewWorld(sim.Config{Seed: 3})}
	c.Dressing(g)
	if g.taxi.ix == nil {
		t.Error("dressing left the game with no index of pick-up places")
	}
	if g.taxiLine() != "" {
		t.Error("a game with no job has a taxi line")
	}
}

func TestTaxiColoursAreKeptOffTheVisionSystems(t *testing.T) {
	reserved := map[[3]uint8]bool{}
	for _, c := range BoxColours() {
		reserved[[3]uint8{c.R, c.G, c.B}] = true
	}
	for _, c := range append(taxiColours(), rankColours()...) {
		if reserved[[3]uint8{c.R, c.G, c.B}] {
			t.Errorf("%v is a colour the vision system draws boxes in", c)
		}
		if c.R == 255 || c.G == 255 || c.B == 255 || c.A != 255 {
			t.Errorf("%v has a channel at full or names a material", c)
		}
	}
}

// The pack's cars have shapes of their own, not the taxi's and van's they
// borrowed.
func TestTaxiCarsAreNotThePlaceholders(t *testing.T) {
	if sim.BodyOf(sim.BlackCab).Spec == sim.BodyOf(sim.Taxi).Spec {
		t.Error("the black cab is still the taxi")
	}
	if sim.BodyOf(sim.CourierVan).Spec == sim.BodyOf(sim.Van).Spec {
		t.Error("the courier van is still the van")
	}
	for _, k := range []sim.BodyKind{sim.BlackCab, sim.CourierVan} {
		if cabins[k].Draw == nil || fittings[k] == nil || blurbs[k] == "" {
			t.Errorf("the %s lacks a cabin, fittings or a blurb", k.Name())
		}
	}
}

func TestBoardingChimeIsSound(t *testing.T) {
	if n := len(boardingChime()); n == 0 {
		t.Error("the chime is empty")
	}
}

// taxiStep posts a notice as a fare boards, once, and the line under the job's
// name follows it. drawTaxi needs a window, so it is only held to its shape.
func TestTaxiStepNoticesBoardingOnce(t *testing.T) {
	var c packContent
	for _, r := range registered {
		if r.ID == pack.Taxi {
			c = r
		}
	}
	c.Jobs()
	g := &Game{world: sim.NewWorld(sim.Config{Seed: 3}), notices: crucihud.New()}
	if _, err := g.jobs.board.Start(taxi.Fare{}, g.world.City, 3, 1, g.world.Player.Pos, 0); err != nil {
		t.Skip(err)
	}
	if got := g.taxiLine(); got != "heading for the pick-up" {
		t.Errorf("line before boarding %q", got)
	}
	g.taxiStep()
	g.jobs.board.Active().Next = 1
	g.taxiStep()
	if got := g.taxiLine(); got != "fare aboard" {
		t.Errorf("line after boarding %q", got)
	}
	if !g.taxi.boarded {
		t.Error("the boarding was not noted")
	}
}
