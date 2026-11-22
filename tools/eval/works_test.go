package main

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/weather"
	"github.com/danielriddell21/autobahn/internal/works"
)

// TestAutopilotGetsPastAConedLane is the whole loop on one stretch of
// roadworks: pixels in, steering out, and a lane of cones in the way. The car
// has to move over for them, and get past, without hitting any. It does it in
// about ten seconds from a minute back, standstill included; half a minute is
// the bound on how long it may wait behind them.
func TestAutopilotGetsPastAConedLane(t *testing.T) {
	for _, seed := range []uint64{5, 6, 7, 8, 9, 10} {
		w := sim.NewWorld(sim.Config{Seed: seed, Map: city.MapMotorway, Works: works.Dense})
		var lane *city.Lane
		var cl works.Closure
		for at := float32(200); at < 20000 && lane == nil; at += 130 {
			w.Time = at
			w.Update(sim.Controls{}, 0.001)
			for _, c := range w.Works.Active() {
				l := w.City.Lanes[c.Lanes[0]]
				if c.Kind == works.KindLane && c.Phase == works.PhaseSolid && c.End-at > 150 &&
					l.Along(c.Cones[0].Center) > 70 {
					lane, cl = l, c
					break
				}
			}
		}
		if lane == nil {
			continue
		}
		first := lane.Along(cl.Cones[0].Center)
		last := lane.Along(cl.Cones[len(cl.Cones)-1].Center)
		start := first - 60
		w.Player.Place(lane.Point(start), lane.HeadingAt(start))
		w.Player.Vel = lane.DirAt(start).Mul(8)
		w.Route.Update(w.Player.Pos, w.Player.Forward())
		s := newSession(w, runConfig{
			mode: autopilot.ModeNormal, perception: 20, width: 420, height: 236,
		})
		hit := 0
		w.Judge.Events.Subscribe(faultWatcher(func(in sim.Infraction) {
			if in.Kind == sim.CollisionInfraction {
				hit++
			}
		}))
		passed := false
		for range 30 * 60 {
			s.step()
			if lane.Run(w.Player.Pos) > last+8 {
				passed = true
				break
			}
		}
		if hit > 0 {
			t.Errorf("seed %d: the autopilot hit something among the cones", seed)
		}
		if !passed {
			t.Errorf("seed %d: the autopilot did not get past the cones in half a minute (at %.0f of %.0f, state %v: %s)",
				seed, lane.Run(w.Player.Pos), last, s.driver.State, s.driver.Reason)
		}
		return
	}
	t.Skip("no suitable coned lane in any of the seeds")
}

func TestWeathersNamed(t *testing.T) {
	got, err := weathersNamed("clear, storm,fog")
	if err != nil || len(got) != 3 || got[0] != weather.Held(weather.Clear) || got[1] != weather.Held(weather.Storm) {
		t.Errorf("got %v, %v", got, err)
	}
	if _, err := weathersNamed("clear,hail"); err == nil {
		t.Error("hail was accepted")
	}
	if _, err := weathersNamed(""); err == nil {
		t.Error("an empty list was accepted")
	}
}

// The gate's numbers were measured with no weather, so a run that does not ask
// for any must drive exactly as one that asks for clear; and a storm is a
// different drive.
func TestEvalDefaultsToClearAndStormDrivesDifferently(t *testing.T) {
	cfg := runConfig{
		world: city.MapCity, mode: autopilot.ModeNormal, seconds: 20, traffic: 20, police: 2,
		perception: 20, width: 420, height: 236, works: works.Off,
	}
	base := driveOne(3, cfg)
	cfg.weather = weather.Held(weather.Clear)
	if got := driveOne(3, cfg); got.distance != base.distance || got.points != base.points {
		t.Error("asking for clear weather changed the drive")
	}
	cfg.weather = weather.Held(weather.Storm)
	if got := driveOne(3, cfg); got.distance == base.distance {
		t.Error("a storm drove exactly as the dry did")
	}
}
