package taxi

import (
	"sync"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Courier tuning.
const (
	// MinDrops and MaxDrops bound how many places a round delivers to.
	MinDrops = 2
	MaxDrops = 4
	// courierPace is the share of the limit the clock allows for.
	courierPace float32 = 0.6
	// courierCost is taken from the pay for each penalty point, which damage to
	// the load counts as.
	courierCost = 12
	// MinFragility and MaxFragility bound how much shaking a load stands.
	MinFragility float32 = 30
	MaxFragility float32 = 60
	// keepTracks is how many loads' tracking is kept.
	keepTracks = 16
)

// Fragility is how much damage the load of a round stands, a function of the
// round alone.
func Fragility(seed uint64, serial int) float32 {
	return MinFragility + (MaxFragility-MinFragility)*city.Chance(seed, fragSalt, serial, 0)
}

type trackKey struct {
	seed   uint64
	serial int
}

// Courier is a round of drops with a fragile load: collect it at a pick-up
// place, then stop at each drop in turn. The car's own motion damages the load,
// and the damage comes off the pay; a load that is destroyed is a failed job.
type Courier struct {
	mu    sync.Mutex
	order []trackKey
	loads map[trackKey]*Cargo
}

// NewCourier returns the courier job type.
func NewCourier() *Courier { return &Courier{loads: map[trackKey]*Cargo{}} }

var courier = NewCourier()

// All lists the job types, for registering. The courier is one shared value,
// so registering twice does not lose the tracking of a round under way.
func All() []job.Type { return []job.Type{Fare{}, Fare{Risky: true}, courier} }

// ID names the type.
func (*Courier) ID() string { return CourierID }

// Name is what the player reads.
func (*Courier) Name() string { return "Courier run" }

// Rules allow no pursuit and charge for penalty points, damage included.
func (*Courier) Rules() job.Rules { return job.Rules{CostPerPoint: courierCost} }

// Settle extends or ends the streak.
func (*Courier) Settle(r job.Result) { Couriers.settle(r) }

// load returns the tracking of a job's cargo, starting it if need be.
func (c *Courier) load(s *job.State) *Cargo {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := trackKey{s.Seed, s.Serial}
	if g, ok := c.loads[k]; ok {
		return g
	}
	g := NewCargo(Fragility(s.Seed, s.Serial))
	c.loads[k] = g
	c.order = append(c.order, k)
	if len(c.order) > keepTracks {
		delete(c.loads, c.order[0])
		c.order = c.order[1:]
	}
	return g
}

// Plan sends the player to a pick-up place and from it to two to four drops,
// each chosen from the last.
func (c *Courier) Plan(cty *city.City, seed uint64, serial int, from mathx.Vec, now float32) (job.State, bool) {
	sp, ok := pickup(cty, seed, serial, from)
	if !ok {
		return job.State{}, false
	}
	drops := MinDrops + int(city.Chance(seed, fragSalt+1, serial, 1)*float32(MaxDrops-MinDrops+1))
	drops = min(drops, MaxDrops)
	lane := cty.Lanes[sp.Lane]
	s := job.State{
		Goals:  []job.Goal{{Pos: sp.Pos, Radius: PickupRadius, Stop: true}},
		Start:  now,
		Streak: Couriers.N(),
	}
	s.Deadline = now + allow(sp.Pos.DistTo(from)*detour, courierPace, lane) + slack
	at := sp.Pos
	for i := range drops {
		t, ok := job.PickTarget(cty, seed, serial*16+2+i, at, 250, 700)
		if !ok {
			if i == 0 {
				return job.State{}, false
			}
			break
		}
		s.Goals = append(s.Goals, job.Goal{Pos: t.Pos, Radius: DropRadius, Stop: true})
		s.Deadline += t.Route/(courierPace*max(t.Lane.SpeedLimit, 1)) + slack/2
		at = t.Pos
	}
	return s, true
}

// Drops is how many places a round delivers to.
func Drops(s job.State) int { return max(len(s.Goals)-1, 0) }

// Step moves the job on and shakes the load. Until it is collected nothing
// can be damaged; after that the car's speed, frame by frame, is what does it.
func (c *Courier) Step(s *job.State, f job.Frame) {
	if s.Next >= len(s.Goals) {
		return
	}
	g := c.load(s)
	if s.Next < 1 {
		g.Skip(f.Now, f.Speed)
	} else {
		g.Feed(f.Now, f.Speed)
		s.Points += g.Take()
		if g.Destroyed() {
			s.Status, s.Reason = job.Failed, "the load was destroyed"
			return
		}
	}
	if reached(s.Goals[s.Next], f) {
		s.Next++
	}
	if s.Next >= len(s.Goals) {
		finish(s)
	}
}

// Damage is how much of its load the job has taken, from zero to one, once it
// is under way.
func (c *Courier) Damage(s job.State) float32 {
	c.mu.Lock()
	g, ok := c.loads[trackKey{s.Seed, s.Serial}]
	c.mu.Unlock()
	if !ok {
		return 0
	}
	return mathx.Clamp(g.Damage/g.Fragility, 0, 1)
}

// Reward grows with the drops and with the time the round allowed, and with the
// streak. The framework takes the cost of the penalty points from it.
func (*Courier) Reward(s job.State) int {
	pay := 100 + 70*Drops(s) + int(s.Deadline-s.Start)*2
	return pay * Multiplier(s.Streak) / 100
}
