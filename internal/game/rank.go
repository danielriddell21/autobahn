package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/taxi"
)

// The colours of a taxi rank and of the pick-up marker, kept off the vision
// system's and with no channel at full. Nothing here is annotated, and nothing
// is stored: a rank is drawn from where its kerb slot is, as the trees are.
var (
	colRankBlue   = rl.NewColor(40, 78, 152, 255)
	colRankFace   = rl.NewColor(222, 220, 204, 255)
	colRankBay    = rl.NewColor(204, 176, 62, 255)
	colPickupMark = rl.NewColor(226, 190, 64, 255)
)

// rankColours is every colour the ranks and the marker are drawn in.
func rankColours() []rl.Color {
	return []rl.Color{colRankBlue, colRankFace, colRankBay, colPickupMark}
}

// Rank geometry, in metres.
const (
	// rankHalf is half the length of a rank's bay, and rankDepth its width.
	rankHalf  float32 = 6
	rankDepth float32 = 2.2
)

// drawTaxi draws the ranks near the eye, and the marker at the pick-up of the
// taxi job under way. It is called from the scene, where the emergency pack's
// sites are drawn.
func (g *Game) drawTaxi(eye mathx.Vec) {
	ix := g.taxi.ix
	if ix == nil {
		return
	}
	ix.Sync()
	var mark *job.Goal
	if s := g.ActiveJob(); s != nil && s.Next == 0 && len(s.Goals) > 0 && isTaxiJob(s.Type) {
		mark = &s.Goals[0]
	}
	for _, sp := range ix.Spots() {
		if sp.Pos.DistTo(eye) > cullProps*1.6 {
			continue
		}
		if sp.Rank {
			g.drawRank(sp)
		}
		if mark != nil && sp.Pos == mark.Pos {
			g.drawPickupMark(sp)
		}
	}
}

func isTaxiJob(id string) bool {
	return id == taxi.FareID || id == taxi.RiskyID || id == taxi.CourierID
}

// drawRank draws a taxi rank: a yellow bay along the kerb and a blue sign on a
// post on the pavement beside it.
func (g *Game) drawRank(sp taxi.Spot) {
	fwd := mathx.FromAngle(sp.Heading)
	out := sp.Kerb.Sub(sp.Pos) // from the lane to the pavement
	if out.LenSq() < 1e-6 {
		return
	}
	side := out.Norm()
	edge := sp.Pos.Add(out.Sub(side.Mul(1.6))) // the kerb itself
	in := edge.Sub(side.Mul(rankDepth))
	groundQuad(g.land,
		in.Sub(fwd.Mul(rankHalf)), in.Add(fwd.Mul(rankHalf)),
		edge.Add(fwd.Mul(rankHalf)), edge.Sub(fwd.Mul(rankHalf)), 0.04, colRankBay)
	g.worksAt(sp.Kerb.Add(side.Mul(0.6)), sp.Heading)
	drawRankSign()
	rl.PopMatrix()
}

// drawRankSign draws the sign at the origin, +X along the road: a blue board
// with a white T on a post.
func drawRankSign() {
	rl.DrawCube(vec3(0, 1.2, 0), 0.1, 2.4, 0.1, colPole)
	rl.DrawCube(vec3(0, 2.3, 0), 0.08, 0.9, 0.9, colRankBlue)
	rl.DrawCube(vec3(-0.05, 2.5, 0), 0.04, 0.16, 0.6, colRankFace)
	rl.DrawCube(vec3(-0.05, 2.2, 0), 0.04, 0.5, 0.16, colRankFace)
}

// drawPickupMark marks the pick-up of the job under way on the pavement: a flat
// yellow disc and a flag over it.
func (g *Game) drawPickupMark(sp taxi.Spot) {
	y := g.land.at(sp.Kerb)
	pillar(sp.Kerb, y+0.03, y+0.08, 1.1, 1.1, 20, colPickupMark)
	pillar(sp.Kerb, y, y+2.2, 0.04, 0.04, 6, colPole)
	g.worksAt(sp.Kerb, sp.Heading)
	rl.DrawCube(vec3(0, 2.0, 0.28), 0.04, 0.34, 0.56, colPickupMark)
	rl.PopMatrix()
}
