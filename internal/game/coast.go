package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// The seaside is the city's generator asked for a low town with the sea down
// one side. What makes it the seaside is here: painted terraces, and a front
// with a promenade, a rail, beach huts, sunshades on the sand and the odd
// pier.

var (
	colSand = rl.NewColor(214, 196, 150, 255)
	colSea  = rl.NewColor(70, 122, 150, 255)
	// seasideShades are the colours a seaside terrace is painted.
	seasideShades = [6]rl.Color{
		rl.NewColor(226, 198, 172, 255),
		rl.NewColor(188, 210, 214, 255),
		rl.NewColor(230, 214, 162, 255),
		rl.NewColor(214, 182, 188, 255),
		rl.NewColor(198, 214, 188, 255),
		rl.NewColor(230, 226, 214, 255),
	}
	hutShades = [5]rl.Color{
		rl.NewColor(196, 84, 76, 255),
		rl.NewColor(74, 132, 168, 255),
		rl.NewColor(222, 190, 92, 255),
		rl.NewColor(98, 156, 124, 255),
		rl.NewColor(226, 222, 210, 255),
	}
)

// seaReach is how far out the water is drawn, in metres: well past the fog.
const seaReach float32 = 6000

// townLook is how a map's buildings are finished.
type townLook struct {
	// facades are the colours the walls are painted.
	facades *[6]rl.Color
	// shops is the share of buildings with a shop on the ground floor; the
	// rest are houses, with a front door.
	shops float32
}

// townFor returns how a map's buildings are finished.
func townFor(m city.Map) townLook {
	switch m {
	case city.MapSeaside:
		return townLook{facades: &seasideShades, shops: 0.6}
	case city.MapVillage:
		return townLook{facades: &villageShades, shops: 0.22}
	}
	return townLook{facades: &buildingShades, shops: 1}
}

// Seafront layout, in metres, measured from the kerb of the front.
const (
	// promDepth is the paved promenade, with the rail along its far edge.
	promDepth float32 = 7
	railPitch float32 = 3
	// hutRow is how far down the sand the beach huts stand, and hutPitch how
	// far apart.
	hutRow   float32 = 12
	hutPitch float32 = 4.2
	// shadePitch is the stretch of beach to each sunshade.
	shadePitch float32 = 9
	// pierShare is how many stretches of beach have a pier, pierLength how
	// far it runs out past the sand and pierWidth how wide its deck is.
	pierShare  float32 = 0.3
	pierLength float32 = 70
	pierWidth  float32 = 7
)

// beachHut is one hut on the sand.
type beachHut struct {
	at    mathx.Vec
	shade int
}

// beachHuts returns the huts along a stretch of beach. They stand in a row
// with gaps in it, the same row every time.
func beachHuts(b city.Block) []beachHut {
	var out []beachHut
	z := b.Min.Z + hutRow
	if !b.Beach || z+4 > b.Max.Z {
		return nil
	}
	pier, piered := hasPier(b)
	for x := b.Min.X + 4; x < b.Max.X-4; x += hutPitch {
		h := scatter(x, z, 51)
		if h[0] < 0.32 {
			continue
		}
		if piered && mathx.Abs(x-pier) < pierWidth/2+2 {
			continue // the pier comes through here
		}
		out = append(out, beachHut{at: mathx.V(x, z), shade: int(h[1] * 97)})
	}
	return out
}

// hasPier reports whether a stretch of beach has a pier, and where along the
// front it leaves from.
func hasPier(b city.Block) (x float32, ok bool) {
	if !b.Beach || b.Max.X-b.Min.X < 40 {
		return 0, false
	}
	h := scatter(b.Min.X, b.Min.Z, 57)
	return b.Min.X + (b.Max.X-b.Min.X)*(0.3+0.4*h[1]), h[0] < pierShare
}

// drawBeach dresses a stretch of beach.
func drawBeach(b city.Block) {
	ground := city.KerbHeight
	y := ground + 0.012
	edge := b.Min.Z + promDepth
	width := b.Max.X - b.Min.X
	mid := (b.Min.X + b.Max.X) / 2

	// The promenade, and the rail between it and the sand.
	groundQuad(mathx.V(b.Min.X, b.Min.Z), mathx.V(b.Max.X, b.Min.Z),
		mathx.V(b.Max.X, edge), mathx.V(b.Min.X, edge), y, colSidewalk)
	for _, up := range [2]float32{0.55, 1.05} {
		rl.DrawCube(vec3(mid, ground+up, edge), width, 0.07, 0.07, colPole)
	}
	for x := b.Min.X; x <= b.Max.X; x += railPitch {
		rl.DrawCube(vec3(x, ground+0.55, edge), 0.09, 1.1, 0.09, colPole)
	}

	for _, hut := range beachHuts(b) {
		drawBeachHut(hut, ground)
	}

	// Sunshades down the sand, between the huts and the water.
	for x, i := b.Min.X+5, 0; x < b.Max.X-5; x, i = x+shadePitch, i+1 {
		h := scatter(x, b.Max.Z, 53)
		lo, hi := b.Min.Z+hutRow+7, b.Max.Z-6
		if h[0] < 0.35 || hi <= lo {
			continue
		}
		at := mathx.V(x+h[1]*4, lo+h[2]*(hi-lo))
		pillar(at, ground, ground+2, 0.04, 0.04, 6, colPole)
		pillar(at, ground+1.75, ground+2.25, 1.35, 0.05, 10, awningShades[i%len(awningShades)])
	}

	if x, ok := hasPier(b); ok {
		drawPier(x, edge, b.Max.Z+pierLength)
	}
}

func drawBeachHut(hut beachHut, ground float32) {
	const w, d, wall, rise = 2.5, 3.0, 2.1, 0.9
	shade := hutShades[hut.shade%len(hutShades)]
	rl.DrawCube(vec3(hut.at.X, ground+wall/2, hut.at.Z), w, wall, d, shade)
	// The door faces the sea.
	rl.DrawCube(vec3(hut.at.X, ground+0.95, hut.at.Z+d/2+0.02), 0.9, 1.9, 0.05, scaleColor(shade, 0.62))
	gable(hut.at, w*1.08, d*1.08, ground+wall, rise, scaleColor(shade, 0.8), shade)
}

// drawPier runs a pier out from the promenade at x, from the rail to its far
// end over the water, with a pavilion on the end of it.
func drawPier(x, from, to float32) {
	const deck = 1.5 // above the sand
	length := to - from
	mid := (from + to) / 2
	rl.DrawCube(vec3(x, deck, mid), pierWidth, 0.3, length, colWood)
	for z := from + 4; z < to; z += 9 {
		for _, side := range [2]float32{-1, 1} {
			rl.DrawCube(vec3(x+side*(pierWidth/2-0.5), deck/2-0.3, z), 0.4, deck+0.6, 0.4, scaleColor(colWood, 0.7))
		}
	}
	for _, side := range [2]float32{-1, 1} {
		rl.DrawCube(vec3(x+side*pierWidth/2, deck+1.1, mid), 0.07, 0.07, length, colPole)
	}
	// The pavilion.
	at := mathx.V(x, to-9)
	rl.DrawCube(vec3(at.X, deck+0.15+1.6, at.Z), pierWidth*0.8, 3.2, 12, seasideShades[5])
	rl.DrawCube(vec3(at.X, deck+1.8, at.Z), pierWidth*0.81, 1.3, 10, scaleColor(colGlass, 0.8))
	gable(at, pierWidth*0.9, 13, deck+3.35, 1.6, roofShades[1], seasideShades[5])
}
