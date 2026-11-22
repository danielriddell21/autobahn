package game

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/sim"
)

var packCars = []sim.BodyKind{
	sim.BlackCab, sim.CourierVan, sim.Ambulance, sim.FireEngine, sim.ResponseCar,
	sim.RaceCoupe, sim.RaceRoadster, sim.RaceProto,
}

// Every pack car is registered in the cabins table, and the base game's cars
// are not: they keep the switches they always had.
func TestPackCarsHaveCabinsAndBaseCarsKeepTheirs(t *testing.T) {
	for _, k := range packCars {
		if _, ok := cabins[k]; !ok {
			t.Errorf("the %s has no entry in cabins", k.Name())
		}
	}
	for _, k := range sim.Bodies() {
		if _, ok := cabins[k]; ok && !contains(packCars, k) {
			t.Errorf("the base game's %s is in the cabins table", k.Name())
		}
	}
	want := map[sim.BodyKind]float32{
		sim.Saloon: 0.19, sim.Taxi: 0.07, sim.Gritter: 0.76, sim.Bus: 0, sim.Lorry: 0.78, sim.HotHatch: 0.06,
	}
	for k, w := range want {
		if got := bootShare(k); got != w {
			t.Errorf("bootShare(%s) = %v, want %v", k.Name(), got, w)
		}
	}
}

func contains(s []sim.BodyKind, k sim.BodyKind) bool {
	for _, x := range s {
		if x == k {
			return true
		}
	}
	return false
}
