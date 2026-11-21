package pack_test

import (
	"slices"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/sim"
)

func TestTheBaseGameIsNeverLocked(t *testing.T) {
	none := pack.Access{}
	if !none.Has(pack.Base) {
		t.Fatal("a player with nothing banked does not have the base game")
	}
	if !none.HasMap(city.MapCity) {
		t.Error("the city is locked to a new player")
	}
	if !none.HasBody(sim.HotHatch) {
		t.Error("the hot hatch is locked to a new player")
	}
	if len(none.Maps()) == 0 || len(none.Bodies()) == 0 {
		t.Error("a new player is offered no map or no car")
	}
}

func TestPacksUnlockInOrderOfPrice(t *testing.T) {
	last := -1
	for _, p := range pack.All() {
		if p.Price <= last {
			t.Errorf("%s costs %d, no more than the pack before it", p.Name, p.Price)
		}
		last = p.Price
		if p.ID == pack.Base {
			continue
		}
		if (pack.Access{Banked: p.Price - 1}).Has(p.ID) {
			t.Errorf("%s is held a point short of its price", p.Name)
		}
		if !(pack.Access{Banked: p.Price}).Has(p.ID) {
			t.Errorf("%s is not held at its price", p.Name)
		}
	}
}

func TestBypassHoldsEverything(t *testing.T) {
	a := pack.Access{Bypass: true}
	for _, p := range pack.All() {
		if !a.Has(p.ID) {
			t.Errorf("bypass does not hold the %s", p.Name)
		}
		if a.Progress(p.ID) != 1 {
			t.Errorf("bypass shows the %s as unfinished", p.Name)
		}
	}
	if !slices.Equal(a.Maps(), city.Maps()) {
		t.Errorf("bypass offers maps %v, want all of %v", a.Maps(), city.Maps())
	}
	if !slices.Equal(a.Bodies(), sim.PlayerBodies()) {
		t.Errorf("bypass offers cars %v, want all of %v", a.Bodies(), sim.PlayerBodies())
	}
}

// Every map and every car the player may drive has to come from exactly one
// pack, or something would be either locked twice or offered to nobody.
func TestEverythingBelongsToOnePack(t *testing.T) {
	maps, bodies := map[city.Map]int{}, map[sim.BodyKind]int{}
	for _, p := range pack.All() {
		for _, m := range p.Maps {
			maps[m]++
			if !m.Valid() {
				t.Errorf("%s lists map %d, which does not exist", p.Name, m)
			}
			if pack.ForMap(m).ID != p.ID {
				t.Errorf("ForMap(%s) is not the %s that lists it", m.Name(), p.Name)
			}
		}
		for _, k := range p.Bodies {
			bodies[k]++
			if !sim.IsPlayerBody(k) {
				t.Errorf("%s lists the %s, which the player cannot drive", p.Name, k.Name())
			}
			if pack.ForBody(k).ID != p.ID {
				t.Errorf("ForBody(%s) is not the %s that lists it", k.Name(), p.Name)
			}
		}
	}
	for m, n := range maps {
		if n > 1 {
			t.Errorf("map %s is in %d packs", m.Name(), n)
		}
	}
	for k, n := range bodies {
		if n > 1 {
			t.Errorf("the %s is in %d packs", k.Name(), n)
		}
	}
	for _, m := range city.Maps() {
		if maps[m] == 0 && pack.ForMap(m).ID != pack.Base {
			t.Errorf("map %s is in no pack and not in the base game", m.Name())
		}
	}
}

func TestProgressRunsFromNothingToOne(t *testing.T) {
	price := pack.Of(pack.Coast).Price
	if got := (pack.Access{}).Progress(pack.Coast); got != 0 {
		t.Errorf("progress with nothing banked = %v, want 0", got)
	}
	if got := (pack.Access{Banked: price / 2}).Progress(pack.Coast); got < 0.49 || got > 0.51 {
		t.Errorf("progress at half the price = %v, want a half", got)
	}
	if got := (pack.Access{Banked: price * 3}).Progress(pack.Coast); got != 1 {
		t.Errorf("progress past the price = %v, want 1", got)
	}
	if got := (pack.Access{Banked: -50}).Progress(pack.Coast); got != 0 {
		t.Errorf("progress on a negative total = %v, want 0", got)
	}
}

func TestNewlyReportsWhatBankingUnlocked(t *testing.T) {
	coast, country := pack.Of(pack.Coast).Price, pack.Of(pack.Country).Price
	if got := pack.Newly(0, coast-1); len(got) != 0 {
		t.Errorf("banking short of the first price unlocked %v", got)
	}
	if got := pack.Newly(coast-1, coast); len(got) != 1 || got[0].ID != pack.Coast {
		t.Errorf("reaching the first price unlocked %v, want the coast pack", got)
	}
	if got := pack.Newly(coast, coast+10); len(got) != 0 {
		t.Errorf("banking past a price already met unlocked %v again", got)
	}
	got := pack.Newly(0, country)
	if len(got) != 2 || got[0].ID != pack.Coast || got[1].ID != pack.Country {
		t.Errorf("one big haul unlocked %v, want coast then country", got)
	}
}

func TestOfAnUnknownPackIsTheBaseGame(t *testing.T) {
	if got := pack.Of(pack.ID(77)); got.ID != pack.Base {
		t.Errorf("Of(77) = %s, want the base game", got.Name)
	}
}
