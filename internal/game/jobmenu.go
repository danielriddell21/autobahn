package game

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"

	crucihud "github.com/danielriddell21/crucible/hud"

	"github.com/danielriddell21/autobahn/internal/emergency"
	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/race"
	"github.com/danielriddell21/autobahn/internal/taxi"
)

// jobEntry is one line of the menu J opens.
type jobEntry struct {
	// ID is the job type it starts; empty for the line that cancels the job under
	// way.
	ID    string
	Label string
}

// jobMenu is the open menu.
type jobMenu struct{ entries []jobEntry }

// jobsOf is what each pack adds to the menu, in the order shown. A job of a pack
// the player does not hold is never listed.
var jobsOf = []struct {
	Pack    pack.ID
	Entries []jobEntry
}{
	{pack.Base, []jobEntry{{job.DropOffID, "Drop-off"}}},
	{pack.Taxi, []jobEntry{
		{taxi.FareID, "Taxi fare"},
		{taxi.RiskyID, "Rush fare"},
		{taxi.CourierID, "Courier run"},
	}},
	{pack.Racing, []jobEntry{
		{race.TrialID, "Time trial"},
		{race.CircuitID, "Circuit race"},
		{race.SprintID, "Street race"},
		{race.DriftID, "Drift event"},
	}},
	{pack.Emergency, []jobEntry{
		{emergency.AmbulanceID, "Ambulance call"},
		{emergency.FireID, "Fire call"},
		{emergency.ResponseID, "Response call"},
	}},
}

// jobEntries is the menu for a player holding a. While a job is under way the
// only thing to offer is giving it up.
func jobEntries(a pack.Access, active bool) []jobEntry {
	if active {
		return []jobEntry{{Label: "Cancel the current job"}}
	}
	var out []jobEntry
	for _, p := range jobsOf {
		if p.Pack == pack.Base || a.Has(p.Pack) {
			out = append(out, p.Entries...)
		}
	}
	return out
}

// access is what the player holds, with the command line's -unlock-all added.
func (g *Game) access() pack.Access {
	a := g.opts.Access
	a.Bypass = a.Bypass || g.opts.UnlockAll
	return a
}

// offerJob is the J key: it opens the menu of jobs, or shuts it.
func (g *Game) offerJob() {
	if g.jobMenu != nil {
		g.jobMenu = nil
		return
	}
	g.jobMenu = &jobMenu{entries: jobEntries(g.access(), g.jobs.board.Active() != nil)}
}

// stepJobMenu reads the keys while the menu is open: a number picks a line,
// escape or J shuts it.
func (g *Game) stepJobMenu() {
	m := g.jobMenu
	switch {
	case rl.IsKeyPressed(rl.KeyEscape), rl.IsKeyPressed(rl.KeyJ):
		g.jobMenu = nil
		return
	}
	for i := range min(len(m.entries), 9) {
		if rl.IsKeyPressed(int32(rl.KeyOne) + int32(i)) {
			g.jobMenu = nil
			g.pickJob(m.entries[i])
			return
		}
	}
}

// pickJob starts the job a line names, or cancels the one under way.
func (g *Game) pickJob(e jobEntry) {
	if e.ID == "" {
		s := g.jobs.board.Active()
		if s == nil {
			return
		}
		// A fare turned down before it boards is not a failure; see taxi.Run.
		reason := "cancelled"
		if (s.Type == taxi.FareID || s.Type == taxi.RiskyID) && !taxi.Boarded(*s) {
			reason = taxi.Declined
		}
		if r, ok := g.jobs.board.Cancel(reason); ok {
			g.finishJob(r)
		}
		return
	}
	t, ok := job.Lookup(e.ID)
	if !ok {
		g.notices.Post("No job: "+e.Label+" is not available", 150, crucihud.Notice)
		return
	}
	// A rush fare is only on offer at some places and times; with none, there is
	// no job.
	if e.ID == taxi.RiskyID && !taxi.Offered(g.world.City.Seed, g.jobs.serial+1, g.world.Player.Pos) {
		g.notices.Post("No rush fare on offer here", 150, crucihud.Notice)
		return
	}
	if err := g.StartJob(t); err != nil {
		g.notices.Post("No job: "+err.Error(), 150, crucihud.Notice)
	}
}

// drawJobMenu draws the open menu.
func (g *Game) drawJobMenu() {
	m := g.jobMenu
	if m == nil {
		return
	}
	w, h := int32(260), int32(40+22*len(m.entries))
	x, y := int32(g.opts.Width)/2-w/2, int32(g.opts.Height)/2-h/2
	panel(x, y, w, h)
	rl.DrawText("JOBS", x+12, y+8, 14, hudAccent)
	for i, e := range m.entries {
		rl.DrawText(fmt.Sprintf("%d  %s", i+1, e.Label), x+12, y+32+int32(i)*22, 16, hudText)
	}
}
