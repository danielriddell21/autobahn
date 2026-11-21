package sim_test

import (
	"math/rand/v2"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
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

func TestEveryShapeHasASpecThatCanDrive(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range sim.Bodies() {
		b := sim.BodyOf(k)
		s := b.Spec
		if b.Kind != k {
			t.Errorf("BodyOf(%s) describes a %s", k.Name(), b.Kind.Name())
		}
		if k.Name() == "" || seen[k.Name()] {
			t.Errorf("shape %d has no name of its own: %q", k, k.Name())
		}
		seen[k.Name()] = true
		if s.Mass <= 0 || s.Inertia <= 0 || s.EngineForce <= 0 || s.BrakeForce <= 0 || s.MaxSpeed <= 0 {
			t.Errorf("a %s has a broken spec: %+v", k.Name(), s)
		}
		// The axles have to be under the body, and it has to stop harder than
		// it goes or traffic would run into whatever it follows.
		if s.FrontAxle >= s.HalfLength || s.RearAxle >= s.HalfLength {
			t.Errorf("a %s has an axle beyond its bumper", k.Name())
		}
		if s.BrakeForce <= s.EngineForce {
			t.Errorf("a %s goes harder than it stops", k.Name())
		}
		// Enough lock to make the tightest turn in town, which is the one
		// round a kerb.
		if radius := s.Wheelbase() / mathx.Tan(s.MaxSteer); radius > 8.5 {
			t.Errorf("a %s turns in %.1f m, too wide for a street corner", k.Name(), radius)
		}
	}
}

func TestTheHeavyShapesAreHeavyAndSlow(t *testing.T) {
	car := sim.TrafficSpec()
	for _, k := range []sim.BodyKind{sim.Bus, sim.Lorry} {
		s := sim.BodyOf(k).Spec
		if s.Mass < car.Mass*3 || s.MaxSpeed >= car.MaxSpeed || s.HalfLength < car.HalfLength*1.5 {
			t.Errorf("a %s should be far heavier, slower and longer than a car: %+v", k.Name(), s)
		}
		if s.EngineForce/s.Mass >= car.EngineForce/car.Mass {
			t.Errorf("a %s pulls away as hard as a car", k.Name())
		}
		// It keeps a car's grip for its weight, so it is slow rather than loose.
		if g, want := s.GripRear/s.Mass, car.GripRear/car.Mass; g < want*0.99 || g > want*1.01 {
			t.Errorf("a %s grips at %.1f per kg, want a car's %.1f", k.Name(), g, want)
		}
		if sim.IsPlayerBody(k) {
			t.Errorf("the garage offers a %s", k.Name())
		}
	}
}

func TestEveryMapsTrafficIsAMixOfAHundred(t *testing.T) {
	for _, m := range city.Maps() {
		rng := rand.New(rand.NewPCG(1, uint64(m)))
		seen := map[sim.BodyKind]int{}
		for range 4000 {
			seen[sim.RandomBody(rng, m).Kind]++
		}
		if len(seen) < 4 {
			t.Errorf("%s traffic is only %d shapes", m.Name(), len(seen))
		}
		if seen[sim.Hatchback] == 0 {
			t.Errorf("%s has no hatchbacks, which is not a street", m.Name())
		}
	}
	// The city has its buses and lorries, but they are the exception.
	rng := rand.New(rand.NewPCG(3, 3))
	big := 0
	for range 4000 {
		if k := sim.RandomBody(rng, city.MapCity).Kind; k == sim.Bus || k == sim.Lorry {
			big++
		}
	}
	if big == 0 || big > 4000/8 {
		t.Errorf("%d of 4000 city vehicles are buses or lorries", big)
	}
}

func TestOnlyWinterTakesGripAway(t *testing.T) {
	for _, s := range city.Seasons() {
		g := sim.SeasonGrip(s)
		if (g < 1) != (s == city.Winter) || g <= 0 || g > 1 {
			t.Errorf("%s gives %.2f of full grip", s.Name(), g)
		}
	}
}

func TestALooseSurfaceTakesGripAndBrakingButNotPower(t *testing.T) {
	dry := sim.CarSpec()
	if got := dry.OnSurface(1); got != dry {
		t.Errorf("a dry road changed the car: %+v", got)
	}
	snow := dry.OnSurface(0.7)
	if snow.GripFront >= dry.GripFront || snow.GripRear >= dry.GripRear || snow.MaxLateral >= dry.MaxLateral {
		t.Error("a car on snow corners as well as one in the dry")
	}
	if snow.BrakeForce >= dry.BrakeForce {
		t.Error("a car on snow stops as well as one in the dry")
	}
	// It loses far less braking than cornering, and nothing else.
	if snow.BrakeForce/dry.BrakeForce <= snow.MaxLateral/dry.MaxLateral {
		t.Error("braking fell away as far as cornering")
	}
	if snow.EngineForce != dry.EngineForce || snow.Mass != dry.Mass || snow.MaxSpeed != dry.MaxSpeed {
		t.Error("the surface changed the car itself")
	}
	// Nothing is ever taken down to no grip at all, or put above the dry.
	if ice := dry.OnSurface(0); ice.MaxLateral <= 0 || ice.BrakeForce <= 0 {
		t.Error("a surface with no grip left the car with none at all")
	}
	if got := dry.OnSurface(3); got != dry {
		t.Error("a surface cannot give more than full grip")
	}
}

func TestEveryVehicleInTheCityHasItsDryGrip(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7})
	if w.Player.Spec != w.PlayerBody.Spec {
		t.Error("the player's car has lost grip in the city, which is driven in the dry")
	}
	for _, a := range w.Agents {
		if a.V.Spec != a.Body.Spec {
			t.Fatalf("a %s has lost grip in the city", a.Body.Kind.Name())
		}
	}
}
