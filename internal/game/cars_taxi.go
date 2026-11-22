package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/sim"
)

// The taxi pack's cars. This file belongs to the taxi pack: their cabins go in
// cabins, and the sign and livery in fittings.
//
// None of the colours below may be brighter than it is written, and none may
// have a channel at full, since every colour the vision system boxes things in
// has one; taxiColours lists them for the test that holds them to it.
var (
	colCabSign   = rl.NewColor(232, 206, 110, 255)
	colCabLetter = rl.NewColor(58, 56, 50, 255)
	colCourier   = rl.NewColor(214, 206, 188, 255)
	colCourierBd = rl.NewColor(196, 112, 36, 255)
	colParcel    = rl.NewColor(150, 112, 74, 255)
)

// taxiColours is every colour the pack's cars are drawn in.
func taxiColours() []rl.Color {
	return []rl.Color{colCabSign, colCabLetter, colCourier, colCourierBd, colParcel}
}

func init() {
	cabins[sim.BlackCab] = cabinStyle{Boot: 0.05, Draw: drawCabCabin}
	cabins[sim.CourierVan] = cabinStyle{Boot: 0.7, Draw: drawCourierCabin}
	fittings[sim.BlackCab] = cabFittings
	fittings[sim.CourierVan] = courierFittings
	blurbs[sim.BlackCab] = "tall, upright and unflappable: it knows every street"
	blurbs[sim.CourierVan] = "a panel van for fragile deliveries, quick enough to be late and gentle enough not to break anything"
}

// drawCabCabin is a tall, upright glasshouse with a partition behind the driver
// and a flat roof, which is what a black cab is.
func drawCabCabin(s sim.Spec, body sim.Body, look carLook, waist float32, paint, dark rl.Color) bool {
	length := s.HalfLength * 2
	rear := -s.HalfLength + length*0.05
	front := rear + length*body.RoofFrac
	roof := waist + body.RoofRise
	half := s.HalfWidth * 0.9
	glass := scaleColor(colGlass, 0.62+0.2*(1-look.night))
	taperBox(rear, front, half, waist, rear+0.06, front-0.2, half*0.96, roof-0.05, glass, dark)
	rl.DrawCube(vec3((rear+0.06+front-0.2)/2, roof-0.025, 0), front-0.2-rear-0.06+0.06, 0.06, half*1.92+0.04, paint)
	// The partition, behind the driver, and a pillar each side where the doors meet.
	mid := (rear + front) / 2
	rl.DrawCube(vec3(mid+0.15, (waist+roof)/2-0.03, 0), 0.08, roof-waist-0.08, half*1.8, dark)
	for _, side := range [2]float32{1, -1} {
		rl.DrawCube(vec3(mid-0.25, (waist+roof)/2-0.03, side*half*0.98), 0.12, roof-waist-0.06, 0.05, dark)
	}
	return true
}

// drawCourierCabin is a short cab at the front of a long panel box.
func drawCourierCabin(s sim.Spec, body sim.Body, look carLook, waist float32, paint, _ rl.Color) bool {
	length := s.HalfLength * 2
	cabin := length * body.RoofFrac
	front := s.HalfLength - length*0.02
	roof := waist + body.RoofRise
	glass := scaleColor(colGlass, 0.62+0.2*(1-look.night))
	drawCabAndBox(s, front-cabin, front, waist, roof, paint, glass, colCourier, 1)
	return true
}

// cabFittings is the TAXI sign on the roof.
func cabFittings(s sim.Spec, body sim.Body, look carLook, waist float32) {
	length := s.HalfLength * 2
	roof := waist + body.RoofRise
	mid := -s.HalfLength + length*0.05 + length*body.RoofFrac*0.55
	sign := colCabSign
	if look.night > 0.3 {
		sign = as(colCabSign, matGlow)
	}
	rl.DrawCube(vec3(mid, roof+0.1, 0), 0.34, 0.2, s.HalfWidth*1.1, sign)
	rl.DrawCube(vec3(mid, roof+0.1, 0), 0.36, 0.05, s.HalfWidth*0.7, colCabLetter)
	rl.DrawCube(vec3(mid, roof+0.1, 0), 0.36, 0.14, 0.05, colCabLetter)
}

// courierFittings is a band along each flank, and a parcel stacked on the
// roof rack.
func courierFittings(s sim.Spec, body sim.Body, look carLook, waist float32) {
	length := s.HalfLength * 2
	box := -s.HalfLength + length*0.34
	for _, side := range [2]float32{1, -1} {
		rl.DrawCube(vec3(box/2-s.HalfLength*0.15, waist+0.35, side*(s.HalfWidth+0.012)),
			length*0.62, 0.22, 0.03, colCourierBd)
	}
	roof := waist + body.RoofRise*0.98
	rl.DrawCube(vec3(-length*0.18, roof+0.03, 0), length*0.5, 0.05, s.HalfWidth*1.5, colTrim)
	rl.DrawCube(vec3(-length*0.22, roof+0.2, 0.1), 0.6, 0.3, 0.5, colParcel)
}
