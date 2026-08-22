package city

import (
	"github.com/danielriddell21/crucible/geom"
	"github.com/danielriddell21/crucible/spatial"
	"github.com/danielriddell21/crucible/worldgen"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

// The city is built outward rather than all at once, so it can extend as far
// as anybody drives.
//
// The awkwardness is that almost nothing about a junction can be decided until
// its surroundings exist. How wide it is depends on the roads meeting there;
// which approaches give way depends on what the other approaches are; where a
// lane may go depends on every road at its far end. Deciding any of that early
// and revising it later would move the world under whoever is driving on it.
//
// So a junction passes through three states, and never goes back:
//
//   - reached — the lattice has a node here, but its neighbours may not exist
//     yet, so nothing about it is known beyond where it is.
//   - settled — every neighbouring cell exists, so the junction's own shape is
//     final: how wide it is, and whether it is a roundabout.
//   - wired — every road meeting it has been built, so its lanes know where
//     they may go and its signs are up.
//
// Roads are built between two settled junctions, blocks are filled once their
// four corners have settled, and nothing is ever revisited. That is what makes
// a city reached a piece at a time identical to the same ground generated in
// one pass, which is the property the tests hold this to.

// growMargin is how far beyond the requested radius the lattice is grown, in
// cells. A junction only settles once its neighbours exist, so the lattice has
// to run ahead of the part of the city being built — one ring of cells for the
// neighbours themselves, and another so those neighbours can settle too and
// the roads between them get built.
const growMargin = 2

// NewGrowing returns a city with nothing in it yet. Call [City.EnsureAround]
// to bring an area into existence.
func NewGrowing(p Params) *City {
	return &City{
		Seed:   p.Seed,
		params: p,
		lanes:  spatial.New[int](laneCellSize),
		lat: worldgen.NewGrowable(p.Seed, worldgen.LatticeConfig{
			MinSpan: float64(p.MinSpan), MaxSpan: float64(p.MaxSpan),
			Thin: thinShare,
		}),
		roadOf: map[int]int{},
	}
}

// EnsureAround brings the city within radius of a point into existence,
// leaving what is already there untouched. It is safe and cheap to call every
// frame with the player's position.
func (c *City) EnsureAround(at mathx.Vec, radius float32) {
	here := geom.Vec2{X: float64(at.X), Y: float64(at.Z)}

	// Grow the plan further than the city, so the junctions at the edge of
	// what is being built have neighbours to settle against.
	cells := grown(c.lat.CellsAround(here, float64(radius)), growMargin)

	// EnsureAround is the only thing that changes the city, so asking for the
	// same cells twice can have nothing to do. That is the common case by a
	// wide margin: this is called every frame, and the rectangle only moves
	// when the car crosses a street.
	if cells == c.lastCells {
		return
	}
	c.lastCells = cells
	c.lat.Grow(cells)

	// Only the ground just grown, and a ring around it, can have anything left
	// to do: a junction settles because a neighbour has appeared, a road is
	// laid because its ends have settled, and so on outward, never more than
	// two cells from a junction that has just changed. Sweeping the whole city
	// instead would make standing still cost more the further you had driven.
	work := grown(cells, 2)

	c.reachNodes()
	c.settleNodes(work)
	c.buildRoads(work)
	c.wireNodes(work)
	c.fillCells(work)
	c.computeBounds()
}

// grown returns cells widened by margin on every side.
func grown(cells geom.Rect, margin int) geom.Rect {
	return geom.Rect{
		X: cells.X - margin, Y: cells.Y - margin,
		W: cells.W + 2*margin, H: cells.H + 2*margin,
	}
}

// eachNode calls fn for every lattice node in a rectangle of cells.
func (c *City) eachNode(cells geom.Rect, fn func(*worldgen.LatticeNode)) {
	for row := cells.Y; row < cells.Y+cells.H; row++ {
		for col := cells.X; col < cells.X+cells.W; col++ {
			if ln := c.lat.Node(col, row); ln != nil {
				fn(ln)
			}
		}
	}
}

// reachNodes gives every lattice node a city junction, so the two index
// alike. A reached junction knows only where it is.
func (c *City) reachNodes() {
	for len(c.Nodes) < len(c.lat.Nodes) {
		n := c.lat.Nodes[len(c.Nodes)]
		c.Nodes = append(c.Nodes, &Node{
			ID:    n.ID,
			Pos:   mathx.V(float32(n.Pos.X), float32(n.Pos.Y)),
			GridI: n.Col, GridJ: n.Row,
		})
	}
}

// settleNodes fixes the shape of every junction whose neighbourhood is now
// complete: how wide it is, whether it is a roundabout, and how entry to it is
// governed.
func (c *City) settleNodes(cells geom.Rect) {
	c.eachNode(cells, func(ln *worldgen.LatticeNode) {
		n := c.Nodes[ln.ID]
		if n.settled || !c.neighboursReached(ln) {
			return
		}
		c.settleShape(n, ln)
		c.settleControl(n, ln)
		n.settled = true
	})
}

// neighboursReached reports whether all four cells around a node exist, which
// is what makes its set of roads final.
func (c *City) neighboursReached(n *worldgen.LatticeNode) bool {
	for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		if c.lat.Node(n.Col+d[0], n.Row+d[1]) == nil {
			return false
		}
	}
	return true
}

// settleShape sets a junction's radius, and promotes some to roundabouts.
//
// Both are read off the lattice rather than off the city's own roads, because
// the roads are not built yet: a road's lanes are trimmed back to the radius
// of the junctions at each end, so the radius has to be known first.
func (c *City) settleShape(n *Node, ln *worldgen.LatticeNode) {
	var widest float32
	for _, id := range ln.Edges {
		widest = max(widest, halfWidthOf(latticeClass(c.lat, c.lat.Edges[id])))
	}
	n.Radius = widest

	// A roundabout needs room and somewhere to send traffic, and the choice
	// has to come from the junction's position rather than from a running
	// stream, or a city reached in a different order would put them elsewhere.
	if len(ln.Edges) < 3 || cellChance(c.Seed, roundaboutSalt, ln.Col, ln.Row) >= c.params.Roundabouts {
		return
	}
	n.Roundabout = true
	n.ringRadius = mathx.Clamp(widest+7, 11, 17)
	// The approach lanes stop just outside the circulating carriageway.
	n.Radius = n.ringRadius + 2.5
}

// settleControl decides how entry to a junction is governed.
func (c *City) settleControl(n *Node, ln *worldgen.LatticeNode) {
	if n.Roundabout {
		n.Control = ControlGiveWay
		return
	}
	if len(ln.Edges) < 3 {
		n.Control = ControlNone
		return
	}
	hasArterial := false
	for _, id := range ln.Edges {
		if latticeClass(c.lat, c.lat.Edges[id]) == Arterial {
			hasArterial = true
		}
	}
	switch {
	case hasArterial:
		n.Control, n.Signalised = ControlSignal, true
	case cellChance(c.Seed, stopSignSalt, ln.Col, ln.Row) < stopSignShare:
		// A minority of priority junctions are signed STOP rather than GIVE
		// WAY, which is roughly how they are distributed in reality.
		n.Control = ControlStop
	default:
		n.Control = ControlGiveWay
	}
}

// buildRoads lays every road whose junctions at both ends have settled, and
// the lanes along it.
func (c *City) buildRoads(cells geom.Rect) {
	c.eachNode(cells, func(ln *worldgen.LatticeNode) {
		for _, id := range ln.Edges {
			e := c.lat.Edges[id]
			if _, done := c.roadOf[e.ID]; done {
				continue
			}
			if !c.Nodes[e.A].settled || !c.Nodes[e.B].settled {
				continue
			}
			r := c.addRoad(e.A, e.B, latticeClass(c.lat, e), axisOf(e))
			c.roadOf[e.ID] = r.ID
			c.buildRoadLanes(r)
		}
	})
}

// wireNodes finishes every settled junction whose roads are all built: its
// lanes learn where they may go, its ring is laid if it has one, and its signs
// go up.
func (c *City) wireNodes(cells geom.Rect) {
	c.eachNode(cells, func(ln *worldgen.LatticeNode) {
		n := c.Nodes[ln.ID]
		if n.wired || !n.settled || !c.roadsBuilt(ln) {
			return
		}
		// Approaches first: a connector's shape depends on whether the lane
		// gives way, and a ring rewires its approaches as it is laid.
		for _, rid := range n.Roads {
			for _, lid := range c.approachLanes(c.Roads[rid], n.ID) {
				c.assignLaneControl(c.Lanes[lid])
			}
		}
		if n.Roundabout {
			c.buildRing(n)
		} else {
			for _, rid := range n.Roads {
				for _, lid := range c.approachLanes(c.Roads[rid], n.ID) {
					c.connectLane(c.Lanes[lid], n)
				}
			}
		}
		c.placeNodeProps(n)
		n.wired = true
	})
}

// roadsBuilt reports whether every edge at a node has become a road.
func (c *City) roadsBuilt(n *worldgen.LatticeNode) bool {
	for _, id := range n.Edges {
		if _, ok := c.roadOf[id]; !ok {
			return false
		}
	}
	return true
}

// fillCells lays the blocks and buildings between grid lines, for every cell
// whose four corners have settled.
func (c *City) fillCells(cells geom.Rect) {
	c.eachNode(cells, func(ln *worldgen.LatticeNode) {
		n := c.Nodes[ln.ID]
		if n.filled {
			return
		}
		corners, ok := c.cellCorners(ln.Col, ln.Row)
		if !ok {
			return
		}
		c.fillCell(ln.Col, ln.Row, corners)
		n.filled = true
	})
}

// cellCorners returns the four junctions bounding the cell whose lower corner
// is the given one, and whether all of them have settled.
func (c *City) cellCorners(col, row int) ([4]*Node, bool) {
	var out [4]*Node
	for i, d := range [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		ln := c.lat.Node(col+d[0], row+d[1])
		if ln == nil || !c.Nodes[ln.ID].settled {
			return out, false
		}
		out[i] = c.Nodes[ln.ID]
	}
	return out, true
}

// computeBounds tracks the extent of everything reached so far.
func (c *City) computeBounds() {
	if len(c.Nodes) == 0 {
		return
	}
	c.Min = mathx.V(float32(c.lat.Min.X), float32(c.lat.Min.Y))
	c.Max = mathx.V(float32(c.lat.Max.X), float32(c.lat.Max.Y))
}

// Chance returns a value in [0, 1) for this junction and a purpose, drawn from
// where the junction is rather than from when it was reached.
//
// Anything outside this package that has to decide something per junction — a
// signal's place in its cycle, say — uses this, so a city grown a piece at a
// time comes out the same as one built in a single pass.
func (n *Node) Chance(seed, salt uint64) float32 {
	return cellChance(seed, salt, n.GridI, n.GridJ)
}

// Complete reports whether a junction is finished: every road meeting it
// exists, its lanes know where they may go, and its signs are up.
//
// A growing city always has a frontier of junctions that are not. They are
// real places with real positions, but their far sides have not been reached,
// so a lane arriving at one may have nowhere to go yet. Anything walking the
// whole city — a test, a map, an analysis — wants to skip them.
func (c *City) Complete(n *Node) bool { return n.wired }

// Settled reports whether a junction's shape has been decided. A junction on
// the frontier has been reached but not settled, and carries nothing but its
// position.
func (n *Node) Settled() bool { return n.settled }

// The salts that keep one positional decision from correlating with another.
const (
	roundaboutSalt = 0x9e37
	stopSignSalt   = 0x51ed
	buildingSalt   = 0x27d4
)

// The salts and shares behind the rest of the positional decisions.
const (
	speedSignSalt = 0x6a09
	parkSalt      = 0x3c6e
)

// The shares each of those decisions comes out at.
const (
	// stopSignShare is how many priority junctions are signed STOP rather than
	// GIVE WAY.
	stopSignShare = 1.0 / 7
	// speedSignShare is how many lanes long enough to carry one get a repeater
	// speed sign shortly after their junction.
	speedSignShare = 0.55
	// parkShare is how many blocks are left as open ground.
	parkShare = 0.10
	// alleyShare is how often the run of buildings along a street is broken by
	// a gap.
	alleyShare = 0.12
)

// downtownFalloff is how far from the middle of the world the towers give way
// to low-rise, in metres.
//
// In a city of fixed size this could be measured against the furthest junction
// generated. A growing one has no furthest junction — it would move as the
// world extended, and buildings already standing would be the wrong height for
// where they are. So the skyline is pinned to a distance instead.
const downtownFalloff = 500

// cellChance returns a value in [0, 1) for a cell and a purpose. Everything
// generated has to be a function of where it is rather than of when it was
// reached, or a city built in a different order would come out different.
func cellChance(seed uint64, salt uint64, col, row int) float32 {
	h := seed ^ salt
	h += 0x9e3779b97f4a7c15
	h ^= uint64(int64(col)) * 0xbf58476d1ce4e5b9
	h = (h ^ (h >> 30)) * 0xbf58476d1ce4e5b9
	h ^= uint64(int64(row)) * 0x94d049bb133111eb
	h = (h ^ (h >> 27)) * 0x94d049bb133111eb
	h ^= h >> 31
	return float32(h>>40) / float32(1<<24)
}

// cellValue is cellChance with an extra index, for the several decisions taken
// within one cell — which building, which gap, how tall.
func cellValue(seed, salt uint64, col, row, i int) float32 {
	return cellChance(seed, salt+uint64(i)*0x2545f491, col, row)
}
