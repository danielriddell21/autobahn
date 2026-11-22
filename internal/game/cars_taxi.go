package game

import "github.com/danielriddell21/autobahn/internal/sim"

// The taxi pack's cars. This file belongs to the taxi pack. The cabins below are
// PLACEHOLDERS that borrow the proportions of existing cars; the pack replaces
// them with its own cabins in cabins, and its extras in fittings.
func init() {
	cabins[sim.BlackCab] = cabinStyle{Boot: bootShare(sim.Taxi)}
	cabins[sim.CourierVan] = cabinStyle{Boot: bootShare(sim.Van)}
	blurbs[sim.BlackCab] = "tall, upright and unflappable: it knows every street"
	blurbs[sim.CourierVan] = "a van for deliveries, quick enough to be late"
}
