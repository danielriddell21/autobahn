package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// Car proportions, in metres. How high a car rides and the size of its wheels
// come from its body.
const wheelWidth float32 = 0.25

// tractorRear is how much bigger a tractor's back wheels are than its front.
const tractorRear float32 = 1.7

var (
	// colCargo is the box on the back of a lorry, and the top half of a camper.
	colCargo   = rl.NewColor(204, 202, 194, 255)
	colSeat    = rl.NewColor(104, 72, 54, 255)
	colIcePink = rl.NewColor(222, 150, 170, 255)
	colWafer   = rl.NewColor(204, 164, 104, 255)
	colHopper  = rl.NewColor(214, 160, 52, 255)
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
	// parked is set for a car nobody is in, whose lamps stay off after dark.
	parked bool
}

// cabinStyle is how a pack's car builds its cabin. The packs fill [cabins]
// from init in their own cars_ files; the base game's shapes are in
// [bootShare] and [drawCabin].
type cabinStyle struct {
	// Boot is how much of the car's length is boot behind the cabin.
	Boot float32
	// Draw, when set, builds the cabin itself and returns true; when nil, or
	// when it returns false, the usual glasshouse is drawn.
	Draw func(s sim.Spec, body sim.Body, look carLook, waist float32, paint, dark rl.Color) bool
}

// cabins and fittings are the lookup tables the packs' cars are registered in.
// fittings are extras drawn on a car after its cabin and lamps: a light bar, a
// livery, a sign on the roof.
var (
	cabins   = map[sim.BodyKind]cabinStyle{}
	fittings = map[sim.BodyKind]func(s sim.Spec, body sim.Body, look carLook, waist float32){}
	// blurbs are the garage's line for each pack car, filled the same way.
	blurbs = map[sim.BodyKind]string{}
)

// bootShare is how much of a car's length is boot behind the cabin, by shape.
func bootShare(k sim.BodyKind) float32 {
	if c, ok := cabins[k]; ok {
		return c.Boot
	}
	return baseBoot(k)
}

// baseBoot is [bootShare] for the base game's shapes.
func baseBoot(k sim.BodyKind) float32 {
	switch k {
	case sim.Saloon:
		return 0.19
	case sim.Estate, sim.Van:
		return 0.02
	case sim.Taxi:
		return 0.07
	case sim.Convertible:
		return 0.2
	case sim.CityCar:
		return 0.04
	case sim.IceCreamVan, sim.Camper:
		return 0.02
	case sim.FourByFour, sim.SUV:
		return 0.03
	case sim.Minibus:
		return 0.02
	case sim.GrandTourer:
		return 0.14 // a long bonnet, so the cabin sits well back
	case sim.Gritter:
		return 0.76 // the hopper; the cab is over the front axle
	case sim.Tractor:
		return 0.04 // the cab sits over the back wheels
	case sim.Bus:
		return 0 // the decks run the whole length
	case sim.Lorry:
		return 0.78 // the load; the cab is over the front axle
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
		drawCar(a.V, a.Body, look, g.footingOf(a.V.Pos, a.V.Yaw))
	}
	if drawPlayer {
		drawCar(g.world.Player, g.world.PlayerBody, carLook{
			paint: g.playerPaint(), blink: g.blink, night: night, wheels: true,
			player: true, boosting: g.world.Player.Boosting, indicator: g.playerIndicator(),
		}, g.footingOf(g.world.Player.Pos, g.world.Player.Yaw))
	}
}

// playerIndicator is the signal the autopilot is showing, while it drives.
func (g *Game) playerIndicator() sim.Indicator {
	if !g.auto {
		return g.indicator
	}
	switch g.lastCmd.Indicator {
	case autopilot.IndicateLeft:
		return sim.IndicatorLeft
	case autopilot.IndicateRight:
		return sim.IndicatorRight
	}
	return sim.IndicatorOff
}

func drawCar(v *sim.Vehicle, body sim.Body, look carLook, on footing) {
	// Renders a vehicle in its own local space, so the whole car rotates with
	// a single matrix push. +X is forward, +Z is the car's right. On a hill
	// it is lifted to the ground and tipped as the ground is.
	s := v.Spec
	length, width := s.HalfLength*2, s.HalfWidth*2
	waist := s.Height - body.RoofRise

	// A soft shadow on the road under it, which is most of what stops a car
	// looking as though it is floating.
	c := v.Box().Corners()
	groundQuad(on.land, c[0], c[1], c[2], c[3], 0.02, colShadow)

	rl.PushMatrix()
	rl.Translatef(v.Pos.X, on.y, v.Pos.Z)
	rl.Rotatef(-v.Yaw*rad2deg, 0, 1, 0)
	if on.pitch != 0 || on.roll != 0 {
		rl.Rotatef(on.pitch*rad2deg, 0, 0, 1)
		rl.Rotatef(on.roll*rad2deg, 1, 0, 0)
	}

	paint := as(look.paint, matPaint)
	dark := scaleColor(look.paint, 0.72)

	// The lower body, with a bumper at each end and a sill along the bottom.
	ride := body.Ride
	rl.DrawCube(vec3(0, (ride+waist)/2, 0), length*0.97, waist-ride, width, paint)
	rl.DrawCube(vec3(0, ride+0.07, 0), length, 0.2, width*1.02, colTrim)
	rl.DrawCube(vec3(s.HalfLength-0.1, waist-0.2, 0), 0.22, 0.2, width*0.7, colTrim)

	drawCabin(s, body, look, waist, paint, dark)
	drawLamps(v, look, waist)
	if fit := fittings[body.Kind]; fit != nil {
		fit(s, body, look, waist)
	}
	if look.police {
		drawPoliceFittings(s, body, look, waist)
	}
	if look.player {
		drawPlayerFittings(s, body, look, waist)
	}
	drawWheels(v, body, look)

	rl.PopMatrix()
}

func drawCabin(s sim.Spec, body sim.Body, look carLook, waist float32, paint, dark rl.Color) {
	if c, ok := cabins[body.Kind]; ok && c.Draw != nil && c.Draw(s, body, look, waist, paint, dark) {
		return
	}
	drawBaseCabin(s, body, look, waist, paint, dark)
}

// drawBaseCabin builds the cabin of the base game's shapes, and the usual
// glasshouse for any other.
func drawBaseCabin(s sim.Spec, body sim.Body, look carLook, waist float32, paint, dark rl.Color) {
	length := s.HalfLength * 2
	cabin := length * body.RoofFrac
	rear := -s.HalfLength + length*bootShare(body.Kind)
	front := rear + cabin
	roof := waist + body.RoofRise
	half := s.HalfWidth * 0.9
	glass := scaleColor(colGlass, 0.62+0.2*(1-look.night))

	switch body.Kind {
	case sim.Bus:
		drawDecks(s, waist, roof, paint, glass)
		return
	case sim.Lorry:
		drawCabAndBox(s, front-cabin, front, waist, roof, paint, glass, colCargo, 1)
		return
	case sim.Gritter:
		drawCabAndBox(s, front-cabin, front, waist, roof, paint, glass, colHopper, 0.7)
		// The beacon on the cab, which never stops.
		beacon := colBeaconDark
		if look.blink {
			beacon = as(bulbAmber, matGlow)
		}
		rl.DrawCube(vec3(front-cabin/2, waist+(roof-waist)*0.62+0.1, 0), 0.3, 0.2, 0.3, beacon)
		return
	case sim.Convertible:
		drawOpenCabin(rear, front, half, waist, roof, glass)
		return
	case sim.Tractor:
		drawTractorCab(s, rear, front, waist, roof, paint, glass)
		return
	case sim.Camper:
		drawCamper(s, waist, roof, glass)
		return
	case sim.Van, sim.IceCreamVan:
		// A van is a cab and a box: glass only at the front.
		split := front - cabin*0.3
		rl.DrawCube(vec3((rear+split)/2, (waist+roof)/2, 0), split-rear, roof-waist, s.HalfWidth*1.96, paint)
		taperBox(split, front, half, waist, split, front-0.45, half*0.94, roof-0.06, glass, dark)
		if body.Kind == sim.IceCreamVan {
			drawIceCreamFittings(s, rear, split, waist, roof, glass)
		}
		return
	default:
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
	if body.Kind == sim.FourByFour {
		// The spare on the back door, and a rack on the roof.
		rl.DrawCube(vec3(-s.HalfLength-0.1, waist+0.05, 0), 0.2, 0.72, 0.72, colTyre)
		rl.DrawCube(vec3((rear+front)/2, roof+0.07, 0), (front-rear)*0.7, 0.05, top*1.8, colTrim)
	}
}

// drawTractorCab builds a tractor on its lower body: a glass cab over the
// back axle, a bonnet out in front of it, and an exhaust up the side.
func drawTractorCab(s sim.Spec, rear, front, waist, roof float32, paint, glass rl.Color) {
	width := s.HalfWidth * 2
	rl.DrawCube(vec3((rear+front)/2, (waist+roof)/2, 0), front-rear, roof-waist, width*0.7, glass)
	rl.DrawCube(vec3((rear+front)/2, roof-0.04, 0), front-rear+0.1, 0.1, width*0.76, paint)
	for _, x := range [2]float32{rear, front} {
		for _, side := range [2]float32{1, -1} {
			rl.DrawCube(vec3(x, (waist+roof)/2, side*width*0.35), 0.09, roof-waist, 0.09, paint)
		}
	}
	// The bonnet, narrower than the body, running out to the nose.
	rl.DrawCube(vec3((front+s.HalfLength)/2, waist+0.22, 0), s.HalfLength-front, 0.44, width*0.5, paint)
	rl.DrawCube(vec3(front+0.25, waist+0.9, width*0.3), 0.09, 1.3, 0.09, colTrim)
}

// drawOpenCabin builds a car with no roof: a raked screen, the seats behind
// it, and a hoop behind each of those.
func drawOpenCabin(rear, front, half, waist, roof float32, glass rl.Color) {
	// The well the seats are in, which is what says there is no roof.
	rl.DrawCube(vec3((rear+front)/2-0.1, waist+0.012, 0), front-rear-0.5, 0.02, half*1.7, colTrim)
	taperBox(front-0.34, front-0.24, half*0.96, waist, front-0.6, front-0.52, half*0.9, roof, glass, colTrim)
	seat := rear + (front-rear)*0.42
	for _, side := range [2]float32{1, -1} {
		z := side * half * 0.48
		rl.DrawCube(vec3(seat, waist+0.13, z), 0.2, 0.34, half*0.62, colSeat)
		rl.DrawCube(vec3(seat-0.22, waist+0.17, z), 0.08, 0.3, half*0.5, colTrim)
	}
}

// drawCamper builds a camper van: a white top half with glass all round it,
// and a roof that lifts.
func drawCamper(s sim.Spec, waist, roof float32, glass rl.Color) {
	length, width := s.HalfLength*2, s.HalfWidth*2
	rl.DrawCube(vec3(0, (waist+roof)/2, 0), length*0.97, roof-waist, width, colCargo)
	rl.DrawCube(vec3(0.02, waist+(roof-waist)*0.48, 0), length*0.975, (roof-waist)*0.44, width*1.012, glass)
	rl.DrawCube(vec3(-length*0.08, roof+0.09, 0), length*0.6, 0.18, width*0.86, colCargo)
}

// drawIceCreamFittings dresses a van to sell ice cream from: a stripe, a
// hatch on the kerb side with a shade over it, and a cone on the roof.
func drawIceCreamFittings(s sim.Spec, rear, split, waist, roof float32, glass rl.Color) {
	mid := (rear + split) / 2
	for _, side := range [2]float32{1, -1} {
		rl.DrawCube(vec3(mid, waist+0.12, side*(s.HalfWidth*0.98+0.012)), split-rear, 0.2, 0.03, colIcePink)
	}
	// Traffic keeps left, so the kerb is on the van's left.
	kerb := -s.HalfWidth * 0.98
	rl.DrawCube(vec3(mid, waist+(roof-waist)*0.55, kerb-0.012), (split-rear)*0.5, (roof-waist)*0.42, 0.03, glass)
	rl.DrawCube(vec3(mid, waist+(roof-waist)*0.82, kerb-0.3), (split-rear)*0.56, 0.05, 0.6, colIcePink)
	at := mathx.V(mid, 0)
	pillar(at, roof, roof+0.55, 0.05, 0.2, 8, colWafer)
	pillar(at, roof+0.55, roof+0.8, 0.22, 0.08, 8, colIcePink)
}

// drawDecks builds a double-decker on top of its lower body: one slab the
// height of both decks, with a band of glass round each.
func drawDecks(s sim.Spec, waist, roof float32, paint, glass rl.Color) {
	length, width := s.HalfLength*2, s.HalfWidth*2
	rl.DrawCube(vec3(0, (waist+roof)/2, 0), length*0.97, roof-waist, width, paint)
	deck := (roof - waist) / 2
	for i := range 2 {
		y := waist + deck*float32(i) + deck*0.5
		rl.DrawCube(vec3(0.03, y, 0), length*0.975, deck*0.52, width*1.012, glass)
	}
}

// drawCabAndBox builds a lorry: a cab over the front axle, with the load in a
// box behind it that stands taller than the cab.
//
// load is the colour of the box and fill how much of the height above the
// waist it stands to: a lorry's box stands to the top, a gritter's hopper
// stops short.
func drawCabAndBox(s sim.Spec, cabRear, front, waist, roof float32, paint, glass, load rl.Color, fill float32) {
	width := s.HalfWidth * 2
	cabTop := waist + (roof-waist)*0.62
	rl.DrawCube(vec3((cabRear+front)/2, (waist+cabTop)/2, 0), front-cabRear, cabTop-waist, width*0.98, paint)
	// The windscreen, and a window in each door.
	rl.DrawCube(vec3(front-0.02, waist+(cabTop-waist)*0.62, 0), 0.08, (cabTop-waist)*0.46, width*0.86, glass)
	rl.DrawCube(vec3((cabRear+front)/2+0.2, waist+(cabTop-waist)*0.64, 0),
		(front-cabRear)*0.5, (cabTop-waist)*0.4, width*0.992, glass)
	rear := -s.HalfLength
	gap := float32(0.3)
	top := waist + (roof-waist)*fill
	rl.DrawCube(vec3((rear+cabRear-gap)/2, (waist+top)/2, 0), cabRear-gap-rear, top-waist, width, load)
}

func drawLamps(v *sim.Vehicle, look carLook, waist float32) {
	s := v.Spec
	// Lamps: white at the front, red at the back, brighter under braking. Once
	// it is dark they are all lit.
	head := rl.NewColor(232, 228, 206, 255)
	tail := rl.NewColor(132, 40, 36, 255)
	if look.night > 0.3 && !look.parked {
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

func drawWheels(v *sim.Vehicle, body sim.Body, look carLook) {
	s, r := v.Spec, body.Wheel
	z := s.HalfWidth - wheelWidth/2 + 0.02
	if !look.wheels {
		// Too far off to tell a wheel from a block.
		for _, x := range [2]float32{s.FrontAxle, -s.RearAxle} {
			for _, side := range [2]float32{1, -1} {
				rl.DrawCube(vec3(x, r, side*z), r*1.8, r*2, wheelWidth, colTyre)
			}
		}
		return
	}
	for _, side := range [2]float32{1, -1} {
		if body.Kind == sim.Tractor {
			// The big wheels are at the back.
			wheel(-s.RearAxle, side*z, r*tractorRear, wheelWidth*1.8, colTyre, colWafer)
		} else {
			wheel(-s.RearAxle, side*z, r, wheelWidth, colTyre, colHub)
		}
		// The front pair turns with the steering.
		rl.PushMatrix()
		rl.Translatef(s.FrontAxle, 0, side*z)
		rl.Rotatef(-v.Steer*rad2deg, 0, 1, 0)
		wheel(0, 0, r, wheelWidth, colTyre, colHub)
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
			rl.DrawCube(vec3(x, body.Ride+(waist-body.Ride)*0.55, side*(s.HalfWidth+0.012)),
				length*0.92/float32(blocks)*0.98, (waist-body.Ride)*0.5, 0.03, block)
		}
	}
	if look.unit == sim.UnitHeavy {
		// A bull bar, which is the whole point of sending one.
		rl.DrawCube(vec3(s.HalfLength+0.12, waist-0.32, 0), 0.14, 0.62, s.HalfWidth*1.7, colTrim)
		rl.DrawCube(vec3(s.HalfLength+0.06, body.Ride+0.1, 0), 0.26, 0.2, s.HalfWidth*1.9, colTrim)
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
		rl.DrawCube(vec3(-s.HalfLength-0.42*flick, body.Ride+0.06, z), 0.84*flick, 0.13, 0.13,
			as(rl.NewColor(110, 176, 250, 255), matGlow))
		rl.DrawCube(vec3(-s.HalfLength-0.2*flick, body.Ride+0.06, z), 0.4*flick, 0.17, 0.17,
			as(rl.NewColor(226, 240, 250, 255), matGlow))
	}
}
