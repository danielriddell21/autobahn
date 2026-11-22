package game

import "github.com/danielriddell21/autobahn/internal/sim"

// The racing pack's cars. This file belongs to the racing pack. Each has a
// cabin of its own, set back and low: the coupe's a fastback, the roadster's
// open with a short tail, the prototype's a bubble far behind a long nose.
func init() {
	cabins[sim.RaceCoupe] = cabinStyle{Boot: 0.1}
	cabins[sim.RaceRoadster] = cabinStyle{Boot: 0.16}
	cabins[sim.RaceProto] = cabinStyle{Boot: 0.24}
	blurbs[sim.RaceCoupe] = "a coupe built for the circuit, stiff and quick, with a tail that comes round"
	blurbs[sim.RaceRoadster] = "open, light and sharp in the corners"
	blurbs[sim.RaceProto] = "a prototype: the fastest thing on the grid"
}
