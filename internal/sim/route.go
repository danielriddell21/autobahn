package sim

import (
	"math/rand/v2"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/works"
)

// Route is a navigation plan for the player's car: which lane it is in and
// which way it will go at the junctions ahead.
//
// This is a satnav, not a perception system. Choosing a direction at a
// junction is a routing decision, and the autopilot is allowed to have one in
// the same way a real car has a planned route. Everything the autopilot does
// with that route still goes through the camera: the lane centreline is drawn
// into the annotated image and must be visually re-detected.
//
// The plan is a queue of turns, decided once and then kept. A satnav that
// changed its mind about the junction ahead every time it was asked would
// make the guidance crawl, and one that re-planned in the middle of a
// junction — where the nearest lane is almost never the one being driven
// onto — would point the car back the way it came.
type Route struct {
	c    *city.City
	rng  *rand.Rand
	lane *city.Lane
	s    float32

	// plan holds the turns ahead, nearest first: plan[0] leaves the current
	// lane, plan[1] leaves the lane plan[0] arrives on, and so on.
	plan []city.Turn
	// crossing is set while the car is past the end of its lane and making
	// plan[0], and u is how far through that connector it has got.
	crossing bool
	u        float32
	// ringSkip is how many exits the plan still passes before it leaves the
	// roundabout its far end is on.
	ringSkip int
	// works is the roadworks, which the plan is not allowed to run into.
	works *works.Index
}

// NewRoute returns a route that will latch onto the nearest lane on its first
// update.
func NewRoute(c *city.City, rng *rand.Rand) *Route {
	return &Route{c: c, rng: rng}
}

// Lane returns the lane the route is currently following, or nil.
func (r *Route) Lane() *city.Lane { return r.lane }

// Crossing reports whether the car is past the end of its lane and on its way
// through the junction.
func (r *Route) Crossing() bool { return r.crossing }

// DistanceToJunction returns how far ahead the current lane ends.
func (r *Route) DistanceToJunction() float32 {
	if r.lane == nil {
		return 1e6
	}
	return r.lane.Length - r.s
}

// NextTurn returns the manoeuvre planned at the next junction.
func (r *Route) NextTurn() (city.TurnKind, bool) {
	r.extend(1)
	if len(r.plan) == 0 {
		return city.TurnStraight, false
	}
	return r.plan[0].Kind, true
}

// Route re-planning thresholds, in metres.
const (
	// strayRange is how far off its planned path the car may drift before the
	// plan is abandoned. It is wider than a carriageway so an ordinary lane
	// change does not trigger it.
	strayRange float32 = 11
	// arriveWidth is how far to the side of the next lane the car may be and
	// still count as having arrived on it. A corner is often cut.
	arriveWidth float32 = 6
	// switchMargin is how much nearer another lane has to be than the planned
	// one before the route moves over to it.
	switchMargin float32 = 1
)

// Update re-locates the route from the car's current pose. It follows the car
// rather than driving it, so a human who turns the "wrong" way simply gets a
// re-planned route.
func (r *Route) Update(pos, fwd mathx.Vec) {
	if r.lane == nil {
		r.relocate(pos, fwd)
		return
	}
	// A roundabout is cut into short chords, so more than one lane boundary
	// can be crossed between two updates.
	for range 4 {
		if !r.arrived(pos) {
			break
		}
	}

	r.revalidate()
	along := r.lane.Run(pos)
	r.s = mathx.Clamp(along, 0, r.lane.Length)
	r.extend(1)
	r.crossing = along > r.lane.Length && len(r.plan) > 0
	r.u = 0
	if r.crossing {
		r.u = r.turnProgress(pos)
	}
	r.replan(pos, fwd)
}

// revalidate drops the part of the plan that now runs into a street that has
// been closed since it was made, so it is chosen again. A turn already being
// made is not taken back.
func (r *Route) revalidate() {
	if r.works == nil {
		return
	}
	for i, t := range r.plan {
		if r.works.Barred(t.Lane) && (i != 0 || !r.crossing) {
			r.plan = r.plan[:i]
			return
		}
	}
}

// arrived moves the route onto the next planned lane once the car is on it.
func (r *Route) arrived(pos mathx.Vec) bool {
	r.extend(1)
	if len(r.plan) == 0 {
		return false
	}
	if r.lane.Run(pos) < r.lane.Length-0.5 {
		return false // still short of the junction
	}
	next := r.c.Lanes[r.plan[0].Lane]
	rel := pos.Sub(next.A)
	if rel.Dot(next.StartDir()) < 0 || mathx.Abs(rel.Cross(next.StartDir())) > arriveWidth {
		return false
	}
	r.lane = next
	r.plan = r.plan[1:]
	return true
}

// replan abandons the plan when the car has plainly gone somewhere else.
func (r *Route) replan(pos, fwd mathx.Vec) {
	off := r.offPath(pos)
	going := r.c.ProjectAlong(pos, fwd)

	if r.crossing {
		// Mid-junction the nearest lane is rarely the one being driven onto,
		// so only a car that is squarely on a different road out of this
		// junction has changed its mind. Anything less is just the turn being
		// made, and the lane alongside the one just left is nearest of all.
		next := r.c.Lanes[r.plan[0].Lane]
		//
		// A roundabout's own lanes never count. A connector on or off the ring
		// runs alongside the ring for most of its length, and the ring would
		// win the car back every time it tried to leave.
		if going.Valid && going.Lane != next && going.Lane.FromNode == r.lane.ToNode &&
			!r.c.Roads[going.Lane.Road].Ring &&
			going.Dist < 2 && going.S > 3 && going.S < going.Lane.Length {
			r.adopt(going.Lane, pos)
			return
		}
	} else if going.Valid && going.Lane != r.lane && going.Dist+switchMargin < off {
		// Somewhere else, and pointing along it: a lane change, or a driver who
		// has left the plan.
		r.adopt(going.Lane, pos)
		return
	}
	if off > strayRange {
		// Far enough off the plan that keeping it is worse than losing it: the
		// guidance would keep pointing back at a lane the car is no longer on.
		r.relocate(pos, fwd)
	}
}

// offPath is how far the car is from the path it is meant to be on.
func (r *Route) offPath(pos mathx.Vec) float32 {
	if !r.crossing {
		return pos.DistTo(r.lane.Point(r.s))
	}
	return pos.DistTo(r.c.TurnPoint(r.lane, r.plan[0], r.u))
}

// turnSamples is how many straight pieces a connector is cut into to find the
// car's place on it.
const turnSamples = 12

// turnProgress finds how far through the connector being made the car is, by
// dropping it onto the nearest piece of the curve. It is continuous: a car that
// creeps forward creeps along the connector too, rather than jumping from one
// sample to the next and taking the guidance ahead of it along for the ride.
func (r *Route) turnProgress(pos mathx.Vec) float32 {
	best, bestDist := float32(0), float32(1e9)
	prev := r.c.TurnPoint(r.lane, r.plan[0], 0)
	for i := 1; i <= turnSamples; i++ {
		next := r.c.TurnPoint(r.lane, r.plan[0], float32(i)/turnSamples)
		seg := next.Sub(prev)
		t := float32(0)
		if l := seg.LenSq(); l > 1e-6 {
			t = mathx.Clamp(pos.Sub(prev).Dot(seg)/l, 0, 1)
		}
		if d := pos.DistTo(prev.Add(seg.Mul(t))); d < bestDist {
			best, bestDist = (float32(i-1)+t)/turnSamples, d
		}
		prev = next
	}
	return best
}

// relocate latches onto whatever lane the car is nearest, preferring one that
// runs the way the car is pointing.
func (r *Route) relocate(pos, fwd mathx.Vec) {
	proj := r.c.ProjectAlong(pos, fwd)
	if !proj.Valid || proj.Dist > strayRange {
		if nearest := r.c.Project(pos); nearest.Valid {
			proj = nearest
		}
	}
	if !proj.Valid {
		return
	}
	r.adopt(proj.Lane, pos)
}

func (r *Route) adopt(l *city.Lane, pos mathx.Vec) {
	r.lane = l
	r.s = l.Along(pos)
	r.plan = r.plan[:0]
	r.crossing, r.u = false, 0
	// Joining a roundabout part-way round, leave by one of the next two exits.
	r.ringSkip = r.rng.IntN(2)
	r.extend(1)
}

// extend plans ahead until n turns are decided, or the built city runs out.
func (r *Route) extend(n int) {
	for len(r.plan) < n {
		from := r.lane
		if len(r.plan) > 0 {
			from = r.c.Lanes[r.plan[len(r.plan)-1].Lane]
		}
		t, ok := r.choose(from)
		if !ok {
			return
		}
		r.plan = append(r.plan, t)
	}
}

// choose decides where the route goes at the end of a lane.
func (r *Route) choose(l *city.Lane) (city.Turn, bool) {
	turns := openTurns(r.works, l)
	if len(turns) == 0 {
		// The frontier of a growing city: the junction ahead is not wired yet.
		return city.Turn{}, false
	}
	if r.c.Roads[l.Road].Ring {
		return r.chooseOnRing(turns), true
	}
	if r.c.Nodes[l.ToNode].IsRoundabout() {
		// Decide the exit on the way in, as a driver does, rather than at each
		// one in turn. Going all the way round to the road just left is not
		// offered.
		r.ringSkip = r.rng.IntN(max(r.c.Nodes[l.ToNode].Degree()-1, 1))
		return turns[0], true
	}
	// Prefer to carry straight on, which keeps the driven line predictable.
	var straight []city.Turn
	for _, t := range turns {
		if t.Kind == city.TurnStraight {
			straight = append(straight, t)
		}
	}
	if len(straight) > 0 && r.rng.Float32() < 0.7 {
		return straight[r.rng.IntN(len(straight))], true
	}
	return turns[r.rng.IntN(len(turns))], true
}

// chooseOnRing carries on round a roundabout until the planned exit comes up.
func (r *Route) chooseOnRing(turns []city.Turn) city.Turn {
	var exits []city.Turn
	round := turns[0]
	for _, t := range turns {
		if t.Kind == city.TurnStraight {
			round = t
		} else {
			exits = append(exits, t)
		}
	}
	if len(exits) == 0 {
		return round
	}
	if r.ringSkip > 0 {
		r.ringSkip--
		return round
	}
	return exits[r.rng.IntN(len(exits))]
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
	// Walk the plan lane by lane, deciding more of it as the lookahead needs.
	lane, s, i := r.lane, r.s, 0
	if r.crossing {
		// Part-way through a junction the walk starts on the connector, from
		// where the car has got to on it.
		t := r.plan[0]
		remain := (1 - r.u) * t.Len
		if d <= remain {
			return r.c.TurnPoint(lane, t, r.u+d/max(t.Len, 0.01))
		}
		d -= remain
		lane, s, i = r.c.Lanes[t.Lane], 0, 1
	}
	for range aheadHops {
		remain := lane.Length - s
		if d <= remain {
			return lane.Point(s + d)
		}
		d -= remain
		if r.extend(i + 1); i >= len(r.plan) {
			return lane.Point(lane.Length)
		}
		t := r.plan[i]
		if d <= t.Len {
			return r.c.TurnPoint(lane, t, d/max(t.Len, 0.01))
		}
		d -= t.Len
		lane, s, i = r.c.Lanes[t.Lane], 0, i+1
	}
	return lane.Point(lane.Length)
}

// StopLine returns the position of the stop line the car is approaching, the
// control governing it and whether one applies within lookahead metres.
func (r *Route) StopLine(lookahead float32) (mathx.Vec, city.ControlKind, bool) {
	if r.lane == nil || r.lane.Control == city.ControlNone || r.crossing {
		return mathx.Vec{}, city.ControlNone, false
	}
	if d := r.DistanceToJunction(); d > lookahead {
		return mathx.Vec{}, city.ControlNone, false
	}
	return r.lane.B, r.lane.Control, true
}
