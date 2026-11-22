package game

import (
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/shop"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// keepTo brings the stored choices back to what the player holds and
// owns: a map whose pack is locked, a car they do not own, or a paint they
// do not own falls back to the starter.
func (s *Settings) keepTo(h shop.Holdings) {
	if !h.Access.HasMap(s.Map) {
		s.Map = city.MapCity
	}
	if !h.Drivable(s.Car) {
		s.Car = sim.HotHatch
	}
	if !h.Wearable(s.Paint) {
		s.Paint = 0
	}
}
