package sim

// The emergency pack's cars. This file belongs to the emergency pack.
//
// The ambulance is a tall box on a van's chassis, heavy for what it carries
// and quick enough to be worth getting out of the way of. The fire engine is
// hauled to nine tonnes: a long way from the lights it is slow to get going
// and slower to turn, and tops out at twenty-five metres a second. The
// response car is a saloon with the engine of something bigger.
func init() {
	shapes[Ambulance] = func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 3.0, 1.05, 2.5
		b.Spec.FrontAxle, b.Spec.RearAxle = 1.7, 1.5
		b.Spec.haul(3300, 11500)
		b.Spec.MaxSpeed = 38
		b.RoofFrac, b.RoofRise, b.Boxy = 0.74, 1.1, true
		b.Ride, b.Wheel = 0.34, 0.38
	}
	shapes[FireEngine] = func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 4.7, 1.25, 3.3
		b.Spec.FrontAxle, b.Spec.RearAxle = 2.9, 2.5
		b.Spec.haul(9000, 21000)
		b.Spec.MaxSpeed = 25
		b.RoofFrac, b.RoofRise, b.Boxy = 0.3, 1.55, true
		b.Ride, b.Wheel = 0.5, 0.52
	}
	shapes[ResponseCar] = func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.5, 0.97, 1.5
		b.Spec.Mass, b.Spec.EngineForce = 1700, 11500
		b.Spec.MaxSpeed = 52
		b.Spec.GripFront, b.Spec.GripRear = 90000, 96000
		b.RoofFrac, b.RoofRise = 0.6, 0.46
	}
}
