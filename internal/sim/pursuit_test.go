package sim_test

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// lead puts the player on the road a given distance in front of a unit, facing
// the same way. The player is moved rather than the unit, because a unit keeps
// itself on its lane and would simply drift back.
func lead(w *sim.World, u *sim.Agent, metres float32) {
	w.Player.Place(u.V.Pos.Add(u.V.Forward().Mul(metres)), u.V.Yaw)
}

// wanted starts a pursuit: the player is put just ahead of a unit, which then
// witnesses an offence. It returns the unit.
func wanted(t *testing.T, w *sim.World) *sim.Agent {
	t.Helper()
	u := w.Police()[0]
	lead(w, u, 25)
	w.Judge.ReportCollision(w.Player, "hit a vehicle", 12, w.Time)
	if !w.Wanted.Active() {
		t.Fatal("setup failed: the offence raised no wanted level")
	}
	return u
}

// chased runs a pursuit for a while with the player held just ahead of the
// unit, which keeps them in plain sight without letting them be stopped.
func chased(w *sim.World, u *sim.Agent, seconds float32) {
	for t := float32(0); t < seconds; t += 1.0 / 60 {
		lead(w, u, 25)
		w.Player.Vel = u.V.Forward().Mul(12)
		w.Update(sim.Controls{Throttle: 0.4}, 1.0/60)
	}
}

// banish puts every unit over the horizon, where none of them can see anything.
func banish(w *sim.World) {
	for _, p := range w.Police() {
		p.V.Place(mathx.V(9000, 9000), 0)
	}
}

func TestBuildingsBlockTheLineOfSight(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 0, Police: 0})
	if len(w.City.Buildings) == 0 {
		t.Fatal("this city has no buildings")
	}
	b := w.City.Buildings[0]
	west := b.Center.Sub(mathx.V(b.W/2+3, 0))
	east := b.Center.Add(mathx.V(b.W/2+3, 0))
	if w.LineOfSight(west, east) {
		t.Error("two points either side of a building can see each other through it")
	}
	// Along the same wall, on the outside, nothing is in the way.
	north := west.Add(mathx.V(0, b.D/2+3))
	if !w.LineOfSight(west, north.Sub(mathx.V(0, 1))) {
		t.Error("two points on the same side of a building cannot see each other")
	}
}

// A pursuit in plain sight gets worse by itself. A driver who simply keeps
// going with a unit behind them is a bigger problem every second.
func TestStayingInSightEscalates(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 0, Police: 4})
	u := wanted(t, w)
	start := w.Wanted.Heat

	chased(w, u, 10)
	if w.Wanted.State != sim.PursuitActive {
		t.Fatalf("state = %v with a unit 25m behind, want an active pursuit", w.Wanted.State)
	}
	if w.Wanted.Heat <= start {
		t.Errorf("heat went from %.1f to %.1f over ten seconds in plain sight", start, w.Wanted.Heat)
	}
	if w.Wanted.Bounty == 0 {
		t.Error("ten seconds of being chased earned no bounty")
	}
}

func TestGettingAwayBanksTheBounty(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 0, Police: 3})
	u := wanted(t, w)
	chased(w, u, 5)
	earned := w.Wanted.Bounty
	if earned == 0 {
		t.Fatal("setup failed: nothing earned to bank")
	}

	for range 60 * 40 {
		banish(w)
		w.Update(sim.Controls{}, 1.0/60)
		if !w.Wanted.Active() {
			break
		}
	}
	if w.Wanted.Active() {
		t.Fatalf("still wanted at level %d after forty seconds out of sight", w.Wanted.Level)
	}
	if w.Wanted.Escapes != 1 {
		t.Errorf("escapes = %d, want 1", w.Wanted.Escapes)
	}
	if w.Wanted.Banked < earned || w.Wanted.Bounty != 0 {
		t.Errorf("banked %d and still holding %d, having earned %d", w.Wanted.Banked, w.Wanted.Bounty, earned)
	}
}

func TestBeingStoppedForfeitsTheBounty(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 0, Police: 3})
	u := wanted(t, w)
	chased(w, u, 5)
	if w.Wanted.Bounty == 0 {
		t.Fatal("setup failed: nothing earned to lose")
	}

	for range 600 {
		w.Player.Place(u.V.Pos.Add(u.V.Forward().Right().Mul(3.5)), u.V.Yaw)
		w.Update(sim.Controls{Brake: 1}, 1.0/60)
		if w.Wanted.Stops > 0 {
			break
		}
	}
	if w.Wanted.Stops != 1 {
		t.Fatalf("stops = %d after sitting beside a unit, want 1", w.Wanted.Stops)
	}
	if w.Wanted.Bounty != 0 || w.Wanted.Banked != 0 {
		t.Errorf("kept %d and banked %d after being stopped", w.Wanted.Bounty, w.Wanted.Banked)
	}
}

// A car that stops somewhere no unit can drive to is not safe there. Being
// stationary in plain sight of a unit that is close by is enough, given time.
func TestACorneredCarIsStoppedEventually(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 0, Police: 3})
	u := wanted(t, w)
	chased(w, u, 1)
	// A unit that is going nowhere, thirty metres off with a clear view.
	u.Blocking = true
	lead(w, u, 30)
	for range 60 * 20 {
		w.Update(sim.Controls{Brake: 1}, 1.0/60)
		if w.Wanted.Stops > 0 {
			return
		}
	}
	t.Errorf("sat still for twenty seconds in plain sight of a unit and was never stopped (busting %.2f)",
		w.Wanted.Busting)
}

// The fast cars and the heavy ones are held back until the ordinary ones have
// had their chance.
func TestHeavierUnitsWaitTheirTurn(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 0, Police: 8})
	wanted(t, w)
	w.Update(sim.Controls{}, 1.0/60)
	if w.Wanted.Level != 1 {
		t.Fatalf("setup: wanted level %d, want 1", w.Wanted.Level)
	}

	kinds := map[sim.UnitKind]int{}
	for _, u := range w.Police() {
		kinds[u.Unit]++
		if u.Pursuing() && u.Unit != sim.UnitPatrol {
			t.Errorf("a %s unit joined the pursuit at wanted level 1", u.Unit.Name())
		}
	}
	if kinds[sim.UnitInterceptor] == 0 || kinds[sim.UnitHeavy] == 0 {
		t.Errorf("eight units came out as %v, want some of each kind", kinds)
	}
}

func TestUnitKindsAreDifferentCars(t *testing.T) {
	patrol, fast, heavy := sim.UnitSpec(sim.UnitPatrol), sim.UnitSpec(sim.UnitInterceptor), sim.UnitSpec(sim.UnitHeavy)
	if fast.MaxSpeed <= patrol.MaxSpeed {
		t.Errorf("an interceptor tops out at %.0f m/s, no faster than a patrol car's %.0f", fast.MaxSpeed, patrol.MaxSpeed)
	}
	if heavy.Mass <= patrol.Mass*1.3 {
		t.Errorf("a heavy unit weighs %.0f kg against a patrol car's %.0f", heavy.Mass, patrol.Mass)
	}
	for _, k := range []sim.UnitKind{sim.UnitPatrol, sim.UnitInterceptor, sim.UnitHeavy} {
		if k.Name() == "" {
			t.Errorf("unit kind %d has no name", k)
		}
	}
}

// At the first level a unit sits behind with its lights on and waits for the
// driver to pull over. It does not drive into them.
func TestAtTheFirstLevelUnitsFollowRatherThanRam(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 0, Police: 3})
	u := wanted(t, w)
	lead(w, u, 35)

	nearest := float32(1e9)
	for range 60 * 8 {
		w.Player.Vel = w.Player.Forward().Mul(7)
		w.Update(sim.Controls{Throttle: 0.2}, 1.0/60)
		if w.Wanted.Level > 1 {
			break // it has escalated, and what follows is a different test
		}
		nearest = min(nearest, u.V.Pos.DistTo(w.Player.Pos))
	}
	if nearest > 20 {
		t.Errorf("a unit 35m behind a slow car got no closer than %.0fm", nearest)
	}
	if nearest < 4.6 {
		t.Errorf("a unit at wanted level 1 closed to %.1fm, which is contact", nearest)
	}
}

// Once the driver has shown they will not stop, the units stop them.
func TestAtHigherLevelsUnitsGoForTheCar(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 0, Police: 3})
	u := wanted(t, w)
	for range 60 {
		if chased(w, u, 1); w.Wanted.Level >= 2 {
			break
		}
	}
	if w.Wanted.Level < 2 {
		t.Fatalf("setup failed: a minute in plain sight only reached wanted level %d", w.Wanted.Level)
	}

	lead(w, u, 30)
	nearest := float32(1e9)
	for range 60 * 10 {
		w.Player.Vel = w.Player.Forward().Mul(7)
		w.Update(sim.Controls{Throttle: 0.2}, 1.0/60)
		nearest = min(nearest, nearestPolice(w))
	}
	if nearest > 6 {
		t.Errorf("no unit at wanted level %d got closer than %.1fm to a car doing 7 m/s",
			w.Wanted.Level, nearest)
	}
}

func TestNitrousBurnsAndComesBack(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 0, Police: 0})
	p := w.Player
	if p.Nitro != 1 {
		t.Fatalf("the player starts with %.2f of a bottle, want a full one", p.Nitro)
	}
	for range 120 {
		w.Update(sim.Controls{Throttle: 1, Boost: true}, 1.0/60)
	}
	if !p.Boosting {
		t.Error("holding boost with a full bottle did not burn it")
	}
	burned := p.Nitro
	if burned >= 1 {
		t.Errorf("two seconds of boost left %.2f of the bottle", burned)
	}
	for range 240 {
		w.Update(sim.Controls{Throttle: 0.3}, 1.0/60)
	}
	if p.Nitro <= burned {
		t.Errorf("the bottle did not refill off the boost: %.2f then %.2f", burned, p.Nitro)
	}
}

func TestBoostIsQuicker(t *testing.T) {
	run := func(boost bool) float32 {
		v := sim.NewVehicle(sim.CarSpec(), mathx.V(0, 0), 0)
		v.Nitro = 1
		for range 180 {
			v.Update(sim.Controls{Throttle: 1, Boost: boost}, 1.0/60)
		}
		return v.Speed()
	}
	plain, boosted := run(false), run(true)
	if boosted <= plain*1.1 {
		t.Errorf("three seconds flat out reached %.1f m/s on nitrous and %.1f without", boosted, plain)
	}
}

// Traffic has no bottle, and holding the button does nothing for it.
func TestTrafficCarriesNoNitrous(t *testing.T) {
	v := sim.NewVehicle(sim.TrafficSpec(), mathx.V(0, 0), 0)
	for range 300 {
		v.Update(sim.Controls{Throttle: 1, Boost: true}, 1.0/60)
	}
	if v.Boosting || v.Nitro != 0 {
		t.Errorf("a car with no nitrous is boosting=%v with %.2f of a bottle", v.Boosting, v.Nitro)
	}
}

func TestKnocksAddUpToAWreck(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 0, Police: 0})
	if w.Damage != 0 {
		t.Fatalf("a new car starts with %.2f damage", w.Damage)
	}
	// Drive at the nearest building, repeatedly, until something gives.
	b := w.City.Buildings[0]
	for range 40 {
		w.Player.Place(b.Center.Sub(mathx.V(b.W/2+6, 0)), 0)
		w.Player.Vel = mathx.V(22, 0)
		for range 30 {
			w.Update(sim.Controls{Throttle: 1}, 1.0/60)
		}
		if w.Damage >= 1 {
			break
		}
	}
	if w.Damage < 1 {
		t.Fatalf("forty crashes into a wall left the car at %.2f damage", w.Damage)
	}
	w.Respawn()
	if w.Damage != 0 {
		t.Errorf("a respawned car still carries %.2f damage", w.Damage)
	}
}
