package game

import (
	"slices"
	"testing"

	crucihud "github.com/danielriddell21/crucible/hud"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/emergency"
	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/race"
	"github.com/danielriddell21/autobahn/internal/taxi"
)

func ids(es []jobEntry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.ID)
	}
	return out
}

func TestJobMenuOffersWhatThePlayerHolds(t *testing.T) {
	price := func(id pack.ID) int { return pack.Of(id).Price }
	cases := []struct {
		name string
		a    pack.Access
		want []string
		not  []string
	}{
		{
			name: "nothing", a: pack.Access{}, want: []string{job.DropOffID},
			not: []string{taxi.FareID, race.TrialID, emergency.FireID},
		},
		{
			name: "taxi", a: pack.Access{Banked: price(pack.Taxi)},
			want: []string{job.DropOffID, taxi.FareID, taxi.RiskyID, taxi.CourierID},
			not:  []string{race.DriftID},
		},
		{
			// The prices are lifetime totals, so racing implies taxi.
			name: "racing", a: pack.Access{Banked: price(pack.Racing)},
			want: []string{job.DropOffID, race.TrialID, race.CircuitID, race.SprintID, race.DriftID},
		},
		{
			name: "everything", a: pack.Access{Bypass: true},
			want: []string{job.DropOffID, taxi.CourierID, race.SprintID, emergency.AmbulanceID, emergency.FireID, emergency.ResponseID},
		},
	}
	for _, c := range cases {
		got := ids(jobEntries(c.a, false))
		for _, w := range c.want {
			if !slices.Contains(got, w) {
				t.Errorf("%s: %s is not offered: %v", c.name, w, got)
			}
		}
		for _, n := range c.not {
			if slices.Contains(got, n) {
				t.Errorf("%s: %s is offered but locked", c.name, n)
			}
		}
	}
}

func TestJobMenuWhileAJobIsUnderWayOffersOnlyToCancel(t *testing.T) {
	es := jobEntries(pack.Access{Bypass: true}, true)
	if len(es) != 1 || es[0].ID != "" {
		t.Errorf("entries %v", es)
	}
}

func TestEveryJobTheMenuNamesIsRegistered(t *testing.T) {
	for _, c := range registered {
		if c.Jobs != nil {
			c.Jobs()
		}
	}
	for _, p := range jobsOf {
		for _, e := range p.Entries {
			if _, ok := job.Lookup(e.ID); !ok {
				t.Errorf("%s is in the menu but not registered", e.ID)
			}
		}
	}
}

func TestJKeyOpensAndShutsTheMenuAndUnlockAllBypasses(t *testing.T) {
	g := bareGame(Options{UnlockAll: true})
	g.offerJob()
	if g.jobMenu == nil || !slices.Contains(ids(g.jobMenu.entries), race.DriftID) {
		t.Fatal("-unlock-all did not offer the racing pack's jobs")
	}
	g.offerJob()
	if g.jobMenu != nil {
		t.Error("the menu did not shut")
	}
	g = bareGame(Options{})
	g.offerJob()
	if slices.Contains(ids(g.jobMenu.entries), taxi.FareID) {
		t.Error("a locked pack's job was offered")
	}
}

func TestCancellingAFareBeforeItBoardsKeepsTheStreak(t *testing.T) {
	for _, c := range registered {
		if c.Jobs != nil {
			c.Jobs()
		}
	}
	g := bareGame(Options{})
	g.notices = crucihud.New()
	taxi.Fares.Reset()
	if err := g.StartJob(taxi.Fare{}); err != nil {
		t.Skip(err)
	}
	g.pickJob(jobEntry{})
	rs := g.TakeJobResults()
	if len(rs) != 1 || rs[0].Reason != taxi.Declined {
		t.Errorf("results %+v", rs)
	}
	if g.ActiveJob() != nil {
		t.Error("the job was not cancelled")
	}
}

// A custom map banks nothing, so what its jobs paid is thrown away unpaid.
func TestCustomGamePaysNoJobMoney(t *testing.T) {
	spec := custom.FromMap(city.MapCity, 3)
	if bareGame(Options{Custom: &spec}).banks() {
		t.Error("a custom game banks, so job pay would reach the career")
	}
}
