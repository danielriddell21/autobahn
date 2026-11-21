package sim_test

import (
	"strings"
	"testing"

	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

func TestAParkedCarIsSolid(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7})
	if len(w.City.Parked) == 0 {
		t.Fatal("nothing is parked in this city")
	}
	p := w.City.Parked[0]
	solid := sim.ParkedBox(p)

	// Run the player's car into the back of it.
	fwd := mathx.FromAngle(p.Heading)
	reach := solid.HalfL + w.Player.Spec.HalfLength
	w.Player.Pos = p.Pos.Sub(fwd.Mul(reach - 0.4))
	w.Player.Yaw = p.Heading
	w.Player.Vel = fwd.Mul(6)
	w.Update(sim.Controls{}, 1.0/60)

	if _, depth, hit := w.Player.Box().Penetration(solid); hit && depth > 0.01 {
		t.Errorf("the player's car is still %.2f m inside the parked one", depth)
	}
	if got := w.Player.Vel.Dot(fwd); got > 0.5 {
		t.Errorf("the player's car is still doing %.1f m/s into it", got)
	}
	found := false
	for _, in := range w.Judge.Log() {
		if strings.HasPrefix(in.Detail, "hit a parked car") {
			found = true
		}
	}
	if !found {
		t.Error("the judge did not see the player hit a parked car")
	}
}

func TestEveryParkedCarIsACarPeoplePark(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7})
	shapes := map[sim.BodyKind]bool{}
	for _, p := range w.City.Parked {
		b := sim.ParkedBody(p)
		shapes[b.Kind] = true
		if b.Kind == sim.Bus || b.Kind == sim.Lorry || b.Kind == sim.Taxi {
			t.Fatalf("a %s is parked in a bay", b.Kind.Name())
		}
		box := sim.ParkedBox(p)
		if box.Center != p.Pos || box.Heading != p.Heading || box.HalfL != b.Spec.HalfLength {
			t.Fatalf("the box of a parked %s is not where it stands", b.Kind.Name())
		}
	}
	if len(shapes) < 3 {
		t.Errorf("only %d shapes of car are parked", len(shapes))
	}
}
