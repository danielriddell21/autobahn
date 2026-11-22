package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/sim"
)

// The emergency pack's cars. This file belongs to the emergency pack: their
// cabins go in cabins, and their livery, light bars and the rest in fittings.
//
// None of the colours below may be brighter than it is written, and none may
// have a channel at full, since every colour the vision system boxes things in
// has one; emergencyColours lists them for the test that holds them to it.
var (
	colAmbulance  = rl.NewColor(232, 232, 222, 255)
	colAmbStripe  = rl.NewColor(214, 196, 44, 255)
	colAmbGreen   = rl.NewColor(52, 150, 88, 255)
	colEngineRed  = rl.NewColor(190, 36, 34, 255)
	colEngineAlum = rl.NewColor(190, 192, 190, 255)
	colEngineHose = rl.NewColor(214, 168, 54, 255)
	colResponseY  = rl.NewColor(226, 218, 168, 255)
	colResponseB  = rl.NewColor(46, 74, 152, 255)
)

// emergencyColours is every colour the pack's cars are drawn in.
func emergencyColours() []rl.Color {
	return []rl.Color{
		colAmbulance, colAmbStripe, colAmbGreen, colEngineRed, colEngineAlum, colEngineHose,
		colResponseY, colResponseB,
	}
}

func init() {
	cabins[sim.Ambulance] = cabinStyle{Boot: 0.02, Draw: drawAmbulanceCabin}
	cabins[sim.FireEngine] = cabinStyle{Boot: 0.7, Draw: drawEngineCabin}
	cabins[sim.ResponseCar] = cabinStyle{Boot: bootShare(sim.Estate)}
	fittings[sim.Ambulance] = ambulanceFittings
	fittings[sim.FireEngine] = engineFittings
	fittings[sim.ResponseCar] = responseFittings
	blurbs[sim.Ambulance] = "a box on a van's chassis, with the right of way and the weight to match"
	blurbs[sim.FireEngine] = "nine tonnes, a ladder and a hose: slow to get going, and nothing stops it"
	blurbs[sim.ResponseCar] = "a fast estate with a light bar and somewhere to be"
}

// drawAmbulanceCabin is a tall patient box behind a lower cab.
func drawAmbulanceCabin(s sim.Spec, body sim.Body, look carLook, waist float32, paint, _ rl.Color) bool {
	length := s.HalfLength * 2
	rear := -s.HalfLength + length*0.02
	front := rear + length*body.RoofFrac
	roof := waist + body.RoofRise
	glass := scaleColor(colGlass, 0.62+0.2*(1-look.night))
	split := front - length*0.2
	cabTop := waist + (roof-waist)*0.7
	rl.DrawCube(vec3((rear+split)/2, (waist+roof)/2, 0), split-rear, roof-waist, s.HalfWidth*1.96, paint)
	rl.DrawCube(vec3((split+front)/2, (waist+cabTop)/2, 0), front-split, cabTop-waist, s.HalfWidth*1.9, paint)
	rl.DrawCube(vec3(front-0.03, waist+(cabTop-waist)*0.6, 0), 0.08, (cabTop-waist)*0.62, s.HalfWidth*1.7, glass)
	for _, side := range [2]float32{1, -1} {
		rl.DrawCube(vec3((split+front)/2+0.1, waist+(cabTop-waist)*0.6, side*s.HalfWidth*0.96),
			(front-split)*0.5, (cabTop-waist)*0.42, 0.04, glass)
	}
	return true
}

// drawEngineCabin is a crew cab ahead of a long locker body.
func drawEngineCabin(s sim.Spec, body sim.Body, look carLook, waist float32, paint, _ rl.Color) bool {
	length := s.HalfLength * 2
	cabin := length * body.RoofFrac
	front := s.HalfLength - length*0.02
	roof := waist + body.RoofRise
	glass := scaleColor(colGlass, 0.62+0.2*(1-look.night))
	drawCabAndBox(s, front-cabin, front, waist, roof, paint, glass, paint, 0.62)
	return true
}

// beaconBar draws a light bar across the roof at x: both halves sit dark until
// the vehicle is on a call, then they alternate blue and red.
func beaconBar(s sim.Spec, look carLook, x, roof float32) {
	left, right := colBeaconDark, colBeaconDark
	if look.lit {
		left, right = as(colBeaconBlue, matGlow), as(colBeaconBlue, matGlow)
		if look.blink {
			left = colBeaconDark
		} else {
			right = colBeaconDark
		}
	}
	rl.DrawCube(vec3(x, roof+0.04, 0), 0.3, 0.08, s.HalfWidth*1.5, colBeaconDark)
	rl.DrawCube(vec3(x, roof+0.12, s.HalfWidth*0.4), 0.26, 0.13, s.HalfWidth*0.66, right)
	rl.DrawCube(vec3(x, roof+0.12, -s.HalfWidth*0.4), 0.26, 0.13, s.HalfWidth*0.66, left)
}

// ambulanceFittings is the green and yellow chequer along each flank, a light
// bar over the cab, and a pair of lamps on the back corners.
func ambulanceFittings(s sim.Spec, body sim.Body, look carLook, waist float32) {
	length := s.HalfLength * 2
	blocks := 6
	for i := range blocks {
		x := (float32(i) - float32(blocks-1)/2) * (length * 0.9 / float32(blocks))
		col := colAmbGreen
		if i%2 == 1 {
			col = colAmbStripe
		}
		for _, side := range [2]float32{1, -1} {
			rl.DrawCube(vec3(x, body.Ride+(waist-body.Ride)*0.55, side*(s.HalfWidth+0.012)),
				length*0.9/float32(blocks)*0.98, (waist-body.Ride)*0.45, 0.03, col)
		}
	}
	roof := waist + body.RoofRise
	beaconBar(s, look, s.HalfLength*0.5, waist+(roof-waist)*0.7)
	for _, z := range [2]float32{s.HalfWidth * 0.8, -s.HalfWidth * 0.8} {
		lamp := colBeaconDark
		if look.lit && look.blink == (z > 0) {
			lamp = as(colBeaconBlue, matGlow)
		}
		rl.DrawCube(vec3(-s.HalfLength-0.02, roof-0.2, z), 0.06, 0.16, 0.16, lamp)
	}
}

// engineFittings is a ladder along the roof of the body, a hose reel on the
// back, a white stripe along each flank and a light bar on the cab.
func engineFittings(s sim.Spec, body sim.Body, look carLook, waist float32) {
	length := s.HalfLength * 2
	roof := waist + body.RoofRise
	top := waist + (roof-waist)*0.62
	rl.DrawCube(vec3(-length*0.12, top+0.1, 0), length*0.62, 0.12, s.HalfWidth*0.5, colEngineAlum)
	for _, z := range [2]float32{s.HalfWidth * 0.28, -s.HalfWidth * 0.28} {
		rl.DrawCube(vec3(-length*0.12, top+0.04, z), length*0.62, 0.08, 0.05, colEngineAlum)
	}
	rl.DrawCube(vec3(-s.HalfLength+0.05, waist-0.1, 0), 0.12, 0.7, s.HalfWidth*0.9, colEngineHose)
	for _, side := range [2]float32{1, -1} {
		rl.DrawCube(vec3(-length*0.14, body.Ride+(waist-body.Ride)*0.5, side*(s.HalfWidth+0.012)),
			length*0.6, 0.1, 0.03, colSignFace)
	}
	beaconBar(s, look, s.HalfLength-length*0.3*0.5, roof)
}

// responseFittings is the chequer of a response car along each flank and a
// light bar on the roof.
func responseFittings(s sim.Spec, body sim.Body, look carLook, waist float32) {
	length := s.HalfLength * 2
	blocks := 5
	for i := range blocks {
		x := (float32(i) - float32(blocks-1)/2) * (length * 0.92 / float32(blocks))
		col := colResponseB
		if i%2 == 1 {
			col = colResponseY
		}
		for _, side := range [2]float32{1, -1} {
			rl.DrawCube(vec3(x, body.Ride+(waist-body.Ride)*0.55, side*(s.HalfWidth+0.012)),
				length*0.92/float32(blocks)*0.98, (waist-body.Ride)*0.5, 0.03, col)
		}
	}
	roof := waist + body.RoofRise
	mid := -s.HalfLength + length*bootShare(body.Kind) + length*body.RoofFrac*0.5
	beaconBar(s, look, mid, roof)
}
