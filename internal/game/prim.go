package game

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

// The handful of shapes raylib's own primitives do not cover: flat polygons on
// the ground, the walls round them, and cylinders that carry normals, which
// raylib's do not and so cannot be lit.

func sqrt32(v float32) float32 { return float32(math.Sqrt(float64(v))) }

// tint sets the colour for the vertices that follow.
func tint(c rl.Color) { rl.Color4ub(c.R, c.G, c.B, c.A) }

// groundFan fills a polygon lying on the ground at height y above it, as a
// fan about a point inside it. The polygon has to be star-shaped about that
// point.
func groundFan(l land, centre mathx.Vec, outline []mathx.Vec, y float32, col rl.Color) {
	if len(outline) < 3 {
		return
	}
	rl.Begin(rl.Triangles)
	tint(col)
	rl.Normal3f(0, 1, 0)
	yc := y + l.at(centre)
	for i, p := range outline {
		q := outline[(i+1)%len(outline)]
		// Wound so the face points up whichever way round the outline runs.
		if p.Sub(centre).Cross(q.Sub(centre)) > 0 {
			p, q = q, p
		}
		rl.Vertex3f(centre.X, yc, centre.Z)
		rl.Vertex3f(p.X, y+l.at(p), p.Z)
		rl.Vertex3f(q.X, y+l.at(q), q.Z)
	}
	rl.End()
}

// groundWall stands a wall round a polygon, from height y0 above the ground
// up to y1, facing outward from the given centre.
func groundWall(l land, centre mathx.Vec, outline []mathx.Vec, y0, y1 float32, col rl.Color) {
	if len(outline) < 3 {
		return
	}
	rl.Begin(rl.Triangles)
	tint(col)
	for i, p := range outline {
		q := outline[(i+1)%len(outline)]
		edge := q.Sub(p)
		if edge.Len() < 1e-4 {
			continue
		}
		out := edge.Norm().Right()
		if p.Sub(centre).Dot(out) < 0 {
			out = out.Mul(-1)
			p, q = q, p
		}
		hp, hq := l.at(p), l.at(q)
		rl.Normal3f(out.X, 0, out.Z)
		rl.Vertex3f(p.X, hp+y0, p.Z)
		rl.Vertex3f(p.X, hp+y1, p.Z)
		rl.Vertex3f(q.X, hq+y1, q.Z)
		rl.Vertex3f(p.X, hp+y0, p.Z)
		rl.Vertex3f(q.X, hq+y1, q.Z)
		rl.Vertex3f(q.X, hq+y0, q.Z)
	}
	rl.End()
}

// groundQuad lays a four-sided patch at height y above the ground. The
// corners are given in order round the patch.
func groundQuad(l land, a, b, c, d mathx.Vec, y float32, col rl.Color) {
	groundPatch(a, b, c, d, [4]float32{y + l.at(a), y + l.at(b), y + l.at(c), y + l.at(d)}, col)
}

// groundPatch lays a four-sided patch whose corners, given in order round
// it, each have a height of their own: a piece of something lying on a slope.
func groundPatch(a, b, c, d mathx.Vec, y [4]float32, col rl.Color) {
	rl.Begin(rl.Triangles)
	tint(col)
	rl.Normal3f(0, 1, 0)
	if b.Sub(a).Cross(c.Sub(a)) > 0 {
		b, d = d, b
		y[1], y[3] = y[3], y[1]
	}
	rl.Vertex3f(a.X, y[0], a.Z)
	rl.Vertex3f(b.X, y[1], b.Z)
	rl.Vertex3f(c.X, y[2], c.Z)
	rl.Vertex3f(a.X, y[0], a.Z)
	rl.Vertex3f(c.X, y[2], c.Z)
	rl.Vertex3f(d.X, y[3], d.Z)
	rl.End()
}

// groundDisc lays a disc at height y above the ground, bright in the middle
// and fading to nothing at the rim, which is how a pool of light on the road
// is drawn.
func groundDisc(l land, centre mathx.Vec, radius, y float32, col rl.Color, sides int) {
	rl.Begin(rl.Triangles)
	// No normal at all, which is how the shader knows this is light rather
	// than something light falls on.
	rl.Normal3f(0, 0, 0)
	yc := y + l.at(centre)
	for i := range sides {
		a0 := float32(i) / float32(sides) * 2 * math.Pi
		a1 := float32(i+1) / float32(sides) * 2 * math.Pi
		p := centre.Add(mathx.FromAngle(a1).Mul(radius))
		q := centre.Add(mathx.FromAngle(a0).Mul(radius))
		tint(col)
		rl.Vertex3f(centre.X, yc, centre.Z)
		rl.Color4ub(col.R, col.G, col.B, 0)
		rl.Vertex3f(p.X, y+l.at(p), p.Z)
		rl.Vertex3f(q.X, y+l.at(q), q.Z)
	}
	rl.End()
}

// groundBeam lays a fan of light on the road ahead of a lamp: bright at the
// lamp and fading out along its throw.
func groundBeam(l land, from, dir mathx.Vec, length, spread, y float32, col rl.Color) {
	side := dir.Right()
	a := from.Add(dir.Mul(length)).Sub(side.Mul(spread))
	b := from.Add(dir.Mul(length)).Add(side.Mul(spread))
	rl.Begin(rl.Triangles)
	rl.Normal3f(0, 0, 0)
	tint(col)
	rl.Vertex3f(from.X, y+l.at(from), from.Z)
	rl.Color4ub(col.R, col.G, col.B, 0)
	rl.Vertex3f(b.X, y+l.at(b), b.Z)
	rl.Vertex3f(a.X, y+l.at(a), a.Z)
	rl.End()
}

// pillar stands a lit cylinder on the ground plane, from y0 up to y1, tapering
// from r0 to r1, with a cap on top.
func pillar(at mathx.Vec, y0, y1, r0, r1 float32, sides int, col rl.Color) {
	rl.Begin(rl.Triangles)
	tint(col)
	for i := range sides {
		a0 := float32(i) / float32(sides) * 2 * math.Pi
		a1 := float32(i+1) / float32(sides) * 2 * math.Pi
		d0, d1 := mathx.FromAngle(a0), mathx.FromAngle(a1)
		// Angle increases from +X toward +Z, which seen from above is
		// clockwise, so the outward face takes the vertices in this order.
		b0, b1 := at.Add(d0.Mul(r0)), at.Add(d1.Mul(r0))
		t0, t1 := at.Add(d0.Mul(r1)), at.Add(d1.Mul(r1))
		rl.Normal3f(d0.X, 0, d0.Z)
		rl.Vertex3f(b0.X, y0, b0.Z)
		rl.Vertex3f(t0.X, y1, t0.Z)
		rl.Normal3f(d1.X, 0, d1.Z)
		rl.Vertex3f(t1.X, y1, t1.Z)
		rl.Normal3f(d0.X, 0, d0.Z)
		rl.Vertex3f(b0.X, y0, b0.Z)
		rl.Normal3f(d1.X, 0, d1.Z)
		rl.Vertex3f(t1.X, y1, t1.Z)
		rl.Vertex3f(b1.X, y0, b1.Z)

		rl.Normal3f(0, 1, 0)
		rl.Vertex3f(at.X, y1, at.Z)
		rl.Vertex3f(t1.X, y1, t1.Z)
		rl.Vertex3f(t0.X, y1, t0.Z)
	}
	rl.End()
}

// wheel draws a tyre in the car's own space: a cylinder lying across the car
// at (x, z), with its axle along Z.
func wheel(x, z, radius, width float32, col, hub rl.Color) {
	const sides = 10
	z0, z1 := z-width/2, z+width/2
	rl.Begin(rl.Triangles)
	for i := range sides {
		a0 := float32(i) / sides * 2 * math.Pi
		a1 := float32(i+1) / sides * 2 * math.Pi
		c0, s0 := mathx.Cos(a0), mathx.Sin(a0)
		c1, s1 := mathx.Cos(a1), mathx.Sin(a1)
		x0, y0 := x+c0*radius, radius+s0*radius
		x1, y1 := x+c1*radius, radius+s1*radius

		// The tread.
		tint(col)
		rl.Normal3f(c0, s0, 0)
		rl.Vertex3f(x0, y0, z0)
		rl.Vertex3f(x0, y0, z1)
		rl.Normal3f(c1, s1, 0)
		rl.Vertex3f(x1, y1, z1)
		rl.Normal3f(c0, s0, 0)
		rl.Vertex3f(x0, y0, z0)
		rl.Normal3f(c1, s1, 0)
		rl.Vertex3f(x1, y1, z1)
		rl.Vertex3f(x1, y1, z0)

		// The two faces, with a paler hub.
		for _, f := range [2]float32{z0, z1} {
			nz := float32(1)
			if f == z0 {
				nz = -1
			}
			rl.Normal3f(0, 0, nz)
			tint(hub)
			rl.Vertex3f(x, radius, f)
			tint(col)
			if nz > 0 {
				rl.Vertex3f(x0, y0, f)
				rl.Vertex3f(x1, y1, f)
			} else {
				rl.Vertex3f(x1, y1, f)
				rl.Vertex3f(x0, y0, f)
			}
		}
	}
	rl.End()
}

// taperBox draws a box that narrows toward the top, in the car's own space:
// a rectangle from x0 to x1 and w0 either side of the centre line at height
// y0, drawn in to run from xt0 to xt1 and w1 either side at height y1. It is
// what gives a car a raked windscreen instead of a brick on its roof.
func taperBox(x0, x1, w0, y0, xt0, xt1, w1, y1 float32, side, top rl.Color) {
	rise := y1 - y0
	quad := func(nx, ny, nz float32, v [4][3]float32) {
		rl.Normal3f(nx, ny, nz)
		for _, i := range [6]int{0, 1, 2, 0, 2, 3} {
			rl.Vertex3f(v[i][0], v[i][1], v[i][2])
		}
	}
	rl.Begin(rl.Triangles)
	tint(side)
	// Front and back, each leaning in by however far the top is set back.
	quad(rise, x1-xt1, 0, [4][3]float32{{x1, y0, -w0}, {x1, y0, w0}, {xt1, y1, w1}, {xt1, y1, -w1}})
	quad(-rise, xt0-x0, 0, [4][3]float32{{x0, y0, w0}, {x0, y0, -w0}, {xt0, y1, -w1}, {xt0, y1, w1}})
	// The two flanks.
	quad(0, w0-w1, rise, [4][3]float32{{x0, y0, w0}, {x1, y0, w0}, {xt1, y1, w1}, {xt0, y1, w1}})
	quad(0, w0-w1, -rise, [4][3]float32{{x1, y0, -w0}, {x0, y0, -w0}, {xt0, y1, -w1}, {xt1, y1, -w1}})
	tint(top)
	quad(0, 1, 0, [4][3]float32{{xt0, y1, -w1}, {xt0, y1, w1}, {xt1, y1, w1}, {xt1, y1, -w1}})
	rl.End()
}
