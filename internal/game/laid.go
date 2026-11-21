package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

// A place may be laid on the ground differently from the way it was planned:
// its junctions moved and turned, its streets bent to join them. Everything
// here that dresses a place works it out on the plan, where streets are
// straight and a row of anything is a straight row, and then puts it on the
// ground one of two ways.
//
// Something that stands up — a building, a bench, a hedge — is stood: drawn
// exactly as planned, but moved to where its own spot lies and turned as far
// as the ground there has turned. It keeps its shape, which a wall has to.
//
// Something that lies flat — paint, a path, a parking bay — is laid: each
// corner of it put where that corner lies. A long line is laid in short
// pieces, so it follows a bend rather than cutting across it.
//
// Where the ground is as planned both come to drawing the thing where it is.

// layPiece is the longest piece a line on the ground is laid in, in metres.
const layPiece float32 = 4

// pushStand begins drawing something planned to stand at one place, where it
// stands on the ground. Whatever is drawn before the matching [rl.PopMatrix]
// is drawn in the plan's own coordinates.
func pushStand(planned, on mathx.Vec, turn float32) {
	rl.PushMatrix()
	rl.Translatef(on.X, 0, on.Z)
	rl.Rotatef(-turn*rad2deg, 0, 1, 0)
	rl.Translatef(-planned.X, 0, -planned.Z)
}

// stand begins drawing something planned to stand at a place, finding where
// that is. Use pushStand where the answer has been kept.
func (g *Game) stand(planned mathx.Vec) {
	on, turn := g.world.City.Lay(planned)
	pushStand(planned, on, turn)
	if up := g.lift(planned); up != 0 {
		// Up is up however the ground has turned.
		rl.Translatef(0, up, 0)
	}
}

// flat lays a four-cornered patch of ground.
func (g *Game) flat(a, b, c, d mathx.Vec, y float32, col rl.Color) {
	city := g.world.City
	if g.rise != nil {
		// On ground that rises a patch is draped: each corner as high as
		// the ground is there, and a little over.
		ya, yb, yc, yd := g.lift(a), g.lift(b), g.lift(c), g.lift(d)
		a, _ = city.Lay(a)
		b, _ = city.Lay(b)
		c, _ = city.Lay(c)
		d, _ = city.Lay(d)
		y += drape
		groundPatch(a, b, c, d, [4]float32{y + ya, y + yb, y + yc, y + yd}, col)
		return
	}
	if city.Laid() {
		a, _ = city.Lay(a)
		b, _ = city.Lay(b)
		c, _ = city.Lay(c)
		d, _ = city.Lay(d)
	}
	groundQuad(a, b, c, d, y, col)
}

// strip lays a band of ground from a to b, so wide, in pieces short enough
// to follow a bend.
func (g *Game) strip(a, b mathx.Vec, width, y float32, col rl.Color) {
	run := b.Sub(a)
	length := run.Len()
	if length < 1e-3 {
		return
	}
	dir := run.Mul(1 / length)
	side := dir.Right().Mul(width / 2)
	pieces := 1
	if g.world.City.Laid() {
		pieces = max(int(length/layPiece), 1)
	}
	for i := range pieces {
		p := a.Add(dir.Mul(length * float32(i) / float32(pieces)))
		q := a.Add(dir.Mul(length * float32(i+1) / float32(pieces)))
		g.flat(p.Sub(side), q.Sub(side), q.Add(side), p.Add(side), y, col)
	}
}

// layOutline lays the outline of a block, adding points along its straight
// sides so that each follows the street it runs beside.
func (g *Game) layOutline(pts []mathx.Vec) []mathx.Vec {
	city := g.world.City
	if !city.Laid() {
		return pts
	}
	out := make([]mathx.Vec, 0, len(pts)*3)
	for i, p := range pts {
		q := pts[(i+1)%len(pts)]
		steps := max(int(p.DistTo(q)/layPiece), 1)
		for k := range steps {
			on, _ := city.Lay(p.Lerp(q, float32(k)/float32(steps)))
			out = append(out, on)
		}
	}
	return out
}
