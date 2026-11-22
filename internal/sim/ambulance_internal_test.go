package sim

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// An ambulance is a role with a count of its own: none by default, so a world
// without them is the world it always was.
func TestAmbulancesAreARoleWithACountOfTheirOwn(t *testing.T) {
	if n := len(NewWorld(Config{Seed: 5, Traffic: 20, Police: 2}).Ambulances()); n != 0 {
		t.Errorf("%d ambulances in a world that asked for none", n)
	}
	w := NewWorld(Config{Seed: 5, Traffic: 20, Police: 2, Ambulances: 3})
	if len(w.Ambulances()) == 0 || len(w.Ambulances()) > 3 {
		t.Fatalf("%d ambulances, want up to 3", len(w.Ambulances()))
	}
	for _, a := range w.Ambulances() {
		if a.Role != RoleAmbulance || a.Body.Kind != Ambulance {
			t.Errorf("ambulance %d is a %v in role %v", a.ID, a.Body.Kind.Name(), a.Role)
		}
		if a.BlueLights() {
			t.Errorf("ambulance %d has its lights on before it has been sent anywhere", a.ID)
		}
	}
	// The rest of the town is as it was: same traffic, same police.
	plain := NewWorld(Config{Seed: 5, Traffic: 20, Police: 2})
	for i, a := range plain.Agents {
		if b := w.Agents[i]; a.V.Pos != b.V.Pos || a.Body.Kind != b.Body.Kind {
			t.Fatalf("agent %d differs once ambulances are added: %v against %v", i, a.V.Pos, b.V.Pos)
		}
	}
}

// An ambulance goes out on a call for a while, comes back, and goes out again.
func TestAmbulanceRunsOnATimetable(t *testing.T) {
	w := NewWorld(Config{Seed: 5, Ambulances: 1})
	a := w.Ambulances()[0]
	on, off := false, false
	for range 60 * 120 {
		a.runClock(1.0 / 60)
		if a.BlueLights() {
			on = true
		} else if on {
			off = true
		}
	}
	if !on || !off {
		t.Errorf("in two minutes the lights were on %v and went off again %v", on, off)
	}
	a.StartRun(5)
	if !a.BlueLights() || !a.BluesAndTwos() {
		t.Error("a run did not put the lights on")
	}
	for range 60 * 6 {
		a.runClock(1.0 / 60)
	}
	if a.BlueLights() {
		t.Error("a five second run was still going after six")
	}
}

// With its lights on, an ambulance behind makes ordinary traffic pull in and
// stop; with them off it is any other van.
func TestTrafficPullsOverForAnAmbulanceWithItsLightsOn(t *testing.T) {
	run := func(lit bool) *Agent {
		w, l := emptyRoad(t, city.Collector, true)
		car := put(w, l, 60, Normal(), 8)
		amb := NewAmbulance(len(w.Agents), w.City, l, 25, w.rng)
		amb.hasNext = false
		amb.V.Vel = l.Fwd.Mul(8)
		w.Agents = append(w.Agents, amb)
		w.ambulances = append(w.ambulances, amb)
		amb.rest = 1e6
		if lit {
			amb.StartRun(1e6)
		}
		step(w, 4, func() bool { return car.lane == l })
		return car
	}
	lit, dark := run(true), run(false)
	if lit.sidestepWant >= 0 {
		t.Errorf("a car with an ambulance behind it is not pulling in: sidestep wanted %.1f", lit.sidestepWant)
	}
	if lit.V.Speed() > dark.V.Speed()-2 {
		t.Errorf("the car pulled in for an ambulance is doing %.1f m/s, one left alone %.1f", lit.V.Speed(), dark.V.Speed())
	}
	if dark.sidestepWant < 0 {
		t.Error("the car pulled in for an ambulance with its lights off")
	}
}

// The ambulance is no part of what a civilian pulls over for until it is
// behind: one ahead, or on another road, changes nothing.
func TestAnAmbulanceAheadDoesNotMakeTrafficPullOver(t *testing.T) {
	w, l := emptyRoad(t, city.Collector, true)
	car := put(w, l, 25, Normal(), 8)
	amb := NewAmbulance(len(w.Agents), w.City, l, 70, w.rng)
	amb.StartRun(1e6)
	w.Agents = append(w.Agents, amb)
	w.ambulances = append(w.ambulances, amb)
	if w.ambulanceBehind(car.V.Pos, car.V.Forward()) {
		t.Error("a car heard an ambulance that is 45 m in front of it")
	}
}

func inLine(t *testing.T, ahead, across, ambSpeed, playerSpeed float32, lit bool) *World {
	t.Helper()
	w, l := emptyRoad(t, city.Collector, true)
	amb := NewAmbulance(len(w.Agents), w.City, l, 20, w.rng)
	amb.V.Vel = l.Fwd.Mul(ambSpeed)
	w.Agents = append(w.Agents, amb)
	w.ambulances = append(w.ambulances, amb)
	if lit {
		amb.StartRun(1e6)
	}
	at := amb.V.Pos.Add(amb.V.Forward().Mul(ahead)).Add(amb.V.Forward().Right().Mul(across))
	w.Player.Place(at, amb.V.Yaw)
	w.Player.Vel = amb.V.Forward().Mul(playerSpeed)
	return w
}

func TestObstructingNeedsTheLightsOnAndAnAmbulanceHeldUp(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		ahead, across, amb, player float32
		lit, want                  bool
	}{
		{"in its lane, 15 m ahead, holding it up", 15, 0, 6, 6, true, true},
		{"stopped in its way", 15, 0, 6, 0, true, true},
		{"lights off", 15, 0, 6, 6, false, false},
		{"beyond twenty-five metres", 30, 0, 6, 6, true, false},
		{"pulled over to the side", 15, 3.6, 6, 6, true, false},
		{"faster than it would go", 15, 0, 6, 25, true, false},
		{"about to be passed", 15, 0, 15, 8, true, false},
		{"behind it", -10, 0, 6, 6, true, false},
	} {
		w := inLine(t, tc.ahead, tc.across, tc.amb, tc.player, tc.lit)
		if got := w.obstructing(); got != tc.want {
			t.Errorf("%s: obstructing %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Blocking one for more than three seconds is a fault, once, and the time
// starts again if the way is cleared.
func TestObstructingAnAmbulanceIsAFaultAfterThreeSeconds(t *testing.T) {
	w := NewWorld(Config{Seed: 5})
	j := NewJudge(w.City)
	var got []Infraction
	j.Events.Subscribe(judgeWatcher(func(in Infraction) { got = append(got, in) }))
	v := w.Player
	const dt = 1.0 / 60

	for i := range 174 {
		j.Obstruct(v, true, float32(i)*dt, dt)
	}
	if len(got) != 0 {
		t.Fatalf("a fault after 2.9 seconds: %+v", got)
	}
	j.Obstruct(v, false, 2.95, dt) // cleared for a moment: starts again
	for i := range 174 {
		j.Obstruct(v, true, 3+float32(i)*dt, dt)
	}
	if len(got) != 0 {
		t.Fatalf("the time did not start again once the way was cleared: %+v", got)
	}
	for i := range 60 {
		j.Obstruct(v, true, 6+float32(i)*dt, dt)
	}
	if len(got) != 1 || got[0].Kind != ObstructedEmergency || got[0].Points != ObstructedEmergency.Penalty() {
		t.Fatalf("after three seconds of blocking: %+v", got)
	}
	if ObstructedEmergency.String() == "COLLISION" || ObstructedEmergency.Penalty() <= 0 {
		t.Error("the fault has no name or no penalty")
	}
}

// A world with an ambulance in the player's way judges it, end to end.
func TestWorldJudgesAPlayerSittingInAnAmbulancesWay(t *testing.T) {
	w := inLine(t, 12, 0, 0, 0, true)
	w.Player.Vel = mathx.Vec{}
	w.Judge.Reset()
	for range 60 * 5 {
		// The player holds the brake; the ambulance is held up behind.
		w.Update(Controls{Brake: 1}, 1.0/60)
		w.Ambulances()[0].V.Vel = mathx.Vec{}
		if w.Judge.Faults > 0 {
			break
		}
	}
	found := false
	for _, in := range w.Judge.Log() {
		found = found || in.Kind == ObstructedEmergency
	}
	if !found {
		t.Errorf("five seconds sitting in front of an ambulance with its lights on and no fault: %+v", w.Judge.Log())
	}
}

// A vehicle held at a red light cannot make way, and is not blamed for it.
func TestAVehicleHeldAtARedIsNotObstructing(t *testing.T) {
	w := NewWorld(Config{Seed: 5})
	j := NewJudge(w.City)
	n := 0
	j.Events.Subscribe(judgeWatcher(func(Infraction) { n++ }))
	j.hasSignal, j.signal = true, SignalRed
	for i := range 60 * 6 {
		j.Obstruct(w.Player, true, float32(i)/60, 1.0/60)
	}
	if n != 0 {
		t.Fatalf("%d faults for a car sitting at a red light", n)
	}
	j.signal = SignalGreen
	for i := range 60 * 4 {
		j.Obstruct(w.Player, true, 6+float32(i)/60, 1.0/60)
	}
	if n != 1 {
		t.Fatalf("%d faults for a car sitting at a green light in the way, want 1", n)
	}
}

// A player who is stopped because something is stopped in front of them cannot
// make way, and is not blamed for it.
func TestObstructingIsExcusedWhenQueuedBehindSomethingStopped(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(w *World)
		want  bool
	}{
		{"nothing in front", func(*World) {}, true},
		{"a stopped car just ahead", func(w *World) {
			p := w.Player
			car := NewVehicle(CarSpec(), p.Pos.Add(p.Forward().Mul(5)), p.Yaw)
			w.Agents = append(w.Agents, &Agent{V: car})
		}, false},
		{"a cone just ahead", func(w *World) {
			p := w.Player
			w.stubs = append(w.stubs, w.stub(0, mathx.OBB{Center: p.Pos.Add(p.Forward().Mul(4)), Heading: p.Yaw}, coneStubHalf))
		}, false},
		{"a car ahead that is moving", func(w *World) {
			p := w.Player
			car := NewVehicle(CarSpec(), p.Pos.Add(p.Forward().Mul(5)), p.Yaw)
			car.Vel = p.Forward().Mul(8)
			w.Agents = append(w.Agents, &Agent{V: car})
		}, true},
		{"a stopped car in the next lane", func(w *World) {
			p := w.Player
			car := NewVehicle(CarSpec(), p.Pos.Add(p.Forward().Mul(5)).Add(p.Forward().Right().Mul(3.5)), p.Yaw)
			w.Agents = append(w.Agents, &Agent{V: car})
		}, true},
		{"a stopped car a long way ahead", func(w *World) {
			p := w.Player
			car := NewVehicle(CarSpec(), p.Pos.Add(p.Forward().Mul(20)), p.Yaw)
			w.Agents = append(w.Agents, &Agent{V: car})
		}, true},
	} {
		w := inLine(t, 12, 0, 0, 0, true)
		tc.build(w)
		if got := w.obstructing(); got != tc.want {
			t.Errorf("%s: obstructing %v, want %v", tc.name, got, tc.want)
		}
	}
}

// faultsFor runs a judge for six seconds of blocking and counts the faults.
func faultsFor(w *World, j *Judge) int {
	n := 0
	j.Events.Subscribe(judgeWatcher(func(Infraction) { n++ }))
	for i := range 60 * 6 {
		j.Obstruct(w.Player, true, float32(i)/60, 1.0/60)
	}
	return n
}

func TestAVehicleThatCannotMoveIsNotObstructing(t *testing.T) {
	w := NewWorld(Config{Seed: 5})

	// Control: a judge with nothing holding the vehicle faults it.
	if n := faultsFor(w, NewJudge(w.City)); n != 1 {
		t.Fatalf("%d faults for a free vehicle in the way, want 1", n)
	}

	t.Run("inside a junction box", func(t *testing.T) {
		j := NewJudge(w.City)
		j.inBox = true
		if n := faultsFor(w, j); n != 0 {
			t.Errorf("%d faults inside a junction", n)
		}
	})

	for _, k := range []city.ControlKind{city.ControlStop, city.ControlGiveWay} {
		var line *city.Lane
		for _, l := range w.City.Lanes {
			if l.Control == k && l.Length > 20 {
				line = l
				break
			}
		}
		if line == nil {
			continue
		}
		t.Run("at a line", func(t *testing.T) {
			pos := line.B.Sub(line.Fwd.Mul(3))
			w.Player.Place(pos, line.Heading)
			j := NewJudge(w.City)
			j.lane = line
			if n := faultsFor(w, j); n != 0 {
				t.Errorf("%d faults for a vehicle held at a line of control %v", n, k)
			}
			// Well back from the line it is a free vehicle again.
			w.Player.Place(line.Point(line.Length/2), line.Heading)
			j = NewJudge(w.City)
			j.lane = line
			if n := faultsFor(w, j); n != 1 {
				t.Errorf("%d faults for a vehicle mid-road on a signed lane, want 1", n)
			}
		})
	}

	t.Run("at a roundabout's entry", func(t *testing.T) {
		for _, l := range w.City.Lanes {
			n := w.City.Nodes[l.ToNode]
			if !n.Roundabout || l.Length < 30 {
				continue
			}
			w.Player.Place(l.B.Sub(l.Fwd.Mul(4)), l.Heading)
			j := NewJudge(w.City)
			j.lane = l
			if got := faultsFor(w, j); got != 0 {
				t.Errorf("%d faults for a vehicle waiting to enter a roundabout", got)
			}
			return
		}
		t.Skip("no roundabout in this city")
	})
}
