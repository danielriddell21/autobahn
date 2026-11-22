// Package works says where the roads are dug up, and when.
//
// Roadworks are a function of the seed, the street and the clock, and of
// nothing else: which streets are closed at a given moment is worked out from
// where each one is on the lattice, never from what has happened, so a street
// reached from the north an hour into a drive is shut exactly when the same
// street reached from the south at the start would have been. This is the
// growth rule of package city carried over to time.
//
// The package never touches the lane graph. It only says which lanes are barred
// and where the cones and barriers stand, and whoever drives (package sim)
// filters the turns it chooses from and collides with what it is told is solid.
// It imports package city to read the ground and package mathx, and nothing
// else, so it knows nothing of the world's clock beyond the number it is given.
package works

import (
	"fmt"
	"math"
	"sort"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Level is how much of the town is dug up. Its zero value is Normal: roadworks
// are part of the world unless somebody turns them off.
type Level int

// The levels.
const (
	Normal Level = iota
	Off
	Dense
)

// Name returns the level's name, as a flag reads it.
func (l Level) Name() string {
	switch l {
	case Off:
		return "off"
	case Dense:
		return "dense"
	default:
		return "normal"
	}
}

// ParseLevel reads a level's name.
func ParseLevel(name string) (Level, bool) {
	for _, l := range [...]Level{Normal, Off, Dense} {
		if l.Name() == name {
			return l, true
		}
	}
	return Normal, false
}

// scale is the factor a level puts on a map's share of works.
func (l Level) scale() float32 {
	switch l {
	case Off:
		return 0
	case Dense:
		return 8
	default:
		return 1
	}
}

// The timetable, in seconds.
const (
	// SlotLength is how long a closure lasts, and how often the dice are
	// rolled again for a street.
	SlotLength float32 = 900
	// SolidAfter is how long after the signs go up that the cones and barriers
	// are solid: long enough for traffic to have found another way.
	SolidAfter float32 = 60
)

// Salts that keep one decision about a street from correlating with another.
// They differ from every other salt in the game, those of the weather included.
const (
	worksSalt  uint64 = 0x6b17a2e5d3f9
	offsetSalt uint64 = 0x2c9f41b87a63
	sideSalt   uint64 = 0x4d5e8f01c7b2
)

// Heights and sizes of what stands in the road, in metres.
const (
	// SignHeight is the height of the top of the post of a roadworks sign.
	SignHeight float32 = 2.4
	// ConeSize is the footprint of a cone, and BarrierDepth how thick a
	// barrier is along the road.
	ConeSize     float32 = 0.4
	BarrierDepth float32 = 0.5
	// ConeSpacing is how far apart the cones of a coned lane stand.
	ConeSpacing float32 = 4
)

// Where things stand along a lane, in metres.
const (
	// barrierAt is how far into a lane its barrier stands from the junction.
	barrierAt float32 = 2.5
	// coneRun is the length of a coned lane, and coneClear how far short of
	// the stop line it ends, so a driver is never made to merge at a junction.
	coneRun   float32 = 16
	coneClear float32 = 30
	// warnBefore is how far before the first cone the warning sign stands, and
	// signBefore how far before a junction the diversion signs.
	warnBefore float32 = 18
	signBefore float32 = 20
)

// Edge names one street of the plan by its lower junction and its axis, which
// is how a street is told from every other however the city was reached. A
// road's own ID is not used: it counts the order roads were built in.
type Edge struct {
	I, J int
	// Axis is 0 for a street running along X and 1 along Z.
	Axis int
}

// String describes the edge.
func (e Edge) String() string { return fmt.Sprintf("%d,%d/%d", e.I, e.J, e.Axis) }

// Kind is how a street is closed.
type Kind int

// The kinds of closure.
const (
	// KindStreet shuts a local street or collector to anyone entering it, a
	// barrier across each lane at its mouth. What is already in it can leave.
	KindStreet Kind = iota
	// KindLane cones off the kerbside lane of an arterial for part of its
	// length. Nothing about the routing changes: traffic merges.
	KindLane
)

// Phase is how far a closure has got.
type Phase int

// The phases, in order.
const (
	// PhaseClear is a street with nothing on it.
	PhaseClear Phase = iota
	// PhaseSigned is a closure with its signs up. Nobody is routed through it
	// yet but the cones and barriers are not solid.
	PhaseSigned
	// PhaseSolid is the closure complete: its barriers and cones are in the
	// road.
	PhaseSolid
)

// SignKind is what a sign says.
type SignKind int

// The kinds of sign.
const (
	// SignClosed is the road closed sign, at the mouth of a closed street.
	SignClosed SignKind = iota
	// SignDiversion warns of a diversion or roadworks ahead.
	SignDiversion
)

// Sign is a roadworks sign standing at the roadside.
type Sign struct {
	Kind SignKind
	Pos  mathx.Vec
	// Heading is the direction of the traffic the sign is addressed to, as a
	// prop's is: it faces back along it.
	Heading float32
}

// Closure is one street with works on it.
type Closure struct {
	Edge Edge
	Kind Kind
	Road int
	// Lanes are the lanes the works are on: for a KindStreet every lane of the
	// street, barred at its start; for a KindLane the one lane coned.
	Lanes []int
	// Phase is how far the closure has got at the time it was asked for.
	Phase Phase
	// Start is when the signs go up, Solid when the cones and barriers become
	// solid, and End when it is all cleared away.
	Start, Solid, End float32
	// Cones and Barriers are the solid things in the road. They are listed
	// whatever the phase: until Solid they are not there.
	Cones, Barriers []mathx.OBB
	Signs           []Sign
}

// Barred reports whether traffic is told not to enter lane id, which is so from
// the moment the signs go up.
func (c *Closure) Barred(id int) bool {
	if c.Kind != KindStreet || c.Phase == PhaseClear {
		return false
	}
	for _, l := range c.Lanes {
		if l == id {
			return true
		}
	}
	return false
}

// Source is a supplier of closures. The roadworks timetable is one; night
// events that shut a street are another, and need only implement this.
type Source interface {
	// Closures appends to dst every closure the source has at time t whose
	// phase is not PhaseClear, and returns it.
	Closures(t float32, dst []Closure) []Closure
}

// Index is everything closed now. It is brought up to the time every step, and
// as the city grows it picks up the new streets without looking at the old.
type Index struct {
	sources []Source
	active  []Closure
	// barred is every lane traffic is told not to enter, found once an update
	// rather than by every driver asking.
	barred map[int]bool
	now    float32
}

// NewIndex returns the index of a city's roadworks, at the level asked for. A
// map's share of works is Params.Works, which the level scales. Extra sources
// are consulted alongside the timetable.
func NewIndex(c *city.City, seed uint64, level Level, extra ...Source) *Index {
	ix := &Index{barred: map[int]bool{}}
	if share := c.Params().Works * level.scale(); share > 0 {
		ix.sources = append(ix.sources, NewScheduled(c, seed, min(share, 1)))
	}
	ix.sources = append(ix.sources, extra...)
	return ix
}

// Add puts another source of closures on the index.
func (ix *Index) Add(s Source) { ix.sources = append(ix.sources, s) }

// Update brings the index to time t.
func (ix *Index) Update(t float32) {
	if ix == nil {
		return
	}
	ix.now = t
	ix.active = ix.active[:0]
	for _, s := range ix.sources {
		ix.active = s.Closures(t, ix.active)
	}
	clear(ix.barred)
	for i := range ix.active {
		if c := &ix.active[i]; c.Kind == KindStreet {
			for _, id := range c.Lanes {
				ix.barred[id] = true
			}
		}
	}
}

// Active returns the closures in force, in any phase but clear. The slice is
// reused by the next Update.
func (ix *Index) Active() []Closure {
	if ix == nil {
		return nil
	}
	return ix.active
}

// Barred reports whether traffic is told not to enter a lane.
func (ix *Index) Barred(id int) bool {
	return ix != nil && ix.barred[id]
}

// Coned reports whether a lane has cones standing in it, and if so the closure
// they belong to.
func (ix *Index) Coned(id int) (*Closure, bool) {
	if ix == nil {
		return nil, false
	}
	for i := range ix.active {
		c := &ix.active[i]
		if c.Kind != KindLane {
			continue
		}
		for _, l := range c.Lanes {
			if l == id {
				return c, true
			}
		}
	}
	return nil, false
}

// Solid returns the boxes of everything that is in the road at the moment,
// for whoever collides with them.
func (ix *Index) Solid(dst []mathx.OBB) []mathx.OBB {
	for i := range ix.Active() {
		c := &ix.active[i]
		if c.Phase != PhaseSolid {
			continue
		}
		dst = append(dst, c.Cones...)
		dst = append(dst, c.Barriers...)
	}
	return dst
}

// Scheduled is the timetable of roadworks: closures on the streets a map's
// parity pattern allows, switched on and off in slots of SlotLength.
type Scheduled struct {
	c     *city.City
	seed  uint64
	share float32

	// seen is how many roads have been looked at, so growth is only ever
	// examined for the roads it added. pending are candidates whose junctions
	// have not finished yet.
	seen    int
	pending []int
	sites   []*site
}

// site is a street that can be dug up, with its geometry fixed once and for all.
type site struct {
	edge    Edge
	offset  float32
	closure Closure
}

// NewScheduled returns the timetable for a city, closing the given share of its
// closable streets at any moment.
func NewScheduled(c *city.City, seed uint64, share float32) *Scheduled {
	return &Scheduled{c: c, seed: seed, share: share}
}

// Sites returns how many streets are known to be closable so far.
func (s *Scheduled) Sites() int { s.sync(); return len(s.sites) }

// edgeKey names the street a road is.
func edgeKey(c *city.City, r *city.Road) Edge {
	a, b := c.Nodes[r.A], c.Nodes[r.B]
	lo := a
	if b.GridI < a.GridI || (b.GridI == a.GridI && b.GridJ < a.GridJ) {
		lo = b
	}
	return Edge{I: lo.GridI, J: lo.GridJ, Axis: r.Axis}
}

func mod(a, n int) int { return ((a % n) + n) % n }

// Closable reports whether the plan lets a street be closed: a pattern on the
// lattice that never puts two closures on one junction, so every junction has
// at most one arm shut and always somewhere left to go.
func (e Edge) Closable() bool {
	if e.Axis == 0 {
		return mod(e.I, 4) == 0 && mod(e.J, 2) == 0
	}
	return mod(e.I, 4) == 2 && mod(e.J, 2) == 1
}

// row folds the axis into the second coordinate of a hash.
func (e Edge) row() int { return e.J*2 + e.Axis }

// Window returns when a street's current slot of works begins and ends, and
// whether there are works in it at all, at time t. It is a pure function.
func Window(seed uint64, e Edge, share, t float32) (start, end float32, on bool) {
	off := city.Chance(seed, offsetSalt, e.I, e.row()) * SlotLength
	slot := int64(math.Floor(float64((t + off) / SlotLength)))
	start = float32(slot)*SlotLength - off
	end = start + SlotLength
	on = city.Chance(seed^uint64(slot)*0x9e3779b97f4a7c15, worksSalt, e.I, e.row()) < share
	return start, end, on
}

// PhaseAt is how far a closure that begins at start has got at time t.
func PhaseAt(start, end, t float32) Phase {
	switch {
	case t < start || t >= end:
		return PhaseClear
	case t < start+SolidAfter:
		return PhaseSigned
	default:
		return PhaseSolid
	}
}

// Closures implements Source.
func (s *Scheduled) Closures(t float32, dst []Closure) []Closure {
	s.sync()
	for _, st := range s.sites {
		start, end, on := Window(s.seed, st.edge, s.share, t)
		if !on {
			continue
		}
		cl := st.closure
		cl.Start, cl.Solid, cl.End = start, start+SolidAfter, end
		if cl.Phase = PhaseAt(start, end, t); cl.Phase != PhaseClear {
			dst = append(dst, cl)
		}
	}
	return dst
}

// sync looks at the roads built since it last ran, and at the candidates still
// waiting for their junctions to be finished.
func (s *Scheduled) sync() {
	for ; s.seen < len(s.c.Roads); s.seen++ {
		r := s.c.Roads[s.seen]
		if r.Ring || r.A == r.B {
			continue
		}
		if edgeKey(s.c, r).Closable() {
			s.pending = append(s.pending, s.seen)
		}
	}
	kept := s.pending[:0]
	for _, id := range s.pending {
		r := s.c.Roads[id]
		if !s.c.Complete(s.c.Nodes[r.A]) || !s.c.Complete(s.c.Nodes[r.B]) {
			kept = append(kept, id)
			continue
		}
		if st, ok := s.build(r); ok {
			s.sites = append(s.sites, st)
		}
	}
	s.pending = kept
}

// build works out the geometry of a street's closure, once both its junctions
// are finished, and whether the street can be closed at all.
func (s *Scheduled) build(r *city.Road) (*site, bool) {
	c := s.c
	if c.Nodes[r.A].Roundabout || c.Nodes[r.B].Roundabout {
		return nil, false
	}
	e := edgeKey(c, r)
	st := &site{
		edge:   e,
		offset: city.Chance(s.seed, offsetSalt, e.I, e.row()) * SlotLength,
		closure: Closure{
			Edge: e, Road: r.ID,
		},
	}
	cl := &st.closure
	if r.Class == city.Arterial {
		return st, s.cone(r, cl)
	}
	return st, s.shut(r, cl)
}

// shut closes a street to anyone entering it: a barrier across the start of
// each of its lanes, and signs. It refuses when any lane arriving at either end
// would be left with nowhere to go.
func (s *Scheduled) shut(r *city.Road, cl *Closure) bool {
	c := s.c
	if len(r.LanesAB) == 0 || len(r.LanesBA) == 0 {
		return false
	}
	cl.Kind = KindStreet
	cl.Lanes = []int{r.LanesAB[0], r.LanesBA[0]}
	for _, id := range cl.Lanes {
		if c.Lanes[id].Length < 3*barrierAt {
			return false
		}
	}
	for _, end := range [2]int{r.A, r.B} {
		if !s.exitsRemain(r, end, cl.Lanes) {
			return false
		}
	}
	for _, id := range cl.Lanes {
		l := c.Lanes[id]
		dir := l.DirAt(barrierAt)
		cl.Barriers = append(cl.Barriers, mathx.OBB{
			Center: l.Point(barrierAt), HalfW: city.LaneWidth / 2,
			HalfL: BarrierDepth / 2, Heading: dir.Angle(),
		})
		cl.Signs = append(cl.Signs, Sign{
			Kind: SignClosed, Pos: kerb(l, r, barrierAt+1.5, 1.2), Heading: dir.Angle(),
		})
	}
	cl.Signs = append(cl.Signs, s.diversionSigns(r)...)
	// The order roads were built in is the order of a junction's arms, and that
	// depends on how the city was reached. The order of the signs must not.
	sort.Slice(cl.Signs, func(i, j int) bool {
		a, b := cl.Signs[i], cl.Signs[j]
		switch {
		case a.Kind != b.Kind:
			return a.Kind < b.Kind
		case a.Pos.X != b.Pos.X:
			return a.Pos.X < b.Pos.X
		default:
			return a.Pos.Z < b.Pos.Z
		}
	})
	return true
}

// diversionSigns puts a diversion sign on every approach to each end of a
// street.
func (s *Scheduled) diversionSigns(r *city.Road) []Sign {
	var out []Sign
	for _, end := range [2]int{r.A, r.B} {
		for _, l := range s.approaches(end) {
			if l.Road == r.ID || l.Index != 0 || l.Length < signBefore+4 {
				continue
			}
			at := l.Length - signBefore
			out = append(out, Sign{
				Kind: SignDiversion, Pos: kerb(l, s.c.Roads[l.Road], at, 1.2), Heading: l.HeadingAt(at),
			})
		}
	}
	return out
}

// cone closes the kerbside lane of an arterial for a stretch of its length in
// one direction, ending well before the stop line.
func (s *Scheduled) cone(r *city.Road, cl *Closure) bool {
	c := s.c
	cl.Kind = KindLane
	lanes := r.LanesAB
	if city.Chance(s.seed, sideSalt, cl.Edge.I, cl.Edge.row()) < 0.5 {
		lanes = r.LanesBA
	}
	if len(lanes) == 0 {
		return false
	}
	l := c.Lanes[lanes[len(lanes)-1]]
	end := l.Length - coneClear
	start := end - coneRun
	if start < warnBefore+2 {
		return false
	}
	cl.Lanes = []int{l.ID}
	for d := start; d <= end+0.01; d += ConeSpacing {
		cl.Cones = append(cl.Cones, mathx.OBB{
			Center: l.Point(d), HalfW: ConeSize / 2, HalfL: ConeSize / 2, Heading: l.DirAt(d).Angle(),
		})
	}
	cl.Signs = append(cl.Signs, Sign{
		Kind: SignDiversion, Pos: kerb(l, r, start-warnBefore, 1.2), Heading: l.HeadingAt(start - warnBefore),
	})
	return true
}

// approaches returns the lanes arriving at a junction.
func (s *Scheduled) approaches(node int) []*city.Lane {
	var out []*city.Lane
	for _, rid := range s.c.Nodes[node].Roads {
		r := s.c.Roads[rid]
		for _, ids := range [2][]int{r.LanesAB, r.LanesBA} {
			for _, id := range ids {
				if l := s.c.Lanes[id]; l.ToNode == node {
					out = append(out, l)
				}
			}
		}
	}
	return out
}

// exitsRemain reports whether every lane arriving at a junction, but those
// of the street itself, keeps at least one turn that does not enter a closed
// lane.
func (s *Scheduled) exitsRemain(r *city.Road, node int, closed []int) bool {
	for _, l := range s.approaches(node) {
		if l.Road == r.ID {
			continue
		}
		open := false
		for _, t := range l.Succ {
			shut := false
			for _, id := range closed {
				if t.Lane == id {
					shut = true
				}
			}
			if !shut {
				open = true
			}
		}
		if !open {
			return false
		}
	}
	return true
}

// kerb is a point s along a lane, out toward the kerb it keeps to, clearance
// metres in from the edge of the carriageway.
func kerb(l *city.Lane, r *city.Road, s, clearance float32) mathx.Vec {
	side := float32(1)
	if city.DriveOnLeft {
		side = -1
	}
	laneOffset := (0.5 + float32(l.Index)) * city.LaneWidth
	out := r.HalfWidth + clearance - laneOffset
	return l.Point(s).Add(l.DirAt(s).Right().Mul(out * side))
}
