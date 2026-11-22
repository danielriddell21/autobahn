package game

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// The goal marker's colours. Neither has a channel at 255 and neither is one
// of the vision system's reserved colours, which are all made of 0, 128 and 255.
var (
	jobBeam = as(rl.NewColor(236, 186, 60, 255), matGlow)
	jobBase = as(rl.NewColor(214, 150, 40, 255), matGlow)
	jobPip  = rl.NewColor(236, 186, 60, 255)
)

// drawJob draws the job's readout and its pip on the minimap, and the marker
// in the world. Nothing here is annotated: the autopilot is told nothing.
func (g *Game) drawJob() {
	s := g.jobs.board.Active()
	if s == nil {
		return
	}
	g.drawJobMarker(s)
	left := max(s.Deadline-g.world.Time, 0)
	pay := fmt.Sprintf("pays %s", money(job.Worth(*s)))
	if s.Assisted || (g.auto && g.opts.AutopilotJobs) {
		pay = "unpaid: autopilot"
	}
	name := "JOB"
	if t, ok := job.Lookup(s.Type); ok {
		name = t.Name()
	}
	col := hudText
	if left < 15 {
		col = hudBad
	}
	x, y := int32(12), int32(60)
	panel(x, y, 232, 66)
	rl.DrawText(name, x+12, y+6, 14, hudAccent)
	rl.DrawText(fmt.Sprintf("%d:%02d", int(left)/60, int(left)%60), x+12, y+22, 28, col)
	rl.DrawText(pay, x+12, y+50, 12, hudDim)
	if len(s.Goals) > 0 && s.Next < len(s.Goals) {
		rl.DrawText(fmt.Sprintf("%.0f m", g.world.Player.Pos.DistTo(s.Goals[s.Next].Pos)),
			x+140, y+30, 20, hudDim)
	}
}

// drawJobMarker raises a beam over the goal, through the shader like the rest
// of the scene. The depth the world left behind hides it behind buildings.
func (g *Game) drawJobMarker(s *job.State) {
	if s.Next >= len(s.Goals) {
		return
	}
	goal := s.Goals[s.Next]
	y := g.land.at(goal.Pos)
	near, far := clipFor(g.cam.Position.Y)
	rl.SetClipPlanes(float64(near), float64(far))
	defer rl.SetClipPlanes(float64(clipNear), float64(clipFar))
	rl.BeginMode3D(g.cam)
	rl.DisableBackfaceCulling()
	if g.scene.ok {
		rl.BeginShaderMode(g.scene.shader)
	}
	pillar(goal.Pos, y, y+0.15, goal.Radius, goal.Radius, 24, jobBase)
	pillar(goal.Pos, y, y+60, 1.2, 0.4, 12, jobBeam)
	if g.scene.ok {
		rl.EndShaderMode()
	}
	rl.EnableBackfaceCulling()
	rl.EndMode3D()
}

// drawJobPip puts the goal on the minimap, held to its edge when it is further
// than the map shows.
func (g *Game) drawJobPip(cx, cy int32, half, scale float32) {
	s := g.jobs.board.Active()
	if s == nil || s.Next >= len(s.Goals) {
		return
	}
	d := s.Goals[s.Next].Pos.Sub(g.world.Player.Pos).Mul(scale)
	if m := max(mathx.Abs(d.X), mathx.Abs(d.Z)); m > half-4 {
		d = d.Mul((half - 4) / m)
	}
	rl.DrawCircle(cx+int32(d.X), cy+int32(d.Z), 4, jobPip)
	rl.DrawCircleLines(cx+int32(d.X), cy+int32(d.Z), 4, hudText)
}
