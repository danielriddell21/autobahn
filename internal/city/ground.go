package city

import (
	"math"

	"github.com/danielriddell21/crucible/spatial"
	"github.com/danielriddell21/crucible/worldgen"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

// This is the city as it lies on the ground, for a place whose ground wanders
// from its plan.
//
// The plan is a whole city of its own, built by the rest of this package
// without knowing it is anything else. This one follows it: each time the
// plan grows, whatever is new in it is laid on the ground and added here
// under the same number, so a lane, a junction or a building is the same one
// in both. What is here is what the simulation drives on and collides with.
// Whatever only has to be drawn is worked out on the plan by whoever draws
// it, and laid with [City.Lay].
//
// Nothing here decides anything. Every choice about a place is made on the
// plan, where streets are straight and junctions square, and this only says
// where on the ground it came out.

// newLaid returns the city a plan comes to when laid on the ground.
func newLaid(drawn *City) *City {
	p := drawn.params
	land := newLie(p.Seed, p.Wind, drawn.lat, drawn.lines)
	if p.Coast {
		land.calm, land.hasCalm = drawn.lines.Y(p.CoastRow), true
	}
	return &City{
		Seed: p.Seed, params: p, lines: drawn.lines, lat: drawn.lat,
		lanes: spatial.New[int](laneCellSize),
		drawn: drawn, land: land, taken: -1,
	}
}

// Plan returns the city as it was drawn, with every street straight. For a
// place laid just as drawn that is the city itself.
func (c *City) Plan() *City {
	if c.drawn != nil {
		return c.drawn
	}
	return c
}

// Lay returns where a point of the plan lies on the ground, and how far
// whatever stands there has turned from the way the plan drew it.
func (c *City) Lay(p mathx.Vec) (mathx.Vec, float32) {
	if c.land == nil {
		return p, 0
	}
	return c.land.lay(p)
}

// Laid reports whether the ground wanders from the plan at all.
func (c *City) Laid() bool { return c.land != nil }

// adopt catches this city up with its plan.
func (c *City) adopt() {
	p := c.drawn
	if p.epoch == c.taken {
		return
	}
	c.taken = p.epoch

	for i := len(c.Nodes); i < len(p.Nodes); i++ {
		c.Nodes = append(c.Nodes, &Node{})
		c.layNode(i)
	}
	for i := len(c.Roads); i < len(p.Roads); i++ {
		c.Roads = append(c.Roads, &Road{})
		c.layRoad(i)
	}
	// Every new lane is laid before any is joined up, since a turn needs to
	// know where the lane it leads to begins.
	for i := len(c.Lanes); i < len(p.Lanes); i++ {
		l := &Lane{}
		c.Lanes = append(c.Lanes, l)
		c.layLane(l, p.Lanes[i])
	}
	// The junctions the plan has just worked on may have changed since they
	// were first laid: settled, been wired, had their approaches trimmed back
	// for a roundabout. They and their lanes are gone over again.
	p.eachNode(p.work, func(ln *worldgen.LatticeNode) {
		c.layNode(ln.ID)
		for _, rid := range p.Nodes[ln.ID].Roads {
			c.layRoad(rid)
			r := p.Roads[rid]
			for _, ids := range [2][]int{r.LanesAB, r.LanesBA} {
				for _, id := range ids {
					c.layLane(c.Lanes[id], p.Lanes[id])
				}
			}
		}
	})
	for _, l := range c.Lanes {
		c.joinLane(l, p.Lanes[l.ID])
	}

	for i := len(c.Buildings); i < len(p.Buildings); i++ {
		c.Buildings = append(c.Buildings, c.layBuilding(p.Buildings[i]))
	}
	for i := len(c.Props); i < len(p.Props); i++ {
		pr := p.Props[i]
		at, turn := c.land.lay(pr.Pos)
		pr.Pos, pr.Heading = at, mathx.WrapPi(pr.Heading+turn)
		c.Props = append(c.Props, pr)
	}
	for i := len(c.Parked); i < len(p.Parked); i++ {
		pk := p.Parked[i]
		at, turn := c.land.lay(pk.Pos)
		pk.Pos, pk.Heading = at, mathx.WrapPi(pk.Heading+turn)
		c.Parked = append(c.Parked, pk)
	}
	// The blocks stay as the plan drew them: nothing drives on a block, and
	// whoever draws one lays it as they go.
	c.Blocks = p.Blocks

	reach := c.land.reach()
	c.Min = mathx.V(p.Min.X-reach, p.Min.Z-reach)
	c.Max = mathx.V(p.Max.X+reach, p.Max.Z+reach)
}

// plotFit is how much of its plot a building is let fill, so that a plot
// the ground has pulled out of square still holds it.
const plotFit = 0.94

// layBuilding lays a building in its plot.
//
// A building cannot bend, and the ground under it can. Stood where its middle
// lies and turned as the ground there has turned, a building on a corner
// would keep its place beside the street it faces and swing its far end out
// over the one round the corner, which is bending its own way. So the four
// corners of its plot are laid instead, each against the street nearest it
// and so each as far back from that street as the plan put it, and the
// building is stood in the middle of them, turned to lie along them and
// drawn in just far enough to fit.
func (c *City) layBuilding(b Building) Building {
	hw, hd := b.W/2, b.D/2
	var at [4]mathx.Vec
	for i, corner := range [4]mathx.Vec{
		mathx.V(-hw, -hd), mathx.V(hw, -hd), mathx.V(hw, hd), mathx.V(-hw, hd),
	} {
		at[i], _ = c.land.lay(b.Center.Add(corner))
	}
	// Along its width is from the first pair of corners to the second;
	// across its depth, from the near pair to the far.
	along := at[1].Sub(at[0]).Add(at[2].Sub(at[3]))
	if along.Len() < 1e-3 {
		b.Center, b.Heading = c.land.lay(b.Center)
		return b
	}
	b.Center = at[0].Add(at[1]).Add(at[2]).Add(at[3]).Mul(0.25)
	b.Heading = along.Angle()
	dir := along.Norm()
	side := dir.Right()
	// The room there is each way is the least the plot offers, measured
	// square to the way the building now lies.
	width := min(at[1].Sub(at[0]).Dot(dir), at[2].Sub(at[3]).Dot(dir))
	depth := min(mathx.Abs(at[3].Sub(at[0]).Dot(side)), mathx.Abs(at[2].Sub(at[1]).Dot(side)))
	b.W = max(min(b.W, width*plotFit), 0)
	b.D = max(min(b.D, depth*plotFit), 0)
	return b
}

// layNode lays a junction: a copy of the plan's, standing where its frame
// puts it.
func (c *City) layNode(id int) {
	pn := c.drawn.Nodes[id]
	f := c.land.frameAt(pn.GridI, pn.GridJ)
	n := c.Nodes[id]
	*n = *pn
	n.Pos, n.Rot = f.pos, f.rot
}

// layRoad lays a road: a copy of the plan's, as long as its curve came out.
func (c *City) layRoad(id int) {
	pr := c.drawn.Roads[id]
	r := c.Roads[id]
	*r = *pr
	if pr.Ring {
		return
	}
	a := c.drawn.Nodes[pr.A]
	axis := worldgen.AxisX
	if pr.Axis != 0 {
		axis = worldgen.AxisY
	}
	r.Length = c.land.curveOf(street{a.GridI, a.GridJ, axis}).length
	if ga, gb := c.Nodes[pr.A].Pos, c.Nodes[pr.B].Pos; ga.DistTo(gb) > 1e-3 {
		r.Dir = gb.Sub(ga).Norm()
	}
}

// layLane lays a lane's centreline, unless it is already laid and the plan
// has not moved it since.
func (c *City) layLane(l, pl *Lane) {
	if l.path != nil && l.drawnA == pl.A && l.drawnB == pl.B {
		return
	}
	steps := max(int(pl.Length/curveStep), 1)
	path := make([]mathx.Vec, 0, steps+1)
	for i := range steps + 1 {
		at, _ := c.land.lay(pl.A.Lerp(pl.B, float32(i)/float32(steps)))
		path = append(path, at)
	}
	succ := l.Succ
	*l = *pl
	l.Succ = succ
	l.drawnA, l.drawnB = pl.A, pl.B
	l.setPath(path)
	// A lane the plan has moved is filed again where it now lies. The index
	// keeps where it used to be as well, which costs a wasted look and no
	// more: a lane is only ever shortened.
	c.indexLane(l)
}

// joinLane brings a laid lane's state up to the plan's: how it is governed,
// and the turns leading on from it.
func (c *City) joinLane(l, pl *Lane) {
	l.Control, l.Group = pl.Control, pl.Group
	l.SpeedLimit, l.FromNode, l.ToNode = pl.SpeedLimit, pl.FromNode, pl.ToNode
	if len(l.Succ) == len(pl.Succ) && len(pl.Succ) > 0 && l.Succ[0].Lane == pl.Succ[0].Lane {
		return
	}
	l.Succ = make([]Turn, len(pl.Succ))
	for i, t := range pl.Succ {
		t.Ctrl, _ = c.land.lay(t.Ctrl)
		t.Len = bezierLength(l.B, t.Ctrl, c.Lanes[t.Lane].A)
		l.Succ[i] = t
	}
}

// setPath gives a lane a centreline that is not straight.
func (l *Lane) setPath(path []mathx.Vec) {
	n := len(path)
	l.path = path
	l.cum = make([]float32, n)
	l.bend = make([]float32, n)
	for i := 1; i < n; i++ {
		l.cum[i] = l.cum[i-1] + path[i].DistTo(path[i-1])
	}
	for i := 1; i < n-1; i++ {
		in, out := path[i].Sub(path[i-1]), path[i+1].Sub(path[i])
		turn := mathx.Abs(mathx.AngleDiff(out.Angle(), in.Angle()))
		if span := (in.Len() + out.Len()) / 2; span > 1e-3 {
			l.bend[i] = turn / span
		}
	}
	l.A, l.B = path[0], path[n-1]
	l.Length = l.cum[n-1]
	l.Fwd = path[n-1].Sub(path[n-2]).Norm()
	l.Heading = l.Fwd.Angle()
}

// Point returns the world position s metres along the lane.
func (l *Lane) Point(s float32) mathx.Vec {
	if l.path == nil {
		return l.A.Add(l.Fwd.Mul(mathx.Clamp(s, 0, l.Length)))
	}
	at, _ := along(l.path, l.cum, s)
	return at
}

// DirAt returns the way the lane runs s metres along it. For a straight lane
// that is the same everywhere; Fwd is the way it runs at its far end.
func (l *Lane) DirAt(s float32) mathx.Vec {
	if l.path == nil {
		return l.Fwd
	}
	_, dir := along(l.path, l.cum, s)
	return dir
}

// HeadingAt returns the heading of the lane s metres along it.
func (l *Lane) HeadingAt(s float32) float32 {
	if l.path == nil {
		return l.Heading
	}
	return l.DirAt(s).Angle()
}

// StartDir returns the way the lane runs where it begins.
func (l *Lane) StartDir() mathx.Vec { return l.DirAt(0) }

// Locate returns how far along the lane the point of it nearest p is, and
// how far to the side of it p lies: positive to the right of the way it runs.
func (l *Lane) Locate(p mathx.Vec) (s, lateral float32) {
	if l.path == nil {
		s = mathx.Clamp(p.Sub(l.A).Dot(l.Fwd), 0, l.Length)
		return s, p.Sub(l.A.Add(l.Fwd.Mul(s))).Dot(l.Fwd.Right())
	}
	best := float32(math.MaxFloat32)
	for i := 1; i < len(l.path); i++ {
		a, b := l.path[i-1], l.path[i]
		span := l.cum[i] - l.cum[i-1]
		if span < 1e-5 {
			continue
		}
		dir := b.Sub(a).Mul(1 / span)
		t := mathx.Clamp(p.Sub(a).Dot(dir), 0, span)
		on := a.Add(dir.Mul(t))
		if d := on.DistTo(p); d < best {
			best, s, lateral = d, l.cum[i-1]+t, p.Sub(on).Dot(dir.Right())
		}
	}
	return s, lateral
}

// Run is [Lane.Along] carried on past the ends: a point beyond the far end of
// the lane is further along it than its length, by as far as it is beyond,
// and a point short of the start is less than nothing along it. It is how
// something that has run off the end of a lane into a junction is told from
// something still on it.
func (l *Lane) Run(p mathx.Vec) float32 {
	s, _ := l.Locate(p)
	switch {
	case s >= l.Length-1e-3:
		return l.Length + max(p.Sub(l.B).Dot(l.Fwd), 0)
	case s <= 1e-3:
		return min(p.Sub(l.A).Dot(l.StartDir()), 0)
	}
	return s
}

// Along returns how far along the lane the point of it nearest p is.
func (l *Lane) Along(p mathx.Vec) float32 {
	s, _ := l.Locate(p)
	return s
}

// Bend returns how sharply the lane turns at its sharpest between s and a
// distance ahead of it, as one over the radius of the turn in metres. A
// straight lane does not turn at all.
func (l *Lane) Bend(s, ahead float32) float32 {
	var sharpest float32
	for i, at := range l.cum {
		if at >= s && at <= s+ahead {
			sharpest = max(sharpest, l.bend[i])
		}
	}
	return sharpest
}
