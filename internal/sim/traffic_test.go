package sim_test

import (
	"fmt"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// The long drives in this package each build a world of their own and run
// beside one another, because a minute and a half of traffic is slow under
// the race detector. Do not ask for coverage in the same run: its counters
// are atomic there, and four worlds contending for them never finish.

// run steps a world with nobody at the wheel of the player's car.
func run(w *sim.World, seconds float32, each func()) {
	for t := float32(0); t < seconds; t += 1.0 / 60 {
		w.Update(sim.Controls{Brake: 1}, 1.0/60)
		if each != nil {
			each()
		}
	}
}

// TestTrafficDoesNotRunRedLights watches every car cross every stop line.
//
// The bug it guards against was a slow one. A car held at a red was being
// nudged back onto its line by a correction that also had a component along
// the road, so it crept forward a few millimetres a step with its brakes on,
// and after a while it was over the line and gone.
func TestTrafficDoesNotRunRedLights(t *testing.T) {
	t.Parallel()
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 60, Police: 0})
	crossing := map[*sim.Agent]bool{}
	run(w, 90, func() {
		for _, a := range w.Agents {
			was := crossing[a]
			crossing[a] = a.InJunction()
			if was || !a.InJunction() {
				continue
			}
			// Just crossed the line of the lane it is leaving.
			l := a.Lane()
			if l.Control == city.ControlSignal && w.Signals.LaneState(l) == sim.SignalRed {
				t.Fatalf("t=%.1fs: car %d (%s) crossed lane %d's stop line on red",
					w.Time, a.ID, a.Style.Name, l.ID)
			}
		}
	})
}

// TestTrafficKeepsMoving is the gridlock test. Junction rules that are each
// reasonable can add up to everybody waiting for everybody else, and a city in
// that state looks fine for the first minute.
func TestTrafficKeepsMoving(t *testing.T) {
	t.Parallel()
	for _, seed := range []uint64{3, 7, 17} {
		t.Run(fmt.Sprint("seed ", seed), func(t *testing.T) {
			t.Parallel()
			w := sim.NewWorld(sim.Config{Seed: seed, Traffic: 70, Police: 5})
			// Park the player on the pavement, out of everybody's way.
			w.Player.Place(w.Player.Pos.Sub(w.Player.Forward().Right().Mul(11)), w.Player.Yaw)
			run(w, 45, nil)

			before := map[*sim.Agent]float32{}
			for _, a := range w.Agents {
				before[a] = a.V.Odometer
			}
			run(w, 75, nil)

			stuck := 0
			for _, a := range w.Agents {
				if a.V.Odometer-before[a] < 15 {
					stuck++
				}
			}
			// A long red and a queue behind it can hold a few cars for a minute.
			// A tenth of the city has not moved because it cannot.
			if limit := len(w.Agents) / 10; stuck > limit {
				t.Errorf("%d of %d cars moved less than 15m in 75s, want at most %d",
					stuck, len(w.Agents), limit)
			}
		})
	}
}

// Traffic already on a roundabout has priority, and must not stop on it to let
// somebody in. Doing so locks the ring solid within a minute.
func TestCirculatingTrafficDoesNotStopOnTheRing(t *testing.T) {
	t.Parallel()
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 80, Police: 0})
	parked := map[*sim.Agent]float32{}
	worst := float32(0)
	run(w, 120, func() {
		for _, a := range w.Agents {
			if w.City.Roads[a.Lane().Road].Ring && !a.InJunction() && a.V.Speed() < 0.2 {
				parked[a] += 1.0 / 60
				worst = max(worst, parked[a])
				continue
			}
			parked[a] = 0
		}
	})
	if worst > 20 {
		t.Errorf("a car sat still on a roundabout for %.0fs", worst)
	}
}

// A car that is hit hard is knocked out of its routine for a moment rather
// than carrying on as if nothing had happened, and then finds the road again.
func TestAHitCarRecovers(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 20, Police: 0})
	a := w.Agents[0]
	run(w, 2, nil)

	// Drive the player into the back of it, hard.
	w.Player.Place(a.V.Pos.Sub(a.V.Forward().Mul(5)), a.V.Yaw)
	w.Player.Vel = a.V.Forward().Mul(a.V.Speed() + 14)
	run(w, 0.5, nil)
	if w.Judge.Faults == 0 {
		t.Fatal("setup failed: the player did not hit the car")
	}

	// Get the player out of the way and give it time to sort itself out.
	w.Player.Place(w.Player.Pos.Add(a.V.Forward().Right().Mul(400)), 0)
	start := a.V.Odometer
	run(w, 25, nil)
	if moved := a.V.Odometer - start; moved < 20 {
		t.Errorf("25s after being hit the car has moved %.0fm", moved)
	}
	// Inside a junction the nearest lane says nothing, so judge it once out.
	for i := 0; i < 600 && a.InJunction(); i++ {
		w.Update(sim.Controls{Brake: 1}, 1.0/60)
	}
	if p := w.City.Project(a.V.Pos); !p.Valid || p.Dist > 4 {
		t.Errorf("the car has ended up %.1fm from any lane", p.Dist)
	}
}

// Something stopped in the road for good is driven round, not queued behind
// for ever.
func TestTrafficGoesRoundAStoppedCar(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 25, Police: 0})
	run(w, 3, nil)

	// Find a car on a long straight and park the player in front of it.
	var a *sim.Agent
	for _, c := range w.Agents {
		l := c.Lane()
		if !c.InJunction() && !w.City.Roads[l.Road].Ring && l.Length > 90 {
			if d := l.Length - c.V.Pos.Sub(l.A).Dot(l.Fwd); d > 70 {
				a = c
				break
			}
		}
	}
	if a == nil {
		t.Skip("no car on a long enough straight")
	}
	block := a.V.Pos.Add(a.V.Forward().Mul(22))
	held := func() { w.Player.Place(block, a.Lane().Heading) }

	held()
	passed := false
	run(w, 40, func() {
		held()
		if a.V.Pos.Sub(block).Dot(w.Player.Forward()) > 6 {
			passed = true
		}
	})
	if !passed {
		t.Errorf("a car sat behind a parked one for 40s without going round it (now %.1fm behind)",
			block.Sub(a.V.Pos).Dot(w.Player.Forward()))
	}
}
