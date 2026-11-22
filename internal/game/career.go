package game

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"github.com/danielriddell21/crucible/store"

	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/shop"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// careerFile is the document the player's career is stored in, beside the
// settings.
const careerFile = "career.json"

// careerVersion is the schema of the career file. A file with no version is
// version 0, from before there was a shop. Raise it, and append a migration to
// careerMigrations, whenever the file's meaning changes.
const careerVersion = 1

// Career is what a player has earned and owns across every session: the
// bounty that unlocks the packs, and the earnings that buy from the shop.
//
// Cars and Paints are stored as integers: a car is its sim.BodyKind and a
// paint its index in playerPaints. Both lists may only grow at the end, or
// every saved file would change meaning.
type Career struct {
	Version int `json:"version"`
	// Banked is the bounty kept from every pursuit the player has got away
	// from at the wheel. It is a lifetime total and is never spent.
	Banked int `json:"banked"`
	// Earnings is the purse the shop spends from.
	Earnings int `json:"earnings"`
	// Cars and Paints are what the player owns.
	Cars   []int `json:"cars"`
	Paints []int `json:"paints"`
}

// careerMigrations[i] brings a career from version i to i+1. They run in
// order, then the result is sanitised.
var careerMigrations = []func(c *Career, set Settings){
	// 0 -> 1: the shop arrives. Whatever the garage had chosen is kept, so
	// nobody loses the car they were driving.
	func(c *Career, set Settings) {
		c.Cars = append(c.Cars, int(sim.HotHatch), int(set.Car))
		c.Paints = append(c.Paints, 0, set.Paint)
	},
}

// careerSanitisers are extra repairs other parts of the career file add from
// their own files (the achievements' sub-struct, for one), run at the end of
// sanitise. Append to it from an init function.
var careerSanitisers []func(c *Career)

func careerPath() (string, error) {
	p, err := store.Path(appName, careerFile)
	if err != nil {
		return "", fmt.Errorf("locating the career file: %w", err)
	}
	return p, nil
}

// LoadCareer reads the player's career, migrating an older file using the
// stored settings. A first run, a corrupt file and a deleted config directory
// all start from the starter car and paint alone.
func LoadCareer() Career { return loadCareerFor(LoadSettings()) }

func loadCareerFor(set Settings) Career {
	c := Career{Version: careerVersion}
	if path, err := careerPath(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			var read Career
			if json.Unmarshal(data, &read) == nil {
				c = read
			}
		}
	}
	return migrate(c, set)
}

// migrate brings a career read from disk to the current version and cleans it.
func migrate(c Career, set Settings) Career {
	for v := max(c.Version, 0); v < careerVersion; v++ {
		careerMigrations[v](&c, set)
	}
	c.Version = careerVersion
	c.sanitise()
	return c
}

// sanitise brings a career back into range: no negatives, no unknown or
// duplicate ids, and always the starter car and paint.
func (c *Career) sanitise() {
	c.Banked = max(c.Banked, 0)
	c.Earnings = max(c.Earnings, 0)
	c.Cars = cleanIDs(append([]int{int(sim.HotHatch)}, c.Cars...),
		func(i int) bool { return sim.IsPlayerBody(sim.BodyKind(i)) })
	c.Paints = cleanIDs(append([]int{0}, c.Paints...),
		func(i int) bool { return i >= 0 && i < len(playerPaints) })
	for _, f := range careerSanitisers {
		f(c)
	}
}

// cleanIDs keeps the valid ids, once each, in the order first seen.
func cleanIDs(ids []int, valid func(int) bool) []int {
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if valid(id) && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// Save writes the career back, reporting a failure so the player is told
// when what they earned will not be kept.
func (c Career) Save() error {
	path, err := careerPath()
	if err != nil {
		return err
	}
	c.Version = careerVersion
	if err := store.Save(path, c); err != nil {
		return fmt.Errorf("saving the career: %w", err)
	}
	return nil
}

// Earn adds to the earnings the shop spends. Anything not positive is ignored.
func (c *Career) Earn(n int) {
	if n > 0 {
		c.Earnings += n
	}
}

// holdings is what the career owns, under the access it is asked with.
func (c Career) holdings(a pack.Access) shop.Holdings {
	return shop.Holdings{Access: a, Earnings: c.Earnings, Cars: c.Cars, Paints: c.Paints}
}

// BuyCar spends earnings on a car, if the shop's rules allow it.
func (c *Career) BuyCar(a pack.Access, k sim.BodyKind) error {
	it := shop.CarItem(k)
	if err := c.holdings(a).CanBuy(it); err != nil {
		return err
	}
	c.Earnings -= it.Price()
	c.Cars = append(c.Cars, int(k))
	return nil
}

// BuyPaint spends earnings on a paint, if the shop's rules allow it.
func (c *Career) BuyPaint(a pack.Access, i int) error {
	it := shop.PaintItem(i)
	if err := c.holdings(a).CanBuy(it); err != nil {
		return err
	}
	c.Earnings -= it.Price()
	c.Paints = append(c.Paints, i)
	return nil
}

// Bank adds bounty to the career and returns the packs it unlocked.
func (c *Career) Bank(bounty int) []pack.Pack {
	if bounty <= 0 {
		return nil
	}
	before := c.Banked
	c.Banked += bounty
	return pack.Newly(before, c.Banked)
}

// bountyMeter counts the bounty a session banks that the player earned
// themselves.
//
// A pursuit counts only if the player drove all of it. One the autopilot had
// a hand in is marked the moment it does, and stays marked until the police
// have lost interest, so the car cannot be left to earn a bounty and taken
// back in time to bank it.
type bountyMeter struct {
	seen     int // the world's banked total when last looked at
	assisted bool
	earned   int // counted but not yet collected
}

// watch looks at the pursuit for one frame. helped says whether anything but
// the player is driving, or the session is one that never counts.
func (m *bountyMeter) watch(wa *sim.Wanted, helped bool) {
	if helped && (wa.Active() || wa.Bounty > 0) {
		m.assisted = true
	}
	if gain := wa.Banked - m.seen; gain > 0 && !m.assisted && !helped {
		m.earned += gain
	}
	m.seen = wa.Banked
	if !wa.Active() && wa.Bounty == 0 {
		m.assisted = false
	}
}

// take returns the bounty counted since it was last asked, and forgets it.
func (m *bountyMeter) take() int {
	e := m.earned
	m.earned = 0
	return e
}

// locked reports why a session cannot be played with the packs the player
// holds, or nil when it can. It knows nothing of ownership; see lockedFor.
func (o Options) locked(a pack.Access) error {
	if p := pack.ForMap(o.Map); !a.Has(p.ID) {
		return lockedErr(p, a, o.Map.Name())
	}
	if p := pack.ForBody(o.Car); !a.Has(p.ID) {
		return lockedErr(p, a, "the "+o.Car.Name())
	}
	return nil
}

// lockedFor is locked and more: a car or paint the player has not bought is
// refused too, unless the access bypasses everything.
func (o Options) lockedFor(h shop.Holdings) error {
	if err := o.locked(h.Access); err != nil {
		return err
	}
	if !h.Drivable(o.Car) {
		return fmt.Errorf("the %s is not yours: buy it in the garage, or pass -unlock-all", o.Car.Name())
	}
	if !h.Wearable(o.Paint) {
		return fmt.Errorf("the %s paint is not yours: buy it in the garage, or pass -unlock-all", paintOf(o.Paint).name)
	}
	return nil
}

func lockedErr(p pack.Pack, a pack.Access, what string) error {
	return fmt.Errorf("%s is in the %s pack: bank %s more to unlock it, or pass -unlock-all",
		what, p.Name, money(p.Price-a.Banked))
}
