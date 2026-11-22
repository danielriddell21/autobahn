package game

import (
	"testing"

	crucihud "github.com/danielriddell21/crucible/hud"
	"github.com/danielriddell21/crucible/netplay"

	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// jobGame is a game with a world and the jobs wiring, and no window.
func jobGame(t *testing.T) *Game {
	t.Helper()
	g := &Game{
		world:   sim.NewWorld(sim.Config{Seed: 3, Traffic: 0, Police: 0}),
		notices: crucihud.New(),
		net:     &netplay.Session[Snapshot, Input]{},
	}
	g.watchJobs()
	return g
}

func TestJobStartsAndRefusesASecond(t *testing.T) {
	g := jobGame(t)
	if err := g.StartJob(job.DropOff{}); err != nil {
		t.Fatal(err)
	}
	if g.ActiveJob() == nil {
		t.Fatal("no active job")
	}
	if err := g.StartJob(job.DropOff{}); err == nil {
		t.Fatal("a second job was started")
	}
}

func TestJobTimesOutAndQueuesResult(t *testing.T) {
	g := jobGame(t)
	if err := g.StartJob(job.DropOff{}); err != nil {
		t.Fatal(err)
	}
	g.world.Time = g.ActiveJob().Deadline + 1
	g.stepJobs()
	if g.ActiveJob() != nil {
		t.Fatal("job outlived its deadline")
	}
	r := g.TakeJobResults()
	if len(r) != 1 || r[0].Status != job.Failed || r[0].Pay != 0 {
		t.Fatalf("results %+v", r)
	}
	if len(g.TakeJobResults()) != 0 {
		t.Fatal("results not cleared once taken")
	}
}

func TestJobPassesAtGoalAndPays(t *testing.T) {
	g := jobGame(t)
	if err := g.StartJob(job.DropOff{}); err != nil {
		t.Fatal(err)
	}
	g.world.Player.Pos = g.ActiveJob().Goals[0].Pos
	g.stepJobs()
	r := g.TakeJobResults()
	if len(r) != 1 || r[0].Status != job.Passed || r[0].Pay <= 0 {
		t.Fatalf("results %+v", r)
	}
}

func TestJobFaultsAreTalliedAcrossAJudgeReset(t *testing.T) {
	g := jobGame(t)
	if err := g.StartJob(job.DropOff{}); err != nil {
		t.Fatal(err)
	}
	g.world.Judge.Events.Publish(sim.Infraction{Kind: sim.Speeding})
	g.stepJobs()
	g.world.Judge.Reset()
	if g.ActiveJob().Points != sim.Speeding.Penalty() {
		t.Fatalf("points %d, want %d", g.ActiveJob().Points, sim.Speeding.Penalty())
	}
}

func TestJobPoliceStopFails(t *testing.T) {
	g := jobGame(t)
	if err := g.StartJob(job.DropOff{}); err != nil {
		t.Fatal(err)
	}
	g.world.Wanted.Stops++
	g.stepJobs()
	if r := g.TakeJobResults(); len(r) != 1 || r[0].Status != job.Failed {
		t.Fatalf("results %+v", r)
	}
}

func TestAutopilotCancelsJobUnlessAllowed(t *testing.T) {
	g := jobGame(t)
	if err := g.StartJob(job.DropOff{}); err != nil {
		t.Fatal(err)
	}
	g.auto = true
	g.stepJobs()
	if g.ActiveJob() != nil {
		t.Fatal("job survived the autopilot")
	}
	if err := g.StartJob(job.DropOff{}); err == nil {
		t.Fatal("a job started under the autopilot")
	}
}

func TestAutopilotJobsStaysAssistedAndUnpaid(t *testing.T) {
	g := jobGame(t)
	g.opts.AutopilotJobs = true
	if err := g.StartJob(job.DropOff{}); err != nil {
		t.Fatal(err)
	}
	g.auto = true
	g.stepJobs()
	g.auto = false
	g.stepJobs()
	if s := g.ActiveJob(); s == nil || !s.Assisted {
		t.Fatalf("state %+v", s)
	}
	g.world.Player.Pos = g.ActiveJob().Goals[0].Pos
	g.stepJobs()
	if r := g.TakeJobResults(); len(r) != 1 || r[0].Status != job.Passed || r[0].Pay != 0 {
		t.Fatalf("results %+v", r)
	}
}
