package sim_test

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/sim"
)

func TestTheUsualCarIsTheHotHatch(t *testing.T) {
	if sim.PlayerBody() != sim.PlayerBodyOf(sim.HotHatch) {
		t.Error("the default player car should be the hot hatch")
	}
	if got := sim.PlayerBodyOf(sim.BodyKind(99)); got != sim.PlayerBody() {
		t.Error("a shape the garage does not offer should fall back to the usual car")
	}
}

func TestEveryPlayerCarKeepsTheAutopilotsCalibration(t *testing.T) {
	ref := sim.PlayerBody().Spec
	for _, k := range sim.PlayerBodies() {
		s := sim.PlayerBodyOf(k).Spec
		if s.FrontAxle != ref.FrontAxle || s.RearAxle != ref.RearAxle || s.MaxSteer != ref.MaxSteer {
			t.Errorf("%s changes the axles or the steering", k.Name())
		}
		// The same braking for its weight, so stopping distances still hold.
		if d, want := s.BrakeForce/s.Mass, ref.BrakeForce/ref.Mass; d < want*0.99 || d > want*1.01 {
			t.Errorf("%s brakes at %.2f m/s^2, want %.2f", k.Name(), d, want)
		}
		if s.Mass <= 0 || s.EngineForce <= 0 || s.MaxSpeed <= 0 {
			t.Errorf("%s has a broken spec: %+v", k.Name(), s)
		}
	}
}

func TestShapesDriveDifferently(t *testing.T) {
	hot, van := sim.PlayerBodyOf(sim.HotHatch).Spec, sim.PlayerBodyOf(sim.Van).Spec
	if !(van.Mass > hot.Mass && van.MaxSpeed < hot.MaxSpeed && van.GripFront < hot.GripFront) {
		t.Errorf("a van should be heavier, slower and less sure than the hot hatch: %+v vs %+v", van, hot)
	}
	if !(van.Height > hot.Height && van.HalfLength > hot.HalfLength) {
		t.Error("a van should be bigger than the hot hatch")
	}
	// Every shape is still the player's car: quicker than traffic of its kind.
	for _, k := range sim.PlayerBodies() {
		if sim.PlayerBodyOf(k).Spec.EngineForce <= sim.BodyOf(k).Spec.EngineForce {
			t.Errorf("the player's %s is no livelier than traffic's", k.Name())
		}
	}
}

func TestWorldTakesThePlayersCar(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, PlayerBody: sim.PlayerBodyOf(sim.Estate)})
	if w.PlayerBody.Kind != sim.Estate || w.Player.Spec != w.PlayerBody.Spec {
		t.Error("the world should build the player's car in the shape asked for")
	}
	w.SetPlayerBody(sim.PlayerBodyOf(sim.Taxi))
	if w.PlayerBody.Kind != sim.Taxi || w.Player.Spec.Mass != sim.PlayerBodyOf(sim.Taxi).Spec.Mass {
		t.Error("changing the car should change the one being driven")
	}
	if sim.NewWorld(sim.Config{Seed: 7}).PlayerBody.Kind != sim.HotHatch {
		t.Error("with no car asked for, the player drives the hot hatch")
	}
}
