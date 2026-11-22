package game

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/emergency"
	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/pack"
)

// emergencyContent is the emergency pack's hand-over to the game. This file
// belongs to that pack.
//
// Its jobs are the three call-outs of package emergency. Its dressing is the
// sign at the kerb of each hospital, station and fire site, and the cones, and
// hose, of the scene a call-out is at, drawn by [Game.drawEmergency] from the
// render. Its siren is the one the police use: updateAudio plays it for any
// vehicle with its blue lights on.
func emergencyContent() packContent {
	return packContent{
		ID: pack.Emergency,
		Jobs: func() {
			for _, t := range emergency.All() {
				job.Register(t)
			}
		},
		Dressing: func(g *Game) {
			g.sites = emergency.NewIndex(g.world.City)
		},
	}
}

// The colours the sites are drawn in, kept off the vision system's, and with no
// channel at full.
var (
	colSiteHospital = rl.NewColor(222, 224, 226, 255)
	colSiteCross    = rl.NewColor(196, 44, 44, 255)
	colSiteStation  = rl.NewColor(52, 82, 160, 255)
	colSiteFire     = rl.NewColor(196, 40, 36, 255)
	colSiteSpray    = rl.NewColor(176, 206, 232, 255)
)

// siteColours is every colour the sites are drawn in.
func siteColours() []rl.Color {
	return []rl.Color{colSiteHospital, colSiteCross, colSiteStation, colSiteFire, colSiteSpray}
}

// siteBoard is how far from the middle of the road a site's sign stands.
const siteSetBack float32 = 3.2

// drawEmergency draws the sites near the eye, and the scene of the call-out
// under way: cones round it, and a hose to the fire once the engine is holding.
func (g *Game) drawEmergency(eye mathx.Vec) {
	if g.sites == nil {
		return
	}
	g.sites.Sync()
	for _, s := range g.sites.Sites() {
		if s.Door.DistTo(eye) > cullProps*1.6 {
			continue
		}
		fwd := mathx.FromAngle(s.Heading)
		at := s.Door.Add(fwd.Right().Mul(siteSetBack))
		g.worksAt(at, s.Heading)
		drawSiteSign(s.Kind)
		rl.PopMatrix()
	}
	st := g.ActiveJob()
	if st == nil || st.Next >= len(st.Goals) {
		return
	}
	if st.Type != emergency.FireID && st.Type != emergency.ResponseID {
		return
	}
	goal := st.Goals[st.Next]
	if goal.Pos.DistTo(eye) > cullProps*2 {
		return
	}
	const cones = 8
	for i := range cones {
		a := float32(i) / cones * 2 * math.Pi
		at := goal.Pos.Add(mathx.FromAngle(a).Mul(goal.Radius))
		g.worksAt(at, a)
		drawCone()
		rl.PopMatrix()
	}
	if prog, hosing := emergency.Hosing(*st, g.world.Time); hosing {
		g.drawSpray(goal.Pos, prog)
	}
}

// drawSiteSign draws a sign on a post at the origin, +X along the road: a white
// board with a red cross for a hospital, a blue one for a station and a red one
// for the site of a fire.
func drawSiteSign(k emergency.Kind) {
	rl.DrawCube(vec3(0, 1.3, 0), 0.12, 2.6, 0.12, colPole)
	face, mark := colSiteHospital, colSiteCross
	switch k {
	case emergency.Station:
		face, mark = colSiteStation, colSiteHospital
	case emergency.FireSite:
		face, mark = colSiteFire, colSiteHospital
	}
	rl.DrawCube(vec3(0, 2.4, 0), 0.08, 1.2, 1.6, face)
	rl.DrawCube(vec3(-0.06, 2.4, 0), 0.04, 0.8, 0.22, mark)
	rl.DrawCube(vec3(-0.06, 2.4, 0), 0.04, 0.22, 0.8, mark)
}

// drawSpray draws the hose of a fire engine on the scene as a string of drops
// thrown from the player's car at the fire, thicker as the hold goes on.
func (g *Game) drawSpray(at mathx.Vec, progress float32) {
	p := g.world.Player
	from := p.Pos.Add(p.Forward().Mul(p.Spec.HalfLength))
	const drops = 14
	for i := range drops {
		t := float32(i) / (drops - 1)
		pos := from.Lerp(at, t)
		y := 2.4 + 3.2*4*t*(1-t) // an arc
		size := 0.16 + 0.22*progress
		g.pushUp(pos)
		rl.DrawCube(vec3(pos.X, y, pos.Z), size, size, size, colSiteSpray)
		rl.PopMatrix()
	}
}
