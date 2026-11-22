package game

import (
	"errors"
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"

	crucihud "github.com/danielriddell21/crucible/hud"
	"github.com/danielriddell21/crucible/netplay"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// Co-op: two people on one team, against the same police.
//
// It is driven like an ordinary game. Each player sees the world from their
// own car through the usual cameras, with the usual speedometer and minimap, so
// the only things this file adds are where the controls come from, how the two
// ends agree on the world, and what is drawn for the partner.
//
// The host simulates, for both players. In two cars the joiner's car is an
// agent the simulation lets a person drive; the joiner sees the world with its
// own car and the host's swapped (see Game.adopt) and sends controls. In one car
// there is a single vehicle, which takes the host's steering and the joiner's
// pedals; see split.

// errCoopAutopilot is why an autopilot cannot be asked for in a shared car.
var errCoopAutopilot = errors.New("the autopilot cannot drive a car two people share")

// coopValid reports why the options cannot be played, or nil. A shared car
// has two people at its controls and nothing the autopilot could take over.
func (o Options) coopValid() error {
	if o.Play.SharedCar() && o.Autopilot {
		return errCoopAutopilot
	}
	return nil
}

// Coop reports whether this is one end of a game where the two players are on
// the same side.
func (g *Game) Coop() bool { return g.Networked() && g.mode.Coop() }

// Mode is what the two players are doing: chase, or one of the co-op games.
func (g *Game) Mode() NetMode { return g.mode }

// joining reports whether this end draws what it is told rather than
// simulating.
func (g *Game) joining() bool { return g.net.Role() == netplay.Joining }

// autoBlocked reports whether the autopilot is not to be used: nobody can take
// the wheel of a car that two people are already driving between them.
func (g *Game) autoBlocked() bool { return g.Networked() && g.mode.SharedCar() }

// seatOf is the seat this end sits in, in a shared car.
func (g *Game) seatOf() Seat {
	if g.joining() {
		return SeatPedals
	}
	return SeatWheel
}

// teamAssisted reports whether either player's autopilot is driving, which is
// what keeps a pursuit from counting toward the packs.
func (g *Game) teamAssisted() bool {
	if g.hosting() {
		return g.auto || g.remoteAuto
	}
	return g.auto || g.assisted
}

// stepCoop runs one frame of a co-op game: the host simulates and publishes,
// the joiner sends controls and draws what comes back.
func (g *Game) stepCoop(dt float32) {
	g.speedSum += g.world.Player.Speed()
	g.peakWanted = max(g.peakWanted, g.world.Wanted.Level)
	g.handleInput()
	g.notices.Tick()
	if g.blinkT += dt; g.blinkT > 0.36 {
		g.blink, g.blinkT = !g.blink, 0
	}
	if !g.joining() {
		g.advanceDay(dt) // the joining player takes the hour from the host
	}
	g.perceive(dt)

	if g.joining() {
		g.joinCoop(dt)
	} else {
		g.driveCoop(dt)
	}

	g.updateEffects(dt)
	g.watchPursuit(dt)
	g.updateCameras(dt)
	g.updateAudio()

	rl.BeginDrawing()
	g.sky(g.opts.Width, g.opts.Height)
	g.drawWorld(g.cam, g.camMode != CameraBonnet)
	if g.showHUD {
		g.drawHUD()
		g.drawCoopHUD()
	}
	rl.EndDrawing()
	g.capture()
}

// ownControls is what this player's own hands, or autopilot, ask of their car.
func (g *Game) ownControls(dt float32) sim.Controls {
	if !g.auto {
		return g.manualControls()
	}
	cmd := g.driver.Drive(g.dets, g.world.Player.Speed(), dt)
	g.lastCmd = cmd
	return sim.Controls{Throttle: cmd.Throttle, Brake: cmd.Brake, Steer: cmd.Steer}
}

// driveCoop advances the authoritative simulation for both players, and
// publishes it.
func (g *Game) driveCoop(dt float32) {
	remote, joined := g.net.Input()
	if !joined {
		remote = Input{}
	}
	g.remoteAuto = joined && remote.Auto

	own := g.ownControls(dt)
	if g.mode.SharedCar() {
		// One car: this player steers, the other works the pedals and the
		// indicators.
		c := split(SeatWheel, inputOf(own, 0, false), settle(remote, g.world.Player.ForwardSpeed()))
		g.indicator = indicatorOf(remote.Indicator)
		g.world.Update(c, dt)
	} else {
		if p := g.world.Partner(); p != nil {
			p.SetInput(controlsOf(settle(remote, p.V.ForwardSpeed())))
			p.SetIndicator(indicatorOf(remote.Indicator))
		}
		g.world.Update(own, dt)
	}
	g.publish()
}

// joinCoop sends this player's controls and adopts the world the host sends
// back. Nothing is simulated here beyond the cameras.
func (g *Game) joinCoop(dt float32) {
	g.net.SendInput(g.seatInput(dt))
	if s, ok := g.net.Snapshot(); ok {
		g.adopt(s, dt)
	}
	// Nothing here runs the simulation, so this is what extends the city as
	// the team drives off the ground it started on.
	g.world.EnsureBuilt(g.world.Player.Pos)
	if p := g.world.Partner(); p != nil {
		g.world.EnsureBuilt(p.V.Pos)
	}
}

// seatInput is what the joining player sends: their autopilot's commands, or
// their keys. The brake is sent as a brake: whether it means reverse is for
// the host to say, which knows how fast the car is really going.
func (g *Game) seatInput(dt float32) Input {
	if g.auto {
		c := g.ownControls(dt)
		return inputOf(c, indicatorOfCommand(g.lastCmd), true)
	}
	in := Input{Indicator: int8(g.myIndicator)}
	if rl.IsKeyDown(rl.KeyW) || rl.IsKeyDown(rl.KeyUp) {
		in.Throttle = 1
	}
	if rl.IsKeyDown(rl.KeyS) || rl.IsKeyDown(rl.KeyDown) {
		in.Brake = 1
	}
	in.Boost = rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)
	if !g.mode.SharedCar() {
		// In a car of their own they steer it as well.
		if rl.IsKeyDown(rl.KeyA) || rl.IsKeyDown(rl.KeyLeft) {
			in.Steer = -1
		}
		if rl.IsKeyDown(rl.KeyD) || rl.IsKeyDown(rl.KeyRight) {
			in.Steer = 1
		}
		in.Handbrake = rl.IsKeyDown(rl.KeySpace)
	}
	return in
}

// indicatorOfCommand is the turn signal an autopilot command shows.
func indicatorOfCommand(cmd autopilot.Command) sim.Indicator {
	switch cmd.Indicator {
	case autopilot.IndicateLeft:
		return sim.IndicatorLeft
	case autopilot.IndicateRight:
		return sim.IndicatorRight
	}
	return sim.IndicatorOff
}

// handleIndicatorKeys reads the turn signal keys: Q for left, E for right.
func (g *Game) handleIndicatorKeys() {
	switch {
	case rl.IsKeyPressed(rl.KeyQ):
		g.toggleIndicator(sim.IndicatorLeft)
	case rl.IsKeyPressed(rl.KeyE):
		g.toggleIndicator(sim.IndicatorRight)
	}
}

// toggleAutopilot is the Tab key, which a shared car refuses.
func (g *Game) toggleAutopilot() {
	if g.autoBlocked() {
		g.notices.Post(errCoopAutopilot.Error(), 150, crucihud.Notice)
		return
	}
	g.SetAutopilot(!g.auto)
}

// offerJobUnlessCoop is the J key. Jobs are not yet shared between two
// players, so a co-op game does not offer them.
func (g *Game) offerJobUnlessCoop() {
	if !g.Coop() {
		g.offerJob()
	}
}

// toggleIndicator flips a turn signal on or off, for whichever end of a game
// the player is at. The joiner's is only a request: the host shows it.
func (g *Game) toggleIndicator(want sim.Indicator) {
	set := &g.indicator
	switch {
	case g.joining():
		set = &g.myIndicator
	case g.Coop() && g.mode.SharedCar():
		return // the other player works the indicators of a shared car
	}
	if *set == want {
		want = sim.IndicatorOff
	}
	*set = want
}

// partnerGap is how far the partner's car is from this player's, and whether
// there is one.
func (g *Game) partnerGap() (float32, bool) {
	p := g.world.Partner()
	if p == nil {
		return 0, false
	}
	return p.V.Pos.DistTo(g.world.Player.Pos), true
}

// drawCoopHUD says what the two players are doing and where the other one is.
func (g *Game) drawCoopHUD() {
	if !g.Coop() {
		return
	}
	g.drawPartnerMarker()

	x, y, w := int32(12), int32(58), int32(232)
	panel(x, y, w, 62)
	rl.DrawText(g.mode.String(), x+12, y+7, 14, hudAccent)
	if g.mode.SharedCar() {
		rl.DrawText("SEAT  "+g.seatOf().String(), x+12, y+26, 14, hudText)
	} else if gap, ok := g.partnerGap(); ok {
		col := hudGood
		if gap > 150 {
			col = hudWarn
		}
		rl.DrawText(fmt.Sprintf("PARTNER  %.0f m", gap), x+12, y+26, 14, col)
	}
	col := hudWarn
	switch {
	case g.net.Err() != nil:
		col = hudBad
	case g.net.Peered():
		col = hudGood
	}
	rl.DrawText(g.net.Status(), x+12, y+44, 11, col)
}

// drawPartnerMarker floats a marker over the partner's car, so it can be found
// down a street or across a junction.
func (g *Game) drawPartnerMarker() {
	p := g.world.Partner()
	if p == nil {
		return
	}
	eye := g.cam.Position
	to := rl.Vector3{X: p.V.Pos.X - eye.X, Y: 0, Z: p.V.Pos.Z - eye.Z}
	look := rl.Vector3{X: g.cam.Target.X - eye.X, Y: 0, Z: g.cam.Target.Z - eye.Z}
	if to.X*look.X+to.Z*look.Z <= 0 {
		return // behind the camera
	}
	s := rl.GetWorldToScreen(vec3(p.V.Pos.X, g.land.at(p.V.Pos)+3.2, p.V.Pos.Z), g.cam)
	x, y := int32(s.X), int32(s.Y)
	if x < 0 || y < 0 || x > int32(g.opts.Width) || y > int32(g.opts.Height) {
		return
	}
	label := fmt.Sprintf("PARTNER %.0f m", p.V.Pos.DistTo(g.world.Player.Pos))
	w := rl.MeasureText(label, 12)
	rl.DrawText(label, x-w/2+1, y-18+1, 12, rl.NewColor(0, 0, 0, 160))
	rl.DrawText(label, x-w/2, y-18, 12, hudAccent)
	rl.DrawTriangle(
		rl.NewVector2(float32(x-6), float32(y-4)),
		rl.NewVector2(float32(x), float32(y+6)),
		rl.NewVector2(float32(x+6), float32(y-4)),
		hudAccent,
	)
}
