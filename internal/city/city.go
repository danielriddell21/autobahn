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

	"github.com/danielriddell21/crucible/geom"
	"github.com/danielriddell21/crucible/spatial"
	"github.com/danielriddell21/crucible/worldgen"

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

	// How far the junction has got. settled means its shape is final; wired,
	// that its roads are built, its lanes joined up and its signs standing;
	// filled, that the block reaching away from it carries its buildings. See
	// grow.go.
	settled bool
	wired   bool
	filled  bool
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

	// lanes indexes lane IDs by position, so Project narrows to a handful of
	// candidates instead of sweeping every lane in the city.
	lanes *spatial.Grid[int]

	// The bookkeeping behind growing outward. lat is the street plan the city
	// is cut from; the rest records how far each part of it has got, so
	// nothing is ever built twice or revised once built. See grow.go.
	params Params
	lat    *worldgen.Lattice
	roadOf map[int]int // lattice edge ID to road ID
	// lastCells is the area the previous growth covered, so asking for it again
	// costs nothing.
	lastCells geom.Rect
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

// thinShare is the fraction of streets the generator tries to delete. A grid
// with every link intact reads as graph paper; losing about one local street
// in fourteen is enough to break that up without making the place hard to
// navigate.
const thinShare = 1.0 / 14

// Generate builds a city of the size the parameters ask for. The same seed
// always produces the same city.
//
// It is the growing city underneath, brought up to its full extent in one
// call: a world that does not need to extend is simply one nobody grows.
func Generate(p Params) *City {
	c := NewGrowing(p)
	// The lattice is centred on the origin, so half the grid lies either side.
	span := float32(max(p.Cols, p.Rows)) * (p.MinSpan + p.MaxSpan) / 4
	c.EnsureAround(mathx.V(0, 0), span)
	return c
}

// axisOf maps a lattice edge's direction onto the city's own axis numbering:
// 0 along X, 1 along Z.
func axisOf(e *worldgen.LatticeEdge) int {
	if e.Axis == worldgen.AxisX {
		return 0
	}
	return 1
}

// latticeClass grades an edge before the city proper exists, which is what
// lets the thinning pass protect the main roads.
func latticeClass(l *worldgen.Lattice, e *worldgen.LatticeEdge) RoadClass {
	return classFor(l.Nodes[e.A].Col, l.Nodes[e.A].Row, axisOf(e))
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

func (c *City) addRoad(a, b int, cls RoadClass, axis int) *Road {
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
	return r
}

// halfWidthOf is a road class's carriageway half-width. A junction needs it
// before any road exists, to work out how wide to be.
func halfWidthOf(cls RoadClass) float32 { return float32(cls.Lanes()) * LaneWidth }

// buildRoadLanes lays the lanes along one road, trimmed back to the junction
// at each end. Both junctions must have settled, or the trim would be wrong.
func (c *City) buildRoadLanes(r *Road) {
	side := trafficSide()
	{
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
			c.indexLane(ab)
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
			c.indexLane(ba)
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

// assignLaneControl decides how one approach is governed. Its junction must
// have settled, so that what the other approaches are is already known.
func (c *City) assignLaneControl(l *Lane) {
	n := c.Nodes[l.ToNode]
	r := c.Roads[l.Road]
	l.Group = r.Axis
	switch n.Control {
	case ControlSignal:
		l.Control = ControlSignal
		return
	case ControlNone:
		l.Control = ControlNone
		return
	}
	// At a priority junction only the minor approaches are signed; the major
	// road runs through. Where every road is the same class, the approaches
	// along Z give way to those along X.
	major := Local
	for _, rid := range n.Roads {
		major = max(major, c.Roads[rid].Class)
	}
	if r.Class < major || (r.Class == major && r.Axis == 1 && sameClassJunction(c, n)) {
		l.Control = n.Control
		return
	}
	l.Control = ControlNone
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

// connectLane works out where one lane may go at its far end. Every road at
// that junction must exist, or an exit would be missed and never revisited.
func (c *City) connectLane(l *Lane, n *Node) {
	l.Succ = c.preferredTurns(l, n)
	if len(l.Succ) == 0 {
		// Fall back to any non-U-turn exit so a vehicle never strands.
		l.Succ = c.anyTurns(l, n)
	}
}

func (c *City) preferredTurns(l *Lane, n *Node) []Turn {
	fromLanes := c.Roads[l.Road].Class.Lanes()
	var out []Turn
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
			if o.Index != preferredExitIndex(kind, l.Index, r.Class.Lanes()) {
				continue
			}
			out = append(out, c.makeTurn(l, o, kind, n))
		}
	}
	return out
}

func (c *City) anyTurns(l *Lane, n *Node) []Turn {
	var out []Turn
	for _, rid := range n.Roads {
		if rid == l.Road {
			continue
		}
		for _, oid := range c.exitLanes(c.Roads[rid], n.ID) {
			o := c.Lanes[oid]
			out = append(out, c.makeTurn(l, o, turnKind(l.Fwd, o.Fwd), n))
		}
	}
	return out
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

// placeNodeProps puts up the signs and lamps belonging to one junction: the
// signals and priority signs on its approaches, the speed signs just past it,
// and the lamps along the arterials leaving it.
//
// Everything is hung off a junction rather than swept over the whole city, so
// a road gets its furniture exactly once — when the junction at its A end is
// wired — however the city was reached.
func (c *City) placeNodeProps(n *Node) {
	side := trafficSide()
	for _, rid := range n.Roads {
		r := c.Roads[rid]
		if r.Ring {
			continue
		}
		for _, lid := range c.approachLanes(r, n.ID) {
			c.placeApproachProp(c.Lanes[lid], r, side)
		}
		// The furniture along a road belongs to the junction it starts from,
		// so it is not put up twice from either end.
		if r.A == n.ID {
			c.placeRoadProps(r, side)
		}
	}
}

// placeApproachProp puts the signal or sign governing one approach at its
// stop line.
func (c *City) placeApproachProp(l *Lane, r *Road, side float32) {
	if l.Index != 0 {
		return
	}
	// The offset is measured from the road centreline rather than from the
	// lane, so a sign lands just inside the kerb instead of drifting out past
	// it on a wide road, where it would leave the camera's view on approach.
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

// placeRoadProps puts the speed signs and street lamps along one road.
func (c *City) placeRoadProps(r *Road, side float32) {
	na, nb := c.Nodes[r.A], c.Nodes[r.B]

	// Speed limit signs shortly after a junction, so a driver sees the limit
	// for the road they have just joined.
	for i, lid := range append(append([]int{}, r.LanesAB...), r.LanesBA...) {
		l := c.Lanes[lid]
		if l.Index != 0 || l.Length < 30 {
			continue
		}
		if cellValue(c.Seed, speedSignSalt, na.GridI, na.GridJ, i) > speedSignShare {
			continue
		}
		pos := kerbsideAt(l, r, l.A.Add(l.Fwd.Mul(10)), 1.2, side)
		c.Props = append(c.Props, Prop{
			Kind: PropSpeedSign, Pos: pos, Heading: l.Heading,
			Limit: l.SpeedLimit, Height: 2.3,
		})
	}

	if r.Class != Arterial {
		return
	}
	for d := na.Radius + 12; d < r.Length-nb.Radius-6; d += 32 {
		p := na.Pos.Add(r.Dir.Mul(d)).Add(r.Dir.Right().Mul(r.HalfWidth + 1.2))
		c.Props = append(c.Props, Prop{Kind: PropStreetLamp, Pos: p, Heading: r.Dir.Angle(), Height: 6})
	}
}

// fillCell lays the block between four junctions: its pavement, and the
// buildings around its edge.
//
// The inset is the widest carriageway on each side, so nothing overlaps a
// road. A roundabout is wider than the road that made it, which the inset
// cannot express — the renderer paves over the difference.
func (c *City) fillCell(col, row int, corners [4]*Node) {
	lo := corners[0].Pos
	hi := corners[3].Pos
	insetX := max(corners[0].Radius, corners[2].Radius, LaneWidth)
	insetZ := max(corners[0].Radius, corners[1].Radius, LaneWidth)
	farX := max(corners[1].Radius, corners[3].Radius, LaneWidth)
	farZ := max(corners[2].Radius, corners[3].Radius, LaneWidth)

	lo = mathx.V(lo.X+insetX, lo.Z+insetZ)
	hi = mathx.V(hi.X-farX, hi.Z-farZ)
	if hi.X-lo.X < 14 || hi.Z-lo.Z < 14 {
		return
	}

	park := cellChance(c.Seed, parkSalt, col, row) < parkShare
	c.Blocks = append(c.Blocks, Block{Min: lo, Max: hi, Park: park})
	if park {
		return
	}
	// downtown runs 0 at the edge of a district to 1 at its middle, and drives
	// how tall the towers get. It is a function of where the block is rather
	// than of how far it happens to be from the furthest node generated so
	// far, which in a growing city would change as the world extended.
	centre := mathx.V((lo.X+hi.X)/2, (lo.Z+hi.Z)/2)
	downtown := 1 - mathx.Clamp(centre.Len()/downtownFalloff, 0, 1)
	c.fillBlock(col, row, lo, hi, downtown)
}

// fillBlock lines the perimeter of a block with buildings facing the street,
// leaving the interior empty. downtown runs 0 at the edge of a district to 1
// at its middle, and drives how tall the towers get.
//
// Every draw comes from the block's own cell rather than from a running
// stream, so a block put up an hour into a drive is the one that would have
// been there from the start.
func (c *City) fillBlock(col, row int, lo, hi mathx.Vec, downtown float32) {
	x0, z0 := lo.X+PavementWidth, lo.Z+PavementWidth
	x1, z1 := hi.X-PavementWidth, hi.Z-PavementWidth
	if x1-x0 < 10 || z1-z0 < 10 {
		return
	}
	depth := min(18, (x1-x0)/2-1, (z1-z0)/2-1)
	if depth < 6 {
		depth = min((x1-x0)/2, (z1-z0)/2)
	}

	// draw hands out this block's numbers, one per call, so the sequence is
	// fixed by the cell rather than by how many blocks came before it.
	n := 0
	draw := func() float32 {
		n++
		return cellValue(c.Seed, buildingSalt, col, row, n)
	}
	add := func(cx, cz, w, d float32) {
		if w < 5 || d < 5 {
			return
		}
		h := 7 + draw()*10 + downtown*downtown*(18+draw()*58)
		c.Buildings = append(c.Buildings, Building{
			Center: mathx.V(cx, cz), W: w, D: d, Height: h,
			Shade:  uint8(draw() * 6),
			Floors: max(1, int(h/3.2)),
		})
	}

	for _, zc := range [2]float32{z0 + depth/2, z1 - depth/2} {
		for x := x0; x < x1-4; {
			w := min(11+draw()*15, x1-x)
			if draw() >= alleyShare { // occasional gap: an alley or courtyard
				add(x+w/2, zc, w-1.2, depth)
			}
			x += w
		}
	}
	// The side strips skip the corners already covered above.
	for _, xc := range [2]float32{x0 + depth/2, x1 - depth/2} {
		for z := z0 + depth; z < z1-depth-4; {
			d := min(11+draw()*15, z1-depth-z)
			if draw() >= alleyShare {
				add(xc, z+d/2, depth, d-1.2)
			}
			z += d
		}
	}
	c.clearRoundabouts(col, row)
}

// clearRoundabouts drops any building of the cell just filled that stands in a
// roundabout's carriageway. A roundabout is far wider than the road that made
// it, and the block inset is worked out from road widths alone, so a block
// against one can overlap it.
func (c *City) clearRoundabouts(col, row int) {
	corners, ok := c.cellCorners(col, row)
	if !ok {
		return
	}
	kept := c.Buildings[:0]
	for _, b := range c.Buildings {
		clash := false
		for _, n := range corners {
			if !n.Roundabout {
				continue
			}
			if b.Center.DistTo(n.Pos) < n.ringRadius+6+max(b.W, b.D)/2 {
				clash = true
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

// laneCellSize is the spatial index's cell size in metres. It is a little
// under the shortest block, so a lookup lands on a cell holding the lanes
// around it rather than half the district.
const laneCellSize = 40

// ground converts a world position to the plane crucible's spatial grid works
// in. The simulation carries float32 on XZ; the engine is float64 on XY.
func ground(p mathx.Vec) geom.Vec2 {
	return geom.Vec2{X: float64(p.X), Y: float64(p.Z)}
}

// indexLane files one lane in the spatial index. A lane is a line, not a
// point, so it is walked and filed in every cell it crosses; InsertOnce
// collapses the steps that land in the same one.
func (c *City) indexLane(l *Lane) {
	steps := int(l.Length/laneCellSize) + 1
	for i := 0; i <= steps; i++ {
		p := l.Point(float32(i) / float32(steps) * l.Length)
		spatial.InsertOnce(c.lanes, ground(p), l.ID)
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
	c.lanes.Near(ground(p), laneCellSize, func(id int) {
		l := c.Lanes[id]
		s := mathx.Clamp(p.Sub(l.A).Dot(l.Fwd), 0, l.Length)
		on := l.A.Add(l.Fwd.Mul(s))
		if d := on.DistTo(p); d < best.Dist {
			best = LaneProjection{
				Lane: l, S: s, Dist: d, Valid: true,
				Lateral: p.Sub(on).Dot(l.Fwd.Right()),
			}
		}
	})
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
