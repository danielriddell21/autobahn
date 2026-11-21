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
// A block that rises is drawn in two parts. Its rim, out to the kerb, is
// level and carries what is built there. Inside the rim is a net of ground
// pegged down round its edge and lifted in the middle. Anything dressing the
// block is put on the net by asking how high it is there: see Game.rise.

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
	// swellCell is the longest side of a square in the net.
	swellCell float32 = 7
	// drape is how far above the net something lying on it is drawn. The net
	// is flat between its knots and the hill is not, so anything closer
	// would sink into it between them.
	drape float32 = 0.06
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

// risen is a block's rising ground as it is drawn: the net, knot by knot,
// and the level rim round it.
type risen struct {
	swell
	// cols and rows count the squares of the net; knots holds its knots
	// where they lie on the ground, row by row, and up the way the ground
	// faces at each.
	cols, rows int
	knots, up  []rl.Vector3
	// rim is the level ground between the kerb and the net, as triangles.
	rim []mathx.Vec
}

// raise works out a block's rising ground, or nil if it has none. kerb is
// the block's outline as it lies.
func (g *Game) raise(b city.Block, kerb []mathx.Vec) *risen {
	s, ok := swellOf(b, g.opts.Map)
	if !ok {
		return nil
	}
	size := s.hi.Sub(s.lo)
	r := &risen{
		swell: s,
		cols:  max(int(size.X/swellCell)+1, 2),
		rows:  max(int(size.Z/swellCell)+1, 2),
	}
	c := g.world.City
	knot := func(i, j int) mathx.Vec {
		return mathx.V(s.lo.X+size.X*float32(i)/float32(r.cols), s.lo.Z+size.Z*float32(j)/float32(r.rows))
	}
	for j := 0; j <= r.rows; j++ {
		for i := 0; i <= r.cols; i++ {
			p := knot(i, j)
			on, _ := c.Lay(p)
			r.knots = append(r.knots, vec3(on.X, city.KerbHeight+s.at(p), on.Z))
		}
	}
	// Which way the ground faces at a knot, from the knots either side.
	for j := 0; j <= r.rows; j++ {
		for i := 0; i <= r.cols; i++ {
			e := r.knot(min(i+1, r.cols), j)
			w := r.knot(max(i-1, 0), j)
			n := r.knot(i, min(j+1, r.rows))
			s := r.knot(i, max(j-1, 0))
			up := rl.Vector3CrossProduct(rl.Vector3Subtract(n, s), rl.Vector3Subtract(e, w))
			if up.Y < 0 {
				up = rl.Vector3Negate(up)
			}
			r.up = append(r.up, rl.Vector3Normalize(up))
		}
	}

	// The edge of the net, wound the way the kerb is, so the two can be
	// stitched together.
	edge := make([]mathx.Vec, 0, 2*(r.cols+r.rows))
	flat := func(i, j int) mathx.Vec { v := r.knot(i, j); return mathx.V(v.X, v.Z) }
	for i := range r.cols {
		edge = append(edge, flat(i, 0))
	}
	for j := range r.rows {
		edge = append(edge, flat(r.cols, j))
	}
	for i := r.cols; i > 0; i-- {
		edge = append(edge, flat(i, r.rows))
	}
	for j := r.rows; j > 0; j-- {
		edge = append(edge, flat(0, j))
	}
	r.rim = stitch(kerb, edge)
	return r
}

func (r *risen) knot(i, j int) rl.Vector3 { return r.knots[j*(r.cols+1)+i] }

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

// draw lays the rim in one colour and the risen ground in another.
func (r *risen) draw(rim, ground rl.Color) {
	rl.Begin(rl.Triangles)
	tint(rim)
	rl.Normal3f(0, 1, 0)
	for i := 0; i+2 < len(r.rim); i += 3 {
		a, b, c := r.rim[i], r.rim[i+1], r.rim[i+2]
		if b.Sub(a).Cross(c.Sub(a)) > 0 {
			b, c = c, b
		}
		rl.Vertex3f(a.X, city.KerbHeight, a.Z)
		rl.Vertex3f(b.X, city.KerbHeight, b.Z)
		rl.Vertex3f(c.X, city.KerbHeight, c.Z)
	}
	rl.End()

	put := func(i, j int) {
		k := j*(r.cols+1) + i
		rl.Normal3f(r.up[k].X, r.up[k].Y, r.up[k].Z)
		rl.Vertex3f(r.knots[k].X, r.knots[k].Y, r.knots[k].Z)
	}
	// A row at a time, so that no one batch of triangles outgrows the
	// buffer they are gathered in.
	for j := range r.rows {
		rl.Begin(rl.Triangles)
		tint(ground)
		for i := range r.cols {
			put(i, j)
			put(i, j+1)
			put(i+1, j+1)
			put(i, j)
			put(i+1, j+1)
			put(i+1, j)
		}
		rl.End()
	}
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
	size := s.hi.Sub(s.lo)
	n := int(size.X * size.Z / copseRoom)
	out := make([]tree, 0, n)
	for i := range n {
		h := scatter(b.Min.X, b.Min.Z, 90+i)
		at := mathx.V(s.lo.X+h[0]*size.X, s.lo.Z+h[1]*size.Z)
		out = append(out, tree{
			at: at, height: 5 + h[2]*4, spread: 1.8 + h[1]*1.2, shade: i,
			lift: s.at(at),
		})
	}
	return out
}
