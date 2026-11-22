package race

import (
	"sort"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Loop is a closed run of streets round a block or two, with a gate at the exit
// of every junction on it.
type Loop struct {
	// Gates are in driving order. Gates[0] is the start and finish line.
	Gates []Gate
	// Legs[i] is how far it is, along the street, from the gate before Gates[i]
	// to Gates[i]; for Legs[0] the gate before is the last.
	Legs []float32
	// Length is the whole way round, and Limit the mean speed limit of it.
	Length, Limit float32
	// Corners is how many times the loop turns.
	Corners int
}

// Status is how far a loop got.
type Status int

// The answers BuildLoop gives.
const (
	// Ready means the loop is made.
	Ready Status = iota
	// Growing means a junction the choice depends on is not complete yet, so
	// nothing can be said until the city has grown further.
	Growing
	// Nowhere means every loop that could be made here is turned down.
	Nowhere
)

// Length limits on a loop, in metres.
const (
	minLoop float32 = 700
	maxLoop float32 = 3600
)

// loopSalt keeps the choice of loop off every other positional draw.
const loopSalt uint64 = 0xa3d1f4c87e2b9065

type cell [2]int

// candidate is a rectangle of grid lines with the junction it is built from at
// one corner, and the way round it.
type candidate struct {
	key    float32
	i1, j1 int
	flip   bool
}

// BuildLoop chooses the loop for a race that begins near from, or says why it
// cannot yet.
//
// It is a rectangle of grid lines on the plan with the junction nearest from
// at one corner. Candidates are tried in an order fixed by a hash of the seed,
// the serial and that junction, and the first whose every junction is complete
// and every street exists is taken. A candidate with a junction still being
// built is not passed over but waited for, because skipping it would make the
// answer depend on how far the city had grown. So the loop is the same
// whatever order the city was reached in.
func BuildLoop(c *city.City, seed uint64, serial int, from mathx.Vec) (Loop, Status) {
	byGrid, anchor := junctions(c, from)
	if anchor == nil {
		return Loop{}, Growing
	}
	roads := map[cell]*city.Road{}
	for _, r := range c.Roads {
		if !r.Ring {
			roads[cell{r.A, r.B}] = r
			roads[cell{r.B, r.A}] = r
		}
	}
	i0, j0 := anchor.GridI, anchor.GridJ
	cands := candidates(seed, serial, i0, j0)

	// First the loops of main roads alone, then any at all.
	for _, mainOnly := range []bool{true, false} {
		for _, cd := range cands {
			nodes, ready := nodesOf(c, byGrid, perimeter(i0, j0, cd.i1, cd.j1, cd.flip))
			if !ready {
				return Loop{}, Growing
			}
			if l, ok := assemble(c, nodes, roads, mainOnly); ok {
				return l, Ready
			}
		}
	}
	return Loop{}, Nowhere
}

// junctions indexes a city's junctions by grid point and finds the one nearest
// a place.
func junctions(c *city.City, from mathx.Vec) (map[cell]*city.Node, *city.Node) {
	byGrid := map[cell]*city.Node{}
	var anchor *city.Node
	for _, n := range c.Nodes {
		byGrid[cell{n.GridI, n.GridJ}] = n
		if anchor == nil || n.Pos.DistTo(from) < anchor.Pos.DistTo(from) {
			anchor = n
		}
	}
	return byGrid, anchor
}

// candidates are the rectangles that may be tried from a junction, in the order
// a hash of the race puts them.
func candidates(seed uint64, serial, i0, j0 int) []candidate {
	var cands []candidate
	k := 0
	for _, si := range []int{1, -1} {
		for _, sj := range []int{1, -1} {
			for w := 1; w <= 3; w++ {
				for h := 1; h <= 3; h++ {
					r := city.Chance(seed^uint64(serial)*0x9e3779b97f4a7c15, loopSalt, i0*64+k, j0)
					cands = append(cands, candidate{key: r, i1: i0 + si*w, j1: j0 + sj*h, flip: r >= 0.5})
					k++
				}
			}
		}
	}
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].key < cands[b].key })
	return cands
}

// nodesOf looks up the junctions at grid points, and reports false if any is
// missing or not yet complete.
func nodesOf(c *city.City, byGrid map[cell]*city.Node, ring []cell) ([]*city.Node, bool) {
	nodes := make([]*city.Node, len(ring))
	for i, g := range ring {
		n := byGrid[g]
		if n == nil || !c.Complete(n) {
			return nil, false
		}
		nodes[i] = n
	}
	return nodes, true
}

// perimeter lists the grid points round a rectangle with one corner at (i0, j0),
// starting there.
func perimeter(i0, j0, i1, j1 int, flip bool) []cell {
	step := func(a, b int) int {
		if b > a {
			return 1
		}
		return -1
	}
	si, sj := step(i0, i1), step(j0, j1)
	var out []cell
	for i := i0; i != i1; i += si {
		out = append(out, cell{i, j0})
	}
	for j := j0; j != j1; j += sj {
		out = append(out, cell{i1, j})
	}
	for i := i1; i != i0; i -= si {
		out = append(out, cell{i, j1})
	}
	for j := j1; j != j0; j -= sj {
		out = append(out, cell{i0, j})
	}
	if flip {
		for a, b := 1, len(out)-1; a < b; a, b = a+1, b-1 {
			out[a], out[b] = out[b], out[a]
		}
	}
	return out
}

// assemble makes the loop of the streets between consecutive junctions, or
// refuses it: a street is missing, it is not a main road when one was asked
// for, or the loop is too short or too long to be worth racing.
func assemble(c *city.City, nodes []*city.Node, roads map[cell]*city.Road, mainOnly bool) (Loop, bool) {
	n := len(nodes)
	l := Loop{Legs: make([]float32, n)}
	lanes := make([]*city.Lane, n)
	for i, a := range nodes {
		b := nodes[(i+1)%n]
		r := roads[cell{a.ID, b.ID}]
		if r == nil || mainOnly && r.Class < city.Collector {
			return Loop{}, false
		}
		ids := r.LanesBA
		if r.A == a.ID {
			ids = r.LanesAB
		}
		if len(ids) == 0 {
			return Loop{}, false
		}
		lanes[i] = c.Lanes[ids[len(ids)-1]] // the one by the kerb
	}
	for i, ln := range lanes {
		half := c.Roads[ln.Road].HalfWidth
		l.Gates = append(l.Gates, Gate{Pos: ln.Point(0), Heading: ln.DirAt(0).Angle(), Half: half})
		through := 2 * c.Nodes[ln.ToNode].Radius
		d := ln.Length + through
		l.Legs[(i+1)%n] = d
		l.Length += d
		l.Limit += ln.SpeedLimit * d
		if i > 0 && mathx.Abs(mathx.AngleDiff(ln.Heading, lanes[i-1].Heading)) > 0.5 ||
			i == 0 && mathx.Abs(mathx.AngleDiff(ln.Heading, lanes[n-1].Heading)) > 0.5 {
			l.Corners++
		}
	}
	l.Limit /= l.Length
	if l.Length < minLoop || l.Length > maxLoop {
		return Loop{}, false
	}
	return l, true
}

// PlanLoop is BuildLoop for a host that can grow the city: it asks the city to
// grow far enough round from to hold any loop, which costs nothing where it
// has already, and builds the loop on what is there.
func PlanLoop(c *city.City, seed uint64, serial int, from mathx.Vec) (Loop, bool) {
	reach := 5*c.Params().MaxSpan + 200
	for try := range 3 {
		c.EnsureAround(from, reach*(1+float32(try)/2))
		l, st := BuildLoop(c, seed, serial, from)
		switch st {
		case Ready:
			return l, true
		case Nowhere:
			return Loop{}, false
		}
	}
	return Loop{}, false
}
