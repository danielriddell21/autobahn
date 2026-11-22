// Package feats is the achievements: what a player has done at the wheel,
// counted beside the bounty career, and the list of things that counts toward.
//
// It is pure. A Tracker is fed Facts, plain values saying what happened since
// it was last fed, and answers with the ids it newly unlocked. Nothing here
// reads the world, the clock or the disk, so every rule is a table row in a
// test.
//
// Only a drive the human took the whole way counts, as with bounty: a Fact
// marked Helped (the autopilot had a hand in it, or it is chase or co-op) or
// Custom (a custom map or sandbox) earns nothing, and is not counted.
//
// Nothing here pays. An achievement may unlock a secret paint, the one thing
// the shop never sells, and that is all.
package feats

import (
	"math/bits"
	"slices"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/weather"
)

// Group is a heading the list is sorted under.
type Group int

// The groups, in the order the list shows them.
const (
	Driving Group = iota
	Police
	Collection
	numGroups
)

// Name words a group for a heading.
func (g Group) Name() string {
	switch g {
	case Police:
		return "police and bounty"
	case Collection:
		return "collection"
	default:
		return "driving"
	}
}

// Groups returns every group, in order.
func Groups() []Group {
	out := make([]Group, 0, numGroups)
	for g := range numGroups {
		out = append(out, g)
	}
	return out
}

// NoPaint is Feat.Paint for an achievement that unlocks nothing.
const NoPaint = -1

// RacingGreen is the paint index of british racing green, in the game's
// paints. The shop never sells it (shop.Secret), which is what lets an
// achievement have it.
const RacingGreen = 5

// Feat is one achievement.
type Feat struct {
	// ID is what is saved, and so never changes once shipped.
	ID    string
	Group Group
	Name  string
	// Note says how it is earned. A Hidden feat's is not shown until it is.
	Note string
	// Hidden feats are left out of the list until they are earned.
	Hidden bool
	// Paint is the secret paint it unlocks, or NoPaint.
	Paint int
}

// Thresholds. Distances are metres.
const (
	firstMile    = 1609
	marathon     = 42195
	longHaul     = 500_000
	cleanShort   = 2000
	cleanLong    = 10_000
	redsInARow   = 25
	getaways     = 10
	bountySmall  = 1000
	bountyLarge  = 10_000
	wantedHeavy  = 3
	wantedTop    = 5
	blocksRunTen = 10
)

// All is every achievement, in the order the list shows them within a group.
var all = []Feat{
	{ID: "first-mile", Group: Driving, Name: "first mile", Note: "drive a mile", Paint: NoPaint},
	{ID: "marathon", Group: Driving, Name: "marathon", Note: "drive 26 miles", Paint: NoPaint},
	{ID: "long-haul", Group: Driving, Name: "long haul", Note: "drive 500 km in all", Paint: NoPaint},
	{ID: "clean-2k", Group: Driving, Name: "clean sheet", Note: "drive 2 km without a fault", Paint: NoPaint},
	{ID: "clean-10k", Group: Driving, Name: "model driver", Note: "drive 10 km without a fault", Paint: NoPaint},
	{ID: "red-discipline", Group: Driving, Name: "red means stop", Note: "stop at 25 red lights in a row", Paint: NoPaint},

	{ID: "first-escape", Group: Police, Name: "got away", Note: "escape a pursuit", Paint: NoPaint},
	{ID: "getaway-driver", Group: Police, Name: "getaway driver", Note: "escape ten pursuits", Paint: NoPaint},
	{ID: "roadblock", Group: Police, Name: "through the cordon", Note: "get through a roadblock", Paint: NoPaint},
	{ID: "wanted-3", Group: Police, Name: "persons of interest", Note: "reach wanted level three", Paint: NoPaint},
	{ID: "bounty-1k", Group: Police, Name: "small change", Note: "bank 1,000 bounty", Paint: NoPaint},
	{ID: "bounty-10k", Group: Police, Name: "most wanted", Note: "bank 10,000 bounty", Paint: NoPaint},
	{ID: "wanted-5", Group: Police, Name: "public enemy", Note: "reach wanted level five", Hidden: true, Paint: NoPaint},
	{ID: "cordon-ten", Group: Police, Name: "unstoppable", Note: "get through ten roadblocks", Hidden: true, Paint: NoPaint},

	{ID: "all-seasons", Group: Collection, Name: "all seasons", Note: "drive in every season", Paint: NoPaint},
	{ID: "all-weather", Group: Collection, Name: "come rain or shine", Note: "drive in every kind of weather", Paint: NoPaint},
	{ID: "all-cars", Group: Collection, Name: "the full garage", Note: "drive every car", Paint: NoPaint},
	{ID: "all-packs", Group: Collection, Name: "all the packs", Note: "drive on a map from every pack", Paint: NoPaint},
	{ID: "all-maps", Group: Collection, Name: "grand tour", Note: "drive every place", Hidden: true, Paint: RacingGreen},
}

// List returns every achievement, grouped in order.
func List() []Feat {
	out := slices.Clone(all)
	slices.SortStableFunc(out, func(a, b Feat) int { return int(a.Group) - int(b.Group) })
	return out
}

// Get returns the achievement with the given id.
func Get(id string) (Feat, bool) {
	for _, f := range all {
		if f.ID == id {
			return f, true
		}
	}
	return Feat{}, false
}

// Fact is what happened in a stretch of one drive, as plain values. Counts
// and Distance are since the last Fact, so a drive is fed as it goes; the
// peak and the context are as they stand.
type Fact struct {
	// Distance is metres driven.
	Distance float32
	// Faults is rule-breaking since the last Fact: a collision, a red run, a
	// kerb mounted, a speeding ticket. Any fault ends a clean run.
	Faults int
	// RedsStopped and RedsRun are the red lights stopped at and gone through.
	RedsStopped, RedsRun int
	// Escapes are pursuits got away from, Roadblocks ones got through.
	Escapes, Roadblocks int
	// Banked is bounty banked, the same figure bounty counts.
	Banked int
	// WantedPeak is the highest wanted level reached, as it stands.
	WantedPeak int

	// Map and Car are city.Map and sim.BodyKind as ints, Season a city.Season
	// and Weather a weather.Cond. Pack is the pack the map belongs to; only a pack that owns a map can be set.
	Map, Car, Season, Weather int
	Pack                      int

	// Helped says anything but the human drove, or the session never counts.
	Helped bool
	// Custom says it was a custom map or a sandbox.
	Custom bool
}

// Feats is what is stored: what has been earned and the running totals the
// next achievement needs. Its zero value is a player who has done nothing.
type Feats struct {
	Unlocked []string `json:"unlocked,omitempty"`

	Distance   float64 `json:"distance,omitempty"` // metres
	Clean      float64 `json:"clean,omitempty"`    // metres since the last fault
	BestClean  float64 `json:"best_clean,omitempty"`
	Reds       int     `json:"reds,omitempty"` // stopped at in a row
	BestReds   int     `json:"best_reds,omitempty"`
	Escapes    int     `json:"escapes,omitempty"`
	Roadblocks int     `json:"roadblocks,omitempty"`
	Bounty     int     `json:"bounty,omitempty"`
	Peak       int     `json:"peak,omitempty"`

	// The collections are bit sets indexed by map, car, season, weather and
	// pack.
	Maps    uint32 `json:"maps,omitempty"`
	Cars    uint32 `json:"cars,omitempty"`
	Seasons uint32 `json:"seasons,omitempty"`
	Weather uint32 `json:"weather,omitempty"`
	Packs   uint32 `json:"packs,omitempty"`
}

// Has reports whether an achievement is earned.
func (f Feats) Has(id string) bool { return slices.Contains(f.Unlocked, id) }

// full is the bit set with the first n bits set.
func full(n int) uint32 { return uint32(1)<<uint(n) - 1 }

func bit(i int) uint32 {
	if i < 0 || i >= 32 {
		return 0
	}
	return 1 << uint(i)
}

// carMask is the set of cars the player may drive.
func carMask() uint32 {
	var m uint32
	for _, k := range sim.PlayerBodies() {
		m |= bit(int(k))
	}
	return m
}

// The sizes of the collections.
func mapMask() uint32     { return full(len(city.Maps())) }
func seasonMask() uint32  { return full(len(city.Seasons())) }
func weatherMask() uint32 { return full(len(weather.Conds())) }
func packMask() uint32    { return mapPacks() }

// mapPacks is the set of packs that own a map. A drive is in a pack by the map
// it is on, which is all a Fact says of it, so the packs that add only cars or
// events cannot be driven in and are not part of the collection.
func mapPacks() uint32 {
	var m uint32
	for _, p := range pack.All() {
		if len(p.Maps) > 0 {
			m |= bit(int(p.ID))
		}
	}
	return m
}

// Sanitise brings a stored Feats back into range: no unknown or duplicate
// ids, nothing negative, no bits for things that do not exist. It never
// grants anything.
func (f *Feats) Sanitise() {
	ids := make([]string, 0, len(f.Unlocked))
	for _, id := range f.Unlocked {
		if _, ok := Get(id); ok && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	f.Unlocked = ids
	if len(ids) == 0 {
		f.Unlocked = nil
	}
	f.Distance = max(f.Distance, 0)
	f.Clean = max(f.Clean, 0)
	f.BestClean = max(f.BestClean, f.Clean, 0)
	f.Reds = max(f.Reds, 0)
	f.BestReds = max(f.BestReds, f.Reds)
	f.Escapes = max(f.Escapes, 0)
	f.Roadblocks = max(f.Roadblocks, 0)
	f.Bounty = max(f.Bounty, 0)
	f.Peak = min(max(f.Peak, 0), wantedTop)
	f.Maps &= mapMask()
	f.Cars &= carMask()
	f.Seasons &= seasonMask()
	f.Weather &= weatherMask()
	f.Packs &= packMask()
}

// Progress counts what is collected out of what there is, for a list to show.
func (f Feats) Progress(id string) (have, of int) {
	count := func(set, all uint32) (int, int) { return bits.OnesCount32(set), bits.OnesCount32(all) }
	switch id {
	case "all-maps":
		return count(f.Maps, mapMask())
	case "all-cars":
		return count(f.Cars, carMask())
	case "all-seasons":
		return count(f.Seasons, seasonMask())
	case "all-weather":
		return count(f.Weather, weatherMask())
	case "all-packs":
		return count(f.Packs, packMask())
	}
	return 0, 0
}

// Tracker counts facts and unlocks achievements.
type Tracker struct {
	F Feats
}

// NewTracker starts a tracker from what was stored.
func NewTracker(f Feats) *Tracker {
	f.Sanitise()
	return &Tracker{F: f}
}

// Add counts a stretch of a drive and returns the achievements it newly
// unlocked, in list order. A helped or custom drive earns, and counts,
// nothing.
func (t *Tracker) Add(x Fact) []Feat {
	if x.Helped || x.Custom {
		return nil
	}
	f := &t.F
	d := float64(max(x.Distance, 0))
	f.Distance += d
	f.Escapes += max(x.Escapes, 0)
	f.Roadblocks += max(x.Roadblocks, 0)
	f.Bounty += max(x.Banked, 0)
	f.Peak = max(f.Peak, min(x.WantedPeak, wantedTop))

	// A fault ends the clean run, and the distance falls on the far side of
	// it: the stretch is counted as clean only if it had no fault in it.
	if x.Faults > 0 {
		f.Clean = 0
	} else {
		f.Clean += d
	}
	f.BestClean = max(f.BestClean, f.Clean)

	// A red run ends the streak. Stops are counted before it, so a stretch
	// that stopped and then ran leaves the streak at nothing.
	f.Reds += max(x.RedsStopped, 0)
	if x.RedsRun > 0 {
		f.Reds = 0
	}
	f.BestReds = max(f.BestReds, f.Reds)

	// What was driven in counts only once something has been driven.
	if d > 0 {
		f.Maps |= bit(x.Map)
		f.Cars |= bit(x.Car)
		f.Seasons |= bit(x.Season)
		f.Weather |= bit(x.Weather)
		f.Packs |= bit(x.Pack)
	}
	f.Sanitise()

	var got []Feat
	for _, a := range List() {
		if !f.Has(a.ID) && t.earned(a.ID) {
			f.Unlocked = append(f.Unlocked, a.ID)
			got = append(got, a)
		}
	}
	return got
}

// rules says, for each achievement, whether the totals meet it.
var rules = map[string]func(f Feats) bool{
	"first-mile":     func(f Feats) bool { return f.Distance >= firstMile },
	"marathon":       func(f Feats) bool { return f.Distance >= marathon },
	"long-haul":      func(f Feats) bool { return f.Distance >= longHaul },
	"clean-2k":       func(f Feats) bool { return f.BestClean >= cleanShort },
	"clean-10k":      func(f Feats) bool { return f.BestClean >= cleanLong },
	"red-discipline": func(f Feats) bool { return f.BestReds >= redsInARow },
	"first-escape":   func(f Feats) bool { return f.Escapes >= 1 },
	"getaway-driver": func(f Feats) bool { return f.Escapes >= getaways },
	"roadblock":      func(f Feats) bool { return f.Roadblocks >= 1 },
	"cordon-ten":     func(f Feats) bool { return f.Roadblocks >= blocksRunTen },
	"wanted-3":       func(f Feats) bool { return f.Peak >= wantedHeavy },
	"wanted-5":       func(f Feats) bool { return f.Peak >= wantedTop },
	"bounty-1k":      func(f Feats) bool { return f.Bounty >= bountySmall },
	"bounty-10k":     func(f Feats) bool { return f.Bounty >= bountyLarge },
	"all-maps":       func(f Feats) bool { return f.Maps&mapMask() == mapMask() },
	"all-cars":       func(f Feats) bool { return f.Cars&carMask() == carMask() },
	"all-seasons":    func(f Feats) bool { return f.Seasons&seasonMask() == seasonMask() },
	"all-weather":    func(f Feats) bool { return f.Weather&weatherMask() == weatherMask() },
	"all-packs":      func(f Feats) bool { return f.Packs&packMask() == packMask() },
}

// earned says whether the totals meet an achievement.
func (t *Tracker) earned(id string) bool {
	rule, ok := rules[id]
	return ok && rule(t.F)
}

// Visible reports whether the list shows an achievement: all of them, except
// a hidden one not yet earned.
func (f Feats) Visible(a Feat) bool { return !a.Hidden || f.Has(a.ID) }

// Paints returns the secret paints the earned achievements unlock, once each.
func (f Feats) Paints() []int {
	var out []int
	for _, a := range all {
		if a.Paint != NoPaint && f.Has(a.ID) && !slices.Contains(out, a.Paint) {
			out = append(out, a.Paint)
		}
	}
	return out
}
