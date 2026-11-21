package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Every street, pavement and building in a place stands at one height, and
// the car is driven on the flat. What gives the place its lie is the ground
// nobody drives on: a field, a green, the middle of a block behind its
// houses. That ground rises, and how far is a function of where it is on the
// plan and nothing else, the same as everything else about a place.
//
// A block whose ground has a shape is drawn as a net of ground, stitched
// round its edge to the kerb. Out by the kerb the net lies on the land and
// carries what is built there; in the middle it is lifted. Anything dressing
// the block is put on the net by asking how high it is there: see Game.rise.
//
// The same net is what lays a block on a hillside. Where the land itself
// climbs (see city.Height) every block has one, whether its own open ground
// rises or not.

// The rise of open ground, in metres.
const (
	// swellRim is how far in from the kerb a field or a green starts to
	// rise, which leaves its hedge on the level.
	swellRim = hedgeInset + 3
	// swellBack is how far in a built block starts to rise: behind the
	// deepest building that can stand on its edge, and its back yard.
	swellBack = city.PavementWidth + 18 + 4
	// swellRun is the distance over which a hill reaches its height, and
	// swellLeast the narrowest open ground worth lifting at all.
	swellRun   float32 = 38
	swellLeast float32 = 24
	// swellTop is the highest a hill stands.
	swellTop float32 = 12
	// greenRun is the width of the bank round a green, whose top is flat for
	// the paths and whatever stands where they meet.
	greenRun float32 = 12
	// swellCell is the longest side of a square in the net, and bankCell
	// the same on a green, whose bank is short and steep and wants a finer
	// net to turn its corners.
	swellCell float32 = 7
	bankCell  float32 = 3.5
	// drape is how far above the net something lying on it is drawn. The net
	// is flat between its knots and the hill is not, so anything closer
	// would sink into it between them.
	drape float32 = 0.03
	// copseRoom is the open ground to each tree on a hill behind houses, in
	// square metres.
	copseRoom float32 = 1300
)

// swell is how the open ground of one block rises.
type swell struct {
	// lo and hi bound the ground that rises, on the plan.
	lo, hi mathx.Vec
	// amp is the height of the highest point, and run the distance in from
	// the edge at which the ground has finished climbing.
	amp, run float32
	// level marks a green: a bank and a flat top, rather than a hill.
	level bool
	// bites are the roundabouts the block is cut back for, which the rise
	// keeps clear of.
	bites [4]city.Corner
}

// grade is how steeply a map's open ground climbs: the height it gains over
// the run of a hill. The city is built on the flat.
func grade(m city.Map) float32 {
	switch m {
	case city.MapVillage:
		return 0.3
	case city.MapSeaside:
		return 0.2
	case city.MapMotorway:
		return 0.15
	}
	return 0
}

// swellOf returns how a block's open ground rises, and false if it does not:
// in the city, on a beach, or where there is too little of it.
func swellOf(b city.Block, m city.Map) (swell, bool) {
	g := grade(m)
	if g == 0 || b.Beach {
		return swell{}, false
	}
	in := float32(swellBack)
	if b.Park {
		in = swellRim
	}
	s := swell{
		lo: mathx.V(b.Min.X+in, b.Min.Z+in), hi: mathx.V(b.Max.X-in, b.Max.Z-in),
		bites: b.Corners,
	}
	size := s.hi.Sub(s.lo)
	narrow := min(size.X, size.Z)
	if narrow < swellLeast {
		return swell{}, false
	}
	c := b.Min.Lerp(b.Max, 0.5)
	h := scatter(c.X, c.Z, 83)
	if b.Park && fieldOf(b, m) == fieldNone {
		s.level = true
		s.run = min(greenRun, narrow*0.3)
		s.amp = 1.1 + h[0]*1.5
		return s, true
	}
	s.run = min(swellRun, narrow/2)
	s.amp = min(s.run*g*(0.45+0.55*h[0]), swellTop)
	return s, true
}

// climb runs from 0 to 1 as t does, leaving and arriving on the level.
func climb(t float32) float32 {
	t = mathx.Clamp(t, 0, 1)
	return t * t * (3 - 2*t)
}

// roll is the shape of the land under a hill, between a third and the whole
// of its height. It is one sheet of ground over the whole map, so two fields
// side by side belong to the same country.
func roll(p mathx.Vec) float32 {
	w := 0.6*mathx.Sin(p.X/47+p.Z/83+1.3) + 0.4*mathx.Sin(p.Z/39-p.X/71+4.1)
	return 0.33 + 0.67*(0.5+0.5*w)
}

// at returns how far above the block's own surface the ground stands at a
// point on the plan. It is nothing anywhere on the rim.
func (s swell) at(p mathx.Vec) float32 {
	x := min(p.X-s.lo.X, s.hi.X-p.X)
	z := min(p.Z-s.lo.Z, s.hi.Z-p.Z)
	if x <= 0 || z <= 0 {
		return 0
	}
	f := climb(x/s.run) * climb(z/s.run)
	for _, c := range s.bites {
		if c.Bite > 0 {
			f *= climb((p.DistTo(c.Centre) - c.Bite - 3) / s.run)
		}
	}
	if s.level {
		return s.amp * f
	}
	return s.amp * f * roll(p)
}

// risen is the ground of one block as it is drawn, where a block's ground
// has a shape at all: because it rises, or because the place stands on hills
// and the block has to lie on them.
type risen struct {
	swell
	// rises says whether the open ground does rise; where it does not the
	// block only follows the land.
	rises bool
	// xs and zs are the lines its net is made on, across the plan.
	xs, zs []float32
	// ground is the block, kerb and all, as the graphics card holds it. It
	// is given back when the block is a long way off and made again on the
	// way back.
	ground *patch
}

// at returns how far the open ground stands above the block's own surface
// at a point on the plan.
//
// It is the height of the net there, not of the hill the net is hung on. The
// net is flat between its knots and a hill is not, and on a steep bank the
// two are half a metre apart: a path laid on the hill would be buried in the
// net on the way up and hang over it at the top.
func (r *risen) at(p mathx.Vec) float32 {
	if !r.rises {
		return 0
	}
	i, u, ok := between(r.xs, p.X)
	j, v, ok2 := between(r.zs, p.Z)
	if !ok || !ok2 {
		return 0 // out on the rim
	}
	h := func(i, j int) float32 { return r.swell.at(mathx.V(r.xs[i], r.zs[j])) }
	a, far := h(i, j), h(i+1, j+1)
	// A square of the net is two triangles, split from its first corner to
	// the one opposite.
	if v >= u {
		b := h(i, j+1)
		return a + (far-b)*u + (b-a)*v
	}
	d := h(i+1, j)
	return a + (d-a)*u + (far-d)*v
}

// lift is at for a block that may have no shaped ground at all.
func (r *risen) lift(p mathx.Vec) float32 {
	if r == nil {
		return 0
	}
	return r.at(p)
}

// between finds which pair of lines a place falls between, and how far
// from the first to the second, and reports false if it is outside them all.
func between(lines []float32, at float32) (i int, f float32, ok bool) {
	if len(lines) < 2 || at < lines[0] || at > lines[len(lines)-1] {
		return 0, 0, false
	}
	for i = 0; i < len(lines)-2 && at > lines[i+1]; i++ {
	}
	return i, (at - lines[i]) / (lines[i+1] - lines[i]), true
}

// netInset returns how far in from a block's kerb its net of ground starts:
// near the kerb, but clear of every corner the block is cut back at.
func netInset(b city.Block) float32 {
	const least = 4
	narrow := min(b.Max.X-b.Min.X, b.Max.Z-b.Min.Z)
	in := float32(least)
	at := [4]mathx.Vec{b.Min, {X: b.Max.X, Z: b.Min.Z}, {X: b.Min.X, Z: b.Max.Z}, b.Max}
	toward := [4]mathx.Vec{{X: 1, Z: 1}, {X: -1, Z: 1}, {X: 1, Z: -1}, {X: -1, Z: -1}}
	for i, c := range b.Corners {
		// A rounded corner cuts the block back by under a third of its
		// radius, measured along either side.
		in = max(in, 0.3*c.Radius+1)
		if c.Bite <= 0 {
			continue
		}
		// A roundabout takes a scoop: step in until the net's corner is
		// outside it.
		for in < narrow/2 && at[i].Add(toward[i].Mul(in)).DistTo(c.Centre) < c.Bite+1 {
			in++
		}
	}
	return in
}

// lines returns where the lines of a net fall between lo and hi: no further
// apart than a square of the net is wide, and one on each of the marks
// given, so that a change of ground falls on a line and not across a square.
func lines(lo, hi float32, marks ...float32) []float32 {
	return linesOf(swellCell, lo, hi, marks...)
}

// linesOf is lines with the squares of the net no wider than cell.
func linesOf(cell, lo, hi float32, marks ...float32) []float32 {
	stops := []float32{lo}
	for _, m := range marks {
		if m > stops[len(stops)-1]+0.5 && m < hi-0.5 {
			stops = append(stops, m)
		}
	}
	stops = append(stops, hi)
	out := []float32{lo}
	for i := 1; i < len(stops); i++ {
		a, b := stops[i-1], stops[i]
		n := max(int((b-a)/cell)+1, 1)
		for k := 1; k <= n; k++ {
			out = append(out, a+(b-a)*float32(k)/float32(n))
		}
	}
	return out
}

// raise works out the ground of a block that has a shape, or returns nil
// for one that is a level slab: any block in a level place whose open
// ground does not rise, and the beach anywhere.
func (g *Game) raise(b city.Block) *risen {
	s, rises := swellOf(b, g.opts.Map)
	if (!rises && g.land == nil) || b.Beach {
		return nil
	}
	return shapeOf(b, s, rises)
}

// shapeOf returns the ground of a block as a net, given how its open ground
// rises, or nil for a block too small to have a middle: a slab on the slope.
func shapeOf(b city.Block, s swell, rises bool) *risen {
	in := netInset(b)
	if min(b.Max.X-b.Min.X, b.Max.Z-b.Min.Z) < 2*in+2 {
		return nil
	}
	r := &risen{swell: s, rises: rises}
	r.xs, r.zs = lines(b.Min.X+in, b.Max.X-in), lines(b.Min.Z+in, b.Max.Z-in)
	if rises {
		cell := swellCell
		if s.level {
			cell = bankCell
		}
		r.xs = linesOf(cell, b.Min.X+in, b.Max.X-in, s.lo.X, s.hi.X)
		r.zs = linesOf(cell, b.Min.Z+in, b.Max.Z-in, s.lo.Z, s.hi.Z)
	}
	return r
}

// build makes the block's ground and hands it to the graphics card. kerb is
// the block's outline as it lies; rim and open are the colours of the level
// ground round the edge and of the open ground inside it.
func (g *Game) build(r *risen, b city.Block, kerb []mathx.Vec, rim, open rl.Color) {
	c := g.world.City
	n := net{rim: rim, open: open, xs: r.xs, zs: r.zs}
	// The knots of the net, where they lie and how high.
	for _, z := range n.zs {
		for _, x := range n.xs {
			p := mathx.V(x, z)
			on, _ := c.Lay(p)
			n.knots = append(n.knots, vec3(on.X, g.land.at(on)+city.KerbHeight+r.at(p), on.Z))
		}
	}

	var w patchwork
	n.weave(&w, r)
	mid, _ := c.Lay(b.Min.Lerp(b.Max, 0.5))
	g.edge(&w, kerb, n.edge(), mid, rim)
	// Whatever lies flat on the block goes in with its ground: a path or a
	// furrow never moves, and there are a great many furrows.
	g.baking = &w
	g.layBlock(b)
	g.baking = nil
	r.ground = w.upload()
}

// layBlock lays what lies flat on a block: the paths of a green, the rows
// of a field.
func (g *Game) layBlock(b city.Block) {
	if k := fieldOf(b, g.opts.Map); k != fieldNone {
		g.layRows(b, k)
		return
	}
	g.layPaths(b)
}

// net is the net of ground in the middle of a block: the lines it is made
// on, across the plan, and the knots where they cross, on the ground.
type net struct {
	xs, zs    []float32
	knots     []rl.Vector3
	rim, open rl.Color
}

func (n *net) cols() int { return len(n.xs) - 1 }
func (n *net) rows() int { return len(n.zs) - 1 }

func (n *net) knot(i, j int) rl.Vector3 { return n.knots[j*len(n.xs)+i] }

// facing is which way the ground faces at a knot, from the knots either side.
func (n *net) facing(i, j int) rl.Vector3 {
	e, w := n.knot(min(i+1, n.cols()), j), n.knot(max(i-1, 0), j)
	north, south := n.knot(i, min(j+1, n.rows())), n.knot(i, max(j-1, 0))
	up := rl.Vector3CrossProduct(rl.Vector3Subtract(north, south), rl.Vector3Subtract(e, w))
	if up.Y < 0 {
		up = rl.Vector3Negate(up)
	}
	return rl.Vector3Normalize(up)
}

// weave adds the net to a patch a square at a time, each in the colour of
// the ground its middle is on.
func (n *net) weave(w *patchwork, r *risen) {
	for j := range n.rows() {
		for i := range n.cols() {
			col := n.rim
			x, z := (n.xs[i]+n.xs[i+1])/2, (n.zs[j]+n.zs[j+1])/2
			if r.rises && x > r.lo.X && x < r.hi.X && z > r.lo.Z && z < r.hi.Z {
				col = n.open
			}
			w.quad(
				w.corner(n.knot(i, j), n.facing(i, j), col),
				w.corner(n.knot(i, j+1), n.facing(i, j+1), col),
				w.corner(n.knot(i+1, j+1), n.facing(i+1, j+1), col),
				w.corner(n.knot(i+1, j), n.facing(i+1, j), col),
			)
		}
	}
}

// edge returns the edge of the net on the ground, wound the way a kerb is,
// so the two can be stitched together.
func (n *net) edge() []mathx.Vec {
	out := make([]mathx.Vec, 0, 2*(n.cols()+n.rows()))
	flat := func(i, j int) mathx.Vec { v := n.knot(i, j); return mathx.V(v.X, v.Z) }
	for i := range n.cols() {
		out = append(out, flat(i, 0))
	}
	for j := range n.rows() {
		out = append(out, flat(n.cols(), j))
	}
	for i := n.cols(); i > 0; i-- {
		out = append(out, flat(i, n.rows()))
	}
	for j := n.rows(); j > 0; j-- {
		out = append(out, flat(0, j))
	}
	return out
}

// edge adds what goes round a net to a patch: the level rim out to the
// kerb, and the kerb itself, a wall from the road up to the pavement facing
// out from the middle of the block.
func (g *Game) edge(w *patchwork, kerb, inner []mathx.Vec, mid mathx.Vec, rim rl.Color) {
	up := vec3(0, 1, 0)
	top := func(p mathx.Vec) rl.Vector3 { return vec3(p.X, g.land.at(p)+city.KerbHeight, p.Z) }
	foot := func(p mathx.Vec) rl.Vector3 { return vec3(p.X, g.land.at(p)-0.02, p.Z) }
	tris := stitch(kerb, inner)
	for i := 0; i+2 < len(tris); i += 3 {
		w.tri(w.corner(top(tris[i]), up, rim), w.corner(top(tris[i+1]), up, rim), w.corner(top(tris[i+2]), up, rim))
	}
	for i, p := range kerb {
		q := kerb[(i+1)%len(kerb)]
		run := q.Sub(p)
		if run.Len() < 1e-4 {
			continue
		}
		out := run.Norm().Right()
		if p.Sub(mid).Dot(out) < 0 {
			out = out.Mul(-1)
		}
		face := vec3(out.X, 0, out.Z)
		w.quad(
			w.corner(foot(p), face, colKerb), w.corner(top(p), face, colKerb),
			w.corner(top(q), face, colKerb), w.corner(foot(q), face, colKerb),
		)
	}
}

// stitch fills the ground between two closed lines, one inside the other and
// wound the same way from about the same place, with triangles that use the
// points of both and no others. That is what lets a level rim meet a net of
// ground without a crack: they share every point along the join.
func stitch(outer, inner []mathx.Vec) []mathx.Vec {
	n, m := len(outer), len(inner)
	if n < 3 || m < 3 {
		return nil
	}
	out := make([]mathx.Vec, 0, 3*(n+m))
	for i, j := 0, 0; i < n || j < m; {
		a, b := outer[i%n], inner[j%m]
		// Step along whichever line leaves the shorter gap to close.
		stepOuter := j >= m
		if i < n && j < m {
			stepOuter = outer[(i+1)%n].DistTo(b) <= a.DistTo(inner[(j+1)%m])
		}
		if stepOuter {
			out = append(out, a, outer[(i+1)%n], b)
			i++
		} else {
			out = append(out, a, inner[(j+1)%m], b)
			j++
		}
	}
	return out
}

// lift is how far above its block's surface something planned at a point
// stands, while a block that rises is being dressed.
func (g *Game) lift(planned mathx.Vec) float32 {
	if g.rise == nil {
		return 0
	}
	return g.rise.at(planned)
}

// copse returns the trees standing on the hill behind a block's houses.
func copse(b city.Block, m city.Map) []tree {
	s, ok := swellOf(b, m)
	if !ok || b.Park {
		return nil
	}
	ground := shapeOf(b, s, true)
	if ground == nil {
		return nil
	}
	size := s.hi.Sub(s.lo)
	n := int(size.X * size.Z / copseRoom)
	out := make([]tree, 0, n)
	for i := range n {
		h := scatter(b.Min.X, b.Min.Z, 90+i)
		at := mathx.V(s.lo.X+h[0]*size.X, s.lo.Z+h[1]*size.Z)
		out = append(out, tree{
			at: at, height: 5 + h[2]*4, spread: 1.8 + h[1]*1.2, shade: i,
			lift: ground.at(at),
		})
	}
	return out
}
