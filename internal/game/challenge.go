package game

import (
	"fmt"
	"strings"

	"github.com/danielriddell21/autobahn/internal/scenario"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/weather"
	"github.com/danielriddell21/autobahn/internal/works"
)

// A challenge is a scenario being driven against the autopilot's ghost. The
// ghost is a number from the checked-in table, never a second car. The host
// builds the world from [challengeConfig] and calls [challenge.step] each frame
// with whether anything but the player is driving; once helped the run cannot
// pass, which is why this is not a job: a job's pay is only withheld, where here
// the autopilot sets the score and has no business collecting the medal.
//
// A challenge retries at once: [challenge.retry] clears the run, and the host
// rebuilds the world from the same config, which is the same place because a
// place is a function of its seed.
type challenge struct {
	run   *scenario.Run
	ghost int
	// reported is set once the result has been handed out, so it is only once.
	reported bool
}

// newChallenge starts an attempt at a scenario. A scenario with no ghost
// checked in is judged against the floor.
func newChallenge(sc scenario.Scenario) *challenge {
	return &challenge{run: scenario.NewRun(sc), ghost: scenario.Ghosts()[sc.ID].Score}
}

// challengeConfig is the world a scenario is driven in.
func challengeConfig(sc scenario.Scenario) sim.Config {
	level, _ := works.ParseLevel(sc.Rules.Works)
	sky, _ := weather.ParseSetting(sc.Rules.Weather)
	return sim.Config{
		Seed: sc.Seed, Map: sc.CityMap(), Traffic: sc.Rules.Traffic, Police: sc.Rules.Police,
		Works: level, Weather: sky,
	}
}

// step feeds the run a frame from the world.
func (c *challenge) step(w *sim.World, helped bool, dt float32) {
	c.run.Step(scenario.Frame{
		Dt: dt, Distance: w.Judge.Distance, Points: w.Judge.Points, Faults: w.Judge.Faults,
		Helped: helped,
	})
}

// retry throws the attempt away. The host rebuilds the world to match.
func (c *challenge) retry() {
	c.run.Retry()
	c.reported = false
}

// finished returns the outcome once, on the step the run ends.
func (c *challenge) finished() (scenario.Outcome, bool) {
	if !c.run.Done() || c.reported {
		return scenario.Outcome{}, false
	}
	c.reported = true
	return c.run.Outcome(c.ghost), true
}

// readout is the lines the HUD shows while a challenge is under way.
func (c *challenge) readout() []string {
	_, _, gold := scenario.Thresholds(c.ghost)
	lines := []string{
		fmt.Sprintf("%s  %ds left", c.run.Scenario().Name, int(c.run.Remaining())+1),
		fmt.Sprintf("score %d  ghost %d", c.run.Score(), c.ghost),
	}
	if c.run.Helped() {
		return append(lines, "autopilot helped: no medal")
	}
	return append(lines, fmt.Sprintf("gold at %d", gold))
}

// verdict words an outcome for the banner.
func verdict(o scenario.Outcome) (title, note string) {
	note = fmt.Sprintf("%d against the ghost's %d", o.Score, o.Ghost)
	if !o.Passed {
		return "CHALLENGE FAILED", note + ": " + o.Reason
	}
	return "CHALLENGE: " + strings.ToUpper(o.Medal.String()), note
}

// stepChallenge feeds the challenge, if there is one, a frame from the world and
// announces its outcome the step it ends.
func (g *Game) stepChallenge(dt float32) {
	if g.chal == nil {
		return
	}
	g.chal.step(g.world, g.helped(), dt)
	if o, ok := g.chal.finished(); ok {
		title, note := verdict(o)
		g.Announce(title, note)
		if !o.Passed {
			g.bannerCol = hudBad
		}
		g.chalDone = &o
	}
}

// TakeChallenge returns the outcome of a challenge that has finished since it
// was last asked, once. The shell records it.
func (g *Game) TakeChallenge() (scenario.Outcome, bool) {
	if g.chalDone == nil {
		return scenario.Outcome{}, false
	}
	o := *g.chalDone
	g.chalDone = nil
	return o, true
}
