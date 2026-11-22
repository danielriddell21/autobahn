package taxi

import (
	"sync"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// The IDs of the job types.
const (
	FareID    = "fare"
	RiskyID   = "fare-risky"
	CourierID = "courier"
)

// Reasons a job ends with. A job passed with no penalty points ends
// [Faultless], which is what extends a streak; a player who turns a fare down
// before boarding tells the board [Declined], which is not a failure.
const (
	Faultless = "faultless"
	Declined  = "declined"
)

// Fare tuning, in metres, seconds and metres a second.
const (
	// PickupRadius and DropRadius are how close the car has to come to rest.
	PickupRadius float32 = 6
	DropRadius   float32 = 8
	// StopSpeed is the speed counted as at rest.
	StopSpeed float32 = 0.5
	// farePace and riskyPace are the shares of the limit the clock allows for:
	// a rush fare gives no time to dawdle, and is not met inside the limit.
	farePace  float32 = 0.55
	riskyPace float32 = 0.95
	slack     float32 = 30
	// A straight line is shorter than the road, so the clock is given more.
	detour float32 = 1.4
	// riskBonus is the extra pay for a rush fare, as a percentage of its fare.
	riskBonus = 60
	// riskyOdds is the share of fares that come with a rush offer.
	riskyOdds = 0.3
	// fareBase is the pay before the length of the run is counted.
	fareBase = 80
)

// Run is a streak of jobs passed without a fault in a row. A job failed, or
// passed with a fault, ends it. It is for the whole session and is only ever
// worth anything as a bonus on the pay of the next.
type Run struct {
	mu sync.Mutex
	n  int
}

// N is how many have been passed in a row.
func (s *Run) N() int { s.mu.Lock(); defer s.mu.Unlock(); return s.n }

// Reset ends the streak.
func (s *Run) Reset() { s.mu.Lock(); s.n = 0; s.mu.Unlock() }

// settle takes in how a job ended. A pass with the autopilot's help counts for
// nothing either way, and a fare turned down before boarding is not a failure.
func (s *Run) settle(r job.Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.Assisted:
	case r.Status == job.Passed && r.Reason == Faultless:
		s.n++
	case r.Status == job.Failed && r.Reason == Declined:
	default:
		s.n = 0
	}
}

// Fares and Couriers are the player's streaks.
var (
	Fares    Run
	Couriers Run
)

// Fare is a passenger waiting at the kerb. The player stops at the pick-up
// marker, the fare boards, and is driven to the drop-off. A fare that is Risky
// asks for a rush, and pays a bonus for it as earnings; the wanted level it
// may bring comes only from the police seeing a rule broken.
type Fare struct{ Risky bool }

// Name is what the player reads.
func (f Fare) Name() string {
	if f.Risky {
		return "Rush fare"
	}
	return "Fare"
}

// ID names the type.
func (f Fare) ID() string {
	if f.Risky {
		return RiskyID
	}
	return FareID
}

func (f Fare) pace() float32 {
	if f.Risky {
		return riskyPace
	}
	return farePace
}

// Rules charge for penalty points, a rush fare less because the rush is what
// was asked, and a rush fare survives a first level of pursuit.
func (f Fare) Rules() job.Rules {
	if f.Risky {
		return job.Rules{MaxWanted: 1, CostPerPoint: 5}
	}
	return job.Rules{CostPerPoint: 10}
}

// Settle extends or ends the streak.
func (Fare) Settle(r job.Result) { Fares.settle(r) }

// Offered reports whether a fare at a place comes with a rush offer. It is the
// host's to put to the player, who may decline it and take the ordinary fare.
func Offered(seed uint64, serial int, from mathx.Vec) bool {
	return city.Chance(seed^uint64(serial)*0x9e3779b97f4a7c15, riskSalt, cellOf(from.X), cellOf(from.Z)) < riskyOdds
}

// allow is how long a distance takes at a pace on a lane.
func allow(dist, pace float32, l *city.Lane) float32 {
	return dist / (pace * max(l.SpeedLimit, 1))
}

// Plan sends the player to a pick-up place, and from it to a drop-off.
func (f Fare) Plan(c *city.City, seed uint64, serial int, from mathx.Vec, now float32) (job.State, bool) {
	sp, ok := pickup(c, seed, serial, from)
	if !ok {
		return job.State{}, false
	}
	t, ok := job.PickTarget(c, seed, serial*16+1, sp.Pos, 350, 900)
	if !ok {
		return job.State{}, false
	}
	pace := f.pace()
	lane := c.Lanes[sp.Lane]
	limit := max(t.Lane.SpeedLimit, 1)
	return job.State{
		Goals: []job.Goal{
			{Pos: sp.Pos, Radius: PickupRadius, Stop: true},
			{Pos: t.Pos, Radius: DropRadius, Stop: true},
		},
		Start:    now,
		Deadline: now + allow(sp.Pos.DistTo(from)*detour, pace, lane) + t.Route/(pace*limit) + slack,
		Streak:   Fares.N(),
	}, true
}

// Boarded reports whether a job is a fare, or a courier's round, that has been
// picked up.
func Boarded(s job.State) bool {
	switch s.Type {
	case FareID, RiskyID, CourierID:
		return s.Next >= 1 && s.Status == job.Active
	}
	return false
}

// Step moves the job on: a goal is reached by coming to rest inside it. The
// first is the pick-up, which boards the fare, and the second the drop-off.
func (Fare) Step(s *job.State, f job.Frame) {
	if s.Next >= len(s.Goals) {
		return
	}
	if reached(s.Goals[s.Next], f) {
		s.Next++
	}
	if s.Next >= len(s.Goals) {
		finish(s)
	}
}

func reached(g job.Goal, f job.Frame) bool {
	return f.Pos.DistTo(g.Pos) <= g.Radius && f.Speed < StopSpeed
}

// finish passes a job, marking it faultless if it took no penalty points.
func finish(s *job.State) {
	s.Status = job.Passed
	if s.Points == 0 {
		s.Reason = Faultless
	}
}

// base is the fare before the tip, which grows with the length of the run. It
// is worked out as though every fare ran at the ordinary pace, so a rush fare
// is not paid less for being given less time.
func (f Fare) base(s job.State) int {
	return fareBase + int(2*(s.Deadline-s.Start)*f.pace()/farePace)
}

// Reward is the fare and its tip, scaled by the streak, and a rush fare's bonus
// on top.
func (f Fare) Reward(s job.State) int {
	base := f.base(s)
	if f.Risky {
		base += base * riskBonus / 100
	}
	return Reward(base, base/2, s.Points, f.Rules().CostPerPoint, s.Streak)
}
