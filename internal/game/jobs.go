package game

import (
	"fmt"

	"github.com/danielriddell21/crucible/telemetry"

	crucihud "github.com/danielriddell21/crucible/hud"

	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// jobs is the game's end of the jobs framework: the board, and what has to be
// carried between frames to build the board's Frame from the world.
type jobs struct {
	board     job.Board
	serial    int
	faults    []job.Fault // infractions since the last step
	seenStops int
	results   []job.Result // finished, not yet collected
}

// watchJobs subscribes to the judge, as watchInfractions does. The tally lives
// here and in the job, so the judge being reset when the autopilot takes over
// cannot wipe it.
func (g *Game) watchJobs() {
	g.world.Judge.Events.Subscribe(telemetry.SubscriberFunc[sim.Infraction](
		func(in sim.Infraction) {
			if g.jobs.board.Active() != nil {
				g.jobs.faults = append(g.jobs.faults, job.Fault{Kind: in.Kind.String(), Points: in.Kind.Penalty()})
			}
		},
	))
}

// StartJob offers the player a job of a type. It says why when it cannot.
func (g *Game) StartJob(t job.Type) error {
	if g.auto && !g.opts.AutopilotJobs {
		return fmt.Errorf("the autopilot is driving")
	}
	g.jobs.serial++
	_, err := g.jobs.board.Start(t, g.world.City, g.world.City.Seed, g.jobs.serial, g.world.Player.Pos, g.world.Time)
	if err != nil {
		g.jobs.serial--
		return err
	}
	g.jobs.faults, g.jobs.seenStops = nil, g.world.Wanted.Stops
	g.notices.Post(t.Name()+" started", 150, crucihud.Diagnostic)
	return nil
}

// offerJob is the J key: a drop-off job, if none is under way.
func (g *Game) offerJob() {
	if err := g.StartJob(job.DropOff{}); err != nil {
		g.notices.Post("No job: "+err.Error(), 150, crucihud.Notice)
	}
}

// ActiveJob returns the job in progress, or nil.
func (g *Game) ActiveJob() *job.State { return g.jobs.board.Active() }

// TakeJobResults returns the jobs that have ended since it was last asked.
// Nothing is paid here: the shell decides what a result is worth.
func (g *Game) TakeJobResults() []job.Result {
	r := g.jobs.results
	g.jobs.results = nil
	return r
}

// stepJobs runs the active job one frame.
func (g *Game) stepJobs() {
	if g.jobs.board.Active() == nil {
		return
	}
	// Without the setting a job and the autopilot do not mix: it would be the
	// autopilot earning.
	if g.auto && !g.opts.AutopilotJobs {
		if r, ok := g.jobs.board.Cancel("the autopilot took over"); ok {
			g.finishJob(r)
		}
		return
	}
	p := g.world.Player
	f := job.Frame{
		Now: g.world.Time, Pos: p.Pos, Speed: p.Speed(),
		Faults:        g.jobs.faults,
		PoliceStopped: g.world.Wanted.Stops > g.jobs.seenStops,
		WantedLevel:   g.world.Wanted.Level,
		Assisted:      g.helped(),
	}
	g.jobs.faults, g.jobs.seenStops = nil, g.world.Wanted.Stops
	if r, ok := g.jobs.board.Step(f); ok {
		g.finishJob(r)
	}
}

func (g *Game) finishJob(r job.Result) {
	g.jobs.results = append(g.jobs.results, r)
	name := r.Type
	if t, ok := job.Lookup(r.Type); ok {
		name = t.Name()
	}
	if r.Status == job.Passed {
		note := name
		if r.Assisted {
			note += ": unpaid, the autopilot helped"
		} else {
			note += ": " + money(r.Pay)
		}
		g.Announce("JOB DONE", note)
		return
	}
	g.Announce("JOB FAILED", name+": "+r.Reason)
	g.bannerCol = hudBad
}
