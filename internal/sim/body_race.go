package sim

// The racing pack's cars. This file belongs to the racing pack. The shapes
// below are PLACEHOLDERS that borrow existing ones until the pack's own are
// drawn.
func init() {
	shapes[RaceCoupe] = shapes[HotHatch]
	shapes[RaceRoadster] = shapes[Convertible]
	shapes[RaceProto] = shapes[GrandTourer]
}
