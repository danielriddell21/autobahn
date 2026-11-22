package game

import (
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/weather"
)

// What the host says and what the joiner does with it, whichever mode the two
// of them are playing. The chase and the co-op games differ in who drives what
// and who wins, not in how the world gets across the wire.

// publish sends the world to the joining player, on its own clock, well below
// the frame rate: nothing here needs sixty updates a second to read correctly.
func (g *Game) publish() {
	g.sinceSnapshot += g.lastDT
	if g.sinceSnapshot < 1.0/SnapshotRate {
		return
	}
	g.sinceSnapshot = 0
	g.net.Send(g.snapshot())
}

// snapshot is the world as the host sees it now.
func (g *Game) snapshot() Snapshot {
	wa := &g.world.Wanted
	runner := poseOf(g.world.Player, false)
	runner.Flags |= indicatorFlags(g.indicator)
	s := Snapshot{
		Proto: Proto, Mode: g.mode,
		Custom: g.customCode(),
		Seed:   g.opts.Seed, Map: int(g.opts.Map), Traffic: g.opts.Traffic, Police: g.opts.Police,
		Car: int(g.opts.Car), Paint: g.opts.Paint,

		Clock:   g.world.Time,
		Hour:    g.clock.hour,
		Weather: uint8(g.world.Climate.Setting),
		Runner:  runner,
		Agents:  make([]Pose, 0, len(g.world.Agents)),
		Chaser:  -1,
		Partner: -1,

		WantedLevel: wa.Level,
		WantedState: int(wa.State),
		Elapsed:     g.chase.Elapsed,
		Over:        g.chase.Over,
		Winner:      g.chase.Winner,

		Bounty: wa.Bounty, Banked: wa.Banked, Stops: wa.Stops, Escapes: wa.Escapes,
		Busting: wa.Busting, Cooldown: wa.Cooldown,
		Assisted: g.teamAssisted(),
	}
	partner := g.world.Partner()
	for i, a := range g.world.Agents {
		p := poseOf(a.V, a.Pursuing())
		switch a {
		case g.chaser:
			s.Chaser = i
		case partner:
			s.Partner = i
			p.Flags |= indicatorFlags(a.Indicator())
		}
		s.Agents = append(s.Agents, p)
	}
	return s
}

// customCode is the share code of the custom map this game is on, or empty.
func (g *Game) customCode() string {
	if g.opts.Custom == nil {
		return ""
	}
	return g.opts.Custom.Encode()
}

// syncMode takes what the two players are doing from the host. A joiner built
// from the lobby already knows it; one started from the command line learns it
// here, and sets up the car it is to drive.
func (g *Game) syncMode(s Snapshot) {
	if g.mode == s.Mode {
		return
	}
	g.mode = s.Mode
	if g.chaser != nil {
		g.chaser.Manual = false // it was never to be driven
		g.chaser = nil
	}
	g.seat()
}

// seat gives the joining player what they drive in the mode being played: a
// police car in a chase, a car of their own in two-car co-op, and nothing at
// all in a shared car, where the one car is the host's.
func (g *Game) seat() {
	switch g.mode {
	case ModeChase:
		g.chaser = g.world.AssignChaser()
	case ModeCoopCars:
		g.world.AssignPartner()
	}
}

// adopt takes the moving parts of the world from the host. The city is
// identical at both ends because it came from the same seed, so nothing else
// travels.
func (g *Game) adopt(s Snapshot, dt float32) {
	g.syncMode(s)
	g.world.Signals.SetClock(s.Clock)
	g.world.Time = s.Clock
	// The host's closures move on with its clock; a joiner never runs Update.
	g.world.SyncWorks()
	g.world.Climate.Setting = weather.Setting(s.Weather)
	g.clock.hour, g.clock.glide = wrapHour(s.Hour), 0
	g.relight()

	// In two-car co-op the joiner sees its own car as the player's, so that
	// the camera, the speedometer, the minimap and the autopilot all work on
	// it unchanged; the host's car is the partner here. The two swap places.
	mine, partner := s.Runner, g.world.Partner()
	swapped := g.mode == ModeCoopCars && partner != nil && s.Partner >= 0 && s.Partner < len(s.Agents)
	if swapped {
		mine = s.Agents[s.Partner]
	}
	g.easeCar(g.world.Player, mine, dt)
	g.indicator = indicatorIn(mine.Flags)
	if swapped {
		g.easeCar(partner.V, s.Runner, dt)
		partner.SetIndicator(indicatorIn(s.Runner.Flags))
	}

	for i, p := range s.Agents {
		if i >= len(g.world.Agents) || (swapped && i == s.Partner) {
			continue
		}
		a := g.world.Agents[i]
		ease(a.V, p, dt)
		a.SetPursuing(p.Flags&FlagPursuing != 0)
	}
	if g.mode == ModeChase && s.Chaser >= 0 && s.Chaser < len(g.world.Agents) {
		g.chaser = g.world.Agents[s.Chaser]
	}

	wa := &g.world.Wanted
	wa.Level, wa.State = s.WantedLevel, sim.PursuitState(s.WantedState)
	wa.Bounty, wa.Banked, wa.Stops, wa.Escapes = s.Bounty, s.Banked, s.Stops, s.Escapes
	wa.Busting, wa.Cooldown = s.Busting, s.Cooldown
	g.assisted = s.Assisted
	g.chase.Elapsed, g.chase.Over, g.chase.Winner = s.Elapsed, s.Over, s.Winner
}

// easeCar eases a car to a pose and takes the nitrous off its flags.
func (g *Game) easeCar(v *sim.Vehicle, p Pose, dt float32) {
	ease(v, p, dt)
	v.Boosting = p.Flags&FlagBoosting != 0
}

func ease(v *sim.Vehicle, p Pose, dt float32) {
	// Snapshots arrive twenty times a second and frames are drawn sixty, so
	// snapping to each one makes every remote car judder. Sliding toward the
	// host's pose instead costs a few centimetres of accuracy and looks like
	// driving. A car that has moved a long way — a respawn, or the first
	// snapshot — is placed outright rather than sliding across the city.
	const rate = 16
	target := mathx.V(p.X, p.Z)
	if dt <= 0 || v.Pos.DistTo(target) > 25 {
		sim.ApplyPose(v, p.X, p.Z, p.Yaw, p.Speed)
		return
	}
	eased := mathx.V(
		mathx.Approach(v.Pos.X, target.X, rate, dt),
		mathx.Approach(v.Pos.Z, target.Z, rate, dt),
	)
	// Yaw is eased the short way round, so a car crossing north does not spin.
	yaw := v.Yaw + mathx.AngleDiff(p.Yaw, v.Yaw)*mathx.Clamp(rate*dt, 0, 1)
	sim.ApplyPose(v, eased.X, eased.Z, yaw, p.Speed)
}

func poseOf(v *sim.Vehicle, pursuing bool) Pose {
	var flags uint8
	if pursuing {
		flags |= FlagPursuing
	}
	if v.Brake > 0.05 {
		flags |= FlagBraking
	}
	if v.Boosting {
		flags |= FlagBoosting
	}
	return Pose{
		X: v.Pos.X, Z: v.Pos.Z, Yaw: v.Yaw, Speed: v.Speed(), Flags: flags,
	}
}
