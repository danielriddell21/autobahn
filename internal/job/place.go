package job

import (
	"container/heap"
	"math"
	"sort"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Target is a place a job can send the player.
type Target struct {
	Lane  *city.Lane
	S     float32
	Pos   mathx.Vec
	Route float32 // metres of lane from the player to it
}

type reach struct {
	lane int
	d    float32
}
type reachHeap []reach

func (h reachHeap) Len() int           { return len(h) }
func (h reachHeap) Less(i, j int) bool { return h[i].d < h[j].d }
func (h reachHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *reachHeap) Push(x any)        { *h = append(*h, x.(reach)) }
func (h *reachHeap) Pop() any {
	o := *h
	x := o[len(o)-1]
	*h = o[:len(o)-1]
	return x
}

// usable reports whether a lane is finished ground: both its junctions
// complete, so nothing about it can still change.
func usable(c *city.City, l *city.Lane) bool {
	if l.FromNode < 0 || l.ToNode < 0 || l.FromNode >= len(c.Nodes) || l.ToNode >= len(c.Nodes) {
		return false
	}
	return c.Complete(c.Nodes[l.FromNode]) && c.Complete(c.Nodes[l.ToNode])
}

// startCell is how coarsely the player's position enters the choice: standing
// a few metres off must not change the job.
const startCell = 100

func mix(seed uint64, serial int, cx, cz int) uint64 {
	h := seed + 0x9e3779b97f4a7c15
	for _, v := range []uint64{uint64(int64(serial)), uint64(int64(cx)), uint64(int64(cz))} {
		h ^= v * 0xbf58476d1ce4e5b9
		h = (h ^ (h >> 30)) * 0xbf58476d1ce4e5b9
		h = (h ^ (h >> 27)) * 0x94d049bb133111eb
		h ^= h >> 31
	}
	return h
}

func cellOf(v float32) int { return int(math.Floor(float64(v) / startCell)) }

// routesFrom measures, by lane and turn, how far it is from a place to the far
// end of each finished lane within maxRoute of it.
func routesFrom(c *city.City, start city.LaneProjection, maxRoute float32) map[int]float32 {
	dist := map[int]float32{}
	h := &reachHeap{}
	first := start.Lane.Length - start.S
	dist[start.Lane.ID] = first
	heap.Push(h, reach{start.Lane.ID, first})
	for h.Len() > 0 {
		r := heap.Pop(h).(reach)
		if r.d > dist[r.lane] || r.d > maxRoute {
			continue
		}
		for _, t := range c.Lanes[r.lane].Succ {
			n := c.Lanes[t.Lane]
			if !usable(c, n) {
				continue
			}
			d := r.d + t.Len + n.Length
			if old, ok := dist[n.ID]; !ok || d < old {
				dist[n.ID] = d
				heap.Push(h, reach{n.ID, d})
			}
		}
	}
	return dist
}

// PickTarget chooses where to send a player standing at from: the middle of a
// lane whose route from there is between minRoute and maxRoute metres.
//
// The choice is a function of seed, serial and the cell the player starts in,
// and of ground that is finished, so it does not depend on when the player
// got there or on how much of the city was built beside it. Distance is
// measured along lanes and turns, and the place found with Point, so it
// holds on a curved map as well as a straight one.
func PickTarget(c *city.City, seed uint64, serial int, from mathx.Vec, minRoute, maxRoute float32) (Target, bool) {
	start := c.Project(from)
	if !start.Valid || !usable(c, start.Lane) {
		return Target{}, false
	}
	dist := routesFrom(c, start, maxRoute)
	var ids []int
	for id, d := range dist {
		l := c.Lanes[id]
		if id == start.Lane.ID || c.Roads[l.Road].Ring {
			continue
		}
		if mid := d - l.Length/2; mid >= minRoute && mid <= maxRoute {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return Target{}, false
	}
	sort.Ints(ids)
	pick := mix(seed, serial, cellOf(from.X), cellOf(from.Z))
	l := c.Lanes[ids[pick%uint64(len(ids))]]
	return Target{Lane: l, S: l.Length / 2, Pos: l.Point(l.Length / 2), Route: dist[l.ID] - l.Length/2}, true
}
