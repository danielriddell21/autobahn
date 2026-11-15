package city

import (
	"math"

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
	// ringVerge is how far the circulating carriageway reaches either side of
	// the line traffic follows round it: out to the kerb, and in to the island.
	ringVerge float32 = 3.3
	// ringApron is how far outside the carriageway the approaches stop, which
	// is where their give way lines are painted.
	ringApron float32 = 1.0
	// giveWayArc is how far back around the ring, in radians, a circulating car
	// has to be before a waiting driver will pull out in front of it.
	giveWayArc float32 = 1.15
	// ringSpeed is the limit on the circulating carriageway.
	ringSpeedMPH float32 = 20
)

// IsRoundabout reports whether the node is a roundabout rather than a
// conventional junction.
func (n *Node) IsRoundabout() bool { return n.Roundabout }

// RingRadius returns the radius of the line traffic follows round a
// roundabout: the middle of its circulating carriageway.
func (n *Node) RingRadius() float32 { return n.ringRadius }

// RingOuter returns the radius of a roundabout's outer kerb.
func (n *Node) RingOuter() float32 { return n.ringRadius + ringVerge }

// RingInner returns the radius of a roundabout's central island.
func (n *Node) RingInner() float32 { return n.ringRadius - ringVerge }

func (c *City) buildRing(n *Node) {
	// One synthetic road carries every lane of the ring, so anything that looks
	// a lane's road up keeps working. It is flagged so the renderer does not
	// try to paint a centre line down a circle.
	road := &Road{
		ID: len(c.Roads), A: n.ID, B: n.ID, Class: Local, Ring: true,
		HalfWidth: ringVerge, Dir: mathx.V(1, 0),
	}
	c.Roads = append(c.Roads, road)

	verts := c.ringVertices(n)
	n.ringEntry = make(map[int]int, len(n.Roads))
	n.ringEntryAngle = make(map[int]float32, len(n.Roads))

	// Lay the ring lanes first, so each vertex knows which lane leaves it.
	first := len(c.Lanes)
	for i, a := range verts {
		b := verts[(i+1)%len(verts)]
		l := &Lane{
			ID: len(c.Lanes), Road: road.ID, Dir: +1, Index: 0,
			A: n.ringPoint(a.theta), B: n.ringPoint(b.theta),
			FromNode: n.ID, ToNode: n.ID,
			SpeedLimit: mathx.MPH(ringSpeedMPH), Class: Local,
			Control: ControlNone,
		}
		l.finish()
		c.Lanes = append(c.Lanes, l)
		// Filed like any other lane, so a car going round is found on the ring
		// rather than on whichever approach happens to be nearest.
		c.indexLane(l)
		if a.entry >= 0 {
			n.ringEntry[a.entry] = l.ID
			n.ringEntryAngle[a.entry] = a.theta
		}
	}
	count := len(verts)

	// Then wire it: every lane continues around, and the ones that end where a
	// road peels off may also leave by it.
	for i := range count {
		l := c.Lanes[first+i]
		next := c.Lanes[first+(i+1)%count]
		l.Succ = append(l.Succ, c.makeTurn(l, next, TurnStraight, n))

		exit := verts[(i+1)%count].exit
		if exit < 0 {
			continue
		}
		for _, oid := range c.exitLanes(c.Roads[exit], n.ID) {
			o := c.Lanes[oid]
			t := c.makeTurn(l, o, TurnLeft, n)
			t.Ctrl = easedControl(t.Ctrl, l.B, o.A, o.Fwd.Mul(-1))
			t.Len = bezierLength(l.B, t.Ctrl, o.A)
			l.Succ = append(l.Succ, t)
		}
	}

	// Approaches give way, then join the ring a little way round from where
	// they arrive, so they merge with the flow instead of meeting it square.
	for _, rid := range n.Roads {
		lane, ok := n.ringEntry[rid]
		if !ok {
			continue
		}
		for _, in := range c.approachLanes(c.Roads[rid], n.ID) {
			l := c.Lanes[in]
			l.Control = ControlGiveWay
			t := c.makeTurn(l, c.Lanes[lane], TurnLeft, n)
			t.Ctrl = easedControl(t.Ctrl, c.Lanes[lane].A, l.B, l.Fwd)
			t.Len = bezierLength(l.B, t.Ctrl, c.Lanes[lane].A)
			l.Succ = append(l.Succ[:0], t)
		}
	}
}

// easeShare is the least of a ring connector's span that its control point
// stands off from the road end, as a fraction of the straight-line distance.
const easeShare float32 = 0.45

// easedControl keeps the bend of a connector between a road and the ring from
// bunching up against the road end.
//
// The natural control point is where the two directions meet, and for a lane
// close to the centre line that is almost on top of the lane's own end: the
// connector runs straight and then turns all at once, which no car can follow.
// Sliding the control back along the lane spreads the turn over the whole
// connector, and keeps it leaving or joining the lane dead straight.
//
// ringEnd is the connector's end on the ring, roadEnd its end on the lane, and
// away the direction from roadEnd toward the junction along that lane.
func easedControl(ctrl, ringEnd, roadEnd, away mathx.Vec) mathx.Vec {
	least := ringEnd.DistTo(roadEnd) * easeShare
	if ctrl.Sub(roadEnd).Dot(away) >= least {
		return ctrl
	}
	return roadEnd.Add(away.Mul(least))
}

// ringVertex is one point on the circulating carriageway. Each road has two:
// one where traffic peels off to leave by it, and one further round where
// traffic arriving by it joins.
type ringVertex struct {
	theta float32
	entry int // the road joining here, or -1
	exit  int // the road leaving here, or -1
}

// ringFlare is the most a road's joining and leaving points are swung round
// from the road itself, in radians. Roads meet a roundabout a quarter turn
// apart, so this keeps one road's entry short of the next road's exit.
const ringFlare float32 = 0.68

func (c *City) ringVertices(n *Node) []ringVertex {
	// Returns the ring's vertices in circulation order: an exit and an entry
	// for every road, plus enough plain vertices between them to keep the
	// segments short.
	type arm struct {
		theta, flare float32
		road         int
	}
	arms := make([]arm, 0, len(n.Roads))
	for _, rid := range n.Roads {
		// A wide road's kerbside lane is a long way round from its centre
		// line, so its traffic needs more of the ring to peel off across.
		flare := min((c.Roads[rid].HalfWidth+3)/n.ringRadius, ringFlare)
		arms = append(arms, arm{theta: c.roadAngle(n, rid), flare: flare, road: rid})
	}
	// Circulation runs in the direction of increasing angle.
	for i := 1; i < len(arms); i++ {
		for j := i; j > 0 && arms[j].theta < arms[j-1].theta; j-- {
			arms[j], arms[j-1] = arms[j-1], arms[j]
		}
	}

	keyed := make([]ringVertex, 0, len(arms)*2)
	for _, a := range arms {
		keyed = append(keyed,
			ringVertex{theta: a.theta - a.flare, entry: -1, exit: a.road},
			ringVertex{theta: a.theta + a.flare, entry: a.road, exit: -1})
	}

	filled := make([]ringVertex, 0, len(keyed)*3)
	for i, v := range keyed {
		filled = append(filled, v)
		span := keyed[(i+1)%len(keyed)].theta - v.theta
		if i == len(keyed)-1 {
			span += 2 * math.Pi
		}
		// Split the gap into pieces no longer than ringSegment.
		steps := int(span*n.ringRadius/ringSegment) + 1
		for s := 1; s < steps; s++ {
			filled = append(filled, ringVertex{
				theta: v.theta + span*float32(s)/float32(steps), entry: -1, exit: -1,
			})
		}
	}
	return filled
}

func (c *City) roadAngle(n *Node, rid int) float32 {
	// Returns the direction from a node to the far end of one of its
	// roads, which is where that road meets the ring.
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

func (c *City) approachLanes(r *Road, node int) []int {
	// Returns the lanes of a road that arrive at the given node.
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

// EntryAngle returns the ring angle at which a lane joins a roundabout: the
// point where its traffic merges with what is going round, which is where a
// gap has to be found.
func (n *Node) EntryAngle(l *Lane) float32 {
	if a, ok := n.ringEntryAngle[l.Road]; ok {
		return a
	}
	return l.B.Sub(n.Pos).Angle()
}

func wrapTwoPi(a float32) float32 {
	const twoPi = 2 * math.Pi
	a = float32(math.Mod(float64(a), twoPi))
	if a < 0 {
		a += twoPi
	}
	return a
}
