package game

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/emergency"
	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// The emergency pack hands over its call-outs, which are registered once and
// found by the job framework, and a sign at each site.
func TestEmergencyPackRegistersItsJobs(t *testing.T) {
	var c packContent
	for _, r := range registered {
		if r.ID == pack.Emergency {
			c = r
		}
	}
	if c.Jobs == nil || c.Dressing == nil {
		t.Fatalf("the emergency pack hands over jobs %v and dressing %v", c.Jobs != nil, c.Dressing != nil)
	}
	c.Jobs()
	for _, id := range []string{emergency.AmbulanceID, emergency.FireID, emergency.ResponseID} {
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
	if g.sites == nil {
		t.Error("dressing left the game with no index of sites")
	}
}

func TestEmergencyColoursAreKeptOffTheVisionSystems(t *testing.T) {
	reserved := map[[3]uint8]bool{}
	for _, c := range BoxColours() {
		reserved[[3]uint8{c.R, c.G, c.B}] = true
	}
	all := append(emergencyColours(), siteColours()...)
	for _, c := range all {
		if reserved[[3]uint8{c.R, c.G, c.B}] {
			t.Errorf("%v is a colour the vision system draws boxes in", c)
		}
		if c.R == 255 || c.G == 255 || c.B == 255 || c.A != 255 {
			t.Errorf("%v has a channel at full or names a material", c)
		}
	}
}

// The pack's cars have shapes of their own, not the van's, lorry's and estate's
// they borrowed.
func TestEmergencyCarsAreNotThePlaceholders(t *testing.T) {
	amb, van := sim.BodyOf(sim.Ambulance), sim.BodyOf(sim.Van)
	fire, lorry := sim.BodyOf(sim.FireEngine), sim.BodyOf(sim.Lorry)
	resp, estate := sim.BodyOf(sim.ResponseCar), sim.BodyOf(sim.Estate)
	if amb.Spec == van.Spec || fire.Spec == lorry.Spec || resp.Spec == estate.Spec {
		t.Error("an emergency car still has the spec of the car it borrowed")
	}
	if fire.Spec.Mass < 8500 || fire.Spec.Mass > 9500 || fire.Spec.MaxSpeed != 25 {
		t.Errorf("the fire engine weighs %.0f kg and tops out at %.0f m/s, want about nine tonnes and 25", fire.Spec.Mass, fire.Spec.MaxSpeed)
	}
	if resp.Spec.EngineForce <= estate.Spec.EngineForce || resp.Spec.MaxSpeed <= estate.Spec.MaxSpeed {
		t.Error("the response car is not quicker than the estate it is built on")
	}
	for k, fits := range map[sim.BodyKind]bool{sim.Ambulance: true, sim.FireEngine: true, sim.ResponseCar: true} {
		if _, ok := fittings[k]; ok != fits {
			t.Errorf("%s fittings registered %v", k.Name(), ok)
		}
	}
}
