package main

import (
	"fmt"
	"os"
	"sync"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/scenario"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/weather"
	"github.com/danielriddell21/autobahn/internal/works"
)

// ghostConfig is the headless harness set up for a scenario's rules. The
// camera and perception are the defaults of the eval flags, so a ghost is the
// same autopilot the gate scores.
func ghostConfig(sc scenario.Scenario) runConfig {
	level, _ := works.ParseLevel(sc.Rules.Works)
	sky, _ := weather.ParseSetting(sc.Rules.Weather)
	return runConfig{
		world: sc.CityMap(), mode: autopilot.ModeNormal, seconds: float32(sc.Rules.Seconds),
		traffic: sc.Rules.Traffic, police: sc.Rules.Police, perception: 20,
		width: 420, height: 236, works: level, weather: sky,
	}
}

// ghostOf drives the autopilot through a scenario, judged by the same
// scenario.Run a player's attempt is, and returns what it scored.
func ghostOf(sc scenario.Scenario) scenario.Ghost {
	cfg := ghostConfig(sc)
	w := sim.NewWorld(sim.Config{
		Seed: sc.Seed, Map: cfg.world, Traffic: cfg.traffic, Police: cfg.police,
		Works: cfg.works, Weather: cfg.weather,
	})
	s := newSession(w, cfg)
	run := scenario.NewRun(sc)
	for !run.Done() {
		s.step()
		// The ghost is the reference: it is not helped, it is the help.
		run.Step(scenario.Frame{
			Dt: fixedStep, Distance: w.Judge.Distance, Points: w.Judge.Points, Faults: w.Judge.Faults,
		})
	}
	return scenario.Ghost{
		Score: run.Score(), Distance: run.Distance(), Points: run.Points(), Faults: run.Faults(),
	}
}

// ghostsFor drives every scenario, in parallel.
func ghostsFor(list []scenario.Scenario, workers int) map[string]scenario.Ghost {
	out := make(map[string]scenario.Ghost, len(list))
	var mu sync.Mutex
	var wg sync.WaitGroup
	work := make(chan scenario.Scenario)
	for range max(workers, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sc := range work {
				g := ghostOf(sc)
				mu.Lock()
				out[sc.ID] = g
				mu.Unlock()
			}
		}()
	}
	for _, sc := range list {
		work <- sc
	}
	close(work)
	wg.Wait()
	return out
}

// writeGhosts writes a ghost table to path. A -ghost flag in main is meant to
// call it with ghostsFor(scenario.All(), runtime.NumCPU()).
func writeGhosts(path string, ghosts map[string]scenario.Ghost) error {
	data, err := scenario.EncodeGhosts(ghosts)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	fmt.Printf("wrote %d ghosts to %s\n", len(ghosts), path)
	return nil
}
