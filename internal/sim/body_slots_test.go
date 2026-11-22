package sim

import "testing"

// Every shape has to be filled in by someone, base game or pack.
func TestEveryBodyKindHasAShapeAndAName(t *testing.T) {
	for k := range numBodies {
		if shapes[k] == nil {
			t.Errorf("body kind %d has no shape", k)
			continue
		}
		b := BodyOf(k)
		if b.Kind != k {
			t.Errorf("BodyOf(%d) is kind %d", k, b.Kind)
		}
		if b.Spec.Mass <= 0 || b.Spec.EngineForce <= 0 || b.Spec.MaxSpeed <= 0 ||
			b.Spec.HalfLength <= 0 || b.Spec.HalfWidth <= 0 || b.Spec.Height <= 0 {
			t.Errorf("%d has an empty spec: %+v", k, b.Spec)
		}
		if bodyNames[k] == "" {
			t.Errorf("body kind %d has no name", k)
		}
	}
}

// The new packs' cars are the garage's, and go at the end of the list.
func TestPackCarsAreInTheGarage(t *testing.T) {
	for _, k := range []BodyKind{
		BlackCab, CourierVan, Ambulance, FireEngine, ResponseCar,
		RaceCoupe, RaceRoadster, RaceProto,
	} {
		if !IsPlayerBody(k) {
			t.Errorf("%s is not a player body", k.Name())
		}
	}
}
