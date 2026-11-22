package game

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// EscapeTime is how long the runner must stay free to win, in seconds.
const EscapeTime float32 = 120

// Chase is the state of a two-player pursuit, whichever end of the wire it is
// being watched from.
type Chase struct {
	// Elapsed is how long the runner has stayed free, in seconds.
	Elapsed float32
	// Over is set once one side has won, and Winner says which.
	Over   bool
	Winner string
}

func (g *Game) runnerControls(dt float32) sim.Controls {
	// The runner is the autopilot in a chase against the machine, and a person
	// at the host's keyboard when two people are playing.
	if !g.auto {
		return g.manualControls()
	}
	g.perceive(dt)
	cmd := g.driver.Drive(g.dets, g.world.Player.Speed(), dt)
	g.lastCmd = cmd
	return sim.Controls{Throttle: cmd.Throttle, Brake: cmd.Brake, Steer: cmd.Steer}
}

func (g *Game) driveChase(runner sim.Controls, dt float32) {
	// Advances the authoritative simulation, and publishes it when somebody
	// else is watching. The police car is driven by whoever is here: the person
	// at this keyboard, or the one who joined over the network.
	switch {
	case !g.hosting() && g.chaser != nil:
		g.chaser.Manual = true
		g.chaser.SetInput(g.chaserControls())
	case g.hosting():
		g.applyRemote()
	}

	if !g.chase.Over {
		g.world.Update(runner, dt)
		g.chase.Elapsed += dt
		g.judgeChase()
	}
	if g.hosting() {
		g.publish()
	}
}

func (g *Game) applyRemote() {
	if in, joined := g.net.Input(); joined && g.chaser != nil {
		g.chaser.SetInput(sim.Controls{
			Throttle: in.Throttle, Brake: in.Brake, Steer: in.Steer,
			Handbrake: in.Handbrake, Reverse: in.Reverse,
		})
		g.chaser.Manual = true
	} else if g.chaser != nil {
		// Nobody is driving it, so hand it back to the simulation rather than
		// leaving a police car sitting in the road.
		g.chaser.Manual = false
	}
}

func (g *Game) judgeChase() {
	switch {
	case g.world.Caught() || g.world.Wanted.State == sim.PursuitStopped:
		g.chase.Over, g.chase.Winner = true, "POLICE"
	case g.chase.Elapsed >= EscapeTime:
		g.chase.Over, g.chase.Winner = true, "RUNNER"
	}
}

func (g *Game) joinChase(dt float32) {
	// Sends this player's controls and adopts the world the host sends
	// back. Nothing is simulated here beyond the cameras.
	in := g.chaserControls()
	g.net.SendInput(Input{
		Throttle: in.Throttle, Brake: in.Brake, Steer: in.Steer,
		Handbrake: in.Handbrake, Reverse: in.Reverse,
	})

	if s, ok := g.net.Snapshot(); ok {
		g.adopt(s, dt)
	}
	// Nothing here runs the simulation, so this is the only thing that extends
	// the city as the chase drives off the ground it started on.
	g.world.EnsureBuilt(g.chaseSubject().Pos)
}

func (g *Game) chaserControls() sim.Controls {
	// Reads the police unit's keys. The joining player drives with
	// the same layout the runner uses, because they are at their own keyboard.
	var c sim.Controls
	if rl.IsKeyDown(rl.KeyW) || rl.IsKeyDown(rl.KeyUp) {
		c.Throttle = 1
	}
	if rl.IsKeyDown(rl.KeyA) || rl.IsKeyDown(rl.KeyLeft) {
		c.Steer = -1
	}
	if rl.IsKeyDown(rl.KeyD) || rl.IsKeyDown(rl.KeyRight) {
		c.Steer = 1
	}
	if rl.IsKeyDown(rl.KeyS) || rl.IsKeyDown(rl.KeyDown) {
		if g.chaser != nil && g.chaser.V.ForwardSpeed() < 0.6 {
			c.Reverse, c.Throttle = true, 1
		} else {
			c.Brake = 1
		}
	}
	c.Handbrake = rl.IsKeyDown(rl.KeySpace)
	return c
}

func (g *Game) chaseSubject() *sim.Vehicle {
	// Is the car this end of the connection is watching: the runner
	// when hosting, the police unit when joined.
	// Whoever is at this keyboard is in the police car, unless they are the
	// runner at the host of a two-player game.
	if g.chaser != nil && !g.hosting() {
		return g.chaser.V
	}
	return g.world.Player
}

func (g *Game) drawChaseHUD() {
	left := max(EscapeTime-g.chase.Elapsed, 0)
	gap := float32(0)
	if g.chaser != nil {
		gap = g.chaser.V.Pos.DistTo(g.world.Player.Pos)
	}

	w := int32(300)
	x := int32(g.opts.Width)/2 - w/2
	panel(x, 8, w, 58)

	role, col := "RUNNER", hudAccent
	if !g.hosting() {
		role, col = "POLICE", hudBad
	}
	rl.DrawText(role, x+16, 14, 14, col)
	rl.DrawText(fmt.Sprintf("%d:%02d", int(left)/60, int(left)%60), x+16, 30, 26, hudText)
	rl.DrawText("to escape", x+96, 40, 11, hudDim)

	gapCol := hudGood
	switch {
	case gap < 20:
		gapCol = hudBad
	case gap < 60:
		gapCol = hudWarn
	}
	rl.DrawText(fmt.Sprintf("%3.0f m", gap), x+w-92, 30, 26, gapCol)
	rl.DrawText("gap", x+w-92, 16, 11, hudDim)

	g.drawLinkState()
	if g.chase.Over {
		g.drawChaseResult()
	}
}

func (g *Game) drawLinkState() {
	if !g.net.Online() {
		return
	}
	// crucible's session already words this for a player to read; all that is
	// left is deciding how alarming it looks.
	text := g.net.Status()
	col := hudWarn
	switch {
	case g.net.Err() != nil:
		col = hudBad
	case g.net.Peered():
		col = hudGood
	}
	w := rl.MeasureText(text, 12) + 28
	x := int32(g.opts.Width)/2 - w/2
	panel(x, 72, w, 26)
	rl.DrawText(text, x+14, 79, 12, col)
}

func (g *Game) drawChaseResult() {
	w, h := int32(420), int32(96)
	x := int32(g.opts.Width)/2 - w/2
	y := int32(g.opts.Height)/2 - h/2
	panel(x, y, w, h)

	col, line := hudAccent, "the runner got away"
	if g.chase.Winner == "POLICE" {
		col, line = hudBad, "the runner was stopped"
	}
	rl.DrawText(g.chase.Winner+" WIN", x+24, y+18, 32, col)
	rl.DrawText(line, x+24, y+56, 14, hudDim)
}

func (g *Game) chaseCamera(dt float32) {
	// Trails whichever car this end is driving.
	v := g.chaseSubject()
	fwd := v.Forward()
	want := v.Pos.Sub(fwd.Mul(9.2 + min(v.Speed()*0.14, 3.6)))
	if dt <= 0 {
		g.camPos = want
	} else {
		g.camPos = mathx.V(
			mathx.Approach(g.camPos.X, want.X, 7, dt),
			mathx.Approach(g.camPos.Z, want.Z, 7, dt),
		)
	}
	g.cam.Fovy = g.speedLens(v, dt)
	g.cam.Position = vec3(g.camPos.X, 4.3, g.camPos.Z)
	ahead := v.Pos.Add(fwd.Mul(9))
	g.cam.Target = vec3(ahead.X, 1.0, ahead.Z)
}
