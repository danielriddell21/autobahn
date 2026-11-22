package race

import (
	"math"
	"sort"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// walkSalt keeps the choice of turns off every other draw.
const walkSalt uint64 = 0xd47a09e1b36c5f28

// Route is a run of streets from one place to another, with a gate at the exit
// of every junction on it and the finish half way along the last street.
type Route struct {
	Gates         []Gate
	Length, Limit float32
	Corners       int
}

// Route lengths, in metres.
const (
	minSprint  float32 = 1300
	spanSprint float32 = 900
	maxSteps           = 80
)

// finished reports whether a lane is ground that cannot change: both its
// junctions complete.
func finished(c *city.City, l *city.Lane) bool {
	if l.FromNode < 0 || l.ToNode < 0 || l.FromNode >= len(c.Nodes) || l.ToNode >= len(c.Nodes) {
		return false
	}
	return c.Complete(c.Nodes[l.FromNode]) && c.Complete(c.Nodes[l.ToNode])
}

// PlanRoute chooses the run of a street race that begins where the car is. It
// walks forward from the lane under it, choosing each turn by a hash of where
// the choice is made, so it is the same wherever the walk was reached from. The
// city is asked to grow along it first.
func PlanRoute(c *city.City, seed uint64, serial int, from mathx.Vec) (Route, bool) {
	c.EnsureAround(from, minSprint+spanSprint+400)
	start := c.Project(from)
	if !start.Valid || !finished(c, start.Lane) {
		return Route{}, false
	}
	// A walk that comes to a dead end is tried again with other turns, in an
	// order that is fixed.
	for attempt := range 6 {
		key := seed ^ uint64(serial)*0x9e3779b97f4a7c15 + uint64(attempt)*0x2545f4914f6cdd1d
		target := minSprint + spanSprint*city.Chance(key, walkSalt, int(math.Floor(float64(from.X/100))), int(math.Floor(float64(from.Z/100))))
		if r, ok := walk(c, start, key, target); ok {
			return r, true
		}
	}
	return Route{}, false
}

// walk is one try at a route of about target metres from a place.
func walk(c *city.City, start city.LaneProjection, key uint64, target float32) (Route, bool) {
	var r Route
	l := start.Lane
	total := l.Length - start.S
	heading := l.Heading
	var walked float32
	for step := 0; step < maxSteps && (total < target || c.Roads[l.Road].Ring); step++ {
		var next []city.Turn
		for _, t := range l.Succ {
			if finished(c, c.Lanes[t.Lane]) {
				next = append(next, t)
			}
		}
		if len(next) == 0 {
			return Route{}, false
		}
		sort.SliceStable(next, func(i, j int) bool {
			if next[i].Kind != next[j].Kind {
				return next[i].Kind < next[j].Kind
			}
			return c.Lanes[next[i].Lane].Index < c.Lanes[next[j].Lane].Index
		})
		pick := int(city.Chance(key, walkSalt^0x77, int(math.Floor(float64(l.B.X/8))), int(math.Floor(float64(l.B.Z/8)))) * float32(len(next)))
		t := next[min(pick, len(next)-1)]
		n := c.Lanes[t.Lane]
		total += t.Len + n.Length
		walked += t.Len + n.Length
		r.Limit += n.SpeedLimit * (t.Len + n.Length)
		l = n
		if c.Roads[n.Road].Ring {
			continue
		}
		if mathx.Abs(mathx.AngleDiff(n.Heading, heading)) > 0.5 {
			r.Corners++
		}
		heading = n.Heading
		r.Gates = append(r.Gates, Gate{Pos: n.Point(0), Heading: n.DirAt(0).Angle(), Half: c.Roads[n.Road].HalfWidth})
	}
	if len(r.Gates) < 3 || c.Roads[l.Road].Ring {
		return Route{}, false
	}
	// The finish is half way along the last street.
	last := &r.Gates[len(r.Gates)-1]
	last.Pos = l.Point(l.Length / 2)
	last.Heading = l.DirAt(l.Length / 2).Angle()
	total -= l.Length / 2
	r.Length = total
	r.Limit /= max(walked, 1)
	return r, true
}
