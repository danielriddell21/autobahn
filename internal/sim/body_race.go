package sim

// The racing pack's cars. This file belongs to the racing pack.
//
// Each is lighter and stiffer than the hot hatch it is measured against, with
// more to pull it and more grip at both axles. Their rear axle holds a little
// less of the front's grip than the road cars' does, so they rotate eagerly
// and will hang their tails out: that is what a drift event asks of them.
func init() {
	// The coupe: a hot hatch's size, a racer's weight, and a tail that comes round.
	shapes[RaceCoupe] = func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.15, 0.95, 1.26
		b.Spec.Mass, b.Spec.EngineForce = 1180, 11600
		b.Spec.MaxSpeed, b.Spec.BrakeForce = 56, 16500
		b.Spec.GripFront, b.Spec.GripRear = 112000, 114000
		b.RoofFrac, b.RoofRise = 0.4, 0.3
		b.Ride = 0.2
	}
	// The roadster: the lightest, open, sharp and quick to turn in.
	shapes[RaceRoadster] = func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.05, 0.93, 1.1
		b.Spec.Mass, b.Spec.EngineForce = 1020, 10600
		b.Spec.MaxSpeed, b.Spec.BrakeForce = 54, 15500
		b.Spec.GripFront, b.Spec.GripRear = 108000, 110000
		b.RoofFrac, b.RoofRise = 0.32, 0.24
		b.Ride = 0.17
	}
	// The prototype: low, wide and a long way past anything else on the road.
	shapes[RaceProto] = func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.45, 1.02, 1.02
		b.Spec.Mass, b.Spec.EngineForce = 1050, 16200
		b.Spec.MaxSpeed, b.Spec.BrakeForce = 74, 20000
		b.Spec.GripFront, b.Spec.GripRear = 132000, 138000
		b.RoofFrac, b.RoofRise = 0.34, 0.22
		b.Ride, b.Wheel = 0.14, 0.35
	}
}
