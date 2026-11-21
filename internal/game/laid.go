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

// drapePiece is the longest piece a line is laid in over ground that has a
// shape of its own.
const drapePiece float32 = 2

// hillLift is how much further above the ground something flat is laid on a
// hill than on the level, in metres.
const hillLift float32 = 0.012

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
	if up := g.lift(planned) + g.land.at(on); up != 0 {
		// Up is up however the ground has turned.
		rl.Translatef(0, up, 0)
	}
}

// standOn begins drawing something planned to stand at one place where it
// stands on the ground, as pushStand does, and as high as the land is there.
func (g *Game) standOn(planned, on mathx.Vec, turn float32) {
	pushStand(planned, on, turn)
	if up := g.land.at(on); up != 0 {
		rl.Translatef(0, up, 0)
	}
}

// pushUp begins drawing something that stands at a point on the ground, as
// high as the land is there. Whatever is drawn before the matching
// [rl.PopMatrix] is drawn where it would be on the level, and lifted.
func (g *Game) pushUp(on mathx.Vec) {
	rl.PushMatrix()
	if up := g.land.at(on); up != 0 {
		rl.Translatef(0, up, 0)
	}
}

// flat lays a four-cornered patch of ground.
func (g *Game) flat(a, b, c, d mathx.Vec, y float32, col rl.Color) {
	city := g.world.City
	var up [4]float32
	if g.rise != nil {
		// On a block whose ground has a shape a patch is draped: each
		// corner as high as the ground is there, and a little over.
		up = [4]float32{g.lift(a), g.lift(b), g.lift(c), g.lift(d)}
		y += drape
	}
	if city.Laid() {
		a, _ = city.Lay(a)
		b, _ = city.Lay(b)
		c, _ = city.Lay(c)
		d, _ = city.Lay(d)
	}
	if g.rise == nil && g.land == nil {
		groundPatch(a, b, c, d, [4]float32{y, y, y, y}, col)
		return
	}
	if g.baking != nil {
		g.baking.lay(a, b, c, d, [4]float32{
			y + up[0] + g.land.at(a), y + up[1] + g.land.at(b),
			y + up[2] + g.land.at(c), y + up[3] + g.land.at(d),
		}, col)
		return
	}
	if g.rise == nil {
		// Paint on a road over a hill: the road under it is flat between
		// its own corners, which are not these, so it is given a hair more
		// room than on the level.
		y += hillLift
	}
	groundPatch(a, b, c, d, [4]float32{
		y + up[0] + g.land.at(a), y + up[1] + g.land.at(b),
		y + up[2] + g.land.at(c), y + up[3] + g.land.at(d),
	}, col)
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
	switch {
	case g.rise != nil:
		// Short enough to go over a bank without cutting through it.
		pieces = max(int(length/drapePiece), 1)
	case g.world.City.Laid() || g.land != nil:
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
