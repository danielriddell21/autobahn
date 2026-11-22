package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/works"
)

// Roadworks are drawn the way the rest of the street furniture is: through the
// scene's shader, standing on the ground where it is, with no material of its
// own. Package works says where each cone, barrier and sign is, on the ground,
// and when; this only gives them a shape and a colour.
//
// None of the colours below may be brighter than it is written, and none may
// have a channel at full: the vision system's colours all do, which is what
// keeps them out of the scene, and the shader holds everything just short of
// it. works_test.go holds them to that.
var (
	colCone      = rl.NewColor(226, 112, 30, 255)
	colConeBand  = rl.NewColor(222, 224, 216, 255)
	colConeBase  = rl.NewColor(58, 58, 60, 255)
	colBoardRed  = rl.NewColor(190, 48, 42, 255)
	colBoardPale = rl.NewColor(222, 220, 210, 255)
	colWarnPlate = rl.NewColor(226, 176, 40, 255)
	colWarnInk   = rl.NewColor(40, 40, 44, 255)
)

// worksColours is every colour roadworks are drawn in.
func worksColours() []rl.Color {
	return []rl.Color{
		colCone, colConeBand, colConeBase, colBoardRed, colBoardPale, colWarnPlate, colWarnInk,
	}
}

// drawWorks draws the signs of every closure that has put them up, and the
// cones and barriers of every one that has put them in the road.
func (g *Game) drawWorks(eye mathx.Vec) {
	for _, cl := range g.world.Works.Active() {
		for _, s := range cl.Signs {
			if s.Pos.DistTo(eye) > cullProps {
				continue
			}
			g.worksAt(s.Pos, s.Heading)
			drawWorksSign(s)
			rl.PopMatrix()
		}
		if cl.Phase != works.PhaseSolid {
			continue
		}
		for _, c := range cl.Cones {
			if c.Center.DistTo(eye) > cullProps {
				continue
			}
			g.worksAt(c.Center, c.Heading)
			drawCone()
			rl.PopMatrix()
		}
		for _, b := range cl.Barriers {
			if b.Center.DistTo(eye) > cullProps {
				continue
			}
			g.worksAt(b.Center, b.Heading)
			drawBarrier(b.HalfW)
			rl.PopMatrix()
		}
	}
}

// worksAt pushes a matrix that stands at a point of the ground with +X the way
// the heading points. The caller pops it.
func (g *Game) worksAt(at mathx.Vec, heading float32) {
	g.pushUp(at)
	rl.Translatef(at.X, 0, at.Z)
	rl.Rotatef(-heading*rad2deg, 0, 1, 0)
}

// drawCone draws a traffic cone on its square base, at the origin.
func drawCone() {
	rl.DrawCube(vec3(0, 0.025, 0), 0.4, 0.05, 0.4, colConeBase)
	rl.DrawCube(vec3(0, 0.21, 0), 0.26, 0.32, 0.26, colCone)
	rl.DrawCube(vec3(0, 0.44, 0), 0.19, 0.16, 0.19, colConeBand)
	rl.DrawCube(vec3(0, 0.6, 0), 0.12, 0.2, 0.12, colCone)
}

// drawBarrier draws a striped board across a lane, half across each way, on two
// legs, at the origin.
func drawBarrier(half float32) {
	const stripes = 5
	width := half * 2 / stripes
	for i := range stripes {
		col := colBoardRed
		if i%2 == 1 {
			col = colBoardPale
		}
		z := -half + width*(float32(i)+0.5)
		rl.DrawCube(vec3(0, 0.78, z), 0.1, 0.32, width, col)
	}
	for _, z := range [2]float32{-half + 0.2, half - 0.2} {
		rl.DrawCube(vec3(0, 0.34, z), 0.1, 0.68, 0.1, colPole)
	}
}

// drawWorksSign draws a roadworks sign on its post, facing back along the
// heading of the traffic it is for, at the origin.
func drawWorksSign(s works.Sign) {
	const face = works.SignHeight - 0.4
	rl.DrawCube(vec3(0, (works.SignHeight-0.5)/2, 0), 0.1, works.SignHeight-0.5, 0.1, colPole)
	switch s.Kind {
	case works.SignClosed:
		// A red disc, near enough, barred across in white.
		rl.DrawCube(vec3(-0.06, face, 0), 0.06, 0.95, 0.95, colBoardRed)
		rl.DrawCube(vec3(-0.1, face, 0), 0.04, 0.2, 0.75, colBoardPale)
	default:
		// A yellow plate with a dark arrow bar across it.
		rl.DrawCube(vec3(-0.06, face, 0), 0.06, 1.0, 1.0, colWarnPlate)
		rl.DrawCube(vec3(-0.1, face, 0), 0.04, 0.16, 0.7, colWarnInk)
		rl.DrawCube(vec3(-0.1, face+0.22, 0), 0.04, 0.2, 0.2, colWarnInk)
	}
}
