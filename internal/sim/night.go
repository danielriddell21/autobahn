package sim

import (
	"github.com/danielriddell21/autobahn/internal/night"
)

// Evening events, as the simulation sees them.
//
// Package night says where the events are and when. Here a crowd slows the
// traffic that drives through it, and the show cars of a street meet are solid.
// The streets an event shuts go through the roadworks, which is where
// [World.EnableNight] hands them, so traffic and the autopilot meet them as
// they meet any other closure.

// EnableNight puts the evening events in the world, on the given clock. Without
// it there are none, which is how the world is built unless somebody asks.
func (w *World) EnableNight(clk night.Clock) {
	w.Night = night.NewIndex(w.City, w.City.Seed, clk)
	w.Works.Add(w.Night)
	w.syncWorks()
}

// syncNight takes the show cars of the events in force. The index itself is
// brought up to the clock by the roadworks, which it is a source for.
func (w *World) syncNight() {
	w.shows = w.Night.Cars(w.shows[:0])
}

// crowdScale is how much of its speed a driver keeps where they are.
func (w *World) crowdScale(a *Agent) float32 {
	if w.Night == nil || a.pursuing || a.BlueLights() {
		return 1
	}
	return w.Night.SpeedScale(a.V.Pos)
}

// collideShows pushes a vehicle out of the show cars at a street meet.
func (w *World) collideShows(v *Vehicle, report bool) {
	for _, b := range w.shows {
		if v.Pos.DistTo(b.Center) > v.Spec.HalfLength+v.Spec.HalfWidth+b.Radius() {
			continue
		}
		w.collideSolid(v, b, "hit a parked car", report)
	}
}
