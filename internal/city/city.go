// Package city procedurally generates the road network, junctions and
// buildings that make up the drivable world.
//
// The layout is a seeded grid with varied block spans. Roads are axis-aligned,
// which keeps junction geometry and rendering straightforward while still
// producing a city with many distinct intersections. The generated [City]
// exposes a lane graph: directed [Lane] values joined by [Turn] connectors,
// which both the traffic simulation and the autopilot navigate.
package city

import (
	"math/rand/v2"

	"github.com/danielriddell21/crucible/rng"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

// StreamLayout is the crucible random stream the city layout is drawn from.
// Naming the stream keeps generation reproducible even as other systems add
// or reorder their own draws.
const StreamLayout uint64 = 1

// DriveOnLeft selects the side of the road traffic drives on. Lane offsets and
// turn permissions are both derived from it. The city is British, so traffic
// keeps left and it is the right turn that crosses oncoming traffic.
const DriveOnLeft = true

// Geometry constants, in metres.
const (
	LaneWidth     float32 = 3.6
	PavementWidth float32 = 4.0
	KerbHeight    float32 = 0.18
)

// RoadClass determines a road's lane count, speed limit and priority at
// junctions.
type RoadClass int

// The road classes, ordered by increasing priority.
const (
	Local RoadClass = iota
	Collector
	Arterial
)

// Lanes returns the number of lanes the class carries in each direction.
func (c RoadClass) Lanes() int {
	if c == Arterial {
		return 2
	}
	return 1
}

// SpeedLimit returns the class's speed limit in metres per second. The posted
// values are the British urban set: 20 on residential streets, 30 as the
// default built-up limit, and 40 on the main roads through town.
func (c RoadClass) SpeedLimit() float32 {
	switch c {
	case Arterial:
		return mathx.MPH(40)
	case Collector:
		return mathx.MPH(30)
	default:
		return mathx.MPH(20)
	}
}

// Name returns a human-readable name for the class.
func (c RoadClass) Name() string {
	switch c {
	case Arterial:
		return "arterial"
	case Collector:
		return "collector"
	default:
		return "local"
	}
}

// ControlKind is how entry into a junction is governed for one approach.
type ControlKind int

// The junction control kinds.
const (
	ControlNone ControlKind = iota
	ControlSignal
	// ControlStop requires a full halt at the line before proceeding.
	ControlStop
	// ControlGiveWay requires yielding to traffic on the major road, but no
	// halt when the junction is clear. It is the common British priority
	// marking, far more frequent than a stop sign.
	ControlGiveWay
)

// TurnKind classifies a connector through a junction.
type TurnKind int

// The turn kinds.
const (
	TurnStraight TurnKind = iota
	TurnLeft
	TurnRight
)

// Node is an intersection between roads.
type Node struct {
	ID     int
	Pos    mathx.Vec
	Roads  []int // incident road IDs
	Radius float32
	// Control is how the junction is governed. Signalised junctions alternate
	// between two phase groups: group 0 is the X-axis approaches, group 1 the
	// Z-axis approaches.
	Control      ControlKind
	Signalised   bool
	GridI, GridJ int

	// Roundabout marks a junction whose traffic circulates around an island
	// instead of being governed by signals or priority.
	Roundabout bool
	ringRadius float32
	ringEntry  map[int]int // road ID to the ring lane it joins
}

// Degree returns the number of roads meeting at the node.
func (n *Node) Degree() int { return len(n.Roads) }

// Road is an undirected segment between two nodes, carrying lanes in both
// directions.
type Road struct {
	ID        int
	A, B      int // node IDs
	Class     RoadClass
	LanesAB   []int
	LanesBA   []int
	HalfWidth float32
	Dir       mathx.Vec // unit vector from A to B
	Length    float32
	Axis      int // 0 when the road runs along X, 1 along Z
	// Ring marks the synthetic road carrying a roundabout's circulating lanes.
	// It has no real geometry, so anything that paints road markings skips it.
	Ring bool
}

// Turn is a connector leading from one lane to a successor lane through a
// junction. The path is a quadratic bezier through Ctrl.
type Turn struct {
	Lane int
	Kind TurnKind
	Ctrl mathx.Vec
	Len  float32
}

// Lane is a directed strip of a road, trimmed back so it stops at the junction
// boundary rather than running into the intersection.
type Lane struct {
	ID    int
	Road  int
	Dir   int // +1 when the lane runs from the road's A node to its B node
	Index int // 0 is the innermost lane, nearest the centreline
	A, B  mathx.Vec

	Heading    float32
	Fwd        mathx.Vec
	Length     float32
	SpeedLimit float32
	Class      RoadClass

	FromNode, ToNode int
	Succ             []Turn

	// Control governs entry into ToNode, and Group is the signal phase group
	// this approach belongs to.
	Control ControlKind
	Group   int
}

// Point returns the world position s metres along the lane.
func (l *Lane) Point(s float32) mathx.Vec {
	return l.A.Add(l.Fwd.Mul(mathx.Clamp(s, 0, l.Length)))
}

// Building is one procedurally placed structure on a city lot.
type Building struct {
	Center mathx.Vec
	W, D   float32
	Height float32
	Shade  uint8 // index into the façade palette
	Floors int
}

// Block is a city block bounded by roads, measured kerb to kerb.
type Block struct {
	Min, Max mathx.Vec
	Park     bool
}

// PropKind identifies a piece of roadside furniture.
type PropKind int

// The prop kinds.
const (
	PropTrafficLight PropKind = iota
	PropStopSign
	PropGiveWaySign
	PropSpeedSign
	PropStreetLamp
)

// Prop is a roadside object. Signals and signs are what the vision system
// annotates, so each records the node and phase group it governs.
type Prop struct {
	Kind PropKind
	Pos  mathx.Vec
	// Heading is the direction of travel of the approach this prop governs.
	// The sign's face points back along it, at the drivers being addressed.
	Heading float32
	Node    int
	Group   int
	Limit   float32 // speed limit in m/s, for PropSpeedSign
	Height  float32
}

// City is a generated world: its lane graph, blocks, buildings and props.
type City struct {
	Seed      uint64
	Nodes     []*Node
	Roads     []*Road
	Lanes     []*Lane
	Blocks    []Block
	Buildings []Building
	Props     []Prop
	Min, Max  mathx.Vec

	grid     map[[2]int][]int // spatial buckets of lane IDs
	cellSize float32
}

// Params tunes city generation.
type Params struct {
	Seed             uint64
	Cols, Rows       int
	MinSpan, MaxSpan float32 // block span range in metres
	// Roundabouts is the share of eligible junctions built as roundabouts.
	Roundabouts float32
}

// DefaultParams returns generation parameters for a reasonably sized city.
func DefaultParams(seed uint64) Params {
	return Params{
		Seed: seed, Cols: 9, Rows: 9, MinSpan: 80, MaxSpan: 150,
		Roundabouts: 0.28,
	}
}

// Generate builds a city from the given parameters. The same seed always
// produces the same city.
func Generate(p Params) *City {
	rng := rng.Stream(p.Seed, StreamLayout)
	c := &City{Seed: p.Seed, cellSize: 40, grid: map[[2]int][]int{}}

	xs := make([]float32, p.Cols)
	zs := make([]float32, p.Rows)
	var x float32
	for i := range p.Cols {
		xs[i] = x
		x += p.MinSpan + rng.Float32()*(p.MaxSpan-p.MinSpan)
	}
	var z float32
	for j := range p.Rows {
		zs[j] = z
		z += p.MinSpan + rng.Float32()*(p.MaxSpan-p.MinSpan)
	}
	// Centre the city on the origin so the camera starts amongst it.
	cx, cz := xs[p.Cols-1]/2, zs[p.Rows-1]/2
	for i := range xs {
		xs[i] -= cx
	}
	for j := range zs {
		zs[j] -= cz
	}

	ids := make([][]int, p.Cols)
	for i := range p.Cols {
		ids[i] = make([]int, p.Rows)
		for j := range p.Rows {
			n := &Node{ID: len(c.Nodes), Pos: mathx.V(xs[i], zs[j]), GridI: i, GridJ: j}
			ids[i][j] = n.ID
			c.Nodes = append(c.Nodes, n)
		}
	}

	for i := range p.Cols {
		for j := range p.Rows {
			if i+1 < p.Cols {
				c.addRoad(ids[i][j], ids[i+1][j], classFor(i, j, 0), 0)
			}
			if j+1 < p.Rows {
				c.addRoad(ids[i][j], ids[i][j+1], classFor(i, j, 1), 1)
			}
		}
	}

	c.removeSomeRoads(rng)
	c.computeRadii()
	c.chooseRoundabouts(rng, p.Roundabouts)
	c.buildLanes()
	c.assignControls()
	c.buildConnectors()
	c.buildRings()
	c.placeProps(rng)
	c.buildBlocks(rng, xs, zs)
	c.clearRoundabouts()
	c.indexLanes()
	c.computeBounds()
	return c
}

func classFor(i, j, axis int) RoadClass {
	// Every third grid line is an arterial, which gives the city a clear
	// skeleton of fast roads with quieter streets between them.
	line := i
	if axis == 0 {
		line = j
	}
	switch line % 3 {
	case 0:
		return Arterial
	case 1:
		return Collector
	default:
		return Local
	}
}

func (c *City) addRoad(a, b int, cls RoadClass, axis int) {
	na, nb := c.Nodes[a], c.Nodes[b]
	d := nb.Pos.Sub(na.Pos)
	r := &Road{
		ID: len(c.Roads), A: a, B: b, Class: cls, Axis: axis,
		Dir: d.Norm(), Length: d.Len(),
		HalfWidth: float32(cls.Lanes()) * LaneWidth,
	}
	c.Roads = append(c.Roads, r)
	na.Roads = append(na.Roads, r.ID)
	nb.Roads = append(nb.Roads, r.ID)
}

func (c *City) removeSomeRoads(rng *rand.Rand) {
	// Deleting a few local roads breaks up the regularity of the grid. Both
	// endpoints must keep at least three connections, so this never creates a
	// dead end or a degenerate junction.
	removed := map[int]bool{}
	target := len(c.Roads) / 14
	for _, id := range rng.Perm(len(c.Roads)) {
		if len(removed) >= target {
			break
		}
		r := c.Roads[id]
		if r.Class != Local {
			continue
		}
		if c.Nodes[r.A].Degree() <= 3 || c.Nodes[r.B].Degree() <= 3 {
			continue
		}
		removed[id] = true
		c.detach(c.Nodes[r.A], id)
		c.detach(c.Nodes[r.B], id)
	}
	if len(removed) == 0 {
		return
	}

	// Compact the road slice and remap the IDs the nodes hold.
	remap := make(map[int]int, len(c.Roads))
	kept := c.Roads[:0]
	for _, r := range c.Roads {
		if removed[r.ID] {
			continue
		}
		remap[r.ID] = len(kept)
		r.ID = len(kept)
		kept = append(kept, r)
	}
	c.Roads = kept
	for _, n := range c.Nodes {
		for k, rid := range n.Roads {
			n.Roads[k] = remap[rid]
		}
	}
}

func (c *City) detach(n *Node, roadID int) {
	for k, rid := range n.Roads {
		if rid == roadID {
			n.Roads = append(n.Roads[:k], n.Roads[k+1:]...)
			return
		}
	}
}

func (c *City) computeRadii() {
	for _, n := range c.Nodes {
		var r float32
		for _, rid := range n.Roads {
			r = max(r, c.Roads[rid].HalfWidth)
		}
		n.Radius = r
	}
}

func (c *City) buildLanes() {
	side := trafficSide()
	for _, r := range c.Roads {
		na, nb := c.Nodes[r.A], c.Nodes[r.B]
		// Trim each end back by the junction radius so lanes stop at the
		// intersection boundary.
		startA := na.Pos.Add(r.Dir.Mul(na.Radius))
		endB := nb.Pos.Sub(r.Dir.Mul(nb.Radius))
		if endB.Sub(startA).Dot(r.Dir) < 6 {
			// Very short block: keep a minimum drivable stub in the middle.
			mid := na.Pos.Lerp(nb.Pos, 0.5)
			startA = mid.Sub(r.Dir.Mul(3))
			endB = mid.Add(r.Dir.Mul(3))
		}

		for idx := range r.Class.Lanes() {
			off := (0.5 + float32(idx)) * LaneWidth * side

			ab := &Lane{
				ID: len(c.Lanes), Road: r.ID, Dir: +1, Index: idx,
				FromNode: r.A, ToNode: r.B,
				SpeedLimit: r.Class.SpeedLimit(), Class: r.Class,
			}
			shift := r.Dir.Right().Mul(off)
			ab.A, ab.B = startA.Add(shift), endB.Add(shift)
			ab.finish()
			c.Lanes = append(c.Lanes, ab)
			r.LanesAB = append(r.LanesAB, ab.ID)

			ba := &Lane{
				ID: len(c.Lanes), Road: r.ID, Dir: -1, Index: idx,
				FromNode: r.B, ToNode: r.A,
				SpeedLimit: r.Class.SpeedLimit(), Class: r.Class,
			}
			shift = r.Dir.Mul(-1).Right().Mul(off)
			ba.A, ba.B = endB.Add(shift), startA.Add(shift)
			ba.finish()
			c.Lanes = append(c.Lanes, ba)
			r.LanesBA = append(r.LanesBA, ba.ID)
		}
	}
}

func (l *Lane) finish() {
	d := l.B.Sub(l.A)
	l.Length = d.Len()
	l.Fwd = d.Norm()
	l.Heading = l.Fwd.Angle()
}

func (c *City) assignControls() {
	for _, n := range c.Nodes {
		if n.Roundabout {
			n.Control = ControlGiveWay
			continue
		}
		if n.Degree() < 3 {
			n.Control = ControlNone
			continue
		}
		hasArterial := false
		for _, rid := range n.Roads {
			if c.Roads[rid].Class == Arterial {
				hasArterial = true
				break
			}
		}
		switch {
		case hasArterial:
			n.Control = ControlSignal
			n.Signalised = true
		case n.ID%7 == 0:
			// A minority of priority junctions are signed STOP rather than
			// GIVE WAY, which is roughly how they are distributed in reality.
			n.Control = ControlStop
		default:
			n.Control = ControlGiveWay
		}
	}

	for _, l := range c.Lanes {
		n := c.Nodes[l.ToNode]
		r := c.Roads[l.Road]
		l.Group = r.Axis
		if n.Control == ControlSignal {
			l.Control = ControlSignal
			continue
		}
		if n.Control == ControlNone {
			l.Control = ControlNone
			continue
		}
		// At a priority junction only the minor approaches are signed; the
		// major road runs through. Where every road is the same class, the
		// approaches along Z give way to those along X.
		major := Local
		for _, rid := range n.Roads {
			major = max(major, c.Roads[rid].Class)
		}
		minor := r.Class < major || (r.Class == major && r.Axis == 1 && sameClassJunction(c, n))
		if minor {
			l.Control = n.Control
		} else {
			l.Control = ControlNone
		}
	}
}

func sameClassJunction(c *City, n *Node) bool {
	first := c.Roads[n.Roads[0]].Class
	for _, rid := range n.Roads[1:] {
		if c.Roads[rid].Class != first {
			return false
		}
	}
	return true
}

func (c *City) buildConnectors() {
	for _, l := range c.Lanes {
		n := c.Nodes[l.ToNode]
		if n.Roundabout {
			continue // wired by buildRing, once the ring lanes exist
		}
		fromLanes := c.Roads[l.Road].Class.Lanes()
		for _, rid := range n.Roads {
			if rid == l.Road {
				continue // no U-turns
			}
			r := c.Roads[rid]
			for _, oid := range c.exitLanes(r, n.ID) {
				o := c.Lanes[oid]
				kind := turnKind(l.Fwd, o.Fwd)
				if !turnAllowed(kind, l.Index, fromLanes) {
					continue
				}
				// Keep the lane index stable where the counts allow, so cars
				// do not weave across the carriageway at every junction.
				want := preferredExitIndex(kind, l.Index, r.Class.Lanes())
				if o.Index != want {
					continue
				}
				l.Succ = append(l.Succ, c.makeTurn(l, o, kind, n))
			}
		}
		if len(l.Succ) == 0 {
			// Fall back to any non-U-turn exit so a vehicle never strands.
			for _, rid := range n.Roads {
				if rid == l.Road {
					continue
				}
				for _, oid := range c.exitLanes(c.Roads[rid], n.ID) {
					o := c.Lanes[oid]
					l.Succ = append(l.Succ, c.makeTurn(l, o, turnKind(l.Fwd, o.Fwd), n))
				}
			}
		}
	}
}

func (c *City) exitLanes(r *Road, node int) []int {
	if r.A == node {
		return r.LanesAB
	}
	return r.LanesBA
}

func (c *City) makeTurn(from, to *Lane, kind TurnKind, n *Node) Turn {
	ctrl := controlPoint(from, to, n)
	return Turn{Lane: to.ID, Kind: kind, Ctrl: ctrl, Len: bezierLength(from.B, ctrl, to.A)}
}

func controlPoint(from, to *Lane, n *Node) mathx.Vec {
	// Put the bezier control where the two lane directions would meet, which
	// produces a natural turn arc. Parallel lanes have no such point, so fall
	// back to the midpoint.
	denom := from.Fwd.Cross(to.Fwd)
	if mathx.Abs(denom) < 1e-4 {
		return from.B.Lerp(to.A, 0.5)
	}
	t := to.A.Sub(from.B).Cross(to.Fwd) / denom
	return from.B.Add(from.Fwd.Mul(mathx.Clamp(t, 0, n.Radius*2.5)))
}

func turnKind(from, to mathx.Vec) TurnKind {
	switch {
	case from.Dot(to) > 0.7:
		return TurnStraight
	case from.Cross(to) > 0:
		return TurnRight
	default:
		return TurnLeft
	}
}

func turnAllowed(k TurnKind, fromIdx, fromLanes int) bool {
	// On a single-lane approach every movement is permitted. On a wider one,
	// left turns are made from the inside lane and right turns from the
	// outside lane.
	if fromLanes == 1 {
		return true
	}
	// The turn away from the centreline is made from the outside lane and the
	// turn across the road from the inside one.
	nearside, offside := fromLanes-1, 0
	if DriveOnLeft {
		nearside, offside = 0, fromLanes-1
	}
	switch k {
	case TurnLeft:
		if DriveOnLeft {
			return fromIdx == nearside
		}
		return fromIdx == offside
	case TurnRight:
		if DriveOnLeft {
			return fromIdx == offside
		}
		return fromIdx == nearside
	default:
		return true
	}
}

func preferredExitIndex(k TurnKind, fromIdx, toLanes int) int {
	if toLanes == 1 {
		return 0
	}
	nearside, offside := toLanes-1, 0
	if DriveOnLeft {
		nearside, offside = 0, toLanes-1
	}
	switch k {
	case TurnLeft:
		if DriveOnLeft {
			return nearside
		}
		return offside
	case TurnRight:
		if DriveOnLeft {
			return offside
		}
		return nearside
	default:
		return min(fromIdx, toLanes-1)
	}
}

func bezierLength(p0, p1, p2 mathx.Vec) float32 {
	const steps = 8
	var total float32
	prev := p0
	for i := 1; i <= steps; i++ {
		p := mathx.Bezier(p0, p1, p2, float32(i)/steps)
		total += p.DistTo(prev)
		prev = p
	}
	return total
}

// TurnPoint returns the position a fraction u along a connector leaving lane
// from.
func (c *City) TurnPoint(from *Lane, t Turn, u float32) mathx.Vec {
	return mathx.Bezier(from.B, t.Ctrl, c.Lanes[t.Lane].A, mathx.Clamp(u, 0, 1))
}

// TurnTangent returns the unit heading a fraction u along a connector leaving
// lane from.
func (c *City) TurnTangent(from *Lane, t Turn, u float32) mathx.Vec {
	return mathx.BezierTangent(from.B, t.Ctrl, c.Lanes[t.Lane].A, mathx.Clamp(u, 0, 1)).Norm()
}

func kerbside(l *Lane, r *Road, clearance, side float32) mathx.Vec {
	return kerbsideAt(l, r, l.B.Sub(l.Fwd.Mul(1.0)), clearance, side)
}

func kerbsideAt(l *Lane, r *Road, along mathx.Vec, clearance, side float32) mathx.Vec {
	// Step back from the lane centre to the road centreline, then out to the
	// kerb on the side traffic drives on. Measuring from the road rather than
	// the lane keeps signs just inside the kerb on wide roads, where they would
	// otherwise drift out past it and leave the camera's view on approach.
	laneOffset := (0.5 + float32(l.Index)) * LaneWidth * side
	out := (r.HalfWidth + clearance) * side
	return along.Add(l.Fwd.Right().Mul(out - laneOffset))
}

func trafficSide() float32 {
	// Lanes sit to the left of the centreline when traffic keeps left.
	if DriveOnLeft {
		return -1
	}
	return 1
}

func (c *City) placeProps(rng *rand.Rand) {
	side := trafficSide()

	// One signal or sign per approach, mounted at the kerb by the stop line.
	// The offset is measured from the road centreline rather than from the
	// lane, so a sign lands just inside the kerb instead of drifting out past
	// it on a wide road, where it would leave the camera's view on approach.
	for _, l := range c.Lanes {
		if l.Index != 0 {
			continue
		}
		r := c.Roads[l.Road]
		pos := kerbside(l, r, 1.2, side)
		switch l.Control {
		case ControlSignal:
			c.Props = append(c.Props, Prop{
				Kind: PropTrafficLight, Pos: pos, Heading: l.Heading,
				Node: l.ToNode, Group: l.Group, Height: 3.4,
			})
		case ControlStop:
			c.Props = append(c.Props, Prop{
				Kind: PropStopSign, Pos: pos, Heading: l.Heading,
				Node: l.ToNode, Group: l.Group, Height: 2.2,
			})
		case ControlGiveWay:
			c.Props = append(c.Props, Prop{
				Kind: PropGiveWaySign, Pos: pos, Heading: l.Heading,
				Node: l.ToNode, Group: l.Group, Height: 2.2,
			})
		}
	}

	// Speed limit signs shortly after a junction, so a driver sees the limit
	// for the road they have just joined.
	for _, l := range c.Lanes {
		if l.Index != 0 || l.Length < 30 || rng.Float32() > 0.55 {
			continue
		}
		r := c.Roads[l.Road]
		pos := kerbsideAt(l, r, l.A.Add(l.Fwd.Mul(10)), 1.2, side)
		c.Props = append(c.Props, Prop{
			Kind: PropSpeedSign, Pos: pos, Heading: l.Heading,
			Limit: l.SpeedLimit, Height: 2.3,
		})
	}

	for _, r := range c.Roads {
		if r.Class != Arterial {
			continue
		}
		na, nb := c.Nodes[r.A], c.Nodes[r.B]
		for d := na.Radius + 12; d < r.Length-nb.Radius-6; d += 32 {
			p := na.Pos.Add(r.Dir.Mul(d)).Add(r.Dir.Right().Mul(r.HalfWidth + 1.2))
			c.Props = append(c.Props, Prop{Kind: PropStreetLamp, Pos: p, Heading: r.Dir.Angle(), Height: 6})
		}
	}
}

func (c *City) buildBlocks(rng *rand.Rand, xs, zs []float32) {
	// A block is inset from its grid lines by the widest road half-width on
	// each side, so buildings never overlap a carriageway.
	hwCol := make([]float32, len(xs))
	hwRow := make([]float32, len(zs))
	for _, r := range c.Roads {
		if r.Ring {
			continue
		}
		if r.Axis == 1 {
			i := c.Nodes[r.A].GridI
			hwCol[i] = max(hwCol[i], r.HalfWidth)
		} else {
			j := c.Nodes[r.A].GridJ
			hwRow[j] = max(hwRow[j], r.HalfWidth)
		}
	}
	for i := range hwCol {
		hwCol[i] = max(hwCol[i], LaneWidth)
	}
	for j := range hwRow {
		hwRow[j] = max(hwRow[j], LaneWidth)
	}

	var maxDist float32 = 1
	for _, n := range c.Nodes {
		maxDist = max(maxDist, n.Pos.Len())
	}

	for i := 0; i+1 < len(xs); i++ {
		for j := 0; j+1 < len(zs); j++ {
			lo := mathx.V(xs[i]+hwCol[i], zs[j]+hwRow[j])
			hi := mathx.V(xs[i+1]-hwCol[i+1], zs[j+1]-hwRow[j+1])
			if hi.X-lo.X < 14 || hi.Z-lo.Z < 14 {
				continue
			}
			park := rng.Float32() < 0.10
			c.Blocks = append(c.Blocks, Block{Min: lo, Max: hi, Park: park})
			if park {
				continue
			}
			centre := mathx.V((lo.X+hi.X)/2, (lo.Z+hi.Z)/2)
			c.fillBlock(rng, lo, hi, 1-mathx.Clamp(centre.Len()/maxDist, 0, 1))
		}
	}
}

func (c *City) fillBlock(rng *rand.Rand, lo, hi mathx.Vec, downtown float32) {
	// Buildings line the perimeter of the block facing the street, leaving the
	// interior empty. downtown runs 0 at the edge of the city to 1 at its
	// centre, and drives how tall the towers get.
	x0, z0 := lo.X+PavementWidth, lo.Z+PavementWidth
	x1, z1 := hi.X-PavementWidth, hi.Z-PavementWidth
	if x1-x0 < 10 || z1-z0 < 10 {
		return
	}
	depth := min(18, (x1-x0)/2-1, (z1-z0)/2-1)
	if depth < 6 {
		depth = min((x1-x0)/2, (z1-z0)/2)
	}

	add := func(cx, cz, w, d float32) {
		if w < 5 || d < 5 {
			return
		}
		h := 7 + rng.Float32()*10 + downtown*downtown*(18+rng.Float32()*58)
		c.Buildings = append(c.Buildings, Building{
			Center: mathx.V(cx, cz), W: w, D: d, Height: h,
			Shade:  uint8(rng.IntN(6)),
			Floors: max(1, int(h/3.2)),
		})
	}

	for _, zc := range [2]float32{z0 + depth/2, z1 - depth/2} {
		for x := x0; x < x1-4; {
			w := min(11+rng.Float32()*15, x1-x)
			if rng.Float32() >= 0.12 { // occasional gap: an alley or courtyard
				add(x+w/2, zc, w-1.2, depth)
			}
			x += w
		}
	}
	// The side strips skip the corners already covered above.
	for _, xc := range [2]float32{x0 + depth/2, x1 - depth/2} {
		for z := z0 + depth; z < z1-depth-4; {
			d := min(11+rng.Float32()*15, z1-depth-z)
			if rng.Float32() >= 0.12 {
				add(xc, z+d/2, depth, d-1.2)
			}
			z += d
		}
	}
}

func (c *City) clearRoundabouts() {
	// A roundabout is far wider than an ordinary junction, so a block laid out
	// against the road widths alone can overlap one. Drop anything standing in
	// the carriageway.
	kept := c.Buildings[:0]
	for _, b := range c.Buildings {
		clash := false
		for _, n := range c.Nodes {
			if !n.Roundabout {
				continue
			}
			if b.Center.DistTo(n.Pos) < n.ringRadius+6+max(b.W, b.D)/2 {
				clash = true
				break
			}
		}
		if !clash {
			kept = append(kept, b)
		}
	}
	c.Buildings = kept
}

// Roundabouts returns the roundabout junctions, for rendering their islands.
func (c *City) Roundabouts() []*Node {
	var out []*Node
	for _, n := range c.Nodes {
		if n.Roundabout {
			out = append(out, n)
		}
	}
	return out
}

func (c *City) indexLanes() {
	for _, l := range c.Lanes {
		steps := int(l.Length/c.cellSize) + 1
		for i := 0; i <= steps; i++ {
			p := l.Point(float32(i) / float32(steps) * l.Length)
			key := [2]int{int(p.X / c.cellSize), int(p.Z / c.cellSize)}
			if b := c.grid[key]; len(b) == 0 || b[len(b)-1] != l.ID {
				c.grid[key] = append(b, l.ID)
			}
		}
	}
}

func (c *City) computeBounds() {
	if len(c.Nodes) == 0 {
		return
	}
	c.Min, c.Max = c.Nodes[0].Pos, c.Nodes[0].Pos
	for _, n := range c.Nodes {
		c.Min.X = min(c.Min.X, n.Pos.X)
		c.Min.Z = min(c.Min.Z, n.Pos.Z)
		c.Max.X = max(c.Max.X, n.Pos.X)
		c.Max.Z = max(c.Max.Z, n.Pos.Z)
	}
}

// LaneProjection describes where a world point sits relative to a lane.
type LaneProjection struct {
	Lane    *Lane
	S       float32 // distance along the lane
	Lateral float32 // signed offset from the centreline, positive to the right
	Dist    float32 // absolute distance to the centreline
	Valid   bool
}

// Project returns the lane whose centreline lies nearest to p. The zero
// LaneProjection with Valid false is returned when no lane is close enough.
func (c *City) Project(p mathx.Vec) LaneProjection {
	best := LaneProjection{Dist: 1e9}
	ci, cj := int(p.X/c.cellSize), int(p.Z/c.cellSize)
	for di := -1; di <= 1; di++ {
		for dj := -1; dj <= 1; dj++ {
			for _, id := range c.grid[[2]int{ci + di, cj + dj}] {
				l := c.Lanes[id]
				s := mathx.Clamp(p.Sub(l.A).Dot(l.Fwd), 0, l.Length)
				on := l.A.Add(l.Fwd.Mul(s))
				if d := on.DistTo(p); d < best.Dist {
					best = LaneProjection{
						Lane: l, S: s, Dist: d, Valid: true,
						Lateral: p.Sub(on).Dot(l.Fwd.Right()),
					}
				}
			}
		}
	}
	return best
}

// RoadHalfWidth returns the carriageway half-width of the road carrying l.
func (c *City) RoadHalfWidth(l *Lane) float32 { return c.Roads[l.Road].HalfWidth }

// RandomLane returns a random lane at least minLen metres long, for spawning.
func (c *City) RandomLane(rng *rand.Rand, minLen float32) *Lane {
	for range 200 {
		if l := c.Lanes[rng.IntN(len(c.Lanes))]; l.Length >= minLen {
			return l
		}
	}
	return c.Lanes[0]
}
