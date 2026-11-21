package sim_test

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// onAHill returns a car standing still on a hill, facing up it.
func onAHill(grade float32) *sim.Vehicle {
	v := sim.NewVehicle(sim.BodyOf(sim.Hatchback).Spec, mathx.Vec{}, 0)
	v.Grade = grade
	return v
}

func drive(v *sim.Vehicle, c sim.Controls, seconds float32) {
	for t := float32(0); t < seconds; t += 1.0 / 60 {
		v.Update(c, 1.0/60)
	}
}

func TestACarLeftOnAHillRollsDownIt(t *testing.T) {
	up := onAHill(0.06)
	drive(up, sim.Controls{}, 3)
	if s := up.ForwardSpeed(); s >= -0.3 {
		t.Errorf("facing uphill with nothing held, the car is doing %.2f m/s", s)
	}
	down := onAHill(-0.06)
	drive(down, sim.Controls{}, 3)
	if s := down.ForwardSpeed(); s <= 0.3 {
		t.Errorf("facing downhill with nothing held, the car is doing %.2f m/s", s)
	}
	level := onAHill(0)
	drive(level, sim.Controls{}, 3)
	if level.Pos != (mathx.Vec{}) {
		t.Errorf("on the level the car has moved to %v", level.Pos)
	}
}

// The creep this guards against is a millimetre a second, which is a car's
// length over a stop line by the time a long red has gone.
func TestABrakeHoldsACarOnAHill(t *testing.T) {
	for _, grade := range []float32{0.08, -0.08, 0.02} {
		for _, brake := range []float32{1, 0.7, 0.3} {
			v := onAHill(grade)
			drive(v, sim.Controls{Brake: brake}, 120)
			if d := v.Pos.Len(); d > 0.001 {
				t.Errorf("grade %v, brake %v: in two minutes the car crept %.3fm", grade, brake, d)
			}
		}
	}
}

func TestBrakingOnAHillStopsTheCarAndKeepsItStopped(t *testing.T) {
	for _, grade := range []float32{0.08, -0.08} {
		v := onAHill(grade)
		v.Vel = v.Forward().Mul(15)
		drive(v, sim.Controls{Brake: 1}, 6)
		at := v.Pos
		if v.Speed() > 1e-3 {
			t.Errorf("grade %v: after six seconds of braking the car is doing %.3f m/s", grade, v.Speed())
		}
		drive(v, sim.Controls{Brake: 1}, 20)
		if v.Pos.DistTo(at) > 0.001 {
			t.Errorf("grade %v: stopped at %v, the car is now at %v", grade, at, v.Pos)
		}
	}
	// Downhill takes longer to stop than uphill.
	stop := func(grade float32) float32 {
		v := onAHill(grade)
		v.Vel = v.Forward().Mul(15)
		drive(v, sim.Controls{Brake: 1}, 8)
		return v.Odometer
	}
	if up, down := stop(0.08), stop(-0.08); down <= up {
		t.Errorf("stopping from 15 m/s takes %.1fm uphill and %.1fm down", up, down)
	}
}

func TestAHillSlowsTheClimbAndEveryCarCanMakeIt(t *testing.T) {
	speedAfter := func(k sim.BodyKind, grade float32) float32 {
		v := sim.NewVehicle(sim.BodyOf(k).Spec, mathx.Vec{}, 0)
		v.Grade = grade
		drive(v, sim.Controls{Throttle: 1}, 8)
		return v.ForwardSpeed()
	}
	if up, flat := speedAfter(sim.Hatchback, 0.08), speedAfter(sim.Hatchback, 0); up >= flat {
		t.Errorf("after 8s flat out: %.1f m/s up a hill, %.1f on the level", up, flat)
	}
	// One in ten is as steep as the land gets.
	for _, k := range sim.Bodies() {
		if s := speedAfter(k, 0.1); s < 4 {
			t.Errorf("%s: flat out up one in ten for 8s reaches %.1f m/s", k.Name(), s)
		}
	}
}

func TestTheWorldTellsCarsAboutTheirHills(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 30, Police: 0, Map: city.MapVillage})
	for range 120 {
		w.Update(sim.Controls{Brake: 1}, 1.0/60)
	}
	graded := 0
	for _, a := range w.Agents {
		want := w.City.Grade(a.V.Pos, a.V.Forward())
		// A step old, and the car has moved since.
		if mathx.Abs(a.V.Grade-want) > 0.01 {
			t.Errorf("car %d thinks the road climbs %.3f where it climbs %.3f", a.ID, a.V.Grade, want)
		}
		if a.V.Grade != 0 {
			graded++
		}
	}
	if graded == 0 {
		t.Error("no car in a village is on any slope")
	}

	flat := sim.NewWorld(sim.Config{Seed: 7, Traffic: 10, Police: 0})
	flat.Update(sim.Controls{}, 1.0/60)
	for _, a := range flat.Agents {
		if a.V.Grade != 0 {
			t.Errorf("car %d in the city is on a slope of %.3f", a.ID, a.V.Grade)
		}
	}
}
