package taxi

import (
	"sort"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Salts that keep each decision about a kerb slot from correlating with any
// other, and with every salt in the weather, the roadworks and the emergency
// sites.
const (
	slotSalt uint64 = 0x3d9a41c6e7b25f08
	rankSalt uint64 = 0x8b60e2d4175c39af
	pickSalt uint64 = 0x14f7c8a92e6d03b5
	riskSalt uint64 = 0xc52b7e0319d8a46f
	fragSalt uint64 = 0x6a08d5f3b1e4972c
)

// Slots are along the kerb lane of a street run, SlotPitch metres apart and
// not within SlotMargin of either end, where a junction is.
const (
	SlotPitch  float32 = 50
	SlotMargin float32 = 15
	// slotShare is the share of slots that are a pick-up place at all, and
	// rankShare the share of those that have a taxi rank.
	slotShare = 0.14
	rankShare = 0.2
	// maxLimit is the fastest street a fare is picked up from.
	maxLimit float32 = 18
	// pavement is how far beyond the kerb the marker stands.
	pavementSet float32 = 1.6
)

// Spot is one place a fare may be picked up from.
type Spot struct {
	// From and To are the plan's grid coordinates of the junctions the street
	// run is between, and Slot which slot of it this is: the spot's identity,
	// the same whenever the ground is reached.
	From, To [2]int
	Slot     int
	// Lane is the lane the spot is on, which is not an identity: it depends on
	// the order the city was built in.
	Lane int
	S    float32
	// Pos is the middle of the kerb lane, where the car stops; Kerb the middle
	// of the pavement beside it, where the marker stands.
	Pos, Kerb mathx.Vec
	// Heading is the way the lane runs there.
	Heading float32
	// Rank is set where there is a taxi rank, with its sign.
	Rank bool
}

func less(a, b Spot) bool {
	for i := range 2 {
		if a.From[i] != b.From[i] {
			return a.From[i] < b.From[i]
		}
	}
	for i := range 2 {
		if a.To[i] != b.To[i] {
			return a.To[i] < b.To[i]
		}
	}
	return a.Slot < b.Slot
}

func pack2(p [2]int) int { return p[0]*100003 + p[1] }

// Index is every pick-up place found so far. Sync brings it up to date, and as
// the city grows it looks only at lanes that have appeared or were unfinished.
type Index struct {
	c       *city.City
	seen    int
	pending []int
	spots   []Spot
}

// NewIndex returns the index of a city's pick-up places.
func NewIndex(c *city.City) *Index { return &Index{c: c} }

// finished reports whether a lane is on finished ground: both its junctions
// complete, so nothing about it can still change.
func finished(c *city.City, l *city.Lane) bool {
	if l.FromNode < 0 || l.ToNode < 0 || l.FromNode >= len(c.Nodes) || l.ToNode >= len(c.Nodes) {
		return false
	}
	return c.Complete(c.Nodes[l.FromNode]) && c.Complete(c.Nodes[l.ToNode])
}

// Sync decides every lane that has been finished since it last ran, and reports
// whether it found a new spot.
func (ix *Index) Sync() bool {
	c := ix.c
	if ix.seen == len(c.Lanes) && len(ix.pending) == 0 {
		return false
	}
	for ; ix.seen < len(c.Lanes); ix.seen++ {
		ix.pending = append(ix.pending, ix.seen)
	}
	found := false
	keep := ix.pending[:0]
	for _, id := range ix.pending {
		l := c.Lanes[id]
		if !finished(c, l) {
			keep = append(keep, id)
			continue
		}
		before := len(ix.spots)
		ix.spots = ix.lane(l, ix.spots)
		found = found || len(ix.spots) > before
	}
	ix.pending = keep
	if found {
		sort.Slice(ix.spots, func(i, j int) bool { return less(ix.spots[i], ix.spots[j]) })
	}
	return found
}

// lane adds the spots of one finished lane.
func (ix *Index) lane(l *city.Lane, out []Spot) []Spot {
	c := ix.c
	r := c.Roads[l.Road]
	n := len(r.LanesAB)
	if l.Dir < 0 {
		n = len(r.LanesBA)
	}
	if r.Ring || l.Index != n-1 || l.SpeedLimit > maxLimit {
		return out
	}
	a, b := c.Nodes[l.FromNode], c.Nodes[l.ToNode]
	from, to := [2]int{a.GridI, a.GridJ}, [2]int{b.GridI, b.GridJ}
	off := r.HalfWidth - (0.5+float32(l.Index))*city.LaneWidth + pavementSet
	for k := 0; ; k++ {
		s := SlotMargin + float32(k)*SlotPitch
		if s > l.Length-SlotMargin {
			break
		}
		col, row := pack2(from), pack2(to)*64+k
		if city.Chance(c.Seed, slotSalt, col, row) >= slotShare {
			continue
		}
		pos, dir := l.Point(s), l.DirAt(s)
		out = append(out, Spot{
			From: from, To: to, Slot: k, Lane: l.ID, S: s,
			Pos: pos, Kerb: pos.Sub(dir.Right().Mul(off)), Heading: dir.Angle(),
			Rank: city.Chance(c.Seed, rankSalt, col, row) < rankShare,
		})
	}
	return out
}

// Spots returns every pick-up place found so far, in a fixed order.
func (ix *Index) Spots() []Spot { return ix.spots }

// Ranks returns the places that have a taxi rank.
func (ix *Index) Ranks() []Spot {
	var out []Spot
	for _, s := range ix.spots {
		if s.Rank {
			out = append(out, s)
		}
	}
	return out
}

// Near returns the spots whose middle lies from min to max metres of a point.
func (ix *Index) Near(p mathx.Vec, min, max float32) []Spot {
	var out []Spot
	for _, s := range ix.spots {
		if d := s.Pos.DistTo(p); d >= min && d <= max {
			out = append(out, s)
		}
	}
	return out
}

func cellOf(v float32) int {
	if v < 0 {
		return int(v/100) - 1
	}
	return int(v / 100)
}

// choose picks one of n by a hash of the job and where the player stands, so a
// few metres either way change nothing.
func choose(seed uint64, serial int, from mathx.Vec, n int) int {
	r := city.Chance(seed^uint64(serial)*0x9e3779b97f4a7c15, pickSalt, cellOf(from.X), cellOf(from.Z))
	return min(int(r*float32(n)), n-1)
}

// pickup chooses the spot a player standing at from is sent to.
func pickup(c *city.City, seed uint64, serial int, from mathx.Vec) (Spot, bool) {
	ix := NewIndex(c)
	ix.Sync()
	// The choice is made from the middle of the cell the player is in, so a few
	// metres either way change nothing.
	anchor := mathx.V((float32(cellOf(from.X))+0.5)*100, (float32(cellOf(from.Z))+0.5)*100)
	at := ix.Near(anchor, 120, 450)
	if len(at) == 0 {
		return Spot{}, false
	}
	return at[choose(seed, serial, from, len(at))], true
}
