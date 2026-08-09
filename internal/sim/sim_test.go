package sim_test

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

func world(t *testing.T) *sim.World {
	t.Helper()
	return sim.NewWorld(sim.Config{Seed: 7, Traffic: 40})
}

// step runs the simulation for a number of seconds at a fixed timestep.
func step(w *sim.World, c sim.Controls, seconds float32) {
	const dt = 1.0 / 60
	for t := float32(0); t < seconds; t += dt {
		w.Update(c, dt)
	}
}

func TestCarAcceleratesAndStops(t *testing.T) {
	w := world(t)
	step(w, sim.Controls{Throttle: 1}, 4)
	if w.Player.Speed() < 5 {
		t.Fatalf("speed after 4s at full throttle = %.1f m/s, want > 5", w.Player.Speed())
	}
	step(w, sim.Controls{Brake: 1}, 5)
	if w.Player.Speed() > 0.6 {
		t.Errorf("speed after 5s of braking = %.1f m/s, want ~0", w.Player.Speed())
	}
}

func TestSteeringTurnsTheCar(t *testing.T) {
	w := world(t)
	start := w.Player.Yaw
	step(w, sim.Controls{Throttle: 1, Steer: 1}, 3)
	if d := mathx.Abs(mathx.WrapPi(w.Player.Yaw - start)); d < 0.2 {
		t.Errorf("heading changed by %.2f rad under full lock, want a real turn", d)
	}
}

func TestSignalCycleReachesEveryAspect(t *testing.T) {
	w := world(t)
	var node int = -1
	for _, n := range w.City.Nodes {
		if n.Signalised {
			node = n.ID
			break
		}
	}
	if node < 0 {
		t.Fatal("no signalised junction in the generated city")
	}

	seen := map[sim.SignalState]bool{}
	for range 4000 {
		w.Signals.Update(1.0 / 60)
		seen[w.Signals.State(node, 0)] = true
	}
	// British signals run red, red and amber, green, amber.
	for _, s := range []sim.SignalState{
		sim.SignalRed, sim.SignalRedAmber, sim.SignalGreen, sim.SignalAmber,
	} {
		if !seen[s] {
			t.Errorf("aspect %v never appeared in the cycle", s)
		}
	}
}

func TestOpposingApproachesAreNeverBothGreen(t *testing.T) {
	w := world(t)
	for _, n := range w.City.Nodes {
		if !n.Signalised {
			continue
		}
		for range 2000 {
			w.Signals.Update(1.0 / 60)
			a := w.Signals.State(n.ID, 0)
			b := w.Signals.State(n.ID, 1)
			if a != sim.SignalRed && b != sim.SignalRed {
				t.Fatalf("junction %d showed %v and %v at once", n.ID, a, b)
			}
		}
		return
	}
}

func TestRedAndAmberStillStopsTraffic(t *testing.T) {
	if !sim.SignalRedAmber.MustStop() {
		t.Error("red and amber must not permit crossing the line")
	}
	if sim.SignalGreen.MustStop() {
		t.Error("green must permit crossing the line")
	}
}

func TestTrafficKeepsToItsLane(t *testing.T) {
	w := world(t)
	step(w, sim.Controls{}, 12)
	for _, a := range w.Agents {
		if a.InJunction() {
			continue
		}
		p := w.City.Project(a.V.Pos)
		if !p.Valid {
			continue
		}
		if p.Dist > w.City.RoadHalfWidth(p.Lane)+1 {
			t.Errorf("agent %d drifted %.1fm from any lane centreline", a.ID, p.Dist)
		}
	}
}

func TestTrafficObeysItsSpeedLimit(t *testing.T) {
	w := world(t)
	step(w, sim.Controls{}, 15)
	for _, a := range w.Agents {
		limit := a.SpeedLimit()
		if a.V.Speed() > limit*1.35 {
			t.Errorf("agent %d doing %.1f m/s in a %.1f m/s limit", a.ID, a.V.Speed(), limit)
		}
	}
}

func TestJudgeRecordsSpeeding(t *testing.T) {
	w := world(t)
	var got []sim.Infraction
	w.Judge.Events.Subscribe(subscriber(func(in sim.Infraction) { got = append(got, in) }))

	// Hold the throttle down well past any urban limit.
	step(w, sim.Controls{Throttle: 1}, 20)

	var speeding bool
	for _, in := range got {
		if in.Kind == sim.Speeding {
			speeding = true
		}
	}
	if !speeding {
		t.Errorf("no speeding recorded after 20s at full throttle (points=%d, faults=%d)",
			w.Judge.Points, w.Judge.Faults)
	}
	if w.Judge.Points == 0 {
		t.Error("penalty points did not accumulate")
	}
}

func TestJudgeHistoryIsBounded(t *testing.T) {
	w := world(t)
	for range sim.HistoryDepth * 3 {
		w.Judge.ReportCollision(w.Player, "test", 9, w.Time)
		w.Update(sim.Controls{}, 6) // clear the per-kind cooldown between reports
	}
	if n := len(w.Judge.Log()); n > sim.HistoryDepth {
		t.Errorf("history holds %d entries, want at most %d", n, sim.HistoryDepth)
	}
	if w.Judge.Faults <= sim.HistoryDepth {
		t.Errorf("Faults = %d, want the full count beyond the retained history", w.Judge.Faults)
	}
}

func TestResetClearsTheScore(t *testing.T) {
	w := world(t)
	w.Judge.ReportCollision(w.Player, "test", 9, 0)
	w.Judge.Reset()
	if w.Judge.Points != 0 || w.Judge.Faults != 0 || len(w.Judge.Log()) != 0 {
		t.Errorf("after reset: points=%d faults=%d log=%d, want all zero",
			w.Judge.Points, w.Judge.Faults, len(w.Judge.Log()))
	}
}

func TestCarsDoNotOverlap(t *testing.T) {
	w := world(t)
	step(w, sim.Controls{}, 10)
	for i, a := range w.Agents {
		for _, b := range w.Agents[i+1:] {
			if a.V.Box().Overlaps(b.V.Box()) {
				t.Errorf("agents %d and %d are interpenetrating", a.ID, b.ID)
			}
		}
	}
}

func TestRouteFollowsTheCar(t *testing.T) {
	w := world(t)
	step(w, sim.Controls{Throttle: 0.6}, 6)
	l := w.Route.Lane()
	if l == nil {
		t.Fatal("route has no lane after driving")
	}
	pts := w.Route.Centreline(4, 7, 6)
	if len(pts) != 6 {
		t.Fatalf("centreline returned %d points, want 6", len(pts))
	}
	// Points should march away from the car, not cluster.
	if pts[0].DistTo(pts[5]) < 10 {
		t.Errorf("centreline spans only %.1fm, want it to run ahead", pts[0].DistTo(pts[5]))
	}
}

func TestGiveWayIsTheCommonControl(t *testing.T) {
	w := world(t)
	var give, stop int
	for _, l := range w.City.Lanes {
		switch l.Control {
		case city.ControlGiveWay:
			give++
		case city.ControlStop:
			stop++
		}
	}
	if give <= stop {
		t.Errorf("give way approaches=%d, stop approaches=%d; give way should dominate", give, stop)
	}
}

// subscriber adapts a function to the telemetry subscriber interface.
type subscriber func(sim.Infraction)

func (f subscriber) OnEvent(in sim.Infraction) { f(in) }
