package sim

import (
	"math/rand/v2"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Route is a navigation plan for the player's car: which lane it is in and
// which way it will go at the next junction.
//
// This is a satnav, not a perception system. Choosing a direction at a
// junction is a routing decision, and the autopilot is allowed to have one in
// the same way a real car has a planned route. Everything the autopilot does
// with that route still goes through the camera: the lane centreline is drawn
// into the annotated image and must be visually re-detected.
type Route struct {
	c    *city.City
	rng  *rand.Rand
	lane *city.Lane
	s    float32

	next    city.Turn
	hasNext bool
}

// NewRoute returns a route that will latch onto the nearest lane on its first
// update.
func NewRoute(c *city.City, rng *rand.Rand) *Route {
	return &Route{c: c, rng: rng}
}

// Lane returns the lane the route is currently following, or nil.
func (r *Route) Lane() *city.Lane { return r.lane }

// DistanceToJunction returns how far ahead the current lane ends.
func (r *Route) DistanceToJunction() float32 {
	if r.lane == nil {
		return 1e6
	}
	return r.lane.Length - r.s
}

// NextTurn returns the manoeuvre planned at the next junction.
func (r *Route) NextTurn() (city.TurnKind, bool) {
	if !r.hasNext {
		return city.TurnStraight, false
	}
	return r.next.Kind, true
}

// Update re-locates the route from the car's current pose. It follows the car
// rather than driving it, so a human who turns the "wrong" way simply gets a
// re-planned route.
func (r *Route) Update(pos, fwd mathx.Vec) {
	proj := r.c.Project(pos)
	if !proj.Valid {
		return
	}
	switch {
	case r.lane == nil:
		r.adopt(proj.Lane)
	case proj.Lane == r.lane:
		// Still on plan.
	case r.hasNext && proj.Lane == r.c.Lanes[r.next.Lane]:
		r.adopt(proj.Lane) // the junction has been crossed
	case proj.Lane.Fwd.Dot(fwd) > 0.3:
		// The car is somewhere else entirely and pointing along a different
		// lane, so re-plan from there.
		r.adopt(proj.Lane)
	case r.strayed(pos):
		// The car has left its planned lane far enough that keeping the plan is
		// worse than losing it: the route would keep pointing back at a lane the
		// car is no longer on, and everything downstream reads from that. Take
		// the nearest lane even though the heading disagrees, and re-plan.
		r.adopt(proj.Lane)
	}
	if r.lane != nil {
		r.s = mathx.Clamp(pos.Sub(r.lane.A).Dot(r.lane.Fwd), 0, r.lane.Length)
	}
}

// strayRange is how far off its planned lane the car may drift before the plan
// is abandoned, in metres. It is wider than a carriageway so an ordinary lane
// change does not trigger it.
const strayRange float32 = 11

func (r *Route) strayed(pos mathx.Vec) bool {
	s := mathx.Clamp(pos.Sub(r.lane.A).Dot(r.lane.Fwd), 0, r.lane.Length)
	return pos.DistTo(r.lane.A.Add(r.lane.Fwd.Mul(s))) > strayRange
}

func (r *Route) adopt(l *city.Lane) {
	r.lane = l
	r.pick()
}

func (r *Route) pick() {
	// Prefer to carry straight on, which keeps the driven line predictable.
	if len(r.lane.Succ) == 0 {
		r.hasNext = false
		return
	}
	var straight []city.Turn
	for _, t := range r.lane.Succ {
		if t.Kind == city.TurnStraight {
			straight = append(straight, t)
		}
	}
	if len(straight) > 0 && r.rng.Float32() < 0.7 {
		r.next = straight[r.rng.IntN(len(straight))]
	} else {
		r.next = r.lane.Succ[r.rng.IntN(len(r.lane.Succ))]
	}
	r.hasNext = true
}

// Centreline returns up to count points spaced along the route ahead of the
// car, starting at the given offset. These are the points the vision system
// draws as lane markers.
func (r *Route) Centreline(offset, spacing float32, count int) []mathx.Vec {
	if r.lane == nil {
		return nil
	}
	pts := make([]mathx.Vec, 0, count)
	for i := range count {
		pts = append(pts, r.ahead(offset+float32(i)*spacing))
	}
	return pts
}

// aheadHops bounds how many lanes a lookahead may cross. The circulating
// carriageway of a roundabout is cut into chords a few metres long, so a
// lookahead across one spends most of its hops there. This covers the furthest
// anything looks — a roadblock, set blockAhead metres up the road — and is
// otherwise only here to stop a ring being walked forever.
const aheadHops = 48

func (r *Route) ahead(d float32) mathx.Vec {
	// Walk lane by lane. Stopping after one hop, as this used to, is enough on
	// a street but not on a roundabout: every marker past the second chord
	// would clamp to the same point, and a camera shown one dot where a lane
	// should be has lost the lane.
	lane, s := r.lane, r.s
	turn, hasTurn := r.next, r.hasNext
	for range aheadHops {
		remain := lane.Length - s
		if d <= remain {
			return lane.Point(s + d)
		}
		d -= remain
		if !hasTurn {
			return lane.Point(lane.Length)
		}
		if d <= turn.Len {
			return r.c.TurnPoint(lane, turn, d/max(turn.Len, 0.01))
		}
		d -= turn.Len
		lane, s = r.c.Lanes[turn.Lane], 0
		turn, hasTurn = onward(lane)
	}
	return lane.Point(d)
}

// onward is the successor a lookahead takes past a lane the route has not
// planned for yet: straight on where that is offered, and the first turn
// otherwise. It reads only the lane, because the markers are redrawn every
// frame and a choice that varied between frames would make them crawl.
func onward(l *city.Lane) (city.Turn, bool) {
	if len(l.Succ) == 0 {
		return city.Turn{}, false
	}
	for _, t := range l.Succ {
		if t.Kind == city.TurnStraight {
			return t, true
		}
	}
	return l.Succ[0], true
}

// StopLine returns the position of the stop line the car is approaching, the
// control governing it and whether one applies within lookahead metres.
func (r *Route) StopLine(lookahead float32) (mathx.Vec, city.ControlKind, bool) {
	if r.lane == nil || r.lane.Control == city.ControlNone {
		return mathx.Vec{}, city.ControlNone, false
	}
	if d := r.DistanceToJunction(); d > lookahead {
		return mathx.Vec{}, city.ControlNone, false
	}
	return r.lane.B, r.lane.Control, true
}
