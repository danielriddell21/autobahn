package game

import "github.com/danielriddell21/autobahn/internal/sim"

// The racing pack's cars. This file belongs to the racing pack. The cabins below are
// PLACEHOLDERS that borrow the proportions of existing cars; the pack replaces
// them with its own cabins in cabins, and its extras in fittings.
func init() {
	cabins[sim.RaceCoupe] = cabinStyle{Boot: bootShare(sim.HotHatch)}
	cabins[sim.RaceRoadster] = cabinStyle{Boot: bootShare(sim.Convertible)}
	cabins[sim.RaceProto] = cabinStyle{Boot: bootShare(sim.GrandTourer)}
	blurbs[sim.RaceCoupe] = "a coupe built for the circuit, stiff and quick"
	blurbs[sim.RaceRoadster] = "open, light and sharp in the corners"
	blurbs[sim.RaceProto] = "a prototype: the fastest thing on the grid"
}
