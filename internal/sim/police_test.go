package sim_test

import (
	"math/rand/v2"
	"testing"

	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

func TestStylesDifferInTheWaysThatMatter(t *testing.T) {
	cautious, aggressive := sim.Cautious(), sim.Aggressive()

	if !(cautious.SpeedBias < aggressive.SpeedBias) {
		t.Errorf("cautious bias %v is not below aggressive %v",
			cautious.SpeedBias, aggressive.SpeedBias)
	}
	if !(cautious.Headway > aggressive.Headway) {
		t.Errorf("cautious headway %v is not above aggressive %v",
			cautious.Headway, aggressive.Headway)
	}
	if !(cautious.GapAcceptance > aggressive.GapAcceptance) {
		t.Error("a cautious driver should want a bigger gap than an aggressive one")
	}
	if cautious.RunsAmber {
		t.Error("a cautious driver should not run an amber")
	}
	if !aggressive.RunsAmber {
		t.Error("an aggressive driver should run an amber")
	}
}

func TestAmberDecisionFollowsStyle(t *testing.T) {
	// Far enough back that stopping is comfortable for anyone.
	const dist, speed float32 = 40, 9
	if !sim.Cautious().StopsForAmber(dist, speed) {
		t.Error("a cautious driver should stop for an amber 40m away")
	}
	if sim.Aggressive().StopsForAmber(dist, speed) {
		t.Error("an aggressive driver should carry on through it")
	}
	// Nobody stops from right on top of the line.
	if sim.Cautious().StopsForAmber(1, 18) {
		t.Error("stopping from 1m at 18 m/s is not possible and should not be tried")
	}
}

func TestRandomStyleStaysWithinTheMix(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	seen := map[string]int{}
	for range 400 {
		seen[sim.RandomStyle(rng).Name]++
	}
	for _, s := range sim.Styles() {
		if seen[s.Name] == 0 {
			t.Errorf("style %q never came up in 400 draws", s.Name)
		}
	}
	// Normal should dominate; aggressive should be the rare one.
	if seen["normal"] <= seen["aggressive"] {
		t.Errorf("normal %d should outnumber aggressive %d",
			seen["normal"], seen["aggressive"])
	}
}

func TestPursuitStyleOutrunsTraffic(t *testing.T) {
	if !(sim.Pursuit().SpeedBias > sim.Aggressive().SpeedBias) {
		t.Error("a pursuit car should be quicker than the worst civilian driver")
	}
	if !(sim.PoliceSpec().MaxSpeed > sim.TrafficSpec().MaxSpeed) {
		t.Error("a police car should out-run ambient traffic")
	}
}

func TestPoliceUnitsArePlaced(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 30, Police: 4})
	units := w.Police()
	if len(units) == 0 {
		t.Fatal("no police units were spawned")
	}
	for _, u := range units {
		if u.Role != sim.RolePolice {
			t.Errorf("unit %d has role %v", u.ID, u.Role)
		}
		if u.Pursuing() {
			t.Errorf("unit %d started out already in pursuit", u.ID)
		}
	}
}

func TestNoPoliceWhenNoneAreConfigured(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 20, Police: 0})
	if n := len(w.Police()); n != 0 {
		t.Errorf("got %d units with police disabled, want 0", n)
	}
	// The judge still scores; there is simply nobody to respond.
	w.Judge.ReportCollision(w.Player, "test", 9, 0)
	w.Update(sim.Controls{}, 1.0/60)
	if w.Wanted.Active() {
		t.Error("a wanted level was raised with no police in the world")
	}
}

// An offence nobody saw should not raise a wanted level.
func TestUnwitnessedOffenceGoesUnnoticed(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 20, Police: 3})
	// Move every unit far away, then offend.
	for _, u := range w.Police() {
		u.V.Place(mathx.V(6000, 6000), 0)
	}
	w.Judge.ReportCollision(w.Player, "test", 9, 0)
	if w.Wanted.Active() {
		t.Error("an offence with no unit nearby raised a wanted level")
	}
}

func TestWitnessedOffenceRaisesTheAlarm(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 20, Police: 3})
	// Park a unit right beside the player so it cannot miss anything.
	u := w.Police()[0]
	u.V.Place(w.Player.Pos.Add(mathx.V(6, 0)), w.Player.Yaw)

	w.Judge.ReportCollision(w.Player, "hit a vehicle", 12, 0)
	if !w.Wanted.Active() {
		t.Fatal("an offence in plain view did not raise a wanted level")
	}
	if w.Wanted.Witnessed != 1 {
		t.Errorf("witnessed = %d, want 1", w.Wanted.Witnessed)
	}

	// A step should put the nearby units onto the call.
	w.Update(sim.Controls{}, 1.0/60)
	if !u.Pursuing() {
		t.Error("the unit that saw it did not join the pursuit")
	}
	if !u.BluesAndTwos() {
		t.Error("a unit on a call should be showing its lights")
	}
}

func TestStayingOutOfSightShedsTheWantedLevel(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 10, Police: 2})
	u := w.Police()[0]
	u.V.Place(w.Player.Pos.Add(mathx.V(6, 0)), w.Player.Yaw)
	w.Judge.ReportCollision(w.Player, "hit a vehicle", 12, 0)

	start := w.Wanted.Level
	if start == 0 {
		t.Fatal("setup failed: no wanted level to shed")
	}
	// Put every unit well over the horizon and let the clock run.
	for _, p := range w.Police() {
		p.V.Place(mathx.V(9000, 9000), 0)
	}
	for range 900 { // 15 seconds
		w.Update(sim.Controls{}, 1.0/60)
		for _, p := range w.Police() {
			p.V.Place(mathx.V(9000, 9000), 0)
		}
	}
	if w.Wanted.Level >= start {
		t.Errorf("wanted level %d did not fall from %d after evading",
			w.Wanted.Level, start)
	}
}

func TestPullingOverEndsThePursuit(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 10, Police: 2})
	u := w.Police()[0]
	u.V.Place(w.Player.Pos.Add(mathx.V(5, 0)), w.Player.Yaw)
	w.Judge.ReportCollision(w.Player, "hit a vehicle", 12, 0)
	if !w.Wanted.Active() {
		t.Fatal("setup failed: no pursuit to end")
	}

	// Pull the car up alongside the unit and hold it there. The player is
	// moved rather than the unit, because a unit keeps itself on its lane and
	// would simply drift back.
	for range 600 {
		beside := u.V.Pos.Add(u.V.Forward().Right().Mul(3.5))
		w.Player.Place(beside, u.V.Yaw)
		w.Update(sim.Controls{Brake: 1}, 1.0/60)
		if w.Wanted.Stops > 0 {
			break
		}
	}
	if w.Wanted.Stops == 0 {
		t.Fatal("stopping beside a unit did not end in being pulled over")
	}
	// The stop clears the level once the notice has been shown.
	for range 400 {
		w.Update(sim.Controls{Brake: 1}, 1.0/60)
	}
	if w.Wanted.Active() {
		t.Errorf("wanted level %d survived being pulled over", w.Wanted.Level)
	}
}

func TestPoliceDriveLawfullyUntilCalled(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 40, Police: 4})
	for range 900 {
		w.Update(sim.Controls{}, 1.0/60)
	}
	for _, u := range w.Police() {
		if u.Pursuing() {
			t.Fatalf("unit %d is pursuing with no offence committed", u.ID)
		}
		// Patrolling units keep to the limit like anyone else.
		if u.V.Speed() > u.SpeedLimit()*1.4 {
			t.Errorf("patrolling unit %d doing %.1f m/s in a %.1f limit",
				u.ID, u.V.Speed(), u.SpeedLimit())
		}
	}
}

// A pursuit is only worth having if the units actually close in, which
// exercises the greedy junction routing that steers them toward the target.
func TestPursuingUnitsCloseIn(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 20, Police: 4})

	// Put a unit within sight, offend, and let the response begin.
	u := w.Police()[0]
	u.V.Place(u.V.Pos, u.V.Yaw)
	w.Player.Place(u.V.Pos.Add(u.V.Forward().Mul(-40)), u.V.Yaw)
	w.Judge.ReportCollision(w.Player, "hit a vehicle", 12, 0)
	if !w.Wanted.Active() {
		t.Fatal("setup failed: the offence was not witnessed")
	}

	// Hold the car still and see whether the nearest unit gets closer. A
	// stationary target is the clearest test of the routing.
	start := nearestPolice(w)
	best := start
	for range 1800 { // 30 seconds
		w.Update(sim.Controls{Brake: 1}, 1.0/60)
		best = min(best, nearestPolice(w))
		if best < 12 {
			break
		}
	}
	if best >= start {
		t.Errorf("nearest unit went from %.0fm to no closer than %.0fm; "+
			"the pursuit is not routing toward the target", start, best)
	}
	t.Logf("closed from %.0fm to %.0fm", start, best)
}

func nearestPolice(w *sim.World) float32 {
	best := float32(1e9)
	for _, u := range w.Police() {
		best = min(best, u.V.Pos.DistTo(w.Player.Pos))
	}
	return best
}
