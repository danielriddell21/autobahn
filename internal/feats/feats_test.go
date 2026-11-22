package feats

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/weather"
)

func ids(fs []Feat) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.ID)
	}
	return out
}

// run feeds facts to a fresh tracker and returns it and the ids it unlocked.
func run(facts ...Fact) (*Tracker, []string) {
	t := NewTracker(Feats{})
	var got []string
	for _, x := range facts {
		got = append(got, ids(t.Add(x))...)
	}
	return t, got
}

func TestEachUnlocksAndNotEarly(t *testing.T) {
	tests := []struct {
		id      string
		short   []Fact // just short: must not unlock it
		enough  []Fact // must unlock it
		exclude bool
	}{
		{"first-mile", []Fact{{Distance: firstMile - 1}}, []Fact{{Distance: firstMile - 1}, {Distance: 1}}, false},
		{"marathon", []Fact{{Distance: marathon - 1}}, []Fact{{Distance: marathon}}, false},
		{"long-haul", []Fact{{Distance: longHaul - 1}}, []Fact{{Distance: longHaul / 2}, {Distance: longHaul / 2}}, false},
		{"clean-2k", []Fact{{Distance: 1500}, {Distance: 499}}, []Fact{{Distance: 1500}, {Distance: 500}}, false},
		{"clean-2k", []Fact{{Distance: 1500}, {Distance: 100, Faults: 1}, {Distance: 1500}}, []Fact{{Distance: 1500, Faults: 1}, {Distance: 2000}}, false},
		{"clean-10k", []Fact{{Distance: 9999}}, []Fact{{Distance: 5000}, {Distance: 5000}}, false},
		{"red-discipline", []Fact{{RedsStopped: 24}}, []Fact{{RedsStopped: 24}, {RedsStopped: 1}}, false},
		{"red-discipline", []Fact{{RedsStopped: 24}, {RedsRun: 1}, {RedsStopped: 24}}, []Fact{{RedsRun: 1}, {RedsStopped: 25}}, false},
		{"first-escape", nil, []Fact{{Escapes: 1}}, false},
		{"getaway-driver", []Fact{{Escapes: 9}}, []Fact{{Escapes: 9}, {Escapes: 1}}, false},
		{"roadblock", nil, []Fact{{Roadblocks: 1}}, false},
		{"cordon-ten", []Fact{{Roadblocks: 9}}, []Fact{{Roadblocks: 10}}, false},
		{"wanted-3", []Fact{{WantedPeak: 2}}, []Fact{{WantedPeak: 3}}, false},
		{"wanted-5", []Fact{{WantedPeak: 4}}, []Fact{{WantedPeak: 5}}, false},
		{"bounty-1k", []Fact{{Banked: 999}}, []Fact{{Banked: 600}, {Banked: 400}}, false},
		{"bounty-10k", []Fact{{Banked: 9999}}, []Fact{{Banked: 10000}}, false},
	}
	for _, tt := range tests {
		if _, ok := Get(tt.id); !ok {
			t.Fatalf("%s is not defined", tt.id)
		}
		if tr, _ := run(tt.short...); tr.F.Has(tt.id) {
			t.Errorf("%s unlocked early by %+v", tt.id, tt.short)
		}
		if tr, _ := run(tt.enough...); !tr.F.Has(tt.id) {
			t.Errorf("%s not unlocked by %+v", tt.id, tt.enough)
		}
	}
}

// collect drives one stretch of 100 m for each value, varying one field.
func collect(n int, set func(*Fact, int)) []Fact {
	var out []Fact
	for i := range n {
		x := Fact{Distance: 100}
		set(&x, i)
		out = append(out, x)
	}
	return out
}

// mapPackIDs are the packs a drive can be in: those that own a map.
var mapPackIDs = func() (ids []int) {
	for _, p := range pack.All() {
		if len(p.Maps) > 0 {
			ids = append(ids, int(p.ID))
		}
	}
	return
}()

func TestCollections(t *testing.T) {
	cars := sim.PlayerBodies()
	tests := []struct {
		id    string
		facts []Fact
	}{
		{"all-maps", collect(len(city.Maps()), func(x *Fact, i int) { x.Map = i })},
		{"all-seasons", collect(len(city.Seasons()), func(x *Fact, i int) { x.Season = i })},
		{"all-weather", collect(len(weather.Conds()), func(x *Fact, i int) { x.Weather = i })},
		{"all-packs", collect(len(mapPackIDs), func(x *Fact, i int) { x.Pack = mapPackIDs[i] })},
		{"all-cars", collect(len(cars), func(x *Fact, i int) { x.Car = int(cars[i]) })},
	}
	for _, tt := range tests {
		if tr, _ := run(tt.facts[:len(tt.facts)-1]...); tr.F.Has(tt.id) {
			t.Errorf("%s unlocked one short", tt.id)
		}
		if tr, _ := run(tt.facts...); !tr.F.Has(tt.id) {
			t.Errorf("%s not unlocked by the full set", tt.id)
		}
		// The same things without moving are not drives.
		still := slices.Clone(tt.facts)
		for i := range still {
			still[i].Distance = 0
		}
		if tr, _ := run(still...); tr.F.Has(tt.id) {
			t.Errorf("%s unlocked without driving", tt.id)
		}
	}
}

func TestHelpedAndCustomEarnNothing(t *testing.T) {
	big := Fact{Distance: longHaul, Escapes: 20, Roadblocks: 20, Banked: 20000, WantedPeak: 5, RedsStopped: 30}
	for name, mod := range map[string]func(*Fact){
		"helped": func(x *Fact) { x.Helped = true },
		"custom": func(x *Fact) { x.Custom = true },
	} {
		x := big
		mod(&x)
		tr, got := run(x)
		if len(got) != 0 || !reflectZero(tr.F) {
			t.Errorf("%s drive earned %v / counted %+v", name, got, tr.F)
		}
	}
	// The same drive by hand does earn.
	if _, got := run(big); len(got) == 0 {
		t.Error("an unassisted drive earned nothing")
	}
}

func reflectZero(f Feats) bool {
	b, _ := json.Marshal(f)
	return string(b) == "{}"
}

func TestUnlockedOnce(t *testing.T) {
	tr := NewTracker(Feats{})
	if got := tr.Add(Fact{Escapes: 1}); len(got) != 1 || got[0].ID != "first-escape" {
		t.Fatalf("first = %v", ids(got))
	}
	if got := tr.Add(Fact{Escapes: 1}); len(got) != 0 {
		t.Errorf("unlocked again: %v", ids(got))
	}
}

func TestSecretUnlocksPaint(t *testing.T) {
	if got := (Feats{}).Paints(); len(got) != 0 {
		t.Errorf("nothing earned unlocks %v", got)
	}
	f := Feats{Unlocked: []string{"all-maps"}}
	if got := f.Paints(); !slices.Equal(got, []int{RacingGreen}) {
		t.Errorf("Paints = %v", got)
	}
	// Only secret things are given, never money: a Feat has no such field,
	// and every paint it names is one a feat holds.
	for _, a := range all {
		if a.Paint != NoPaint && !a.Hidden {
			t.Errorf("%s gives a secret paint but is not hidden", a.ID)
		}
	}
}

func TestHiddenUntilEarned(t *testing.T) {
	a, _ := Get("wanted-5")
	if (Feats{}).Visible(a) || !(Feats{Unlocked: []string{"wanted-5"}}).Visible(a) {
		t.Error("hidden feat visibility wrong")
	}
	b, _ := Get("first-mile")
	if !(Feats{}).Visible(b) {
		t.Error("ordinary feat hidden")
	}
}

func TestSanitise(t *testing.T) {
	f := Feats{
		Unlocked: []string{"first-mile", "nonsense", "first-mile", "all-maps"},
		Distance: -5, Escapes: -1, Peak: 99, Maps: ^uint32(0), Cars: ^uint32(0),
	}
	f.Sanitise()
	if !slices.Equal(f.Unlocked, []string{"first-mile", "all-maps"}) {
		t.Errorf("Unlocked = %v", f.Unlocked)
	}
	if f.Distance != 0 || f.Escapes != 0 || f.Peak != wantedTop {
		t.Errorf("not clamped: %+v", f)
	}
	if f.Maps != mapMask() || f.Cars != carMask() {
		t.Errorf("stray bits kept: %b %b", f.Maps, f.Cars)
	}
}

func TestListIsConsistent(t *testing.T) {
	seen := map[string]bool{}
	last := Group(0)
	for _, a := range List() {
		if seen[a.ID] || a.ID == "" || a.Name == "" || a.Note == "" {
			t.Errorf("bad or repeated feat %+v", a)
		}
		seen[a.ID] = true
		if a.Group < last {
			t.Errorf("%s out of group order", a.ID)
		}
		last = a.Group
		if _, ok := rules[a.ID]; !ok {
			t.Errorf("%s has no rule", a.ID)
		}
		if (&Tracker{}).earned(a.ID) {
			t.Errorf("%s earned by a player who has done nothing", a.ID)
		}
	}
}

func TestJSONRoundTrip(t *testing.T) {
	tr, _ := run(Fact{Distance: 3000, Escapes: 2, Banked: 1200, WantedPeak: 3, RedsStopped: 4, Map: 1, Season: 2})
	b, err := json.Marshal(tr.F)
	if err != nil {
		t.Fatal(err)
	}
	var back Feats
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(back.Unlocked, tr.F.Unlocked) || back.Distance != tr.F.Distance ||
		back.Maps != tr.F.Maps || back.Bounty != tr.F.Bounty || back.BestReds != 4 {
		t.Errorf("round trip changed it: %+v -> %+v", tr.F, back)
	}
	if string(mustJSON(t, Feats{})) != "{}" {
		t.Error("the zero value should marshal to nothing")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A drive is in a pack by its map, so driving every map must be enough to earn
// all-packs: nothing in a Fact can name a pack that owns no map.
func TestAllPacksIsEarnedByDrivingEveryMap(t *testing.T) {
	var facts []Fact
	for _, m := range city.Maps() {
		facts = append(facts, Fact{Distance: 100, Map: int(m), Pack: int(pack.ForMap(m).ID)})
	}
	tr, _ := run(facts...)
	if !tr.F.Has("all-packs") {
		t.Errorf("driving every map did not earn all-packs: packs %b", tr.F.Packs)
	}
	if have, of := tr.F.Progress("all-packs"); have != of || of != len(mapPackIDs) {
		t.Errorf("progress %d of %d, want %d of %d", have, of, len(mapPackIDs), len(mapPackIDs))
	}
}
