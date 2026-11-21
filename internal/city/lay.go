package city

import (
	"math"

	"github.com/danielriddell21/crucible/geom"
	"github.com/danielriddell21/crucible/worldgen"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

// A place is planned on a grid and then laid on the ground, and the two are
// not the same shape.
//
// The plan is what the rest of this package builds: straight streets meeting
// at right angles, which is what makes a junction something that can be
// reasoned about. The ground is what anybody drives on. Between them is the
// lie of the land: every junction is moved a little from where the plan put
// it and turned a little, and each street is run between its two junctions as
// a curve that leaves each one square. So the junctions are exactly what the
// plan drew, only somewhere else and facing another way, and the streets bend
// to join them up.
//
// Like everything else it is a function of where a place is. How far a
// junction is moved depends on its place in the plan and on nothing that has
// been built, so a street reached from either end is the same curve.

// lie is the lie of the land for one place.
type lie struct {
	seed uint64
	// amp is how far the ground wanders from the plan, in metres. It is the
	// size of the longest wave; the shorter ones are in proportion.
	amp  float64
	plan worldgen.Plan
	lat  *worldgen.Lattice
	// calm is a line beyond which the ground does not wander at all, and
	// hasCalm whether there is one. The land settles toward it over settle
	// metres. It is what keeps a seafront straight.
	calm    float64
	hasCalm bool
	phase   [6]float64

	frames map[[2]int]frame
	curves map[street]*curve
}

// The waves the land is made of: how long each is, in metres, and how big
// beside the longest. Three of them, at odd lengths, never line up twice.
//
// The long wave carries most of it. What bends a street is how fast the land
// moves from one end of it to the other, which the short waves do most for
// their size; so the place is given its shape by the long one, and the short
// ones only keep any two streets from bending alike.
var landWaves = [3]struct{ length, share float64 }{
	{1030, 1}, {430, 0.25}, {570, 0.2},
}

const (
	// settle is how far short of a calm line the land starts to flatten, and
	// rest how far short of it the land is already flat, in metres.
	settle = 330
	rest   = 40
	// armLength is how far a street runs dead straight out of a junction
	// before it may start to bend, in metres, and armShare the most of a
	// street's length either arm may take. Between its arms a street has to
	// make up whatever the two junctions disagree by, and the less room it
	// has to do it in the tighter it has to turn, so a short street keeps
	// its arms short: armLeast, which is still the whole of an ordinary
	// junction, laid rigidly.
	armLength = 32
	armShare  = 0.24
	armLeast  = 14
	// bandIn is how far from a street the ground is still laid exactly
	// alongside it, and bandOut where it has become the middle of a block,
	// laid by its corners instead. Everything that faces a street stands
	// inside the first.
	bandIn  = 28
	bandOut = 48
	// curveStep is how far apart the points of a laid street are, in metres.
	curveStep = 3
)

func newLie(seed uint64, amp float32, lat *worldgen.Lattice, plan worldgen.Plan) *lie {
	l := &lie{
		seed: seed, amp: float64(amp), plan: plan, lat: lat,
		frames: map[[2]int]frame{}, curves: map[street]*curve{},
	}
	for i := range l.phase {
		l.phase[i] = float64(cellChance(seed, landSalt, i, 0)) * 2 * math.Pi
	}
	return l
}

// landSalt is the salt behind the phases of the land's waves.
const landSalt = 0x7e21

// street names a street by the lower of its ends and the way it runs.
type street struct {
	col, row int
	axis     worldgen.Axis
}

// shift is how far the ground at a point of the plan has wandered from it.
func (l *lie) shift(x, z float64) (dx, dz float64) {
	k := func(i int) float64 { return 2 * math.Pi / landWaves[i].length }
	a := func(i int) float64 { return l.amp * landWaves[i].share }
	dx = a(0)*math.Sin(z*k(0)+l.phase[0]) +
		a(1)*math.Sin(z*k(1)+x*k(1)*0.3+l.phase[1]) +
		a(2)*math.Sin((x+z)*k(2)+l.phase[2])
	dz = a(0)*math.Sin(x*k(0)*0.93+l.phase[3]) +
		a(1)*math.Sin(x*k(1)*1.05-z*k(1)*0.3+l.phase[4]) +
		a(2)*math.Sin((x-z)*k(2)*0.95+l.phase[5])
	if l.hasCalm {
		f := mathx.Clamp(float32((l.calm-rest-z)/(settle-rest)), 0, 1)
		f = f * f * (3 - 2*f)
		dx, dz = dx*float64(f), dz*float64(f)
	}
	return dx, dz
}

// frame is where a junction stands on the ground and how far it has turned
// from the way the plan drew it.
type frame struct {
	pos mathx.Vec
	rot float32
}

// frameAt returns the frame of the junction at a grid point.
func (l *lie) frameAt(col, row int) frame {
	key := [2]int{col, row}
	if f, ok := l.frames[key]; ok {
		return f
	}
	x, z := l.plan.X(col), l.plan.Y(row)
	dx, dz := l.shift(x, z)
	// The turn is the rotation in the way the land moves round the point.
	const e = 1.0
	xa, _ := l.shift(x, z+e)
	xb, _ := l.shift(x, z-e)
	_, za := l.shift(x+e, z)
	_, zb := l.shift(x-e, z)
	rot := 0.5 * ((za-zb)/(2*e) - (xa-xb)/(2*e))
	f := frame{
		pos: mathx.V(float32(x+dx), float32(z+dz)),
		rot: mathx.Clamp(float32(rot), -0.6, 0.6),
	}
	l.frames[key] = f
	return f
}

// turned returns v turned through an angle, in the sense that adds to a
// heading.
func turned(v mathx.Vec, by float32) mathx.Vec {
	c, s := mathx.Cos(by), mathx.Sin(by)
	return mathx.V(v.X*c-v.Z*s, v.X*s+v.Z*c)
}

// curve is a street as it lies on the ground: a run of points from one
// junction to the other, with how far along each is.
type curve struct {
	pts []mathx.Vec
	cum []float32
	// planned is how long the plan drew it, arm how much of each end is dead
	// straight, and length how long it came out.
	planned, arm, length float32
}

// curveOf returns the street leaving a grid point along an axis, as laid.
func (l *lie) curveOf(s street) *curve {
	if c, ok := l.curves[s]; ok {
		return c
	}
	far := street{s.col + 1, s.row, s.axis}
	axis := mathx.V(1, 0)
	if s.axis == worldgen.AxisY {
		far = street{s.col, s.row + 1, s.axis}
		axis = mathx.V(0, 1)
	}
	f0, f1 := l.frameAt(s.col, s.row), l.frameAt(far.col, far.row)
	planned := float32(math.Hypot(
		l.plan.X(far.col)-l.plan.X(s.col), l.plan.Y(far.row)-l.plan.Y(s.row)))
	arm := mathx.Clamp(planned*armShare, armLeast, armLength)
	t0, t1 := turned(axis, f0.rot), turned(axis, f1.rot)
	e0, e1 := f0.pos.Add(t0.Mul(arm)), f1.pos.Sub(t1.Mul(arm))

	// Square out of one junction, a curve across, square into the other.
	c := &curve{planned: planned, arm: arm}
	c.add(f0.pos)
	c.add(e0)
	reach := e1.Sub(e0).Len()
	steps := max(int(reach/curveStep), 2)
	for i := 1; i < steps; i++ {
		u := float32(i) / float32(steps)
		h00, h10 := 2*u*u*u-3*u*u+1, u*u*u-2*u*u+u
		h01, h11 := -2*u*u*u+3*u*u, u*u*u-u*u
		c.add(e0.Mul(h00).Add(t0.Mul(h10 * reach)).Add(e1.Mul(h01)).Add(t1.Mul(h11 * reach)))
	}
	c.add(e1)
	c.add(f1.pos)
	c.length = c.cum[len(c.cum)-1]
	l.curves[s] = c
	return c
}

func (c *curve) add(p mathx.Vec) {
	d := float32(0)
	if n := len(c.pts); n > 0 {
		d = c.cum[n-1] + p.DistTo(c.pts[n-1])
	}
	c.pts = append(c.pts, p)
	c.cum = append(c.cum, d)
}

// at returns the point of the street that the plan put s metres along it,
// and the way the street runs there. The arms are laid metre for metre, so a
// junction is exactly the size the plan made it; whatever the street gained
// or lost in bending is spread along the middle.
func (c *curve) at(s float32) (mathx.Vec, mathx.Vec) {
	s = mathx.Clamp(s, 0, c.planned)
	d := s
	switch {
	case s > c.planned-c.arm:
		d = c.length - (c.planned - s)
	case s > c.arm:
		d = c.arm + (s-c.arm)/(c.planned-2*c.arm)*(c.length-2*c.arm)
	}
	return along(c.pts, c.cum, d)
}

// along returns the point d metres along a run of points, and the direction
// of the run there.
func along(pts []mathx.Vec, cum []float32, d float32) (mathx.Vec, mathx.Vec) {
	n := len(pts)
	d = mathx.Clamp(d, 0, cum[n-1])
	lo, hi := 0, n-1
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		if cum[mid] <= d {
			lo = mid
		} else {
			hi = mid
		}
	}
	span := cum[hi] - cum[lo]
	dir := pts[hi].Sub(pts[lo])
	if span < 1e-5 {
		return pts[lo], dir.Norm()
	}
	return pts[lo].Add(dir.Mul((d - cum[lo]) / span)), dir.Mul(1 / span)
}

// cellOf returns the cell of the plan a point falls in: the grid point at
// its lower corner.
func (l *lie) cellOf(p mathx.Vec) (col, row int) {
	col, row = l.lat.Cell(geom.Vec2{X: float64(p.X), Y: float64(p.Z)})
	for l.plan.X(col) > float64(p.X) {
		col--
	}
	for l.plan.X(col+1) <= float64(p.X) {
		col++
	}
	for l.plan.Y(row) > float64(p.Z) {
		row--
	}
	for l.plan.Y(row+1) <= float64(p.Z) {
		row++
	}
	return col, row
}

// lay returns where a point of the plan lies on the ground, and how far
// whatever stands there has turned from the way the plan drew it.
//
// Near a street a point is laid alongside it: as far along the curve as the
// plan put it along the street, and exactly as far to the side. That is what
// keeps a lane its width and a pavement its depth round a bend, and since the
// two blocks either side of a street are both laid against it, they meet
// across it. Out in the middle of a block there is no street to go by, and a
// point is laid between the block's four corners instead.
func (l *lie) lay(p mathx.Vec) (mathx.Vec, float32) {
	col, row := l.cellOf(p)
	x0, x1 := float32(l.plan.X(col)), float32(l.plan.X(col+1))
	z0, z1 := float32(l.plan.Y(row)), float32(l.plan.Y(row+1))

	// The nearest of the four streets round the cell, how far along it the
	// point is and how far to the side.
	s := street{col, row, worldgen.AxisX}
	run, aside, near := p.X-x0, p.Z-z0, p.Z-z0
	if d := z1 - p.Z; d < near {
		s, run, aside, near = street{col, row + 1, worldgen.AxisX}, p.X-x0, p.Z-z1, d
	}
	if d := p.X - x0; d < near {
		s, run, aside, near = street{col, row, worldgen.AxisY}, p.Z-z0, p.X-x0, d
	}
	if d := x1 - p.X; d < near {
		s, run, aside, near = street{col + 1, row, worldgen.AxisY}, p.Z-z0, p.X-x1, d
	}
	on, dir := l.curveOf(s).at(run)
	// To the side is toward the higher of the other coordinate, as on the plan.
	side, drawn := mathx.V(-dir.Z, dir.X), float32(0)
	if s.axis == worldgen.AxisY {
		side, drawn = mathx.V(dir.Z, -dir.X), math.Pi/2
	}
	beside := on.Add(side.Mul(aside))
	turn := mathx.WrapPi(dir.Angle() - drawn)
	if near <= bandIn {
		return beside, turn
	}

	// Between the corners, for the middle of the block.
	u, v := (p.X-x0)/(x1-x0), (p.Z-z0)/(z1-z0)
	f00, f10 := l.frameAt(col, row), l.frameAt(col+1, row)
	f01, f11 := l.frameAt(col, row+1), l.frameAt(col+1, row+1)
	within := f00.pos.Mul((1 - u) * (1 - v)).Add(f10.pos.Mul(u * (1 - v))).
		Add(f01.pos.Mul((1 - u) * v)).Add(f11.pos.Mul(u * v))
	spin := f00.rot*(1-u)*(1-v) + f10.rot*u*(1-v) + f01.rot*(1-u)*v + f11.rot*u*v

	w := mathx.Clamp((near-bandIn)/(bandOut-bandIn), 0, 1)
	w = w * w * (3 - 2*w)
	return beside.Lerp(within, w), turn + mathx.AngleDiff(spin, turn)*w
}

// reach is the furthest the ground can be from the plan, in metres.
func (l *lie) reach() float32 {
	total := 0.0
	for _, w := range landWaves {
		total += l.amp * w.share
	}
	return float32(total * 1.5)
}

// unlay returns the point of the plan that lies at a point on the ground,
// near enough. It is found by guessing and correcting, which settles in a
// few rounds because the land wanders slowly. Near enough is a metre or two:
// out in the middle of a block the ground is laid less exactly than beside a
// street, and all this is used for is deciding how much plan to build.
func (l *lie) unlay(g mathx.Vec) mathx.Vec {
	p := g
	for range 14 {
		on, _ := l.lay(p)
		// A part of the way each time: where the land is moving fastest, a
		// whole step overshoots and the guesses circle the answer.
		p = p.Add(g.Sub(on).Mul(0.6))
	}
	return p
}
