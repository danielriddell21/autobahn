package game

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/netplay"
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

func (g *Game) hostChase(runner sim.Controls, dt float32) {
	// Advances the authoritative simulation and publishes it. The host
	// owns the world; the joining player only ever contributes the controls of the
	// unit they are driving.
	if in, joined := g.host.Input(); joined && g.chaser != nil {
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

	if !g.paused && !g.chase.Over {
		g.world.Update(runner, dt)
		g.chase.Elapsed += dt
		g.judgeChase()
	}
	g.publish()
}

func (g *Game) judgeChase() {
	switch {
	case g.world.Caught() || g.world.Wanted.State == sim.PursuitStopped:
		g.chase.Over, g.chase.Winner = true, "POLICE"
	case g.chase.Elapsed >= EscapeTime:
		g.chase.Over, g.chase.Winner = true, "RUNNER"
	}
}

func (g *Game) publish() {
	// Snapshots go out on their own clock, well below the frame rate: the
	// chase does not need sixty updates a second to read correctly.
	g.sinceSnapshot += g.lastDT
	if g.sinceSnapshot < 1.0/netplay.SnapshotRate {
		return
	}
	g.sinceSnapshot = 0

	s := netplay.Snapshot{
		Clock:  g.world.Time,
		Runner: poseOf(g.world.Player, false),
		Agents: make([]netplay.Pose, 0, len(g.world.Agents)),
		Chaser: -1,

		WantedLevel: g.world.Wanted.Level,
		WantedState: int(g.world.Wanted.State),
		Elapsed:     g.chase.Elapsed,
		Over:        g.chase.Over,
		Winner:      g.chase.Winner,
	}
	for i, a := range g.world.Agents {
		s.Agents = append(s.Agents, poseOf(a.V, a.Pursuing()))
		if a == g.chaser {
			s.Chaser = i
		}
	}
	g.host.Send(s)
}

func poseOf(v *sim.Vehicle, pursuing bool) netplay.Pose {
	var flags uint8
	if pursuing {
		flags |= netplay.FlagPursuing
	}
	if v.Brake > 0.05 {
		flags |= netplay.FlagBraking
	}
	return netplay.Pose{
		X: v.Pos.X, Z: v.Pos.Z, Yaw: v.Yaw, Speed: v.Speed(), Flags: flags,
	}
}

func (g *Game) joinChase(dt float32) {
	// Sends this player's controls and adopts the world the host sends
	// back. Nothing is simulated here beyond the cameras.
	in := g.chaserControls()
	g.client.Send(netplay.Input{
		Throttle: in.Throttle, Brake: in.Brake, Steer: in.Steer,
		Handbrake: in.Handbrake, Reverse: in.Reverse,
	})

	s, ok := g.client.Snapshot()
	if !ok {
		return
	}
	g.adopt(s)
	_ = dt
}

func (g *Game) adopt(s netplay.Snapshot) {
	// The city is identical at both ends because it came from the same seed, so
	// only the moving parts are taken from the host.
	g.world.Signals.SetClock(s.Clock)
	g.world.Time = s.Clock
	sim.ApplyPose(g.world.Player, s.Runner.X, s.Runner.Z, s.Runner.Yaw, s.Runner.Speed)

	for i, p := range s.Agents {
		if i >= len(g.world.Agents) {
			break
		}
		a := g.world.Agents[i]
		sim.ApplyPose(a.V, p.X, p.Z, p.Yaw, p.Speed)
		a.SetPursuing(p.Flags&netplay.FlagPursuing != 0)
	}
	if s.Chaser >= 0 && s.Chaser < len(g.world.Agents) {
		g.chaser = g.world.Agents[s.Chaser]
	}

	g.world.Wanted.Level = s.WantedLevel
	g.world.Wanted.State = sim.PursuitState(s.WantedState)
	g.chase.Elapsed, g.chase.Over, g.chase.Winner = s.Elapsed, s.Over, s.Winner
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
	if g.client != nil && g.chaser != nil {
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
	if g.client != nil {
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
	var text string
	var col rl.Color

	switch {
	case g.host != nil && !g.host.Joined():
		text, col = "waiting for a player on "+g.host.Addr(), hudWarn
	case g.host != nil:
		text, col = "player connected", hudGood
	case g.client != nil && g.client.Err() != nil:
		text, col = "connection lost: "+g.client.Err().Error(), hudBad
	case g.client != nil:
		if _, ok := g.client.Snapshot(); !ok {
			text, col = "waiting for the host", hudWarn
		}
	}
	if text == "" {
		return
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
	g.cam.Position = vec3(g.camPos.X, 4.3, g.camPos.Z)
	ahead := v.Pos.Add(fwd.Mul(9))
	g.cam.Target = vec3(ahead.X, 1.0, ahead.Z)
}
