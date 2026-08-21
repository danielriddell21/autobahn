package sim_test

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/sim"
)

func TestRoadblockDeploysWhenWantedRises(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 20, Police: 8})
	// Offend repeatedly with a unit alongside until the level is high enough.
	u := w.Police()[0]
	for range 6 {
		u.V.Place(w.Player.Pos.Add(w.Player.Forward().Mul(8)), w.Player.Yaw)
		w.Judge.ReportCollision(w.Player, "hit a vehicle", 12, w.Time)
		for range 400 {
			w.Update(sim.Controls{Throttle: 1}, 1.0/60)
			if w.Roadblocked() {
				break
			}
		}
		if w.Roadblocked() {
			break
		}
	}
	if !w.Roadblocked() {
		t.Fatalf("no roadblock at wanted level %d", w.Wanted.Level)
	}
	var parked int
	for _, p := range w.Police() {
		if p.Blocking {
			parked++
			if p.V.Speed() > 2 {
				t.Errorf("a unit in the block is doing %.1f m/s", p.V.Speed())
			}
		}
	}
	if parked == 0 {
		t.Error("the block has no units in it")
	}
	t.Logf("wanted %d, %d units parked across the road", w.Wanted.Level, parked)
}
