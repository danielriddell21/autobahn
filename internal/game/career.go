package game

import (
	"fmt"

	"github.com/danielriddell21/crucible/store"

	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// careerFile is the document the player's career is stored in, beside the
// settings.
const careerFile = "career.json"

// Career is what a player has earned across every session, which is what
// unlocks the packs.
type Career struct {
	// Banked is the bounty kept from every pursuit the player has got away
	// from at the wheel.
	Banked int `json:"banked"`
}

func careerPath() (string, error) {
	p, err := store.Path(appName, careerFile)
	if err != nil {
		return "", fmt.Errorf("locating the career file: %w", err)
	}
	return p, nil
}

// LoadCareer reads the player's career. A first run, a corrupt file and a
// deleted config directory all start from nothing.
func LoadCareer() Career {
	path, err := careerPath()
	if err != nil {
		return Career{}
	}
	c := store.Load(path, Career{})
	c.Banked = max(c.Banked, 0)
	return c
}

// Save writes the career back, reporting a failure so the player is told
// when what they earned will not be kept.
func (c Career) Save() error {
	path, err := careerPath()
	if err != nil {
		return err
	}
	if err := store.Save(path, c); err != nil {
		return fmt.Errorf("saving the career: %w", err)
	}
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

// locked reports why a session cannot be played with what the player holds,
// or nil when it can.
func (o Options) locked(a pack.Access) error {
	if p := pack.ForMap(o.Map); !a.Has(p.ID) {
		return lockedErr(p, a, o.Map.Name())
	}
	if p := pack.ForBody(o.Car); !a.Has(p.ID) {
		return lockedErr(p, a, "the "+o.Car.Name())
	}
	return nil
}

func lockedErr(p pack.Pack, a pack.Access, what string) error {
	return fmt.Errorf("%s is in the %s pack: bank %s more to unlock it, or pass -unlock-all",
		what, p.Name, money(p.Price-a.Banked))
}
