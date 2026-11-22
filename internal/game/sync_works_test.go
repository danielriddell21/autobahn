package game

import (
	"slices"
	"testing"

	crucihud "github.com/danielriddell21/crucible/hud"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/works"
)

func worksGame(m city.Map) *Game {
	o := DefaultOptions()
	o.Seed = 5
	return &Game{
		opts: o,
		world: sim.NewWorld(sim.Config{
			Seed: o.Seed, Map: m, Works: works.Dense,
		}),
		notices: crucihud.New(),
		lastDT:  1.0 / 60,
	}
}

func activeEdges(w *sim.World) []works.Edge {
	var out []works.Edge
	for _, cl := range w.Works.Active() {
		out = append(out, cl.Edge)
	}
	return out
}

// A joiner never runs World.Update, so the roadworks it draws and the cones it
// is meant to meet have to follow the host's clock through adopt.
func TestJoinerRoadworksFollowTheHostsClock(t *testing.T) {
	host := worksGame(city.MapCity)
	join := worksGame(city.MapCity)

	moved := false
	for at := float32(1000); at < 9000; at += 900 {
		host.world.Time = at
		host.world.SyncWorks()
		want := activeEdges(host.world)

		join.adopt(Snapshot{Mode: join.mode, Clock: at, Partner: -1, Chaser: -1}, 0)
		got := activeEdges(join.world)
		if !slices.Equal(got, want) {
			t.Fatalf("at %.0f s the joiner has closures %v, the host %v", at, got, want)
		}
		if len(want) > 0 {
			moved = true
		}
	}
	if !moved {
		t.Fatal("the test never saw a closure, so it proves nothing")
	}
}
