package sim

// The taxi pack's cars. This file belongs to the taxi pack. The shapes below
// are PLACEHOLDERS that borrow existing ones until the pack's own are drawn.
func init() {
	shapes[BlackCab] = shapes[Taxi]
	shapes[CourierVan] = shapes[Van]
}
