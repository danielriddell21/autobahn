package sim_test

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// coopWorld is a world with a second player's car in it, and a unit to be
// chased by.
func coopWorld(t *testing.T) (*sim.World, *sim.Agent, *sim.Agent) {
	t.Helper()
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 12, Police: 3})
	p := w.AssignPartner()
	if p == nil || w.Partner() != p {
		t.Fatal("setup failed: no partner was assigned")
	}
	if w.AssignPartner() != p {
		t.Error("assigning a partner twice gave a second car")
	}
	if w.Chaser() != nil {
		t.Error("the partner counts as a human-driven police unit")
	}
	return w, p, w.Police()[0]
}

// farFromPolice keeps the first car well away from every unit and moving.
func farFromPolice(w *sim.World, u *sim.Agent) {
	w.Player.Place(u.V.Pos.Add(mathx.V(2500, 0)), 0)
	w.Player.Vel = mathx.V(15, 0)
}

// aheadOf keeps the partner's car in plain sight of a unit, driving.
func aheadOf(p, u *sim.Agent) {
	p.V.Place(u.V.Pos.Add(u.V.Forward().Mul(25)), u.V.Yaw)
	p.V.Vel = u.V.Forward().Mul(12)
}

func TestAPartnersOffenceRaisesTheSharedLevel(t *testing.T) {
	w, p, u := coopWorld(t)
	aheadOf(p, u)
	farFromPolice(w, u)
	if w.Suspect() != w.Player {
		t.Fatal("setup failed: the first car is not the suspect before anything happens")
	}

	w.PartnerJudge.ReportCollision(p.V, "hit a vehicle", 12, w.Time)
	if !w.Wanted.Active() {
		t.Fatal("the partner's collision in front of a unit raised no wanted level")
	}
	if w.Suspect() != p.V {
		t.Error("the police are not after the car that offended")
	}

	// And the first car's offence hands the pursuit back to it.
	w.Judge.ReportCollision(w.Player, "hit a vehicle", 12, w.Time)
	if w.Suspect() != w.Player {
		t.Error("the suspect did not follow the latest offender")
	}
}

func TestEitherCarBeingStoppedBustsTheTeam(t *testing.T) {
	w, p, u := coopWorld(t)
	aheadOf(p, u)
	farFromPolice(w, u)
	w.PartnerJudge.ReportCollision(p.V, "hit a vehicle", 12, w.Time)
	for range 60 * 5 {
		aheadOf(p, u)
		farFromPolice(w, u)
		w.Update(sim.Controls{Throttle: 0.4}, 1.0/60)
	}
	if !w.Wanted.Active() || w.Wanted.Stops != 0 {
		t.Fatalf("setup failed: level %d, stops %d", w.Wanted.Level, w.Wanted.Stops)
	}

	// The first car keeps driving; the partner is stopped beside a unit.
	for range 60 * 15 {
		p.V.Place(u.V.Pos.Add(u.V.Forward().Right().Mul(3.5)), u.V.Yaw)
		p.SetInput(sim.Controls{Brake: 1})
		farFromPolice(w, u)
		w.Update(sim.Controls{Throttle: 0.4}, 1.0/60)
		if w.Wanted.Stops > 0 {
			return
		}
	}
	t.Errorf("the partner sat beside a unit for fifteen seconds and the team was never stopped (busting %.2f)", w.Wanted.Busting)
}

func TestEscapeNeedsBothCarsOutOfSight(t *testing.T) {
	w, p, u := coopWorld(t)
	aheadOf(p, u)
	farFromPolice(w, u)
	w.PartnerJudge.ReportCollision(p.V, "hit a vehicle", 12, w.Time)

	// One car long gone, the other still in view: nobody has got away.
	for range 60 * 45 {
		aheadOf(p, u)
		farFromPolice(w, u)
		w.Update(sim.Controls{Throttle: 0.4}, 1.0/60)
	}
	if !w.Wanted.Active() || w.Wanted.Escapes != 0 {
		t.Fatalf("level %d, escapes %d: the team got away with one car in plain sight", w.Wanted.Level, w.Wanted.Escapes)
	}

	for range 60 * 45 {
		banish(w)
		p.V.Place(mathx.V(-9000, -9000), 0)
		w.Player.Place(mathx.V(9000, -9000), 0)
		w.Update(sim.Controls{}, 1.0/60)
		if !w.Wanted.Active() {
			break
		}
	}
	if w.Wanted.Active() || w.Wanted.Escapes != 1 {
		t.Errorf("level %d, escapes %d with both cars out of sight, want a clean escape", w.Wanted.Level, w.Wanted.Escapes)
	}
}
