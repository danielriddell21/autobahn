package sim

// The taxi pack's cars. This file belongs to the taxi pack.
//
// The black cab is tall and upright, with room to stand in, and is no sports
// car: a heavy diesel that is happiest at thirty. The courier van is a long
// panel van with an engine that has been asked to be quick, since its load is
// always late.
func init() {
	shapes[BlackCab] = func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.42, 0.99, 1.78
		b.Spec.Mass, b.Spec.EngineForce = 1820, 7000
		b.Spec.MaxSpeed = 36
		b.RoofFrac, b.RoofRise, b.Boxy = 0.62, 0.7, true
		b.Livery = 3 // the black cab
	}
	shapes[CourierVan] = func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.7, 1.03, 2.05
		b.Spec.Mass, b.Spec.EngineForce = 1750, 8800
		b.Spec.MaxSpeed, b.Spec.BrakeForce = 40, 13000
		b.Spec.GripFront, b.Spec.GripRear = 80000, 86000
		b.RoofFrac, b.RoofRise, b.Boxy = 0.3, 0.95, true
		b.Ride, b.Wheel = 0.3, 0.35
	}
}
