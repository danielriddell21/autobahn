package game

import (
	"testing"
	"time"

	crucihud "github.com/danielriddell21/crucible/hud"
	"github.com/danielriddell21/crucible/netplay"

	"github.com/danielriddell21/autobahn/internal/sim"
)

// coopGame is one end of a co-op game with a world and no window, on a session
// that is already connected.
func coopGame(t *testing.T, mode NetMode, session *netplay.Session[Snapshot, Input]) *Game {
	t.Helper()
	o := DefaultOptions()
	o.Seed, o.Traffic, o.Police = 5, 20, 3
	g := &Game{
		opts:    o,
		world:   sim.NewWorld(sim.Config{Seed: o.Seed, Traffic: o.Traffic, Police: o.Police}),
		notices: crucihud.New(),
		net:     session,
		mode:    mode,
		lastDT:  1.0 / 60,
	}
	g.seat()
	return g
}

// connect starts a host on a free local port and a joiner on it, and waits for
// the host to see the joiner.
func connect(t *testing.T) (host, join *netplay.Session[Snapshot, Input]) {
	t.Helper()
	host, join = &netplay.Session[Snapshot, Input]{}, &netplay.Session[Snapshot, Input]{}
	if err := host.StartHosting("127.0.0.1:0"); err != nil {
		t.Fatalf("hosting: %v", err)
	}
	t.Cleanup(host.Leave)
	if err := join.StartJoining(host.Addr()); err != nil {
		t.Fatalf("joining: %v", err)
	}
	t.Cleanup(join.Leave)
	waitFor(t, "the host to see the joiner", host.Peered)
	return host, join
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

// The joiner builds nothing until the first snapshot, and the first snapshot has
// to say everything it needs to build: which mode, which world.
func TestTheJoinerLearnsTheModeFromTheFirstSnapshot(t *testing.T) {
	for _, mode := range []NetMode{ModeCoopCars, ModeCoopCar, ModeChase} {
		t.Run(mode.String(), func(t *testing.T) {
			hs, js := connect(t)
			host := coopGame(t, mode, hs)

			var snap Snapshot
			waitFor(t, "the first snapshot", func() bool {
				host.publish()
				s, ok := js.Snapshot()
				snap = s
				return ok
			})
			if snap.Mode != mode || snap.Proto != Proto {
				t.Fatalf("first snapshot says mode %v, protocol %d; want %v, %d", snap.Mode, snap.Proto, mode, Proto)
			}
			if snap.Seed != host.opts.Seed || snap.Traffic != host.opts.Traffic || snap.Police != host.opts.Police {
				t.Errorf("snapshot names world %d/%d/%d, want %d/%d/%d",
					snap.Seed, snap.Traffic, snap.Police, host.opts.Seed, host.opts.Traffic, host.opts.Police)
			}
			if (snap.Partner >= 0) != (mode == ModeCoopCars) {
				t.Errorf("partner index %d in mode %v", snap.Partner, mode)
			}

			// What the lobby builds the joiner's game from.
			sh := &Shell{settings: DefaultSettings(), net: js}
			o := sh.joinerOptions(DefaultOptions())
			if o.Play != mode || o.Seed != snap.Seed || o.Traffic != snap.Traffic {
				t.Errorf("joiner would build mode %v, seed %d, %d cars", o.Play, o.Seed, o.Traffic)
			}
			if err := sh.protoErr(); err != nil {
				t.Errorf("matching builds were refused: %v", err)
			}
		})
	}
}

func TestAMismatchedBuildFailsInTheLobby(t *testing.T) {
	hs, js := connect(t)
	host := coopGame(t, ModeCoopCars, hs)
	waitFor(t, "a snapshot", func() bool {
		host.publish()
		_, ok := js.Snapshot()
		return ok
	})
	// The check is on the number; to see it fail, pretend to be a different build.
	sh := &Shell{settings: DefaultSettings(), net: js}
	snap, _ := js.Snapshot()
	if snap.Proto == Proto+1 || sh.protoErr() != nil {
		t.Fatal("setup failed: the builds already disagree")
	}
	hs.Send(Snapshot{Proto: Proto + 1})
	waitFor(t, "the odd snapshot", func() bool {
		s, _ := js.Snapshot()
		return s.Proto == Proto+1
	})
	if sh.protoErr() == nil {
		t.Error("a host speaking another protocol was accepted")
	}
}

// Two cars: the joiner's controls reach the partner's car, and the joiner then
// sees its own car as the player's, where the host put it.
func TestTwoCarCoopCarriesTheJoinersControlsAndSwapsTheCars(t *testing.T) {
	hs, js := connect(t)
	host := coopGame(t, ModeCoopCars, hs)
	partner := host.world.Partner()
	if partner == nil {
		t.Fatal("the host has no partner car")
	}
	if host.world.Chaser() != nil || host.chaser != nil {
		t.Error("a co-op game has a police car driven by a person")
	}

	join := coopGame(t, ModeChase, js) // knows nothing yet
	join.world.AssignPartner()

	js.SendInput(Input{Throttle: 1, Steer: 0.5, Boost: true, Indicator: 2})
	waitFor(t, "the controls to arrive", func() bool {
		host.driveCoop(1.0 / 60)
		return partner.Controls().Throttle == 1
	})
	if c := partner.Controls(); c.Steer != 0.5 || !c.Boost {
		t.Errorf("the partner's controls are %+v, want the joiner's", c)
	}
	if partner.Indicator() != sim.IndicatorRight {
		t.Errorf("the partner's indicator is %v, want right", partner.Indicator())
	}
	if host.world.Player.Throttle != 0 {
		t.Error("the joiner's throttle reached the host's own car")
	}

	var snap Snapshot
	waitFor(t, "a snapshot", func() bool {
		host.driveCoop(1.0 / 60)
		s, ok := js.Snapshot()
		snap = s
		return ok
	})
	join.adopt(snap, 0)
	if join.mode != ModeCoopCars {
		t.Fatalf("the joiner is in mode %v after its first snapshot", join.mode)
	}
	if got, want := join.world.Player.Pos, snap.Agents[snap.Partner]; got.X != want.X || got.Z != want.Z {
		t.Errorf("the joiner's own car is at %v, want where the host put the partner, (%v, %v)", got, want.X, want.Z)
	}
	if got := join.world.Partner().V.Pos; got.X != snap.Runner.X || got.Z != snap.Runner.Z {
		t.Errorf("the joiner sees the host's car at %v, want (%v, %v)", got, snap.Runner.X, snap.Runner.Z)
	}
}

// One car: the host steers and the joiner works the pedals, and neither can
// reach the other's half.
func TestSharedCarTakesTheWheelFromTheHostAndThePedalsFromTheJoiner(t *testing.T) {
	hs, js := connect(t)
	host := coopGame(t, ModeCoopCar, hs)
	if host.world.Partner() != nil {
		t.Fatal("a shared car has a second car")
	}

	// A joiner who is pushing the wheel and the pedals both.
	js.SendInput(Input{Throttle: 1, Steer: -1, Handbrake: true, Indicator: 1})
	waitFor(t, "the pedals to arrive", func() bool {
		host.driveCoop(1.0 / 60)
		return host.world.Player.Throttle == 1
	})
	if host.world.Player.Steer != 0 || host.world.Player.Handbrake {
		t.Errorf("the pedal player reached the wheel: steer %v, handbrake %v",
			host.world.Player.Steer, host.world.Player.Handbrake)
	}
	if host.indicator != sim.IndicatorLeft {
		t.Errorf("the joiner's indicator is %v on the host, want left", host.indicator)
	}
}

func TestTheAutopilotIsRefusedInASharedCarOnly(t *testing.T) {
	hs, _ := connect(t)
	shared := coopGame(t, ModeCoopCar, hs)
	shared.driver = nil // it must not be reached
	shared.SetAutopilot(true)
	if shared.Autopilot() {
		t.Error("the autopilot took the wheel of a shared car")
	}
	if shared.teamAssisted() {
		t.Error("a refused autopilot still counts as help")
	}
}

func TestTheAutopilotIsAllowedInTwoCars(t *testing.T) {
	hs, _ := connect(t)
	g := coopGame(t, ModeCoopCars, hs)
	if g.autoBlocked() {
		t.Error("the autopilot is blocked in two-car co-op")
	}
	g.auto = true
	if !g.teamAssisted() {
		t.Error("a team with the autopilot driving is not counted as helped")
	}
	g.auto, g.remoteAuto = false, true
	if !g.teamAssisted() || !g.helped() {
		t.Error("the other player's autopilot does not count as help")
	}
}

func TestOptionsRefuseTheAutopilotWithACarTwoShare(t *testing.T) {
	if err := (Options{Play: ModeCoopCar, Autopilot: true}).coopValid(); err == nil {
		t.Error("-autopilot was accepted with a shared car")
	}
	for _, o := range []Options{
		{Play: ModeCoopCar}, {Play: ModeCoopCars, Autopilot: true}, {Play: ModeChase, Autopilot: true},
	} {
		if err := o.coopValid(); err != nil {
			t.Errorf("%+v was refused: %v", o, err)
		}
	}
}

// Whatever the host's world holds, both players bank the same pursuit, but only
// if nobody's autopilot touched it. That is the bountyMeter's rule; this holds
// the co-op end of it to the same thing.
func TestBothPlayersBankATeamPursuitUnlessAnAutopilotHelped(t *testing.T) {
	hs, _ := connect(t)
	g := coopGame(t, ModeCoopCars, hs)

	chase := func(helped bool) int {
		g.career = bountyMeter{}
		g.remoteAuto = helped
		wa := &g.world.Wanted
		wa.Level, wa.Bounty = 2, 400
		g.career.watch(wa, g.helped())
		wa.Level, wa.Bounty, wa.Banked = 0, 0, wa.Banked+400
		g.career.watch(wa, g.helped())
		return g.career.take()
	}
	if got := chase(false); got != 400 {
		t.Errorf("a team pursuit with nobody's autopilot banked %d, want 400", got)
	}
	if got := chase(true); got != 0 {
		t.Errorf("a team pursuit the other player's autopilot helped with banked %d, want 0", got)
	}
}
