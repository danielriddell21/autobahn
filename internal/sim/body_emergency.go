package sim

// The emergency pack's cars. This file belongs to the emergency pack. The
// shapes below are PLACEHOLDERS that borrow existing ones until the pack's own
// are drawn.
func init() {
	shapes[Ambulance] = shapes[Van]
	shapes[FireEngine] = shapes[Lorry]
	shapes[ResponseCar] = shapes[Estate]
}
