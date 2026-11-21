package game

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// The dressing is what makes the city a place rather than a street plan: the
// roofs, the shop fronts, the things standing on the pavement, what is in the
// parks, and the cars left at the kerb. All of it is worked out from where it
// stands, so the same city is always dressed the same, and none of it is
// anything the autopilot is told about.

var (
	roofShades = [3]rl.Color{
		rl.NewColor(132, 84, 68, 255), // tile
		rl.NewColor(86, 90, 98, 255),  // slate
		rl.NewColor(112, 94, 80, 255), // weathered
	}
	awningShades = [6]rl.Color{
		rl.NewColor(150, 62, 58, 255),
		rl.NewColor(58, 98, 132, 255),
		rl.NewColor(74, 120, 88, 255),
		rl.NewColor(196, 160, 84, 255),
		rl.NewColor(120, 80, 120, 255),
		rl.NewColor(206, 200, 186, 255),
	}
	colShopLit   = rl.NewColor(240, 206, 140, 255)
	colPlant     = rl.NewColor(120, 124, 128, 255)
	colWood      = rl.NewColor(124, 96, 68, 255)
	colBin       = rl.NewColor(52, 62, 56, 255)
	colPillarBox = rl.NewColor(176, 46, 42, 255)
	colStone     = rl.NewColor(158, 154, 146, 255)
	colPath      = rl.NewColor(178, 168, 146, 255)
	colWater     = rl.NewColor(88, 126, 150, 255)
	colBronze    = rl.NewColor(84, 108, 98, 255)
)

// roofKind is what a building has on top.
type roofKind int

const (
	// roofFlat is a parapet with the plant that keeps the building running.
	roofFlat roofKind = iota
	// roofGable is a pitched roof, on the low buildings away from the centre.
	roofGable
	// roofStepped sets a smaller storey back from the edge.
	roofStepped
	// roofTower is the setback tower and mast of the tallest.
	roofTower
)

// Building heights that decide the roof, in metres.
const (
	towerHeight   float32 = 34
	gableHeight   float32 = 15
	steppedHeight float32 = 18
)

// roofOf decides a building's roof from its height, where it stands and how
// the place is built.
func roofOf(b city.Building, town townLook) roofKind {
	h := scatter(b.Center.X, b.Center.Z, 11)
	switch {
	case b.Height > towerHeight:
		return roofTower
	case b.Height < gableHeight && h[0] < town.pitched:
		return roofGable
	case b.Height >= steppedHeight && h[0] < 0.4:
		return roofStepped
	}
	return roofFlat
}

// drawBuilding draws one building: its walls, its roof and its shop front.
func drawBuilding(b city.Building, town townLook, night float32, snow bool) {
	shade := town.facades[int(b.Shade)%len(town.facades)]
	h := scatter(b.Center.X, b.Center.Z, 11)
	x, z, top := b.Center.X, b.Center.Z, b.Height

	// The walls are marked as a façade, which is what tells the shader to
	// put windows in them.
	rl.DrawCube(vec3(x, top/2, z), b.W, top, b.D, as(shade, matFacade))
	// A fascia over the shop fronts, which is where the signs would hang.
	rl.DrawCube(vec3(x, 3.5, z), b.W*1.012, 0.5, b.D*1.012, scaleColor(shade, 0.55))

	parapet := func() {
		rl.DrawCube(vec3(x, top+0.35, z), b.W*0.97, 0.7, b.D*0.97, scaleColor(shade, 0.74))
		if snow {
			rl.DrawCube(vec3(x, top+0.74, z), b.W*0.93, 0.1, b.D*0.93, colSnow)
		}
	}
	switch roofOf(b, town) {
	case roofGable:
		rise := 1.6 + h[1]*1.8
		rl.DrawCube(vec3(x, top+0.1, z), b.W*1.03, 0.2, b.D*1.03, scaleColor(shade, 0.7))
		tiles := roofShades[int(h[2]*3)%len(roofShades)]
		if snow {
			tiles = colSnow
		}
		gable(b.Center, b.W*1.03, b.D*1.03, top+0.2, rise, tiles, shade)
		// A chimney, off to one end.
		cx, cz := x+b.W*0.28, z
		if b.D > b.W {
			cx, cz = x, z+b.D*0.28
		}
		rl.DrawCube(vec3(cx, top+rise*0.5+0.6, cz), 0.7, rise+1.2, 0.7, scaleColor(shade, 0.62))
	case roofStepped:
		parapet()
		rl.DrawCube(vec3(x, top+1.7, z), b.W*0.7, 3.4, b.D*0.7, as(scaleColor(shade, 0.92), matFacade))
		rl.DrawCube(vec3(x, top+3.5, z), b.W*0.72, 0.3, b.D*0.72, scaleColor(shade, 0.7))
	case roofTower:
		parapet()
		// A setback tower so the skyline is not all boxes, and a mast on it
		// with a light for the aircraft.
		rl.DrawCube(vec3(x, top+4.5, z), b.W*0.55, 8, b.D*0.55, as(scaleColor(shade, 0.88), matFacade))
		rl.DrawCube(vec3(x, top+11.5, z), 0.24, 6, 0.24, colPole)
		beacon := scaleColor(bulbRed, 0.5)
		if night > 0.3 {
			beacon = as(bulbRed, matGlow)
		}
		rl.DrawCube(vec3(x, top+14.6, z), 0.4, 0.4, 0.4, beacon)
	default:
		parapet()
		// The plant: an air handler, and on some a water tank.
		rl.DrawCube(vec3(x+b.W*(h[1]-0.5)*0.4, top+0.95, z+b.D*(h[2]-0.5)*0.4), 2.4, 1.3, 1.8, colPlant)
		if h[0] > 0.5 {
			at := mathx.V(x-b.W*(h[1]-0.5)*0.4, z-b.D*(h[2]-0.5)*0.4)
			pillar(at, top+0.7, top+2.5, 0.9, 0.9, 10, scaleColor(colPlant, 0.8))
		}
	}
	drawFront(b, town, shade, h, night)
}

// gable draws a pitched roof over a w by d rectangle centred on c: two slopes
// rising from height y to a ridge along the longer side, and a triangle of
// wall closing each end.
func gable(c mathx.Vec, w, d, y, rise float32, roof, wall rl.Color) {
	// along is half the ridge, across half the span the slopes cover.
	along, across, ridgeX := w/2, d/2, true
	if d > w {
		along, across, ridgeX = d/2, w/2, false
	}
	// at places a point given along and across the ridge.
	at := func(al, ac, up float32) (float32, float32, float32) {
		if ridgeX {
			return c.X + al, y + up, c.Z + ac
		}
		return c.X + ac, y + up, c.Z + al
	}
	vertex := func(al, ac, up float32) { rl.Vertex3f(at(al, ac, up)) }
	normal := func(al, ac, up float32) {
		if ridgeX {
			rl.Normal3f(al, up, ac)
			return
		}
		rl.Normal3f(ac, up, al)
	}
	slant := sqrt32(across*across + rise*rise)

	rl.Begin(rl.Triangles)
	tint(roof)
	for _, side := range [2]float32{-1, 1} {
		normal(0, side*rise/slant, across/slant)
		vertex(-along, side*across, 0)
		vertex(along, side*across, 0)
		vertex(along, 0, rise)
		vertex(-along, side*across, 0)
		vertex(along, 0, rise)
		vertex(-along, 0, rise)
	}
	tint(wall)
	for _, end := range [2]float32{-1, 1} {
		normal(end, 0, 0)
		vertex(end*along, -across, 0)
		vertex(end*along, across, 0)
		vertex(end*along, 0, rise)
	}
	rl.End()
}

// shopHasAwning and shopLitAtNight decide, from a building's numbers, the two
// things that vary about its shop front.
func shopHasAwning(h [3]float32) bool  { return h[1] < 0.55 }
func shopLitAtNight(h [3]float32) bool { return h[2] < 0.65 }

// isShop decides whether a building is a shop or somebody's house, from
// where it stands and how much of the place is shops.
func isShop(b city.Building, town townLook) bool {
	return scatter(b.Center.X, b.Center.Z, 17)[0] < town.shops
}

// drawFront dresses the side of a building that faces its street. A shop has
// its ground floor glazed, and an awning over some; a house has a front door.
func drawFront(b city.Building, town townLook, shade rl.Color, h [3]float32, night float32) {
	f := b.Front
	if f.X == 0 && f.Z == 0 {
		return
	}
	// face is how wide the front is, and reach how far it is from the centre.
	face, reach := b.W, b.D/2
	if f.X != 0 {
		face, reach = b.D, b.W/2
	}
	at := b.Center.Add(f.Mul(reach))
	// slab stands a flat piece against the front: out from the wall, at a
	// height, so thick, so tall and so wide.
	slab := func(out, y, thick, tall, wide float32, col rl.Color) {
		p := at.Add(f.Mul(out))
		if f.X != 0 {
			rl.DrawCube(vec3(p.X, y, p.Z), thick, tall, wide, col)
			return
		}
		rl.DrawCube(vec3(p.X, y, p.Z), wide, tall, thick, col)
	}

	if !isShop(b, town) {
		slab(0.03, 1.05, 0.08, 2.1, 1.0, scaleColor(shade, 0.5))
		slab(0.3, 0.09, 0.6, 0.18, 1.5, colStone)
		return
	}

	glass := scaleColor(colGlass, 0.7)
	if night > 0.35 && shopLitAtNight(h) {
		glass = as(colShopLit, matGlow)
	}
	slab(0.03, 1.8, 0.1, 2.4, face*0.84, glass)
	slab(0.05, 0.3, 0.14, 0.6, face*0.86, scaleColor(shade, 0.6))
	if shopHasAwning(h) {
		cloth := awningShades[int(h[0]*97)%len(awningShades)]
		slab(0.7, 3.12, 1.4, 0.1, face*0.8, cloth)
		slab(1.38, 2.97, 0.05, 0.3, face*0.8, scaleColor(cloth, 0.8))
	}
}

// furnitureKind is one of the things that stand on a pavement.
type furnitureKind int

const (
	furnBench furnitureKind = iota
	furnBin
	furnPostBox
	furnPhoneBox
	furnPlanter
	furnShelter
	numFurniture
)

// furnishing is one piece of street furniture: what it is, where it stands,
// and which way the road beside it runs. out points away from the road.
type furnishing struct {
	kind       furnitureKind
	at         mathx.Vec
	along, out mathx.Vec
}

// Street furniture spacing, in metres.
const (
	// furnSetback is how far in from the kerb furniture stands: behind the
	// trees and the parking bays, in front of the shops.
	furnSetback float32 = 3.1
	// furnMargin keeps it clear of the junctions, and furnPitch is how often
	// along a road a piece may stand.
	furnMargin float32 = 16
	furnPitch  float32 = 11
	// furnShare is how many of those places have something.
	furnShare float32 = 0.4
)

// furnitureFor picks what stands at a place from its number. A bus shelter
// only goes where a bus would run.
func furnitureFor(pick float32, class city.RoadClass) furnitureKind {
	switch {
	case pick < 0.26:
		return furnBench
	case pick < 0.46:
		return furnBin
	case pick < 0.58:
		return furnPostBox
	case pick < 0.68:
		return furnPhoneBox
	case pick < 0.86:
		return furnPlanter
	case class == city.Local:
		return furnBench
	}
	return furnShelter
}

// furnish returns the furniture along a road. It depends only on where the
// road is.
func furnish(c *city.City, r *city.Road) []furnishing {
	if r.Ring {
		return nil
	}
	na, nb := c.Nodes[r.A], c.Nodes[r.B]
	var out []furnishing
	for d := na.Radius + furnMargin; d < r.Length-nb.Radius-furnMargin; d += furnPitch {
		for _, side := range [2]float32{-1, 1} {
			away := r.Dir.Right().Mul(side)
			at := na.Pos.Add(r.Dir.Mul(d)).Add(away.Mul(r.HalfWidth + furnSetback))
			h := scatter(at.X, at.Z, 23)
			if h[0] >= furnShare {
				continue
			}
			out = append(out, furnishing{
				kind: furnitureFor(h[1], r.Class), at: at, along: r.Dir, out: away,
			})
		}
	}
	return out
}

// drawFurniture draws the street furniture near the eye, working out what
// stands along any road that has appeared since it last looked.
func (g *Game) drawFurniture(eye mathx.Vec) {
	c := g.world.City
	for ; g.furnishedRoads < len(c.Roads); g.furnishedRoads++ {
		g.furniture = append(g.furniture, furnish(c, c.Roads[g.furnishedRoads])...)
	}
	for _, f := range g.furniture {
		if f.at.DistTo(eye) > cullProps {
			continue
		}
		drawFurnishing(f)
	}
}

func drawFurnishing(f furnishing) {
	ground := city.KerbHeight
	// box stands a block on the pavement: a metres along the road and o out
	// from the piece's own spot, its foot at y, so long, so tall and so deep.
	box := func(a, o, y, long, tall, deep float32, col rl.Color) {
		p := f.at.Add(f.along.Mul(a)).Add(f.out.Mul(o))
		if f.along.X != 0 {
			rl.DrawCube(vec3(p.X, ground+y+tall/2, p.Z), long, tall, deep, col)
			return
		}
		rl.DrawCube(vec3(p.X, ground+y+tall/2, p.Z), deep, tall, long, col)
	}
	switch f.kind {
	case furnBench:
		// It faces the road, with its back to the shops.
		box(0, 0, 0.42, 1.7, 0.08, 0.5, colWood)
		box(0, 0.24, 0.5, 1.7, 0.45, 0.07, colWood)
		for _, a := range [2]float32{-0.7, 0.7} {
			box(a, 0, 0, 0.08, 0.42, 0.46, colTrim)
		}
	case furnBin:
		box(0, 0, 0, 0.52, 0.95, 0.52, colBin)
		box(0, 0, 0.95, 0.58, 0.07, 0.58, colTrim)
	case furnPostBox:
		pillar(f.at, ground, ground+1.35, 0.27, 0.27, 12, colPillarBox)
		pillar(f.at, ground+1.35, ground+1.5, 0.31, 0.2, 12, scaleColor(colPillarBox, 0.8))
		pillar(f.at, ground, ground+0.14, 0.3, 0.3, 12, colTrim)
	case furnPhoneBox:
		box(0, 0, 0, 0.92, 2.4, 0.92, colPillarBox)
		box(0, 0, 0.45, 0.95, 1.55, 0.7, scaleColor(colGlass, 0.8))
		box(0, 0, 0.45, 0.7, 1.55, 0.95, scaleColor(colGlass, 0.8))
		box(0, 0, 2.4, 0.86, 0.12, 0.86, scaleColor(colPillarBox, 0.8))
	case furnPlanter:
		box(0, 0, 0, 1.6, 0.55, 0.7, colStone)
		box(0, 0, 0.55, 1.4, 0.4, 0.5, colEvergreen)
	case furnShelter:
		// A glass back, a post at each end, and a roof out over the pavement.
		box(0, 0.55, 0.15, 3.2, 2.05, 0.06, scaleColor(colGlass, 0.9))
		for _, a := range [2]float32{-1.6, 1.6} {
			box(a, 0.55, 0, 0.1, 2.3, 0.1, colPole)
			box(a, -0.6, 0, 0.1, 2.3, 0.1, colPole)
		}
		box(0, 0, 2.3, 3.5, 0.1, 1.5, colTrim)
		box(0, 0.3, 0.42, 2.4, 0.07, 0.4, colWood)
	}
}

// parkKind is what stands in the middle of a park.
type parkKind int

const (
	parkFountain parkKind = iota
	parkStatue
	parkBandstand
	numParkKinds
)

// Park layout, in metres.
const (
	pathWidth float32 = 2.6
	// parkCircle is the radius of the paved circle where the paths cross.
	parkCircle float32 = 6
	// parkRoom is the smallest a park can be and still have paths laid
	// through it.
	parkRoom float32 = 30
)

// parkFeature returns what is in the middle of a park and where the middle
// is. A park too small for paths has nothing.
func parkFeature(b city.Block) (kind parkKind, centre mathx.Vec, ok bool) {
	size := b.Max.Sub(b.Min)
	if !b.Park || size.X < parkRoom || size.Z < parkRoom {
		return 0, mathx.Vec{}, false
	}
	centre = b.Min.Lerp(b.Max, 0.5)
	h := scatter(centre.X, centre.Z, 31)
	return parkKind(int(h[0]*float32(numParkKinds)) % int(numParkKinds)), centre, true
}

// onParkPath reports whether a point in a park is on one of its paths or the
// circle where they meet, which is where a tree must not be planted.
func onParkPath(b city.Block, p mathx.Vec) bool {
	_, c, ok := parkFeature(b)
	if !ok {
		return false
	}
	clear := pathWidth/2 + 1.6
	return mathx.Abs(p.X-c.X) < clear || mathx.Abs(p.Z-c.Z) < clear || p.DistTo(c) < parkCircle+2
}

// drawPark lays a park's paths and stands its centrepiece.
func drawPark(b city.Block) {
	kind, c, ok := parkFeature(b)
	if !ok {
		return
	}
	y := city.KerbHeight + 0.012
	half := pathWidth / 2
	// The paths stop short of the kerb, whose corners are rounded.
	const inset = 7
	groundQuad(mathx.V(b.Min.X+inset, c.Z-half), mathx.V(b.Max.X-inset, c.Z-half),
		mathx.V(b.Max.X-inset, c.Z+half), mathx.V(b.Min.X+inset, c.Z+half), y, colPath)
	groundQuad(mathx.V(c.X-half, b.Min.Z+inset), mathx.V(c.X+half, b.Min.Z+inset),
		mathx.V(c.X+half, b.Max.Z-inset), mathx.V(c.X-half, b.Max.Z-inset), y, colPath)
	pillar(c, y, y+0.01, parkCircle, parkCircle, 28, colPath)

	ground := city.KerbHeight
	switch kind {
	case parkFountain:
		pillar(c, ground, ground+0.55, 3.2, 3.2, 20, colStone)
		pillar(c, ground+0.55, ground+0.57, 2.9, 2.9, 20, colWater)
		pillar(c, ground, ground+1.5, 0.45, 0.3, 10, colStone)
		pillar(c, ground+1.5, ground+1.62, 1.1, 1.1, 14, colStone)
		pillar(c, ground+1.62, ground+1.64, 0.95, 0.95, 14, colWater)
	case parkStatue:
		rl.DrawCube(vec3(c.X, ground+0.9, c.Z), 1.5, 1.8, 1.5, colStone)
		pillar(c, ground+1.8, ground+3.4, 0.38, 0.26, 8, colBronze)
		pillar(c, ground+3.4, ground+3.85, 0.24, 0.2, 8, colBronze)
	default:
		// A bandstand: a low platform, a ring of posts and a roof to a point.
		pillar(c, ground, ground+0.6, 3.6, 3.6, 8, colStone)
		for i := range 8 {
			post := c.Add(mathx.FromAngle(float32(i) * math.Pi / 4).Mul(3.2))
			rl.DrawCube(vec3(post.X, ground+1.9, post.Z), 0.14, 2.6, 0.14, colTrim)
		}
		pillar(c, ground+3.2, ground+4.6, 4.0, 0.2, 8, roofShades[1])
	}
}

// parkedCar is a car standing in a bay, ready to be drawn.
type parkedCar struct {
	v     *sim.Vehicle
	body  sim.Body
	paint rl.Color
}

// bayLine is the width of the paint marking out a bay, in metres.
const bayLine float32 = 0.12

// drawParking draws the bays beside the quieter streets and the cars in
// them, taking in whatever has been built since it last looked.
func (g *Game) drawParking(eye mathx.Vec) {
	c := g.world.City
	for ; g.bayedRoads < len(c.Roads); g.bayedRoads++ {
		g.bays = append(g.bays, c.Bays(c.Roads[g.bayedRoads])...)
	}
	for i := len(g.parked); i < len(c.Parked); i++ {
		p := c.Parked[i]
		body := sim.ParkedBody(p)
		g.parked = append(g.parked, parkedCar{
			v: sim.NewVehicle(body.Spec, p.Pos, p.Heading), body: body,
			paint: carPaints[int(p.Paint)%len(carPaints)],
		})
	}

	y := city.KerbHeight + 0.012
	for _, b := range g.bays {
		if b.Start.DistTo(eye) > cullMarkings {
			continue
		}
		length := city.BayLength * city.BaySlots
		far, in := b.Start.Add(b.Dir.Mul(length)), b.Out.Mul(city.BayDepth)
		groundQuad(b.Start, far, far.Add(in), b.Start.Add(in), y, as(colAsphalt, matRoad))
		// A line across each end, and one between the spaces.
		for i := range city.BaySlots + 1 {
			a := b.Start.Add(b.Dir.Mul(city.BayLength*float32(i) - bayLine/2))
			q := a.Add(b.Dir.Mul(bayLine))
			groundQuad(a, q, q.Add(in), a.Add(in), y+0.006, colLinePale)
		}
	}

	rl.PushMatrix()
	rl.Translatef(0, city.KerbHeight, 0)
	for _, p := range g.parked {
		d := p.v.Pos.DistTo(eye)
		if d > cullTrees {
			continue
		}
		drawCar(p.v, p.body, carLook{paint: p.paint, wheels: d < cullWheels, night: g.light.night, parked: true})
	}
	rl.PopMatrix()
}

// inBay reports whether a spot beside a road, d metres along it on the given
// side, falls within one of its parking bays.
func inBay(bays []city.Bay, d, side float32) bool {
	for _, b := range bays {
		if b.Side == side && d > b.Along-2 && d < b.Along+city.BayLength*city.BaySlots+2 {
			return true
		}
	}
	return false
}
