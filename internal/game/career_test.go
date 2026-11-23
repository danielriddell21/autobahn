package game

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/sim"
)

func TestCareerSurvivesBeingSavedAndLoaded(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("AppData", dir)

	if got := LoadCareer(); got.Banked != 0 {
		t.Fatalf("a first run starts with %d banked, want nothing", got.Banked)
	}
	if err := (Career{Banked: 4321}).Save(); err != nil {
		t.Fatalf("saving: %v", err)
	}
	if got := LoadCareer(); got.Banked != 4321 {
		t.Errorf("loaded %d banked, want 4321", got.Banked)
	}
	if err := (Career{Banked: -9}).Save(); err != nil {
		t.Fatalf("saving: %v", err)
	}
	if got := LoadCareer(); got.Banked != 0 {
		t.Errorf("an edited file gave %d banked, want it held at nothing", got.Banked)
	}
}

func TestBankingReportsThePacksItUnlocks(t *testing.T) {
	price := pack.Of(pack.Coast).Price
	var c Career
	if got := c.Bank(price - 1); len(got) != 0 {
		t.Errorf("banking short of the price unlocked %v", got)
	}
	if got := c.Bank(0); len(got) != 0 || c.Banked != price-1 {
		t.Errorf("banking nothing changed the career to %d", c.Banked)
	}
	if got := c.Bank(-500); len(got) != 0 || c.Banked != price-1 {
		t.Errorf("banking a loss changed the career to %d", c.Banked)
	}
	got := c.Bank(1)
	if len(got) != 1 || got[0].ID != pack.Coast {
		t.Errorf("reaching the price unlocked %v, want the coast pack", got)
	}
}

// chased puts a pursuit in progress on the wanted state.
func chased(wa *sim.Wanted, bounty int) {
	wa.Level, wa.Bounty = 2, bounty
}

// escaped ends the pursuit the way getting away does.
func escaped(wa *sim.Wanted) {
	wa.Banked += wa.Bounty
	wa.Bounty = 0
	wa.Clear()
}

func TestAPursuitThePlayerDroveCounts(t *testing.T) {
	var m bountyMeter
	var wa sim.Wanted
	chased(&wa, 600)
	m.watch(&wa, false)
	escaped(&wa)
	m.watch(&wa, false)
	if got := m.take(); got != 600 {
		t.Errorf("counted %d, want the 600 banked", got)
	}
	if got := m.take(); got != 0 {
		t.Errorf("counted the same bounty again: %d", got)
	}
}

func TestAPursuitTheAutopilotTouchedDoesNot(t *testing.T) {
	var m bountyMeter
	var wa sim.Wanted
	// The autopilot earns it, and the player takes the wheel to bank it.
	chased(&wa, 900)
	m.watch(&wa, true)
	m.watch(&wa, false)
	escaped(&wa)
	m.watch(&wa, false)
	if got := m.take(); got != 0 {
		t.Errorf("counted %d from a pursuit the autopilot drove part of", got)
	}

	// The next one, driven throughout, counts again.
	chased(&wa, 300)
	m.watch(&wa, false)
	escaped(&wa)
	m.watch(&wa, false)
	if got := m.take(); got != 300 {
		t.Errorf("counted %d from the pursuit after, want 300", got)
	}
}

func TestBountyBankedWhileHelpedDoesNotCount(t *testing.T) {
	var m bountyMeter
	var wa sim.Wanted
	chased(&wa, 450)
	m.watch(&wa, false)
	escaped(&wa)
	// Helped on the very frame it is banked.
	m.watch(&wa, true)
	if got := m.take(); got != 0 {
		t.Errorf("counted %d banked with the autopilot driving", got)
	}
}

func TestBeingBustedCountsNothing(t *testing.T) {
	var m bountyMeter
	var wa sim.Wanted
	chased(&wa, 700)
	m.watch(&wa, false)
	wa.Bounty = 0 // forfeited
	wa.Clear()
	m.watch(&wa, false)
	if got := m.take(); got != 0 {
		t.Errorf("counted %d from a pursuit that ended in a stop", got)
	}
}

func TestASessionIsLockedOnlyByWhatThePlayerLacks(t *testing.T) {
	for _, m := range city.Maps() {
		for _, k := range sim.PlayerBodies() {
			o := DefaultOptions()
			o.Map, o.Car = m, k
			if err := o.locked(pack.Access{Bypass: true}); err != nil {
				t.Errorf("%s in the %s is locked with everything bypassed: %v", m.Name(), k.Name(), err)
			}
			base := pack.ForMap(m).ID == pack.Base && pack.ForBody(k).ID == pack.Base
			if err := o.locked(pack.Access{}); (err == nil) != base {
				t.Errorf("%s in the %s for a new player: locked = %v, want %v", m.Name(), k.Name(), err != nil, !base)
			}
		}
	}
}
