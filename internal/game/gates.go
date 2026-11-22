package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/race"
)

// The colours the checkpoint gates are drawn in, kept off the vision system's
// and with no channel at full. The gate to take next is amber and the ones
// after it chequered; none is annotated, since the simulation sees a car cross
// the line and the autopilot is told nothing.
var (
	colGatePost  = rl.NewColor(150, 152, 156, 255)
	colGatePale  = rl.NewColor(218, 220, 222, 255)
	colGateDark  = rl.NewColor(36, 38, 42, 255)
	colGateAmber = rl.NewColor(214, 150, 40, 255)
)

// gateColours is every colour the gates are drawn in.
func gateColours() []rl.Color {
	return []rl.Color{colGatePost, colGatePale, colGateDark, colGateAmber}
}

// Gate sizes, in metres.
const (
	gateHeight float32 = 5.2 // clear of a bus
	gatePost   float32 = 0.3
	gateAhead          = 3 // how many gates are drawn, the next and the ones after
)

// isRace reports whether a job is one of the race types, which run over gates.
func isRace(t string) bool {
	return t == race.TrialID || t == race.CircuitID || t == race.SprintID
}

// drawGates draws the next few checkpoint gates of a race under way. Each is
// worked out from where it stands: the heading of the street at its goal, found
// on the ground it lies on, and standing on the land there.
func (g *Game) drawGates(eye mathx.Vec) {
	s := g.ActiveJob()
	if s == nil || !isRace(s.Type) {
		return
	}
	for i := s.Next; i < min(s.Next+gateAhead, len(s.Goals)); i++ {
		goal := s.Goals[i]
		if goal.Pos.DistTo(eye) > cullProps*2 {
			continue
		}
		g.drawGate(goal, i == s.Next)
	}
}

// drawGate draws one gate of a job: two posts and a banner across the street.
func (g *Game) drawGate(goal job.Goal, next bool) {
	heading := float32(0)
	if p := g.world.City.Project(goal.Pos); p.Valid {
		heading = p.Lane.HeadingAt(p.S)
	}
	g.worksAt(goal.Pos, heading)
	defer rl.PopMatrix()
	half := goal.Radius
	for _, z := range []float32{-half, half} {
		rl.DrawCube(vec3(0, gateHeight/2, z), gatePost, gateHeight, gatePost, colGatePost)
	}
	if next {
		rl.DrawCube(vec3(0, gateHeight+0.2, 0), 0.5, 0.7, half*2, colGateAmber)
		return
	}
	const squares = 8
	width := half * 2 / squares
	for i := range squares {
		col := colGatePale
		if i%2 == 1 {
			col = colGateDark
		}
		rl.DrawCube(vec3(0, gateHeight+0.2, -half+width*(float32(i)+0.5)), 0.5, 0.7, width, col)
	}
}
