package sim

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/night"
	"github.com/danielriddell21/autobahn/internal/works"
)

func nightWorld(hour float32, seed uint64) *World {
	w := NewWorld(Config{Seed: seed, Map: city.MapCity, Traffic: 0, Police: 0, Works: works.Off})
	w.EnableNight(night.Fixed(hour))
	return w
}

func TestNoEventsUnlessEnabled(t *testing.T) {
	w := NewWorld(Config{Seed: 3, Traffic: 0, Police: 0, Works: works.Off})
	w.Update(Controls{}, 0.1)
	if w.Night != nil || len(w.Works.Active()) != 0 || len(w.shows) != 0 {
		t.Error("a world with events not enabled has some")
	}
	if got := w.crowdScale(&Agent{V: &Vehicle{}}); got != 1 {
		t.Errorf("crowd scale %v with no events", got)
	}
}

func TestEventClosuresReachTheWorldThroughTheWorks(t *testing.T) {
	for seed := uint64(1); seed < 12; seed++ {
		w := nightWorld(22, seed)
		w.Update(Controls{}, 0.1)
		for _, cl := range w.Works.Active() {
			if cl.Kind == works.KindStreet && !w.Works.Barred(cl.Lanes[0]) {
				t.Fatal("an event's street is not barred")
			}
		}
		if len(w.Works.Active()) > 0 && len(w.solid) == 0 {
			t.Fatal("the cones and barriers of an event are not solid")
		}
	}
	by := nightWorld(12, 4)
	by.Update(Controls{}, 0.1)
	if len(by.Works.Active()) != 0 || len(by.solid) != 0 || len(by.shows) != 0 {
		t.Error("events by day")
	}
}

func TestCrowdSlowsAgentsOnlyNearAnEvent(t *testing.T) {
	w := nightWorld(22, 6)
	w.Update(Controls{}, 0.1)
	evs := w.Night.Events()
	if len(evs) == 0 {
		t.Skip("no event tonight")
	}
	near := &Agent{V: &Vehicle{Pos: evs[0].Pos}}
	far := &Agent{V: &Vehicle{Pos: mathx.V(1e5, 1e5)}}
	if got := w.crowdScale(near); got >= 1 {
		t.Errorf("crowd scale %v at an event", got)
	}
	if got := w.crowdScale(far); got != 1 {
		t.Errorf("crowd scale %v far from any event", got)
	}
	near.pursuing = true
	if got := w.crowdScale(near); got != 1 {
		t.Errorf("a unit on a call is held back by a crowd: %v", got)
	}
}

func TestShowCarsAreSolid(t *testing.T) {
	for seed := uint64(1); seed < 40; seed++ {
		for n := range 6 {
			w := NewWorld(Config{Seed: seed, Traffic: 0, Police: 0, Works: works.Off})
			w.EnableNight(night.Fixed(float32(n*24 + 22)))
			w.Update(Controls{}, 0.1)
			if len(w.shows) == 0 {
				continue
			}
			b := w.shows[0]
			w.Player.Pos, w.Player.Vel = b.Center, mathx.Vec{}
			w.collideShows(w.Player, false)
			if w.Player.Pos.DistTo(b.Center) < 0.5 {
				t.Fatal("the player sits inside a show car")
			}
			return
		}
	}
	t.Skip("no street meet in any of the cities tried")
}
