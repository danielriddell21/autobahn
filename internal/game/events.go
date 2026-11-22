package game

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/night"
	"github.com/danielriddell21/autobahn/internal/works"
)

// Evening events are drawn the way the rest of the street furniture is: through
// the scene's shader, standing where they are, worked out from where that is and
// from the clock, and stored nowhere. Package night says where each event is and
// which kind it is tonight; this only gives it a shape and a colour. Nothing
// here is annotated, so the autopilot never sees any of it, and there are no
// people in it: a crowd is a sound and a slower road.
//
// None of the colours below may be brighter than it is written, and none may
// have a channel at full: the vision system's colours all do, which is what
// keeps them out of the scene, and the shader holds everything just short of
// it. events_test.go holds them to that. A glow is a lamp, so it is unlit; the
// sparks of a firework fade by being scaled down, never up.
var (
	colStallWood  = rl.NewColor(150, 104, 64, 255)
	colAwningRed  = rl.NewColor(200, 64, 56, 255)
	colAwningPale = rl.NewColor(222, 218, 204, 255)
	colMast       = rl.NewColor(96, 98, 104, 255)
	colWheel      = rl.NewColor(176, 180, 188, 255)
	colFlood      = rl.NewColor(238, 234, 200, 255)
	colUnderglow  = rl.NewColor(90, 200, 236, 255)
	colBodyShadow = rl.NewColor(34, 36, 42, 255)
)

// eventBulbs are the colours of festival lights and of the gondolas of a fair,
// eventPaints the paint of the show cars at a meet, and eventSparks the colours
// a firework bursts in.
var (
	eventBulbs = [4]rl.Color{
		rl.NewColor(236, 196, 92, 255), rl.NewColor(120, 200, 150, 255),
		rl.NewColor(232, 110, 140, 255), rl.NewColor(110, 170, 236, 255),
	}
	eventPaints = [5]rl.Color{
		rl.NewColor(186, 40, 60, 255), rl.NewColor(44, 96, 186, 255), rl.NewColor(224, 190, 50, 255),
		rl.NewColor(70, 160, 90, 255), rl.NewColor(200, 200, 206, 255),
	}
	eventSparks = [5]rl.Color{
		rl.NewColor(238, 170, 80, 255), rl.NewColor(120, 220, 160, 255), rl.NewColor(236, 120, 150, 255),
		rl.NewColor(130, 170, 238, 255), rl.NewColor(230, 230, 170, 255),
	}
)

// eventColours is every colour the events are drawn in.
func eventColours() []rl.Color {
	out := []rl.Color{
		colStallWood, colAwningRed, colAwningPale, colMast, colWheel, colFlood, colUnderglow, colBodyShadow,
	}
	out = append(out, eventBulbs[:]...)
	out = append(out, eventPaints[:]...)
	return append(out, eventSparks[:]...)
}

// eventReach is how far from the eye a venue is still drawn, past its own size.
const eventReach float32 = cullProps * 2

// drawEvents draws the events in force. The signs that go with a closure are
// the roadworks', drawn by drawWorks.
func (g *Game) drawEvents(eye mathx.Vec) {
	ix := g.world.Night
	if ix == nil || ix.Phase() != works.PhaseSolid {
		return
	}
	now := g.world.Time
	for _, e := range ix.Events() {
		if e.Pos.DistTo(eye) > eventReach+e.Radius {
			continue
		}
		if e.Kind == night.Meet {
			g.drawMeet(e)
			continue
		}
		g.standOn(mathx.Vec{}, e.Pos, e.Turn)
		switch e.Kind {
		case night.Fireworks:
			drawFair(e.Half, now)
			drawBursts(e.Half, now, e.Cell[0]*31+e.Cell[1])
		case night.Match:
			drawFloodlights(e.Half)
		default:
			drawMarket(e.Half)
		}
		rl.PopMatrix()
	}
}

// bulb draws a festival lamp at a point of the block's own coordinates.
func bulb(x, y, z float32, col rl.Color) {
	rl.DrawCube(vec3(x, y, z), 0.22, 0.22, 0.22, as(col, matGlow))
}

// drawMarket draws a row of stalls along the near kerb of a block, in the
// block's own coordinates about its middle, and a string of festival lights
// above them.
func drawMarket(half mathx.Vec) {
	z := half.Z - 3.2
	n := int(half.X * 2 / 5)
	for i := range n {
		x := (float32(i) - float32(n-1)/2) * 5
		rl.DrawCube(vec3(x, 0.55, z), 3, 1.1, 1.6, colStallWood)
		for k := range 3 {
			col := colAwningRed
			if (i+k)%2 == 1 {
				col = colAwningPale
			}
			rl.DrawCube(vec3(x+float32(k-1), 2.35, z), 1, 0.08, 2.4, col)
		}
		for j, dx := range [2]float32{-1.4, 1.4} {
			rl.DrawCube(vec3(x+dx, 1.2, z-1), 0.08, 2.4, 0.08, colMast)
			bulb(x+dx, 2.3, z-1.2, eventBulbs[(i+j)%4])
		}
	}
	// The lights are strung between two masts, sagging between them.
	for k := 0; float32(k)*2 <= half.X*2; k++ {
		x := -half.X + float32(k)*2
		u := x / half.X
		sag := 1.2 * (1 - u*u)
		bulb(x, 4.6-sag, z-2.4, eventBulbs[k%4])
	}
	for _, x := range [2]float32{-half.X, half.X} {
		rl.DrawCube(vec3(x, 2.4, z-2.4), 0.12, 4.8, 0.12, colMast)
	}
}

// drawFloodlights draws the four floodlight masts of a football ground at the
// corners of a block, each with a bank of lamps, and a string of lamps along the
// near side.
func drawFloodlights(half mathx.Vec) {
	for _, sx := range [2]float32{-1, 1} {
		for _, sz := range [2]float32{-1, 1} {
			x, z := sx*(half.X-3), sz*(half.Z-3)
			rl.DrawCube(vec3(x, 7, z), 0.3, 14, 0.3, colMast)
			rl.DrawCube(vec3(x, 14.4, z), 2.4, 1.4, 0.4, as(colFlood, matGlow))
		}
	}
	for k := 0; float32(k)*3 <= half.X*2-6; k++ {
		bulb(-half.X+3+float32(k)*3, 3, half.Z-3, eventBulbs[k%4])
	}
}

// drawMeet draws the show cars of a street meet where they stand on the ground.
func (g *Game) drawMeet(e night.Event) {
	for i, b := range e.Cars {
		g.worksAt(b.Center, b.Heading)
		paint := eventPaints[(i+e.Cell[0]+e.Cell[1]+8)%len(eventPaints)]
		rl.DrawCube(vec3(0, 0.22, 0), b.HalfL*2, 0.1, b.HalfW*2-0.3, as(colUnderglow, matGlow))
		rl.DrawCube(vec3(0, 0.55, 0), b.HalfL*2, 0.6, b.HalfW*2, as(paint, matPaint))
		rl.DrawCube(vec3(-0.2, 1.05, 0), b.HalfL, 0.45, b.HalfW*2-0.3, colBodyShadow)
		for _, dz := range [2]float32{-0.6, 0.6} {
			rl.DrawCube(vec3(b.HalfL, 0.62, dz), 0.05, 0.14, 0.3, as(colFlood, matGlow))
		}
		rl.PopMatrix()
	}
}

// wheelTurn is how long a ferris wheel takes to go round, in seconds.
const wheelTurn = 40

// drawFair draws a ferris wheel at the middle of a block, turning with the
// clock, with a lit gondola on each of its spokes.
func drawFair(half mathx.Vec, now float32) {
	r := min(11, half.Z*0.6)
	hub := r + 2
	const spokes = 8
	rl.DrawCube(vec3(0, hub/2, -1.2), 0.3, hub, 0.3, colMast)
	rl.DrawCube(vec3(0, hub/2, 1.2), 0.3, hub, 0.3, colMast)
	rot := float64(now) / wheelTurn * 2 * math.Pi
	for i := range spokes {
		a := rot + float64(i)/spokes*2*math.Pi
		s, c := float32(math.Sin(a)), float32(math.Cos(a))
		for _, z := range [2]float32{-1, 1} {
			for f := float32(0.25); f <= 1; f += 0.25 {
				rl.DrawCube(vec3(c*r*f, hub+s*r*f, z), 0.14, 0.14, 0.14, colWheel)
			}
		}
		// Gondolas hang from the rim and stay upright.
		rl.DrawCube(vec3(c*r, hub+s*r-0.7, 0), 0.9, 0.9, 2.4, as(eventBulbs[i%4], matGlow))
	}
}
