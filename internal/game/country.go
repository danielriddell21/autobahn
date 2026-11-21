package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// The village is the city's generator asked for long lanes, low houses and a
// great deal of open ground. What makes it the country is here: stone and
// whitewash, and the open ground turned into fields with hedges round them.

var (
	// villageShades are stone, render and whitewash.
	villageShades = [6]rl.Color{
		rl.NewColor(196, 184, 160, 255),
		rl.NewColor(214, 206, 190, 255),
		rl.NewColor(176, 160, 136, 255),
		rl.NewColor(186, 172, 150, 255),
		rl.NewColor(152, 140, 124, 255),
		rl.NewColor(206, 190, 164, 255),
	}
	colHedge  = rl.NewColor(58, 96, 60, 255)
	colCrop   = rl.NewColor(200, 190, 86, 255)
	colPlough = rl.NewColor(128, 106, 84, 255)
	colSheep  = rl.NewColor(226, 222, 212, 255)
)

// fieldKind is what a piece of open ground in the country is used for.
type fieldKind int

const (
	// fieldNone is not a field at all: a green, laid out like a park.
	fieldNone fieldKind = iota
	fieldPasture
	fieldCrop
	fieldPlough
)

// Field layout, in metres.
const (
	// hedgeInset is how far in from the kerb a field's hedge stands, leaving
	// a verge, and hedgeHeight how tall it is.
	hedgeInset  float32 = 8
	hedgeHeight float32 = 1.3
	// rowPitch is the gap between the rows of a crop or the furrows of a
	// ploughed field.
	rowPitch float32 = 4
	// sheepRoom is the pasture to each sheep, in square metres.
	sheepRoom float32 = 700
	// hedgeTree is how far apart the trees in a hedgerow stand.
	hedgeTree float32 = 26
	// hedgePiece is the longest length a hedge is stood in.
	hedgePiece float32 = 6
)

// fieldOf decides what a block of open ground is. Only the village has
// fields; and even there one in a few is the green.
func fieldOf(b city.Block, m city.Map) fieldKind {
	if m != city.MapVillage || !b.Park {
		return fieldNone
	}
	c := b.Min.Lerp(b.Max, 0.5)
	switch h := scatter(c.X, c.Z, 61)[0]; {
	case h < 0.15:
		return fieldNone
	case h < 0.5:
		return fieldPasture
	case h < 0.8:
		return fieldCrop
	}
	return fieldPlough
}

// fieldShade is the colour of the ground in a field.
func fieldShade(k fieldKind, lk seasonLook) rl.Color {
	switch k {
	case fieldCrop:
		return colCrop
	case fieldPlough:
		return colPlough
	}
	return lk.park
}

// hedgeRun is one straight length of hedge, from a to b.
type hedgeRun struct{ a, b mathx.Vec }

// hedges returns the hedge round a field: a length along each side, set in
// from the kerb and stopping short of each corner. A corner cut back for a
// roundabout is stood well clear of, which leaves the gap a gate would go in.
func hedges(b city.Block) []hedgeRun {
	// clear is how far along a side from each corner the hedge starts.
	var clear [4]float32
	for i, c := range b.Corners {
		clear[i] = max(hedgeInset, c.Bite+2)
	}
	lo := mathx.V(b.Min.X+hedgeInset, b.Min.Z+hedgeInset)
	hi := mathx.V(b.Max.X-hedgeInset, b.Max.Z-hedgeInset)
	runs := []hedgeRun{
		{mathx.V(b.Min.X+clear[0], lo.Z), mathx.V(b.Max.X-clear[1], lo.Z)},
		{mathx.V(b.Min.X+clear[2], hi.Z), mathx.V(b.Max.X-clear[3], hi.Z)},
		{mathx.V(lo.X, b.Min.Z+clear[0]), mathx.V(lo.X, b.Max.Z-clear[2])},
		{mathx.V(hi.X, b.Min.Z+clear[1]), mathx.V(hi.X, b.Max.Z-clear[3])},
	}
	out := runs[:0]
	for _, r := range runs {
		if r.b.X-r.a.X > 4 || r.b.Z-r.a.Z > 4 {
			out = append(out, r)
		}
	}
	return out
}

// drawField dresses a field: its hedge, and what is growing or grazing in it.
func (g *Game) drawField(b city.Block, k fieldKind) {
	ground := city.KerbHeight
	for _, h := range hedges(b) {
		// A hedge is stood a length at a time, so it follows the lane it
		// runs beside.
		length := h.a.DistTo(h.b)
		pieces := max(int(length/hedgePiece), 1)
		for i := range pieces {
			p := h.a.Lerp(h.b, float32(i)/float32(pieces))
			q := h.a.Lerp(h.b, float32(i+1)/float32(pieces))
			mid := p.Lerp(q, 0.5)
			g.stand(mid)
			rl.DrawCube(vec3(mid.X, ground+hedgeHeight/2, mid.Z),
				max(q.X-p.X+0.3, 1.1), hedgeHeight, max(q.Z-p.Z+0.3, 1.1), colHedge)
			rl.PopMatrix()
		}
	}

	switch {
	case k == fieldPasture:
		for _, at := range flock(b) {
			g.stand(at)
			rl.DrawCube(vec3(at.X, ground+0.62, at.Z), 1.05, 0.55, 0.62, colSheep)
			rl.DrawCube(vec3(at.X+0.6, ground+0.78, at.Z), 0.3, 0.3, 0.3, colTrim)
			rl.DrawCube(vec3(at.X, ground+0.18, at.Z), 0.8, 0.36, 0.4, colTrim)
			rl.PopMatrix()
		}
	case g.rise == nil:
		// On ground with a shape the rows went in with the ground.
		g.layRows(b, k)
	}
}

// layRows lays the rows of a crop or the furrows of a ploughed field,
// running the long way of it.
func (g *Game) layRows(b city.Block, k fieldKind) {
	if k != fieldCrop && k != fieldPlough {
		return
	}
	in := hedgeInset + 3
	lo, hi := mathx.V(b.Min.X+in, b.Min.Z+in), mathx.V(b.Max.X-in, b.Max.Z-in)
	if hi.X-lo.X < 10 || hi.Z-lo.Z < 10 {
		return
	}
	row := scaleColor(fieldShade(k, seasonLook{}), 0.82)
	y := city.KerbHeight + 0.012
	if hi.X-lo.X >= hi.Z-lo.Z {
		for z := lo.Z; z < hi.Z; z += rowPitch {
			g.strip(mathx.V(lo.X, z+0.4), mathx.V(hi.X, z+0.4), 0.8, y, row)
		}
		return
	}
	for x := lo.X; x < hi.X; x += rowPitch {
		g.strip(mathx.V(x+0.4, lo.Z), mathx.V(x+0.4, hi.Z), 0.8, y, row)
	}
}

// flock returns where the sheep in a pasture stand.
func flock(b city.Block) []mathx.Vec {
	in := hedgeInset + 4
	lo, hi := mathx.V(b.Min.X+in, b.Min.Z+in), mathx.V(b.Max.X-in, b.Max.Z-in)
	size := hi.Sub(lo)
	if size.X < 6 || size.Z < 6 {
		return nil
	}
	n := int(size.X * size.Z / sheepRoom)
	out := make([]mathx.Vec, 0, n)
	for i := range n {
		h := scatter(b.Min.X, b.Min.Z, 70+i)
		out = append(out, mathx.V(lo.X+h[0]*size.X, lo.Z+h[1]*size.Z))
	}
	return out
}

// hedgerow returns the trees standing in the hedge round a field.
func hedgerow(b city.Block) []tree {
	var out []tree
	for _, h := range hedges(b) {
		along := h.b.Sub(h.a)
		length := along.Len()
		dir := along.Norm()
		for d, i := hedgeTree/2, 0; d < length; d, i = d+hedgeTree, i+1 {
			at := h.a.Add(dir.Mul(d))
			s := scatter(at.X, at.Z, 63)
			if s[0] < 0.4 {
				continue
			}
			out = append(out, tree{at: at, height: 5 + s[1]*3.5, spread: 1.7 + s[2]*1.2, shade: i})
		}
	}
	return out
}
