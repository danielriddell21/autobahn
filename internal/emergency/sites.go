// Package emergency places the places an emergency call-out goes to, and says
// what a call-out is.
//
// A hospital, a station and the site of a fire are a function of where they
// are on the plan and of nothing else, as roadworks and weather are: the
// street grid is cut into super-cells, each super-cell has one site, and which
// block of it, and which kind of site, are worked out by hashing its lattice
// coordinates. A super-cell is decided only once every junction round it is
// complete, so it is decided once, however the city was reached, and the site
// never moves.
//
// The package never touches the generator. It reads the plan's blocks and
// puts what it finds on the ground with City.Lay. It imports package city,
// package job and package mathx, and nothing of the simulation.
package emergency

import (
	"sort"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Kind is what stands on a site.
type Kind int

// The kinds of site.
const (
	// Hospital is where an ambulance takes its patient.
	Hospital Kind = iota
	// Station is where a response car is sent to attend.
	Station
	// FireSite is where a fire engine is sent.
	FireSite
	numKinds
)

// String names the kind.
func (k Kind) String() string {
	switch k {
	case Station:
		return "station"
	case FireSite:
		return "fire"
	default:
		return "hospital"
	}
}

// Super is how many cells of the street grid a super-cell is wide.
const Super = 3

// Salts that keep each decision about a super-cell from correlating with any
// other, and with every salt in the weather and the roadworks.
const (
	pickSalt uint64 = 0x5e1d7b39a4c2
	kindSalt uint64 = 0x1f6a93c8e0d5
)

// minBlock is the least a block may measure for a site to be put on it.
const minBlock float32 = 24

// Site is one place on the map.
type Site struct {
	Kind Kind
	// Cell is the super-cell the site stands in.
	Cell [2]int
	// Pos is the middle of its block on the ground, and Door the nearest road
	// to the kerb it faces: the place a job sends the player to.
	Pos, Door mathx.Vec
	// Heading is the way the lane runs at the door.
	Heading float32
	// Lane is that lane.
	Lane int
}

// Index is every site found so far. It is brought up to date by Sync, and as
// the city grows it looks only at what has appeared.
type Index struct {
	c    *city.City
	plan *city.City

	seen    int
	nodes   map[[2]int]*city.Node
	pending map[[2]int]bool
	sites   []Site
}

// NewIndex returns the index of a city's sites.
func NewIndex(c *city.City) *Index {
	return &Index{c: c, plan: c.Plan(), nodes: map[[2]int]*city.Node{}, pending: map[[2]int]bool{}}
}

func floorDiv(a, n int) int {
	q := a / n
	if a%n != 0 && (a < 0) != (n < 0) {
		q--
	}
	return q
}

// Sync decides every super-cell that has been completed since it last ran, and
// reports whether it found a new site.
func (ix *Index) Sync() bool {
	for ; ix.seen < len(ix.plan.Nodes); ix.seen++ {
		n := ix.plan.Nodes[ix.seen]
		ix.nodes[[2]int{n.GridI, n.GridJ}] = n
		// The node is a corner of up to four cells, which may be in up to
		// four super-cells.
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
	return found
}

// complete reports whether every junction round a super-cell is finished.
func (ix *Index) complete(cell [2]int) bool {
	for i := 0; i <= Super; i++ {
		for j := 0; j <= Super; j++ {
			n, ok := ix.nodes[[2]int{cell[0]*Super + i, cell[1]*Super + j}]
			if !ok || !ix.plan.Complete(n) {
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

// decide chooses the site of a finished super-cell. The cells are tried in an
// order the hash gives, and the first with a block fit to build on wins, so
// the answer rests on the finished ground and on nothing else.
func (ix *Index) decide(cell [2]int) (Site, bool) {
	seed := ix.plan.Seed
	type cand struct {
		i, j int
		key  float32
	}
	order := make([]cand, 0, Super*Super)
	for i := range Super {
		for j := range Super {
			ci, cj := cell[0]*Super+i, cell[1]*Super+j
			order = append(order, cand{ci, cj, city.Chance(seed, pickSalt, ci, cj)})
		}
	}
	sort.Slice(order, func(a, b int) bool { return order[a].key < order[b].key })
	for _, o := range order {
		b, ok := ix.blockOf(o.i, o.j)
		if !ok || b.Park || b.Beach || b.Max.X-b.Min.X < minBlock || b.Max.Z-b.Min.Z < minBlock {
			continue
		}
		kind := Kind(int(city.Chance(seed, kindSalt, cell[0], cell[1]) * float32(numKinds)))
		centre := mathx.V((b.Min.X+b.Max.X)/2, (b.Min.Z+b.Max.Z)/2)
		pos, _ := ix.c.Lay(centre)
		// The door is on the street along the block's far side, so which of
		// the lanes round the block is nearest is never a close thing.
		kerb, _ := ix.c.Lay(mathx.V(centre.X, b.Max.Z))
		proj := ix.c.Project(kerb)
		if !proj.Valid {
			continue
		}
		return Site{
			Kind: kind, Cell: cell, Pos: pos,
			Door: proj.Lane.Point(proj.S), Heading: proj.Lane.HeadingAt(proj.S), Lane: proj.Lane.ID,
		}, true
	}
	return Site{}, false
}

// Sites returns every site found so far, in order of super-cell.
func (ix *Index) Sites() []Site { return ix.sites }

// Near returns the sites of a kind whose door lies from min to max metres of
// a point, in order of super-cell.
func (ix *Index) Near(k Kind, p mathx.Vec, min, max float32) []Site {
	var out []Site
	for _, s := range ix.sites {
		if d := s.Door.DistTo(p); s.Kind == k && d >= min && d <= max {
			out = append(out, s)
		}
	}
	return out
}

// Nearest returns the site of a kind whose door is nearest a point.
func (ix *Index) Nearest(k Kind, p mathx.Vec) (Site, bool) {
	var best Site
	found := false
	for _, s := range ix.sites {
		if s.Kind != k {
			continue
		}
		if !found || s.Door.DistTo(p) < best.Door.DistTo(p) {
			best, found = s, true
		}
	}
	return best, found
}
