// Package night says where the evening events are, and when.
//
// An event is a function of the seed, the place and the clock, and of nothing
// else, as roadworks and weather are: the street grid is cut into super-cells,
// each super-cell has one venue, a block of the plan put on the ground with
// City.Lay, and whether the venue holds an event on a given night, and which
// kind, is worked out by hashing the super-cell and the night's number. A
// super-cell is decided only once every junction round it is complete, so it is
// decided once however the city was reached, and nothing is stored about a
// night that has passed. An event is open from dusk until one in the morning,
// with its signs up a quarter of an hour before.
//
// Events change the driving in three ways, all of them through things the
// simulation already has. A crowd slows the traffic near it (see SpeedScale);
// a street beside the venue is shut by handing the roadworks a closure, so the
// autopilot reads the same signs and cones it always has; and a street meet
// leaves show cars standing at the kerb, which are solid and nothing more.
// There are no people anywhere: a crowd is a sound and a slower road.
//
// The package imports package city, package works and package mathx, and
// nothing of the simulation.
package night

import (
	"math"
	"sort"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/works"
)

// Kind is what a venue holds on a night it has an event.
type Kind int

// The kinds of event.
const (
	// Market is a street market under festival lights.
	Market Kind = iota
	// Fireworks is a fair with fireworks over it.
	Fireworks
	// Meet is a street meet: show cars at the kerb.
	Meet
	// Match is a football crowd round floodlit ground.
	Match
	numKinds
)

// String names the kind.
func (k Kind) String() string {
	switch k {
	case Fireworks:
		return "fireworks"
	case Meet:
		return "meet"
	case Match:
		return "match"
	default:
		return "market"
	}
}

// crowd is how fast traffic goes among the kind's crowd, as a share of what it
// would.
func (k Kind) crowd() float32 {
	switch k {
	case Fireworks:
		return 0.6
	case Meet:
		return 0.8
	case Match:
		return 0.6
	default:
		return 0.7
	}
}

// The evening, in hours on the clock. Close is one in the morning of the next
// day, which is how a clock that counts past 24 writes it.
const (
	Dusk  float32 = 19
	Close float32 = 25
	// Lead is how long before dusk the signs go up.
	Lead float32 = 0.25
)

// Clock says what hour it is, counted from the start of day zero: 24 is the
// first midnight, and so on. It is the game's clock, which the simulation does
// not keep.
type Clock func() float32

// Fixed is a clock held at one hour of the first day.
func Fixed(h float32) Clock { return func() float32 { return h } }

// Evening returns the night an hour on the clock belongs to and how far that
// night's events have got: signs up, in force, or not on at all. Night n begins
// at dusk of day n and ends at one in the morning of day n+1.
func Evening(h float32) (n int, ph works.Phase) {
	day := int(math.Floor(float64(h) / 24))
	hod := h - float32(day)*24
	switch {
	case hod >= Dusk:
		return day, works.PhaseSolid
	case hod >= Dusk-Lead:
		return day, works.PhaseSigned
	case hod < Close-24:
		return day - 1, works.PhaseSolid
	}
	return day, works.PhaseClear
}

// Super is how many cells of the street grid a super-cell is wide.
const Super = 3

// Share is how many super-cells hold an event on a given night.
const Share float32 = 0.5

// Salts that keep each decision about a super-cell from correlating with any
// other, and with every salt in the weather, the roadworks and the sites.
const (
	pickSalt  uint64 = 0x7c3e91a5d2f4
	nightSalt uint64 = 0x2a8d6f13b7c9
	kindSalt  uint64 = 0x91b4e7c05a3d
)

// golden spreads a night's number over the seed.
const golden uint64 = 0x9e3779b97f4a7c15

// On reports whether a super-cell holds an event on a night.
func On(seed uint64, cell [2]int, n int) bool {
	return city.Chance(seed^uint64(int64(n))*golden, nightSalt, cell[0], cell[1]) < Share
}

// KindOf is the kind of event a super-cell holds on a night.
func KindOf(seed uint64, cell [2]int, n int) Kind {
	r := city.Chance(seed^uint64(int64(n))*golden, kindSalt, cell[0], cell[1])
	return min(Kind(r*float32(numKinds)), numKinds-1)
}

// Venue measures.
const (
	// minBlock is the least a block may measure for a venue to be put on it.
	minBlock float32 = 30
	// Fade is how far beyond a venue's radius the crowd thins out to nothing.
	Fade float32 = 60
	// reach is how far past its block's corner a crowd spreads.
	reach float32 = 30
	// Show cars: their size, how far in from the kerb they stand, and how far
	// apart.
	carHalfW, carHalfL float32 = 0.9, 2.1
	carInset           float32 = 1.6
	carPitch           float32 = 5.2
	maxCars                    = 6
)

// Site is one venue.
type Site struct {
	// Cell is the super-cell the venue stands in.
	Cell [2]int
	// Pos is the middle of its block on the ground, and Turn how far the ground
	// has turned the block from the way the plan drew it.
	Pos  mathx.Vec
	Turn float32
	// Half is half the measure of the block on the plan, along X and Z.
	Half mathx.Vec
	// Radius is how far from Pos the crowd reaches at full strength.
	Radius float32
	// Closure is the street the venue shuts, when it has one that can be.
	Closure works.Closure
	Closes  bool
	// Cars are the show cars a street meet leaves at the kerb.
	Cars []mathx.OBB
}

// Event is a venue on a night it is in use.
type Event struct {
	*Site
	Kind Kind
}

// Index is every venue found so far, and the events on tonight. Update brings it
// to the clock, and as the city grows it looks only at what has appeared.
type Index struct {
	c    *city.City
	plan *city.City
	seed uint64
	clk  Clock

	seen    int
	nodes   map[[2]int]*city.Node
	pending map[[2]int]bool
	sites   []*Site

	hour   float32
	night  int
	built  int // how many sites events was last made from
	phase  works.Phase
	events []Event
}

// NewIndex returns the venues of a city, driven by a clock.
func NewIndex(c *city.City, seed uint64, clk Clock) *Index {
	return &Index{
		c: c, plan: c.Plan(), seed: seed, clk: clk,
		nodes: map[[2]int]*city.Node{}, pending: map[[2]int]bool{}, night: math.MinInt,
	}
}

func floorDiv(a, n int) int {
	q := a / n
	if a%n != 0 && (a < 0) != (n < 0) {
		q--
	}
	return q
}

// Sites returns every venue found so far, in order of super-cell.
func (ix *Index) Sites() []*Site { return ix.sites }

// Events returns tonight's events as of the last Update, in order of
// super-cell. It is empty by day, and the slice is reused by the next Update.
func (ix *Index) Events() []Event {
	if ix == nil || ix.phase == works.PhaseClear {
		return nil
	}
	return ix.events
}

// Phase is how far tonight has got as of the last Update: clear by day, signed
// in the quarter of an hour before dusk, solid from dusk to one.
func (ix *Index) Phase() works.Phase { return ix.phase }

// Update brings the index to the clock: it decides any venue that growth has
// completed, and works out which of them hold an event.
func (ix *Index) Update() {
	if ix == nil {
		return
	}
	ix.sync()
	ix.hour = ix.clk()
	n, ph := Evening(ix.hour)
	ix.phase = ph
	if ph == works.PhaseClear {
		return
	}
	if n == ix.night && ix.built == len(ix.sites) {
		return
	}
	ix.night, ix.built = n, len(ix.sites)
	ix.events = ix.events[:0]
	for _, s := range ix.sites {
		if On(ix.seed, s.Cell, n) {
			ix.events = append(ix.events, Event{Site: s, Kind: KindOf(ix.seed, s.Cell, n)})
		}
	}
}

// Closures implements works.Source: the street each event shuts, signed from a
// quarter of an hour before dusk and solid from dusk until it closes. A street
// meet shuts nothing.
func (ix *Index) Closures(t float32, dst []works.Closure) []works.Closure {
	ix.Update()
	for _, e := range ix.Events() {
		if !e.Closes || e.Kind == Meet {
			continue
		}
		start := float32(ix.night) * 24
		cl := e.Closure
		cl.Phase = ix.phase
		// Times are in seconds from the same instant as t.
		at := func(h float32) float32 { return t + (start+h-ix.hour)*3600 }
		cl.Start, cl.Solid, cl.End = at(Dusk-Lead), at(Dusk), at(Close)
		dst = append(dst, cl)
	}
	return dst
}

// SpeedScale is how much of its usual speed traffic keeps at a point: one away
// from every event, and less in the crowd of one that is in force, thinning
// back to one over [Fade] metres. Nothing is slowed before dusk.
func (ix *Index) SpeedScale(p mathx.Vec) float32 {
	if ix == nil || ix.phase != works.PhaseSolid {
		return 1
	}
	scale := float32(1)
	for _, e := range ix.events {
		d := p.DistTo(e.Pos)
		if d >= e.Radius+Fade {
			continue
		}
		f := mathx.Clamp((d-e.Radius)/Fade, 0, 1)
		scale = min(scale, mathx.Lerp(e.Kind.crowd(), 1, f))
	}
	return scale
}

// Cars appends the show cars of every street meet in force.
func (ix *Index) Cars(dst []mathx.OBB) []mathx.OBB {
	if ix == nil || ix.phase != works.PhaseSolid {
		return dst
	}
	for _, e := range ix.events {
		if e.Kind == Meet {
			dst = append(dst, e.Cars...)
		}
	}
	return dst
}

// sync decides every super-cell that has been completed since it last ran.
func (ix *Index) sync() {
	for ; ix.seen < len(ix.plan.Nodes); ix.seen++ {
		n := ix.plan.Nodes[ix.seen]
		ix.nodes[[2]int{n.GridI, n.GridJ}] = n
		for _, di := range [2]int{-1, 0} {
			for _, dj := range [2]int{-1, 0} {
				ix.pending[[2]int{floorDiv(n.GridI+di, Super), floorDiv(n.GridJ+dj, Super)}] = true
			}
		}
	}
	found := false
	for cell := range ix.pending {
		if !ix.complete(cell) {
			continue
		}
		delete(ix.pending, cell)
		if s, ok := ix.decide(cell); ok {
			ix.sites = append(ix.sites, s)
			found = true
		}
	}
	if found {
		sort.Slice(ix.sites, func(i, j int) bool {
			a, b := ix.sites[i].Cell, ix.sites[j].Cell
			return a[0] < b[0] || (a[0] == b[0] && a[1] < b[1])
		})
	}
}

// complete reports whether every junction round a super-cell is finished, on
// the plan and on the ground.
func (ix *Index) complete(cell [2]int) bool {
	for i := 0; i <= Super; i++ {
		for j := 0; j <= Super; j++ {
			n, ok := ix.nodes[[2]int{cell[0]*Super + i, cell[1]*Super + j}]
			if !ok || !ix.plan.Complete(n) || n.ID >= len(ix.c.Nodes) || !ix.c.Complete(ix.c.Nodes[n.ID]) {
				return false
			}
		}
	}
	return true
}

// blockOf finds the block laid in the cell whose lower corner is (i, j).
func (ix *Index) blockOf(i, j int) (city.Block, bool) {
	lo, hi := ix.nodes[[2]int{i, j}].Pos, ix.nodes[[2]int{i + 1, j + 1}].Pos
	const eps = 0.01
	for _, b := range ix.plan.Blocks {
		if b.Min.X >= lo.X-eps && b.Min.Z >= lo.Z-eps && b.Max.X <= hi.X+eps && b.Max.Z <= hi.Z+eps {
			return b, true
		}
	}
	return city.Block{}, false
}

// roadBetween finds the street joining two junctions of the grid.
func (ix *Index) roadBetween(a, b [2]int) *city.Road {
	na, nb := ix.nodes[a], ix.nodes[b]
	if na == nil || nb == nil {
		return nil
	}
	for _, rid := range ix.c.Nodes[na.ID].Roads {
		r := ix.c.Roads[rid]
		if !r.Ring && (r.A == na.ID && r.B == nb.ID || r.A == nb.ID && r.B == na.ID) {
			return r
		}
	}
	return nil
}

// decide chooses the venue of a finished super-cell. The cells are tried in an
// order the hash gives, and the first with a block fit to build on wins, so the
// answer rests on the finished ground and on nothing else.
func (ix *Index) decide(cell [2]int) (*Site, bool) {
	type cand struct {
		i, j int
		key  float32
	}
	order := make([]cand, 0, Super*Super)
	for i := range Super {
		for j := range Super {
			ci, cj := cell[0]*Super+i, cell[1]*Super+j
			order = append(order, cand{ci, cj, city.Chance(ix.seed, pickSalt, ci, cj)})
		}
	}
	sort.Slice(order, func(a, b int) bool { return order[a].key < order[b].key })
	for _, o := range order {
		b, ok := ix.blockOf(o.i, o.j)
		if !ok || b.Park || b.Beach || b.Max.X-b.Min.X < minBlock || b.Max.Z-b.Min.Z < minBlock {
			continue
		}
		centre := mathx.V((b.Min.X+b.Max.X)/2, (b.Min.Z+b.Max.Z)/2)
		half := mathx.V((b.Max.X-b.Min.X)/2, (b.Max.Z-b.Min.Z)/2)
		pos, turn := ix.c.Lay(centre)
		s := &Site{
			Cell: cell, Pos: pos, Turn: turn, Half: half,
			Radius: half.Len() + reach,
		}
		s.Closure, s.Closes = ix.street(o.i, o.j)
		s.Cars = ix.showCars(b)
		return s, true
	}
	return nil, false
}

// street finds a street round the cell whose lower corner is (i, j) that can be
// shut, trying its four sides in a fixed order.
func (ix *Index) street(i, j int) (works.Closure, bool) {
	for _, side := range [4][2][2]int{
		{{i, j}, {i + 1, j}},
		{{i, j + 1}, {i + 1, j + 1}},
		{{i, j}, {i, j + 1}},
		{{i + 1, j}, {i + 1, j + 1}},
	} {
		if r := ix.roadBetween(side[0], side[1]); r != nil {
			if cl, ok := works.Shut(ix.c, ix.seed, r); ok {
				return cl, true
			}
		}
	}
	return works.Closure{}, false
}

// showCars stands a row of cars along the far kerb of a block, on the plan,
// and puts each on the ground.
func (ix *Index) showCars(b city.Block) []mathx.OBB {
	width := b.Max.X - b.Min.X
	n := min(maxCars, int((width-8)/carPitch))
	mid := (b.Min.X + b.Max.X) / 2
	out := make([]mathx.OBB, 0, n)
	for k := range n {
		x := mid + (float32(k)-float32(n-1)/2)*carPitch
		at, turn := ix.c.Lay(mathx.V(x, b.Max.Z-carInset))
		out = append(out, mathx.OBB{Center: at, HalfW: carHalfW, HalfL: carHalfL, Heading: turn})
	}
	return out
}
