package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Most of the scene is drawn a shape at a time, each handed over corner by
// corner every frame. That suits a thing made of a few boxes. It does not
// suit ground: a hillside is thousands of corners that never move, and
// handing them over one at a time costs more than everything else in the
// frame together. So ground that has a shape is worked out once, handed to
// the graphics card, and drawn from there with one call.
//
// A patch goes through the same shader as everything else and is in the
// world's own coordinates, so it is lit, fogged and marked as a material
// exactly as a shape drawn by hand is. It must be drawn with nothing on the
// matrix stack.

// land is how high the ground stands at a point, and nil where a place is
// level, which costs nothing to ask.
type land func(mathx.Vec) float32

func (l land) at(p mathx.Vec) float32 {
	if l == nil {
		return 0
	}
	return l(p)
}

// patchwork gathers the corners and triangles of a patch being made.
type patchwork struct {
	at, up []float32
	col    []uint8
	tris   []uint16
}

// corner adds a corner and returns its number. A patch's triangles name
// their corners by a sixteen-bit number, so it can have 65,535 of them and
// no more: a block of the largest kind has a few thousand.
func (w *patchwork) corner(at, up rl.Vector3, col rl.Color) uint16 {
	n := uint16(len(w.at) / 3)
	w.at = append(w.at, at.X, at.Y, at.Z)
	w.up = append(w.up, up.X, up.Y, up.Z)
	w.col = append(w.col, col.R, col.G, col.B, col.A)
	return n
}

// tri adds a triangle between three corners, turned to face up.
func (w *patchwork) tri(a, b, c uint16) {
	p := func(i uint16) (x, z float32) { return w.at[3*int(i)], w.at[3*int(i)+2] }
	ax, az := p(a)
	bx, bz := p(b)
	cx, cz := p(c)
	if (bx-ax)*(cz-az)-(bz-az)*(cx-ax) > 0 {
		b, c = c, b
	}
	w.tris = append(w.tris, a, b, c)
}

// quad adds a four-cornered piece, its corners given in order round it.
func (w *patchwork) quad(a, b, c, d uint16) {
	w.tris = append(w.tris, a, b, c, a, c, d)
}

// lay adds a four-cornered piece lying on the ground, its corners given in
// order round it with the height of each.
func (w *patchwork) lay(a, b, c, d mathx.Vec, y [4]float32, col rl.Color) {
	up := rl.NewVector3(0, 1, 0)
	at := func(p mathx.Vec, h float32) uint16 { return w.corner(rl.NewVector3(p.X, h, p.Z), up, col) }
	i, j, k, l := at(a, y[0]), at(b, y[1]), at(c, y[2]), at(d, y[3])
	w.tri(i, j, k)
	w.tri(i, k, l)
}

// patch is a piece of ground held by the graphics card.
type patch struct{ mesh rl.Mesh }

// upload hands the patch over, and returns nil if there is nothing in it.
func (w *patchwork) upload() *patch {
	if len(w.tris) == 0 {
		return nil
	}
	p := &patch{mesh: rl.Mesh{
		VertexCount:   int32(len(w.at) / 3),
		TriangleCount: int32(len(w.tris) / 3),
		Vertices:      &w.at[0],
		Normals:       &w.up[0],
		Colors:        &w.col[0],
		Indices:       &w.tris[0],
	}}
	rl.UploadMesh(&p.mesh, false)
	// The card has its own copy now, and drawing needs only its name for
	// it. Letting go of ours is what lets the memory be had back. The list
	// of triangles is kept: it is by finding one that the card is asked to
	// draw corners by number rather than three at a time as they come.
	p.mesh.Vertices, p.mesh.Normals, p.mesh.Colors = nil, nil, nil
	return p
}

// draw draws the patch. It is a method on a patch that may be nil, which
// draws nothing.
func (p *patch) draw(m rl.Material) {
	if p != nil {
		rl.DrawMesh(p.mesh, m, rl.MatrixIdentity())
	}
}

// free gives the patch back.
func (p *patch) free() {
	if p != nil && p.mesh.VaoID != 0 {
		rl.UnloadMesh(&p.mesh)
		p.mesh.VaoID = 0
	}
}
