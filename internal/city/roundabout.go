package city

import (
	"math"
	"math/rand/v2"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Roundabout geometry.
const (
	// ringSegment is the longest a piece of the circulating carriageway may be,
	// in metres. The ring is built from short straight lanes rather than a
	// curve, so that every system that already understands a lane — the traffic
	// agents, the navigation route, the judge, the annotator — works on a
	// roundabout with no changes at all. Keeping the pieces short makes the
	// polygon read as a circle.
	ringSegment float32 = 4.5
	// ringLanes is how many pieces the smallest ring is split into, so even a
	// tight roundabout is not a triangle.
	ringLanes = 12
	// giveWayArc is how far back around the ring, in radians, a circulating car
	// has to be before a waiting driver will pull out in front of it.
	giveWayArc float32 = 1.15
	// ringSpeed is the limit on the circulating carriageway.
	ringSpeedMPH float32 = 20
)

// IsRoundabout reports whether the node is a roundabout rather than a
// conventional junction.
func (n *Node) IsRoundabout() bool { return n.Roundabout }

// RingRadius returns the radius of a roundabout's circulating carriageway.
func (n *Node) RingRadius() float32 { return n.ringRadius }

// chooseRoundabouts promotes a share of the junctions to roundabouts. They are
// picked before lanes are built, because a roundabout needs a much larger
// junction radius and the approach lanes are trimmed back to it.
func (c *City) chooseRoundabouts(rng *rand.Rand, share float32) {
	for _, n := range c.Nodes {
		if n.Degree() < 3 || rng.Float32() >= share {
			continue
		}
		var widest float32
		for _, rid := range n.Roads {
			widest = max(widest, c.Roads[rid].HalfWidth)
		}
		n.Roundabout = true
		n.ringRadius = mathx.Clamp(widest+7, 11, 17)
		// The approach lanes stop just outside the circulating carriageway.
		n.Radius = n.ringRadius + 2.5
	}
}

// buildRings lays the circulating carriageway of every roundabout. Each ring is
// a closed run of short lanes travelling in the direction entering traffic
// turns into, which is clockwise seen from above where traffic keeps left.
func (c *City) buildRings() {
	for _, n := range c.Nodes {
		if !n.Roundabout {
			continue
		}
		c.buildRing(n)
	}
}

func (c *City) buildRing(n *Node) {
	// One synthetic road carries every lane of the ring, so anything that looks
	// a lane's road up keeps working. It is flagged so the renderer does not
	// try to paint a centre line down a circle.
	road := &Road{
		ID: len(c.Roads), A: n.ID, B: n.ID, Class: Local, Ring: true,
		HalfWidth: LaneWidth * 1.5, Dir: mathx.V(1, 0),
	}
	c.Roads = append(c.Roads, road)

	angles := c.ringAngles(n)
	n.ringEntry = make(map[int]int, len(n.Roads))

	// Lay the ring lanes first, so each vertex knows which lane leaves it.
	first := len(c.Lanes)
	for i, a := range angles {
		b := angles[(i+1)%len(angles)]
		l := &Lane{
			ID: len(c.Lanes), Road: road.ID, Dir: +1, Index: 0,
			A: n.ringPoint(a.theta), B: n.ringPoint(b.theta),
			FromNode: n.ID, ToNode: n.ID,
			SpeedLimit: mathx.MPH(ringSpeedMPH), Class: Local,
			Control: ControlNone,
		}
		l.finish()
		c.Lanes = append(c.Lanes, l)
		if a.road >= 0 {
			n.ringEntry[a.road] = l.ID
		}
	}
	count := len(angles)

	// Then wire it: every lane continues around, and the ones that end at a
	// road may also leave by it.
	for i := range count {
		l := c.Lanes[first+i]
		next := c.Lanes[first+(i+1)%count]
		l.Succ = append(l.Succ, c.makeTurn(l, next, TurnStraight, n))

		exit := angles[(i+1)%count].road
		if exit < 0 {
			continue
		}
		for _, oid := range c.exitLanes(c.Roads[exit], n.ID) {
			o := c.Lanes[oid]
			l.Succ = append(l.Succ, c.makeTurn(l, o, TurnLeft, n))
		}
	}

	// Approaches give way, then join the ring at their own vertex.
	for _, rid := range n.Roads {
		lane, ok := n.ringEntry[rid]
		if !ok {
			continue
		}
		for _, in := range c.approachLanes(c.Roads[rid], n.ID) {
			l := c.Lanes[in]
			l.Control = ControlGiveWay
			l.Succ = append(l.Succ[:0], c.makeTurn(l, c.Lanes[lane], TurnLeft, n))
		}
	}
}

// ringVertex is one point on the circulating carriageway. Vertices that carry a
// road are where traffic joins and leaves.
type ringVertex struct {
	theta float32
	road  int // the road attaching here, or -1 for a plain vertex
}

// ringAngles returns the ring's vertices in circulation order: one at every
// road, plus enough plain vertices between them to keep the segments short.
func (c *City) ringAngles(n *Node) []ringVertex {
	var out []ringVertex
	for _, rid := range n.Roads {
		out = append(out, ringVertex{theta: c.roadAngle(n, rid), road: rid})
	}
	// Circulation runs in the direction of increasing angle.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].theta < out[j-1].theta; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}

	filled := make([]ringVertex, 0, len(out)*3)
	for i, v := range out {
		filled = append(filled, v)
		next := out[(i+1)%len(out)].theta
		span := next - v.theta
		if i == len(out)-1 {
			span += 2 * math.Pi
		}
		// Split the gap into pieces no longer than ringSegment.
		steps := int(span*n.ringRadius/ringSegment) + 1
		steps = max(steps, ringLanes/max(len(out), 1))
		for s := 1; s < steps; s++ {
			filled = append(filled, ringVertex{
				theta: v.theta + span*float32(s)/float32(steps), road: -1,
			})
		}
	}
	return filled
}

// roadAngle returns the direction from a node to the far end of one of its
// roads, which is where that road meets the ring.
func (c *City) roadAngle(n *Node, rid int) float32 {
	r := c.Roads[rid]
	far := c.Nodes[r.B].Pos
	if r.B == n.ID {
		far = c.Nodes[r.A].Pos
	}
	return far.Sub(n.Pos).Angle()
}

func (n *Node) ringPoint(theta float32) mathx.Vec {
	return n.Pos.Add(mathx.FromAngle(theta).Mul(n.ringRadius))
}

// approachLanes returns the lanes of a road that arrive at the given node.
func (c *City) approachLanes(r *Road, node int) []int {
	if r.B == node {
		return r.LanesAB
	}
	return r.LanesBA
}

// RingOccupied reports whether a car on the circulating carriageway is close
// enough behind the given entry angle that a driver waiting there should give
// way to it. positions are supplied by the caller, so this stays free of any
// dependency on the simulation.
func (n *Node) RingOccupied(entry float32, positions func(yield func(mathx.Vec, float32) bool), tolerance float32) bool {
	blocked := false
	positions(func(p mathx.Vec, speed float32) bool {
		rel := p.Sub(n.Pos)
		d := rel.Len()
		if d > n.ringRadius+5 || d < n.ringRadius-6 {
			return true // not on the ring
		}
		// How far back around the ring the car is, travelling the way the ring
		// runs. A small gap means it is about to arrive at the entry.
		gap := wrapTwoPi(entry - rel.Angle())
		if gap < giveWayArc*tolerance && speed > 0.3 {
			blocked = true
			return false
		}
		return true
	})
	return blocked
}

// EntryAngle returns the ring angle at which a lane joins a roundabout.
func (n *Node) EntryAngle(l *Lane) float32 { return l.B.Sub(n.Pos).Angle() }

func wrapTwoPi(a float32) float32 {
	const twoPi = 2 * math.Pi
	a = float32(math.Mod(float64(a), twoPi))
	if a < 0 {
		a += twoPi
	}
	return a
}
