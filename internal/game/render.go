package game

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/paint"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// The world palette. Every colour here is deliberately muted: the vision
// system reserves the fully saturated combinations of 0, 128 and 255 for its
// annotation boxes, so nothing in the scene can be mistaken for a detection.
var (
	colSky      = rl.NewColor(139, 174, 206, 255)
	colGround   = rl.NewColor(86, 104, 72, 255)
	colAsphalt  = rl.NewColor(58, 58, 63, 255)
	colSidewalk = rl.NewColor(131, 129, 123, 255)
	colKerb     = rl.NewColor(154, 151, 144, 255)
	colPark     = rl.NewColor(74, 112, 64, 255)
	colLinePale = rl.NewColor(216, 214, 206, 255)
	// Yellow paint in Britain means a parking restriction at the kerb, never a
	// lane divider, so it only ever appears as the double lines by the pavement.
	colLineWarm = rl.NewColor(198, 170, 74, 255)
	colPole     = rl.NewColor(94, 96, 99, 255)
	colSignBack = rl.NewColor(74, 76, 80, 255)
	colStopFace = rl.NewColor(178, 52, 48, 255)
	colSignFace = rl.NewColor(226, 224, 216, 255)
	colGlass    = rl.NewColor(96, 116, 134, 255)
	colTyre     = rl.NewColor(38, 38, 41, 255)
	colLampHead = rl.NewColor(206, 200, 172, 255)

	buildingShades = [6]rl.Color{
		rl.NewColor(148, 142, 132, 255),
		rl.NewColor(122, 124, 128, 255),
		rl.NewColor(166, 150, 134, 255),
		rl.NewColor(110, 116, 122, 255),
		rl.NewColor(138, 128, 118, 255),
		rl.NewColor(96, 100, 106, 255),
	}

	carPaints = [8]rl.Color{
		rl.NewColor(186, 62, 58, 255),
		rl.NewColor(58, 92, 152, 255),
		rl.NewColor(212, 208, 198, 255),
		rl.NewColor(52, 56, 62, 255),
		rl.NewColor(196, 152, 60, 255),
		rl.NewColor(74, 128, 104, 255),
		rl.NewColor(148, 150, 156, 255),
		rl.NewColor(126, 74, 132, 255),
	}

	bulbRed   = rl.NewColor(232, 58, 48, 255)
	bulbAmber = rl.NewColor(232, 158, 44, 255)
	bulbGreen = rl.NewColor(72, 196, 96, 255)
	bulbDark  = rl.NewColor(46, 44, 44, 255)
)

// Draw distances in metres.
const (
	cullBuildings float32 = 300
	cullBlocks    float32 = 320
	cullMarkings  float32 = 150
	cullDashes    float32 = 110
	cullProps     float32 = 130
	cullCrossing  float32 = 95
)

// lightingVS is the vertex shader used for the whole scene. It matches
// raylib's default attribute names so it works with immediate-mode drawing.
const lightingVS = `#version 330
in vec3 vertexPosition;
in vec2 vertexTexCoord;
in vec4 vertexColor;
in vec3 vertexNormal;
uniform mat4 mvp;
uniform mat4 matNormal;
out vec2 fragTexCoord;
out vec4 fragColor;
out vec3 fragNormal;
void main() {
    fragTexCoord = vertexTexCoord;
    fragColor = vertexColor;
    fragNormal = normalize(vec3(matNormal * vec4(vertexNormal, 1.0)));
    gl_Position = mvp * vec4(vertexPosition, 1.0);
}`

// lightingFS applies a single directional light plus ambient, which is enough
// to read the shape of the city without any texture assets.
const lightingFS = `#version 330
in vec2 fragTexCoord;
in vec4 fragColor;
in vec3 fragNormal;
uniform sampler2D texture0;
uniform vec4 colDiffuse;
out vec4 finalColor;
void main() {
    vec3 sun = normalize(vec3(-0.45, 0.82, 0.35));
    float lambert = max(dot(normalize(fragNormal), sun), 0.0);
    float shade = 0.46 + 0.54 * lambert;
    vec4 base = texture(texture0, fragTexCoord) * colDiffuse * fragColor;
    finalColor = vec4(base.rgb * shade, base.a);
}`

func vec3(x, y, z float32) rl.Vector3 { return rl.NewVector3(x, y, z) }

// drawWorld renders the whole scene from the given camera. When drawPlayer is
// false the player's own car is omitted, which is what the bonnet-mounted AI
// camera wants.
func (g *Game) drawWorld(cam rl.Camera3D, drawPlayer bool) {
	eye := mathx.V(cam.Position.X, cam.Position.Z)

	rl.BeginMode3D(cam)
	if g.hasShader {
		rl.BeginShaderMode(g.shader)
	}

	g.drawGround()
	g.drawBlocks(eye)
	g.drawMarkings(eye)
	g.drawBuildings(eye)
	g.drawProps(eye)

	for _, a := range g.world.Agents {
		if a.V.Pos.DistTo(eye) > cullBuildings {
			continue
		}
		drawCar(a.V, carPaints[a.Paint%len(carPaints)], a.Indicator(), g.blink)
	}
	if drawPlayer {
		drawCar(g.world.Player, carPaints[0], sim.IndicatorOff, false)
	}

	if g.hasShader {
		rl.EndShaderMode()
	}
	rl.EndMode3D()
}

func (g *Game) drawGround() {
	c := g.world.City
	// A large grass plane under everything, then an asphalt slab across the
	// built-up area. The roads are simply the parts of the slab that the
	// pavements do not cover.
	rl.DrawPlane(vec3(0, -0.06, 0), rl.NewVector2(6000, 6000), colGround)
	w := c.Max.X - c.Min.X + 260
	d := c.Max.Z - c.Min.Z + 260
	cx := (c.Max.X + c.Min.X) / 2
	cz := (c.Max.Z + c.Min.Z) / 2
	rl.DrawPlane(vec3(cx, -0.02, cz), rl.NewVector2(w, d), colAsphalt)
}

func (g *Game) drawBlocks(eye mathx.Vec) {
	for _, b := range g.world.City.Blocks {
		cx := (b.Min.X + b.Max.X) / 2
		cz := (b.Min.Z + b.Max.Z) / 2
		if mathx.V(cx, cz).DistTo(eye) > cullBlocks {
			continue
		}
		w := b.Max.X - b.Min.X
		d := b.Max.Z - b.Min.Z
		// The pavement slab, raised to make a kerb.
		rl.DrawCube(vec3(cx, city.KerbHeight/2, cz), w, city.KerbHeight, d, colKerb)
		surface := colSidewalk
		if b.Park {
			surface = colPark
		}
		rl.DrawCube(vec3(cx, city.KerbHeight, cz), w-0.5, 0.02, d-0.5, surface)
	}
}

func (g *Game) drawMarkings(eye mathx.Vec) {
	c := g.world.City
	const y = 0.015
	for _, r := range c.Roads {
		mid := c.Nodes[r.A].Pos.Lerp(c.Nodes[r.B].Pos, 0.5)
		if mid.DistTo(eye) > cullMarkings+r.Length/2 {
			continue
		}
		a := c.Nodes[r.A].Pos.Add(r.Dir.Mul(c.Nodes[r.A].Radius))
		b := c.Nodes[r.B].Pos.Sub(r.Dir.Mul(c.Nodes[r.B].Radius))
		length := b.Sub(a).Dot(r.Dir)
		if length <= 1 {
			continue
		}
		centre := a.Lerp(b, 0.5)

		// The centre line dividing opposing traffic is white here: solid and
		// doubled on the main roads, a long broken line elsewhere.
		if r.Class == city.Arterial {
			for _, off := range [2]float32{-0.18, 0.18} {
				p := centre.Add(r.Dir.Right().Mul(off))
				drawStripe(p, r.Axis, length, 0.14, y, colLinePale)
			}
		} else if mid.DistTo(eye) < cullDashes {
			for t := float32(2); t < length-2; t += 9 {
				p := a.Add(r.Dir.Mul(t + 2))
				drawStripe(p, r.Axis, 4, 0.14, y, colLinePale)
			}
		}

		// Double yellow lines against both kerbs: no waiting at any time.
		for _, s := range [2]float32{-1, 1} {
			for _, off := range [2]float32{0.26, 0.46} {
				p := centre.Add(r.Dir.Right().Mul(s * (r.HalfWidth - off)))
				drawStripe(p, r.Axis, length, 0.1, y, colLineWarm)
			}
		}

		// Dashed dividers between same-direction lanes on multi-lane roads.
		if r.Class.Lanes() > 1 && mid.DistTo(eye) < cullDashes {
			for _, s := range [2]float32{-1, 1} {
				off := s * city.LaneWidth
				for t := float32(2); t < length-2; t += 6.5 {
					p := a.Add(r.Dir.Mul(t + 1.6)).Add(r.Dir.Right().Mul(off))
					drawStripe(p, r.Axis, 3.2, 0.14, y, colLinePale)
				}
			}
		}
	}

	// Stop lines and pedestrian crossings on controlled approaches.
	for _, l := range c.Lanes {
		if l.Control == city.ControlNone || l.B.DistTo(eye) > cullMarkings {
			continue
		}
		axis := c.Roads[l.Road].Axis
		bar := l.B.Sub(l.Fwd.Mul(0.5))
		if l.Control == city.ControlGiveWay {
			// A give way line is broken, and there are two of them.
			for _, back := range [2]float32{0.0, 0.55} {
				p := bar.Sub(l.Fwd.Mul(back))
				for i := range 4 {
					off := (float32(i) - 1.5) * 0.85
					drawStripe(p.Add(l.Fwd.Right().Mul(off)), 1-axis, 0.5, 0.28, y, colLinePale)
				}
			}
		} else {
			drawStripe(bar, 1-axis, city.LaneWidth-0.3, 0.5, y, colLinePale)
		}

		if l.Index == 0 && l.B.DistTo(eye) < cullCrossing {
			// Zebra stripes just beyond the stop line, running with the
			// direction of travel and spaced across the lane.
			base := l.B.Add(l.Fwd.Mul(1.7))
			for i := range 5 {
				off := (float32(i) - 2) * 0.8
				p := base.Add(l.Fwd.Right().Mul(off))
				drawStripe(p, axis, 2.6, 0.4, y, colLinePale)
			}
		}
	}
}

func signOf(positive bool) float32 {
	if positive {
		return 1
	}
	return -1
}

// drawStripe paints a flat rectangle on the road. axis 0 runs along X, axis 1
// along Z; length follows the axis and width crosses it.
func drawStripe(p mathx.Vec, axis int, length, width, y float32, col rl.Color) {
	if axis == 0 {
		rl.DrawCube(vec3(p.X, y, p.Z), length, 0.02, width, col)
		return
	}
	rl.DrawCube(vec3(p.X, y, p.Z), width, 0.02, length, col)
}

func (g *Game) drawBuildings(eye mathx.Vec) {
	for _, b := range g.world.City.Buildings {
		if b.Center.DistTo(eye) > cullBuildings {
			continue
		}
		shade := buildingShades[int(b.Shade)%len(buildingShades)]
		rl.DrawCube(vec3(b.Center.X, b.Height/2, b.Center.Z), b.W, b.Height, b.D, shade)
		// A darker parapet caps the roof and reads as a silhouette detail.
		rl.DrawCube(vec3(b.Center.X, b.Height+0.35, b.Center.Z),
			b.W*0.97, 0.7, b.D*0.97, scaleColor(shade, 0.74))
		// Tall buildings get a setback tower so the skyline is not all boxes.
		if b.Height > 34 {
			rl.DrawCube(vec3(b.Center.X, b.Height+4.5, b.Center.Z),
				b.W*0.55, 8, b.D*0.55, scaleColor(shade, 0.88))
		}
		// A glazed band at street level.
		rl.DrawCube(vec3(b.Center.X, 2.1, b.Center.Z), b.W*1.005, 2.6, b.D*1.005, colGlass)
	}
}

// scaleColor dims a colour, delegating the arithmetic to crucible's paint
// package so brightness scaling matches the rest of the family.
func scaleColor(c rl.Color, f float32) rl.Color {
	v := paint.Scale(color.RGBA{R: c.R, G: c.G, B: c.B, A: c.A}, float64(f))
	return rl.NewColor(v.R, v.G, v.B, v.A)
}

func (g *Game) drawProps(eye mathx.Vec) {
	for _, p := range g.world.City.Props {
		if p.Pos.DistTo(eye) > cullProps {
			continue
		}
		switch p.Kind {
		case city.PropTrafficLight:
			g.drawTrafficLight(p)
		case city.PropStopSign:
			drawSignPost(p, colStopFace, 0.42)
		case city.PropGiveWaySign:
			drawGiveWaySign(p)
		case city.PropSpeedSign:
			drawSignPost(p, colSignFace, 0.36)
		case city.PropStreetLamp:
			drawLamp(p)
		}
	}
}

func (g *Game) drawTrafficLight(p city.Prop) {
	state := g.world.Signals.State(p.Node, p.Group)
	rl.DrawCylinderEx(vec3(p.Pos.X, 0, p.Pos.Z), vec3(p.Pos.X, p.Height-0.9, p.Pos.Z),
		0.07, 0.07, 8, colPole)

	rl.PushMatrix()
	rl.Translatef(p.Pos.X, 0, p.Pos.Z)
	rl.Rotatef(-p.Heading*rl.Rad2deg, 0, 1, 0)
	head := p.Height - 0.35
	rl.DrawCube(vec3(0, head, 0), 0.22, 1.0, 0.34, colSignBack)
	// Three bulbs, only the active one lit.
	bulbs := [3]rl.Color{bulbDark, bulbDark, bulbDark}
	switch state {
	case sim.SignalGreen:
		bulbs[2] = bulbGreen
	case sim.SignalAmber:
		bulbs[1] = bulbAmber
	case sim.SignalRedAmber:
		bulbs[0], bulbs[1] = bulbRed, bulbAmber
	default:
		bulbs[0] = bulbRed
	}
	for i, col := range bulbs {
		rl.DrawCube(vec3(-0.13, head+0.3-float32(i)*0.3, 0), 0.06, 0.2, 0.2, col)
	}
	rl.PopMatrix()
}

func drawSignPost(p city.Prop, face rl.Color, size float32) {
	rl.DrawCylinderEx(vec3(p.Pos.X, 0, p.Pos.Z), vec3(p.Pos.X, p.Height-size, p.Pos.Z),
		0.05, 0.05, 8, colPole)
	rl.PushMatrix()
	rl.Translatef(p.Pos.X, 0, p.Pos.Z)
	rl.Rotatef(-p.Heading*rl.Rad2deg, 0, 1, 0)
	rl.DrawCube(vec3(-0.06, p.Height-size, 0), 0.06, size*2, size*2, face)
	rl.PopMatrix()
}

// drawGiveWaySign renders the British inverted triangle: a white face with a
// red border, pointing down.
func drawGiveWaySign(p city.Prop) {
	const size float32 = 0.5
	rl.DrawCylinderEx(vec3(p.Pos.X, 0, p.Pos.Z), vec3(p.Pos.X, p.Height-size, p.Pos.Z),
		0.05, 0.05, 8, colPole)
	rl.PushMatrix()
	rl.Translatef(p.Pos.X, 0, p.Pos.Z)
	rl.Rotatef(-p.Heading*rl.Rad2deg, 0, 1, 0)
	// Three tapering slabs approximate the downward triangle.
	for i := range 3 {
		f := 1 - float32(i)*0.32
		rl.DrawCube(vec3(-0.06, p.Height-size*float32(i)*0.34, 0),
			0.05, size*0.36, size*2*f, colStopFace)
		rl.DrawCube(vec3(-0.09, p.Height-size*float32(i)*0.34, 0),
			0.04, size*0.24, size*1.6*f, colSignFace)
	}
	rl.PopMatrix()
}

func drawLamp(p city.Prop) {
	rl.DrawCylinderEx(vec3(p.Pos.X, 0, p.Pos.Z), vec3(p.Pos.X, p.Height, p.Pos.Z),
		0.09, 0.07, 8, colPole)
	rl.PushMatrix()
	rl.Translatef(p.Pos.X, 0, p.Pos.Z)
	rl.Rotatef(-p.Heading*rl.Rad2deg, 0, 1, 0)
	rl.DrawCube(vec3(0, p.Height, -1.1), 0.1, 0.1, 2.2, colPole)
	rl.DrawCube(vec3(0, p.Height-0.12, -2.1), 0.42, 0.14, 0.8, colLampHead)
	rl.PopMatrix()
}

// drawCar renders a vehicle as a stack of boxes in its own local space, so the
// whole car rotates with a single matrix push.
func drawCar(v *sim.Vehicle, paint rl.Color, ind sim.Indicator, blinkOn bool) {
	s := v.Spec
	rl.PushMatrix()
	rl.Translatef(v.Pos.X, 0, v.Pos.Z)
	rl.Rotatef(-v.Yaw*rl.Rad2deg, 0, 1, 0)

	length := s.HalfLength * 2
	width := s.HalfWidth * 2

	// Body, then a narrower cabin set back from the nose.
	rl.DrawCube(vec3(0, 0.62, 0), length, 0.62, width, paint)
	rl.DrawCube(vec3(-0.15, 1.12, 0), length*0.52, 0.44, width*0.88, scaleColor(paint, 0.86))
	rl.DrawCube(vec3(-0.15, 1.12, 0), length*0.5, 0.30, width*0.92, colGlass)
	// A little nose and tail definition.
	rl.DrawCube(vec3(length*0.42, 0.42, 0), length*0.14, 0.26, width*0.96, scaleColor(paint, 0.9))

	// Wheels. The front pair turns with the steering angle.
	for _, f := range [2]float32{1, -1} {
		for _, side := range [2]float32{1, -1} {
			x := f * s.FrontAxle
			if f < 0 {
				x = -s.RearAxle
			}
			z := side * (s.HalfWidth - 0.08)
			if f > 0 {
				rl.PushMatrix()
				rl.Translatef(x, 0.33, z)
				rl.Rotatef(-v.Steer*rl.Rad2deg, 0, 1, 0)
				rl.DrawCube(vec3(0, 0, 0), 0.66, 0.66, 0.24, colTyre)
				rl.PopMatrix()
				continue
			}
			rl.DrawCube(vec3(x, 0.33, z), 0.66, 0.66, 0.24, colTyre)
		}
	}

	// Lamps: white at the front, red at the back, brighter under braking.
	tail := rl.NewColor(150, 44, 40, 255)
	if v.Brake > 0.05 {
		tail = rl.NewColor(226, 62, 52, 255)
	}
	for _, side := range [2]float32{1, -1} {
		z := side * (s.HalfWidth - 0.22)
		rl.DrawCube(vec3(s.HalfLength-0.04, 0.66, z), 0.1, 0.16, 0.3, rl.NewColor(224, 222, 200, 255))
		rl.DrawCube(vec3(-s.HalfLength+0.04, 0.66, z), 0.1, 0.16, 0.3, tail)
	}

	// Indicators.
	if blinkOn && ind != sim.IndicatorOff {
		z := s.HalfWidth - 0.1
		if ind == sim.IndicatorRight {
			z = -z
		}
		amber := rl.NewColor(232, 158, 44, 255)
		rl.DrawCube(vec3(s.HalfLength-0.1, 0.66, z), 0.12, 0.16, 0.18, amber)
		rl.DrawCube(vec3(-s.HalfLength+0.1, 0.66, z), 0.12, 0.16, 0.18, amber)
	}

	rl.PopMatrix()
}
