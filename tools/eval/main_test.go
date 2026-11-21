package main

import (
	"math/rand/v2"
	"testing"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// crawler is a driver who will not go faster than a fraction of the limit and
// never gets out of anybody's way.
func crawler(frac float32) sim.Style {
	st := sim.Cautious()
	st.Name, st.SpeedBias, st.Overtake, st.Patience = "crawler", frac, 0, 1e6
	st.PassesOncoming = false
	return st
}

// behindCrawler builds an empty city, puts the autopilot's car near the start of
// the longest straight lane of the given class and a crawler in front of it,
// and returns the session and the crawler.
func behindCrawler(t *testing.T, cls city.RoadClass, kerbside bool, mode autopilot.Mode) (*session, *sim.Agent, *city.Lane) {
	t.Helper()
	for _, seed := range []uint64{7, 3, 11, 19, 23, 29, 31} {
		w := sim.NewWorld(sim.Config{Seed: seed})
		var best *city.Lane
		for _, l := range w.City.Lanes {
			if l.Class != cls || w.City.Roads[l.Road].Ring || len(l.Succ) == 0 {
				continue
			}
			if kerbside != (l.Index == cls.Lanes()-1) {
				continue
			}
			if best == nil || l.Length > best.Length {
				best = l
			}
		}
		if best == nil || best.Length < 105 {
			continue
		}
		w.Player.Place(best.Point(4), best.Heading)
		w.Player.Vel = best.Fwd.Mul(6)
		w.Route.Update(w.Player.Pos, w.Player.Forward())

		a := sim.NewAgent(1, w.City, best, 22, rand.New(rand.NewPCG(seed, 1)))
		a.Style = crawler(0.3)
		a.V.Vel = best.Fwd.Mul(4)
		w.Agents = append(w.Agents, a)

		s := newSession(w, runConfig{mode: mode, perception: 20, width: 420, height: 236})
		return s, a, best
	}
	t.Skip("no city in the seeds tried has a long enough lane")
	return nil, nil, nil
}

// drivePast runs the autopilot until it has got past the crawler or reached the
// end of the road, and reports whether it passed and the states it went
// through.
func drivePast(s *session, a *sim.Agent, l *city.Lane, seconds float32) (bool, map[autopilot.State]bool) {
	states := map[autopilot.State]bool{}
	for t := float32(0); t < seconds; t += fixedStep {
		s.step()
		states[s.driver.State] = true
		p := s.w.Player.Pos
		if p.Sub(l.A).Dot(l.Fwd) > l.Length {
			break // into the junction
		}
		if p.Sub(a.V.Pos).Dot(l.Fwd) > 6 {
			return true, states
		}
	}
	return false, states
}

func TestAutopilotPassesOnASingleCarriageway(t *testing.T) {
	s, a, l := behindCrawler(t, city.Collector, true, autopilot.ModeAggressive)
	passed, states := drivePast(s, a, l, 30)
	if !states[autopilot.StateOvertaking] {
		t.Fatalf("an aggressive autopilot never pulled out round a car at a third of the limit (states %v)", states)
	}
	if !passed {
		t.Error("the overtake did not get past the crawler")
	}
	if s.w.Judge.Faults != 0 {
		t.Errorf("the overtake broke the rules: %v", s.w.Judge.Log())
	}
}

func TestNormalAutopilotStaysOnItsOwnSide(t *testing.T) {
	s, a, l := behindCrawler(t, city.Collector, true, autopilot.ModeNormal)
	_, states := drivePast(s, a, l, 20)
	if states[autopilot.StateOvertaking] {
		t.Error("the normal autopilot overtook on a single carriageway")
	}
}

func TestAutopilotChangesLaneToPass(t *testing.T) {
	s, a, l := behindCrawler(t, city.Arterial, true, autopilot.ModeBrisk)
	passed, states := drivePast(s, a, l, 30)
	if !states[autopilot.StateChangingLane] {
		t.Fatalf("a brisk autopilot never moved out to pass a car at a third of the limit (states %v)", states)
	}
	if !passed {
		t.Error("the lane change did not get it past the crawler")
	}
	if s.w.Judge.Faults != 0 {
		t.Errorf("the lane change broke the rules: %v", s.w.Judge.Log())
	}
}

func TestMapsNamedReadsOneMapOrAllOfThem(t *testing.T) {
	all, err := mapsNamed("all")
	if err != nil || len(all) != len(city.Maps()) {
		t.Fatalf("all = %v, %v; want every map", all, err)
	}
	for _, m := range city.Maps() {
		got, err := mapsNamed(m.Name())
		if err != nil || len(got) != 1 || got[0] != m {
			t.Errorf("mapsNamed(%q) = %v, %v", m.Name(), got, err)
		}
	}
	if _, err := mapsNamed("atlantis"); err == nil {
		t.Error("a map that does not exist should be refused")
	}
}
