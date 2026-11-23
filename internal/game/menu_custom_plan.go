package game

import (
	"math"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// The plan on the custom map page: a small patch of the map the spec asks for,
// as flat shapes (docs/menus.md, "Custom map"). It is the real generator run on
// `custom.Spec.Params` and then read back, not a picture of what the sliders
// might mean, so it shows what will be driven.
//
// Nothing here draws and nothing here touches the generator beyond calling it:
// the patch is its own city (city.NewGrowing, then EnsureAround, which is how
// the game grows one), built apart from any game and thrown away once it has
// been read. The shapes it keeps are plain data in metres, so the same spec
// makes the same plan and a test can compare two with ==.
//
// A patch costs between about 7 and 30 ms (BenchmarkCustomPlan), so a plan is
// built once for a spec and kept: [planCache] hands back the last one until the
// parameters it was built from change, and builds the next only after the spec
// has been still for a moment, so holding a key down does not build a city for
// every step.

// Size of the patch, in metres. It is about as wide as it is tall as the pane
// it is drawn in, which crops what does not fit.
const (
	planW, planH = 1000, 880
	// planRim is how far past the patch the city is grown, so the roads and
	// blocks at its edge are built.
	planRim = 160
	// planStep is how far apart the points of a bent street or block edge
	// are, in metres, and planGrid the spacing of the height samples.
	planStep = 30
	planGrid = 25
	// planSeaAt is how far down the patch the sea is put when the map has a
	// coast, as a share of its height.
	planSeaAt = 0.68
)

// planKey is everything a plan depends on, and so what decides whether the
// one in hand is still right. The weather, grip, traffic and the rest of what
// the spec says are not in it: they change nothing drawn.
type planKey struct {
	Params city.Params
	Season city.Season
}

// keyOf is the key of a spec.
func keyOf(sp custom.Spec) planKey {
	p, _ := sp.Params()
	return planKey{Params: p, Season: sp.Season()}
}

// planKind is what a block of the plan is.
type planKind uint8

const (
	planBuilt planKind = iota
	planOpen           // a park, field or green
	planBeach
)

// planBlock is one block as a polygon on the ground, wound round its edge.
type planBlock struct {
	Pts  []mathx.Vec
	Kind planKind
	// Shade is how tall the buildings on it are, from 0 to 1.
	Shade float32
}

// planRoad is one street between two junctions, as it lies on the ground.
type planRoad struct {
	Pts   []mathx.Vec
	Class city.RoadClass
}

// planRound is a roundabout: where it is and how wide, in metres.
type planRound struct {
	At mathx.Vec
	R  float32
}

// planSeg is a piece of a height contour.
type planSeg struct {
	A, B mathx.Vec
	// Major marks every other level, which is drawn stronger.
	Major bool
}

// customPlan is a patch of the map the spec asks for.
type customPlan struct {
	Key planKey
	// Min and Max are the corners of the patch on the ground.
	Min, Max mathx.Vec
	Blocks   []planBlock
	Roads    []planRoad
	Rounds   []planRound
	Contours []planSeg
	// Sea is whether the map has a coast; SeaZ is where the water starts and
	// FrontZ the seafront road, both on the ground.
	Sea    bool
	SeaZ   float32
	FrontZ float32
	// Low and High are the lowest and highest the land stands in the patch,
	// in metres.
	Low, High float32
	Season    city.Season
	// Winding says the streets lie bent on the ground.
	Winding bool
}

// Hilly says the land in the patch rises and falls enough to show.
func (p *customPlan) Hilly() bool { return len(p.Contours) > 0 }

// buildPlan reads a patch of the map for a spec. It is the whole cost of the
// plan, and is pure: the same spec gives the same plan.
func buildPlan(sp custom.Spec) *customPlan {
	key := keyOf(sp)
	c := city.NewGrowing(key.Params)
	pl := &customPlan{Key: key, Season: key.Season, Winding: c.Laid()}

	// Where the patch lies: about the origin, or, with a coast, far enough
	// down that the sea is in the lower part of it.
	cz := float32(0)
	if front, sea, ok := c.Shoreline(); ok {
		pl.Sea, pl.FrontZ, pl.SeaZ = true, front, sea
		cz = sea - planH*(planSeaAt-0.5)
	}
	pl.Min = mathx.V(-planW/2, cz-planH/2)
	pl.Max = mathx.V(planW/2, cz+planH/2)
	c.EnsureAround(mathx.V(0, cz), float32(math.Hypot(planW, planH))/2+planRim)

	// How far the ground can lie from where the plan drew it, so a plan shape
	// just outside the patch is not left out when it lies in it.
	slack := float32(0)
	if c.Laid() {
		slack = key.Params.Wind * 2
	}
	r := planReader{pl: pl, c: c, plan: c.Plan(), slack: slack}
	r.roads()
	r.rounds()
	r.blocks()
	pl.contour(c)
	return pl
}

// planReader reads a grown city into a plan.
type planReader struct {
	pl *customPlan
	// c is the city on the ground and plan the city as it was drawn, where
	// every choice was made; they are one city for a place that is not bent.
	c, plan *city.City
	slack   float32
}

// near says whether a box of the plan, give or take the bend of the ground,
// reaches the patch.
func (r planReader) near(lo, hi mathx.Vec) bool {
	pl := r.pl
	return hi.X+r.slack >= pl.Min.X && lo.X-r.slack <= pl.Max.X &&
		hi.Z+r.slack >= pl.Min.Z && lo.Z-r.slack <= pl.Max.Z
}

// roads reads the streets between junctions that have been wired.
func (r planReader) roads() {
	for _, rd := range r.plan.Roads {
		if rd.Ring {
			continue
		}
		a, b := r.plan.Nodes[rd.A], r.plan.Nodes[rd.B]
		if !r.plan.Complete(a) || !r.plan.Complete(b) || !r.near(minVec(a.Pos, b.Pos), maxVec(a.Pos, b.Pos)) {
			continue
		}
		r.pl.Roads = append(r.pl.Roads, planRoad{Pts: laidLine(r.c, a.Pos, b.Pos), Class: rd.Class})
	}
}

// rounds reads the roundabouts.
func (r planReader) rounds() {
	for _, n := range r.plan.Roundabouts() {
		if !r.plan.Complete(n) {
			continue
		}
		if at, _ := r.c.Lay(n.Pos); r.near(at, at) {
			r.pl.Rounds = append(r.pl.Rounds, planRound{At: at, R: n.RingOuter()})
		}
	}
}

// blocks reads the blocks, and shades each by how tall what is built on it is.
func (r planReader) blocks() {
	for _, b := range r.plan.Blocks {
		if !r.near(b.Min, b.Max) {
			continue
		}
		pb := planBlock{Pts: laidBlock(r.c, b.Min, b.Max)}
		switch {
		case b.Beach:
			pb.Kind = planBeach
		case b.Park:
			pb.Kind = planOpen
		}
		sum, n := float32(0), float32(0)
		for _, bd := range r.plan.Buildings {
			if p := bd.Center; p.X >= b.Min.X && p.X <= b.Max.X && p.Z >= b.Min.Z && p.Z <= b.Max.Z {
				sum, n = sum+bd.Height, n+1
			}
		}
		if n > 0 {
			pb.Shade = mathx.Clamp((sum/n-5)/30, 0, 1)
		}
		r.pl.Blocks = append(r.pl.Blocks, pb)
	}
}

func minVec(a, b mathx.Vec) mathx.Vec { return mathx.V(min(a.X, b.X), min(a.Z, b.Z)) }
func maxVec(a, b mathx.Vec) mathx.Vec { return mathx.V(max(a.X, b.X), max(a.Z, b.Z)) }

// laidLine is the straight line a plan street is, from a to b, as it lies on
// the ground: one segment if the ground is the plan, and a point every
// planStep metres if it has been bent.
func laidLine(c *city.City, a, b mathx.Vec) []mathx.Vec {
	n := 1
	if c.Laid() {
		n = max(int(math.Ceil(float64(b.Sub(a).Len())/planStep)), 1)
	}
	pts := make([]mathx.Vec, 0, n+1)
	for i := 0; i <= n; i++ {
		at, _ := c.Lay(a.Lerp(b, float32(i)/float32(n)))
		pts = append(pts, at)
	}
	return pts
}

// laidBlock is the edge of a block as it lies on the ground, wound from the
// Min corner round through (Max.X, Min.Z). The edges are cut into pieces on a
// bent ground, so a block follows the streets either side of it.
func laidBlock(c *city.City, lo, hi mathx.Vec) []mathx.Vec {
	corners := [4]mathx.Vec{lo, mathx.V(hi.X, lo.Z), hi, mathx.V(lo.X, hi.Z)}
	var pts []mathx.Vec
	for i, a := range corners {
		line := laidLine(c, a, corners[(i+1)%4])
		pts = append(pts, line[:len(line)-1]...)
	}
	return pts
}

// contour reads the land's height over the patch and cuts it into contour
// lines, a few levels of them, by marching squares.
func (pl *customPlan) contour(c *city.City) {
	if !c.Hilly() {
		return
	}
	nx := int(math.Ceil(planW / planGrid))
	nz := int(math.Ceil(planH / planGrid))
	g := heightGrid{
		nx: nx, min: pl.Min,
		sx: float32(planW) / float32(nx), sz: float32(planH) / float32(nz),
		h: make([]float32, (nx+1)*(nz+1)),
	}
	lo, hi := float32(math.Inf(1)), float32(math.Inf(-1))
	for j := 0; j <= nz; j++ {
		for i := 0; i <= nx; i++ {
			v := c.Height(g.pos(i, j))
			g.h[j*(nx+1)+i] = v
			lo, hi = min(lo, v), max(hi, v)
		}
	}
	pl.Low, pl.High = lo, hi
	step := contourStep(hi - lo)
	if step == 0 {
		return
	}
	for k := int(math.Floor(float64(lo/step))) + 1; float32(k)*step < hi; k++ {
		for j := 0; j < nz; j++ {
			for i := 0; i < nx; i++ {
				pl.Contours = g.cut(pl.Contours, i, j, float32(k)*step, k%2 == 0)
			}
		}
	}
}

// heightGrid is the land's height sampled on a grid over the patch.
type heightGrid struct {
	nx     int
	min    mathx.Vec
	sx, sz float32
	h      []float32
}

// pos is where the grid point (i, j) is on the ground.
func (g heightGrid) pos(i, j int) mathx.Vec {
	return mathx.V(g.min.X+float32(i)*g.sx, g.min.Z+float32(j)*g.sz)
}

// cut adds to segs the pieces of the contour at level that cross the grid
// square whose top left corner is (i, j).
func (g heightGrid) cut(segs []planSeg, i, j int, level float32, major bool) []planSeg {
	at := func(i, j int) float32 { return g.h[j*(g.nx+1)+i] }
	// The corners clockwise from the top left, and where the level crosses
	// each edge that it does cross.
	v := [4]float32{at(i, j), at(i+1, j), at(i+1, j+1), at(i, j+1)}
	p := [4]mathx.Vec{g.pos(i, j), g.pos(i+1, j), g.pos(i+1, j+1), g.pos(i, j+1)}
	var cross []mathx.Vec
	for e := range 4 {
		a, b := v[e], v[(e+1)%4]
		if (a < level) != (b < level) {
			cross = append(cross, p[e].Lerp(p[(e+1)%4], (level-a)/(b-a)))
		}
	}
	for k := 0; k+1 < len(cross); k += 2 {
		segs = append(segs, planSeg{cross[k], cross[k+1], major})
	}
	return segs
}

// contourStep is the height between contour lines for land that rises and
// falls by span metres: about six lines, at a round number. Land that is all
// but level has none.
func contourStep(span float32) float32 {
	if span < 0.4 {
		return 0
	}
	for _, s := range [...]float32{0.5, 1, 2, 2.5, 5, 10} {
		if span/s <= 7 {
			return s
		}
	}
	return 10
}

// planCache keeps the plan for the spec being edited, and builds the next
// one when the spec has stood still long enough.
//
// Ask is called every frame with the spec as it is and costs a comparison. A
// patch builds in a few tens of milliseconds, so it is built where it is asked
// for, on the frame the spec has held still for [planSettle] frames, and not
// before: holding a key down does not build a city for every step. Until then
// the plan in hand stays on show, drawn as stale.
type planCache struct {
	shown *customPlan
	want  planKey
	// still counts the frames want has been unchanged, and set says there is one.
	still int
	set   bool
	// builds counts the plans built, for tests.
	builds int
}

// planSettle is how many frames the spec has to hold still before a plan is
// built for it. A page's first plan is built at once.
const planSettle = 8

// Ask says what to draw for a spec: the plan for it, or the one in hand while
// the spec is still being changed, or nil before the first is built.
func (pc *planCache) Ask(sp custom.Spec) *customPlan {
	key := keyOf(sp)
	if !pc.set || pc.want != key {
		pc.want, pc.set, pc.still = key, true, 0
	} else if pc.still < planSettle {
		pc.still++
	}
	if pc.shown != nil && (pc.shown.Key == key || pc.still < planSettle) {
		return pc.shown
	}
	pc.shown = buildPlan(sp)
	pc.builds++
	return pc.shown
}

// Stale says the plan on show is for something other than the spec as it
// stands, which is drawn dimmed.
func (pc *planCache) Stale(sp custom.Spec) bool {
	return pc.shown != nil && pc.shown.Key != keyOf(sp)
}
