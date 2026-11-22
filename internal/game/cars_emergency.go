package game

import "github.com/danielriddell21/autobahn/internal/sim"

// The emergency pack's cars. This file belongs to the emergency pack. The cabins below are
// PLACEHOLDERS that borrow the proportions of existing cars; the pack replaces
// them with its own cabins in cabins, and its extras in fittings.
func init() {
	cabins[sim.Ambulance] = cabinStyle{Boot: bootShare(sim.Van)}
	cabins[sim.FireEngine] = cabinStyle{Boot: bootShare(sim.Lorry)}
	cabins[sim.ResponseCar] = cabinStyle{Boot: bootShare(sim.Estate)}
	blurbs[sim.Ambulance] = "a van with the right of way, and the weight to match"
	blurbs[sim.FireEngine] = "huge and slow to get going, and nothing stops it"
	blurbs[sim.ResponseCar] = "a fast estate with a light bar and somewhere to be"
}
