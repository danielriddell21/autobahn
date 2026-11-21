package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// Car proportions, in metres.
const (
	rideHeight  float32 = 0.27 // sill above the road
	wheelRadius float32 = 0.33
	wheelWidth  float32 = 0.25
)

// carLook is everything about how one car is drawn that is not its shape.
type carLook struct {
	paint     rl.Color
	indicator sim.Indicator
	blink     bool
	night     float32
	wheels    bool // near enough to be worth drawing round wheels
	player    bool
	boosting  bool
	flashing  bool // the driver is flashing their headlights
	// police is set for a marked unit, and lit while it is on a call.
	police bool
	unit   sim.UnitKind
	lit    bool
}

// bootShare is how much of a car's length is boot behind the cabin, by shape.
func bootShare(k sim.BodyKind) float32 {
	switch k {
	case sim.Saloon:
		return 0.19
	case sim.Estate, sim.Van:
		return 0.02
	case sim.Taxi:
		return 0.07
	default:
		return 0.06
	}
}

func (g *Game) drawTraffic(eye mathx.Vec, drawPlayer bool) {
	night := g.light.night
	for _, a := range g.world.Agents {
		d := a.V.Pos.DistTo(eye)
		if d > cullBuildings {
			continue
		}
		look := carLook{
			paint: carPaints[a.Paint%len(carPaints)], indicator: a.Indicator(),
			blink: g.blink, night: night, wheels: d < cullWheels, flashing: a.Flashing(),
		}
		if a.Role == sim.RolePolice {
			look.police, look.unit, look.lit = true, a.Unit, a.BluesAndTwos()
			look.paint = colPoliceBody
			if a.Unit != sim.UnitPatrol {
				look.paint = colPoliceDark
			}
		}
		drawCar(a.V, a.Body, look)
	}
	if drawPlayer {
		drawCar(g.world.Player, g.world.PlayerBody, carLook{
			paint: g.playerPaint(), blink: g.blink, night: night, wheels: true,
			player: true, boosting: g.world.Player.Boosting, indicator: g.playerIndicator(),
		})
	}
}

// playerIndicator is the signal the autopilot is showing, while it drives.
func (g *Game) playerIndicator() sim.Indicator {
	if !g.auto {
		return sim.IndicatorOff
	}
	switch g.lastCmd.Indicator {
	case autopilot.IndicateLeft:
		return sim.IndicatorLeft
	case autopilot.IndicateRight:
		return sim.IndicatorRight
	}
	return sim.IndicatorOff
}

func drawCar(v *sim.Vehicle, body sim.Body, look carLook) {
	// Renders a vehicle in its own local space, so the whole car rotates with
	// a single matrix push. +X is forward, +Z is the car's right.
	s := v.Spec
	length, width := s.HalfLength*2, s.HalfWidth*2
	waist := s.Height - body.RoofRise

	// A soft shadow on the road under it, which is most of what stops a car
	// looking as though it is floating.
	c := v.Box().Corners()
	groundQuad(c[0], c[1], c[2], c[3], 0.02, colShadow)

	rl.PushMatrix()
	rl.Translatef(v.Pos.X, 0, v.Pos.Z)
	rl.Rotatef(-v.Yaw*rad2deg, 0, 1, 0)

	paint := as(look.paint, matPaint)
	dark := scaleColor(look.paint, 0.72)

	// The lower body, with a bumper at each end and a sill along the bottom.
	rl.DrawCube(vec3(0, (rideHeight+waist)/2, 0), length*0.97, waist-rideHeight, width, paint)
	rl.DrawCube(vec3(0, rideHeight+0.07, 0), length, 0.2, width*1.02, colTrim)
	rl.DrawCube(vec3(s.HalfLength-0.1, waist-0.2, 0), 0.22, 0.2, width*0.7, colTrim)

	drawCabin(s, body, look, waist, paint, dark)
	drawLamps(v, look, waist)
	if look.police {
		drawPoliceFittings(s, body, look, waist)
	}
	if look.player {
		drawPlayerFittings(s, body, look, waist)
	}
	drawWheels(v, look)

	rl.PopMatrix()
}

func drawCabin(s sim.Spec, body sim.Body, look carLook, waist float32, paint, dark rl.Color) {
	length := s.HalfLength * 2
	cabin := length * body.RoofFrac
	rear := -s.HalfLength + length*bootShare(body.Kind)
	front := rear + cabin
	roof := waist + body.RoofRise
	half := s.HalfWidth * 0.9
	glass := scaleColor(colGlass, 0.62+0.2*(1-look.night))

	if body.Kind == sim.Van {
		// A van is a cab and a box: glass only at the front.
		split := front - cabin*0.3
		rl.DrawCube(vec3((rear+split)/2, (waist+roof)/2, 0), split-rear, roof-waist, s.HalfWidth*1.96, paint)
		taperBox(split, front, half, waist, split, front-0.45, half*0.94, roof-0.06, glass, dark)
		return
	}
	rake, slope := float32(0.55), float32(0.4)
	if body.Boxy {
		rake, slope = 0.3, 0.12
	}
	top := half * 0.86
	if body.Boxy {
		top = half * 0.95
	}
	// The glasshouse, raked at the front and the back, with the roof on top.
	taperBox(rear, front, half, waist, rear+slope, front-rake, top, roof-0.05, glass, dark)
	rl.DrawCube(vec3((rear+slope+front-rake)/2, roof-0.025, 0),
		front-rake-rear-slope+0.06, 0.06, top*2+0.04, paint)
	// A pillar each side, where the doors meet.
	mid := (rear + front) / 2
	for _, side := range [2]float32{1, -1} {
		rl.DrawCube(vec3(mid, (waist+roof)/2-0.03, side*(half+top)/2),
			0.13, roof-waist-0.04, 0.05, dark)
	}
}

func drawLamps(v *sim.Vehicle, look carLook, waist float32) {
	s := v.Spec
	// Lamps: white at the front, red at the back, brighter under braking. Once
	// it is dark they are all lit.
	head := rl.NewColor(232, 228, 206, 255)
	tail := rl.NewColor(132, 40, 36, 255)
	if look.night > 0.3 {
		head = as(rl.NewColor(246, 242, 222, 255), matGlow)
		tail = as(rl.NewColor(178, 44, 38, 255), matGlow)
	}
	if look.flashing {
		head = as(rl.NewColor(252, 250, 236, 255), matGlow)
	}
	if v.Brake > 0.05 {
		tail = as(rl.NewColor(238, 64, 52, 255), matGlow)
	}
	y := waist - 0.2
	for _, side := range [2]float32{1, -1} {
		z := side * (s.HalfWidth - 0.26)
		rl.DrawCube(vec3(s.HalfLength-0.03, y, z), 0.1, 0.17, 0.36, head)
		rl.DrawCube(vec3(-s.HalfLength+0.03, y, z), 0.1, 0.17, 0.36, tail)
	}

	if look.blink && look.indicator != sim.IndicatorOff {
		z := s.HalfWidth - 0.08
		if look.indicator == sim.IndicatorLeft {
			z = -z
		}
		amber := as(rl.NewColor(240, 164, 44, 255), matGlow)
		rl.DrawCube(vec3(s.HalfLength-0.08, y, z), 0.14, 0.17, 0.16, amber)
		rl.DrawCube(vec3(-s.HalfLength+0.08, y, z), 0.14, 0.17, 0.16, amber)
	}
}

func drawWheels(v *sim.Vehicle, look carLook) {
	s := v.Spec
	z := s.HalfWidth - wheelWidth/2 + 0.02
	if !look.wheels {
		// Too far off to tell a wheel from a block.
		for _, x := range [2]float32{s.FrontAxle, -s.RearAxle} {
			for _, side := range [2]float32{1, -1} {
				rl.DrawCube(vec3(x, wheelRadius, side*z), wheelRadius*1.8, wheelRadius*2, wheelWidth, colTyre)
			}
		}
		return
	}
	for _, side := range [2]float32{1, -1} {
		wheel(-s.RearAxle, side*z, wheelRadius, wheelWidth, colTyre, colHub)
		// The front pair turns with the steering.
		rl.PushMatrix()
		rl.Translatef(s.FrontAxle, 0, side*z)
		rl.Rotatef(-v.Steer*rad2deg, 0, 1, 0)
		wheel(0, 0, wheelRadius, wheelWidth, colTyre, colHub)
		rl.PopMatrix()
	}
}

func drawPoliceFittings(s sim.Spec, body sim.Body, look carLook, waist float32) {
	// Battenburg: alternating blocks along both flanks.
	length := s.HalfLength * 2
	blocks := 5
	for i := range blocks {
		x := (float32(i) - float32(blocks-1)/2) * (length * 0.92 / float32(blocks))
		block := colPoliceBlue
		if i%2 == 1 {
			block = colPoliceYellow
		}
		for _, side := range [2]float32{1, -1} {
			rl.DrawCube(vec3(x, rideHeight+(waist-rideHeight)*0.55, side*(s.HalfWidth+0.012)),
				length*0.92/float32(blocks)*0.98, (waist-rideHeight)*0.5, 0.03, block)
		}
	}
	if look.unit == sim.UnitHeavy {
		// A bull bar, which is the whole point of sending one.
		rl.DrawCube(vec3(s.HalfLength+0.12, waist-0.32, 0), 0.14, 0.62, s.HalfWidth*1.7, colTrim)
		rl.DrawCube(vec3(s.HalfLength+0.06, rideHeight+0.1, 0), 0.26, 0.2, s.HalfWidth*1.9, colTrim)
	}

	// The light bar. Both halves sit dark until the unit is on a call, then
	// they alternate blue and red.
	left, right := colBeaconDark, colBeaconDark
	if look.lit {
		left, right = as(colBeaconBlue, matGlow), as(colBeaconRed, matGlow)
		if look.blink {
			left, right = right, left
		}
	}
	roof := waist + body.RoofRise
	mid := -s.HalfLength + length*bootShare(body.Kind) + length*body.RoofFrac*0.5
	rl.DrawCube(vec3(mid, roof+0.04, 0), 0.3, 0.08, s.HalfWidth*1.5, colBeaconDark)
	rl.DrawCube(vec3(mid, roof+0.12, s.HalfWidth*0.4), 0.26, 0.13, s.HalfWidth*0.66, right)
	rl.DrawCube(vec3(mid, roof+0.12, -s.HalfWidth*0.4), 0.26, 0.13, s.HalfWidth*0.66, left)
}

func drawPlayerFittings(s sim.Spec, body sim.Body, look carLook, waist float32) {
	// A pair of stripes over the bonnet and a wing on the back, so the car
	// being driven is never mistaken for one of the others.
	for _, z := range [2]float32{0.2, -0.2} {
		rl.DrawCube(vec3(s.HalfLength*0.52, waist+0.004, z), s.HalfLength*0.9, 0.012, 0.15, colSignFace)
	}
	rl.DrawCube(vec3(-s.HalfLength+0.12, waist+0.3, 0), 0.34, 0.05, s.HalfWidth*1.86, colTrim)
	for _, z := range [2]float32{s.HalfWidth - 0.24, -s.HalfWidth + 0.24} {
		rl.DrawCube(vec3(-s.HalfLength+0.14, waist+0.15, z), 0.16, 0.3, 0.05, colTrim)
	}
	_ = body

	if !look.boosting {
		return
	}
	// Flame from the exhausts while the nitrous is burning.
	flick := float32(0.75)
	if look.blink {
		flick = 1.05
	}
	for _, z := range [2]float32{0.34, -0.34} {
		rl.DrawCube(vec3(-s.HalfLength-0.42*flick, rideHeight+0.06, z), 0.84*flick, 0.13, 0.13,
			as(rl.NewColor(110, 176, 250, 255), matGlow))
		rl.DrawCube(vec3(-s.HalfLength-0.2*flick, rideHeight+0.06, z), 0.4*flick, 0.17, 0.17,
			as(rl.NewColor(226, 240, 250, 255), matGlow))
	}
}
