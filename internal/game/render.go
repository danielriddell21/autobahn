package game

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/paint"

	"github.com/danielriddell21/autobahn/internal/annotate"
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/vision"
)

// The world palette. Every colour here is deliberately muted: the vision
// system reserves the fully saturated combinations of 0, 128 and 255 for its
// annotation boxes, so nothing in the scene can be mistaken for a detection.
var (
	colSky      = rl.NewColor(139, 174, 206, 255)
	colAsphalt  = rl.NewColor(62, 62, 68, 255)
	colSidewalk = rl.NewColor(150, 147, 140, 255)
	colKerb     = rl.NewColor(176, 172, 164, 255)
	colLinePale = rl.NewColor(222, 220, 212, 255)
	// Yellow paint in Britain means a parking restriction at the kerb, never a
	// lane divider, so it only ever appears as the double lines by the pavement.
	colLineWarm = rl.NewColor(204, 176, 78, 255)
	colPole     = rl.NewColor(94, 96, 99, 255)
	colSignBack = rl.NewColor(64, 66, 70, 255)
	colStopFace = rl.NewColor(186, 54, 50, 255)
	colSignFace = rl.NewColor(230, 228, 220, 255)
	colGlass    = rl.NewColor(82, 102, 122, 255)
	colTyre     = rl.NewColor(34, 34, 37, 255)
	colHub      = rl.NewColor(132, 134, 140, 255)
	colTrim     = rl.NewColor(44, 46, 50, 255)
	colLampHead = rl.NewColor(206, 200, 172, 255)
	colLampLit  = rl.NewColor(246, 222, 164, 255)
	colTrunk    = rl.NewColor(92, 74, 58, 255)
	colSkid     = rl.NewColor(18, 18, 20, 120)
	colShadow   = rl.NewColor(0, 0, 0, 86)

	buildingShades = [6]rl.Color{
		rl.NewColor(168, 160, 146, 255),
		rl.NewColor(136, 140, 146, 255),
		rl.NewColor(184, 164, 142, 255),
		rl.NewColor(122, 130, 140, 255),
		rl.NewColor(156, 138, 124, 255),
		rl.NewColor(108, 114, 124, 255),
	}

	carPaints = [8]rl.Color{
		rl.NewColor(186, 62, 58, 255),
		rl.NewColor(58, 92, 152, 255),
		rl.NewColor(212, 208, 198, 255),
		rl.NewColor(52, 56, 62, 255),
		rl.NewColor(196, 152, 60, 255),
		rl.NewColor(74, 128, 104, 255),
		rl.NewColor(148, 150, 156, 255),
		rl.NewColor(126, 74, 132, 255),
	}

	// Police livery: a white car with the reflective blue-and-yellow blocks of
	// British battenburg markings, and a light bar that runs blues and reds.
	colPoliceBody   = rl.NewColor(232, 232, 236, 255)
	colPoliceDark   = rl.NewColor(38, 44, 60, 255)
	colPoliceBlue   = rl.NewColor(36, 76, 168, 255)
	colPoliceYellow = rl.NewColor(226, 198, 62, 255)
	colBeaconBlue   = rl.NewColor(64, 110, 245, 255)
	colBeaconRed    = rl.NewColor(238, 66, 58, 255)
	colBeaconDark   = rl.NewColor(52, 54, 62, 255)

	bulbRed   = rl.NewColor(232, 58, 48, 255)
	bulbAmber = rl.NewColor(232, 158, 44, 255)
	bulbGreen = rl.NewColor(72, 196, 96, 255)
	bulbDark  = rl.NewColor(46, 44, 44, 255)
)

// Draw distances in metres.
const (
	cullBuildings float32 = 330
	cullBlocks    float32 = 350
	cullMarkings  float32 = 170
	cullDashes    float32 = 120
	cullProps     float32 = 150
	cullCrossing  float32 = 100
	cullTrees     float32 = 190
	cullWheels    float32 = 90
	cullPools     float32 = 140
)

func vec3(x, y, z float32) rl.Vector3 { return rl.NewVector3(x, y, z) }

// rad2deg converts the simulation's radians to the degrees raylib's matrix
// helpers expect.
const rad2deg = rl.Rad2deg

func (g *Game) drawWorld(cam rl.Camera3D, drawPlayer bool) {
	// Renders the whole scene from the given camera. When drawPlayer is
	// false the player's own car is omitted, which is what the bonnet-mounted AI
	// camera wants.
	eye := mathx.V(cam.Position.X, cam.Position.Z)
	// Fog is measured from the eye, so a view from a long way up pushes it
	// back by as much, or the plan it exists to show would be a blank haze.
	g.scene.use(g.light, cam.Position, max(g.cullRange()/cullBlocks, cam.Position.Y/90))

	rl.BeginMode3D(cam)
	// Everything here is a closed shape or lies flat on the ground, so nothing
	// is gained by culling back faces, and the hand-built pieces need not all
	// agree about which way round they are wound.
	rl.DisableBackfaceCulling()
	if g.scene.ok {
		rl.BeginShaderMode(g.scene.shader)
	}

	g.drawGround(eye)
	g.drawBlocks(eye)
	g.drawRoundabouts(eye)
	g.drawMarkings(eye)
	g.drawSkids(eye)
	g.drawBuildings(eye)
	g.drawTrees(eye)
	g.drawProps(eye)
	g.drawFurniture(eye)
	g.drawBarriers(eye)
	g.drawParking(eye)
	g.drawTraffic(eye, drawPlayer)
	g.drawLightPools(eye, drawPlayer)
	g.drawFall(eye)
	g.drawSmoke()

	if g.scene.ok {
		rl.EndShaderMode()
	}
	rl.EnableBackfaceCulling()
	rl.EndMode3D()
}

// cullRange is how far from the eye scenery is still drawn. From the car it is
// the fixed distance the scene was tuned for; from a plan view it grows with
// the ground being shown, or the layout the view exists to display would be
// culled away and leave nothing but the asphalt slab.
func (g *Game) cullRange() float32 {
	if g.camMode != CameraHigh {
		return cullBlocks
	}
	return max(cullBlocks, g.overheadHeight*2.2)
}

// groundEdge is how far past the outermost junction the asphalt reaches, in
// metres. It is enough to carry the pavements at the city's edge and no more,
// so the town ends in countryside rather than in a bare grey apron.
const groundEdge float32 = 24

func (g *Game) drawGround(eye mathx.Vec) {
	// A grass plane under everything, then an asphalt slab over the built-up
	// ground. The roads are simply the parts of the slab that the pavements do
	// not cover.
	//
	// Both are cut around the eye rather than around the city, because a city
	// that grows as it is driven has no settled extent. Its bounding box is
	// also not the shape of what exists: drive east and then north, and the box
	// covers a whole quadrant nobody has been to. Paving that box would turn
	// the quadrant into a featureless grey plain.
	rl.DrawPlane(vec3(eye.X, -0.06, eye.Z), rl.NewVector2(6000, 6000), g.look.ground)

	c := g.world.City
	// Never reach beyond the radius the city is kept built to, or the slab
	// would run out over ground that has not been generated yet.
	r := min(g.cullRange()+groundEdge, sim.GrowRadius)
	loX := max(eye.X-r, c.Min.X-groundEdge)
	loZ := max(eye.Z-r, c.Min.Z-groundEdge)
	hiX := min(eye.X+r, c.Max.X+groundEdge)
	hiZ := min(eye.Z+r, c.Max.Z+groundEdge)
	if _, sea, ok := c.Shoreline(); ok {
		// The sea, from the foot of the beach out past anything in view, and
		// no road surface under it.
		rl.DrawPlane(vec3(eye.X, -0.035, sea+seaReach/2), rl.NewVector2(seaReach, seaReach), colSea)
		hiZ = min(hiZ, sea)
	}
	if hiX <= loX || hiZ <= loZ {
		return
	}
	rl.DrawPlane(vec3((loX+hiX)/2, -0.02, (loZ+hiZ)/2),
		rl.NewVector2(hiX-loX, hiZ-loZ), as(colAsphalt, matRoad))
}

// outline returns the kerb line of a block, worked out once and kept: the
// blocks never change, and their corners are curves.
func (g *Game) outline(i int) []mathx.Vec {
	for len(g.outlines) <= i {
		g.outlines = append(g.outlines, g.world.City.Blocks[len(g.outlines)].Outline())
	}
	return g.outlines[i]
}

func (g *Game) drawBlocks(eye mathx.Vec) {
	for i, b := range g.world.City.Blocks {
		centre := b.Min.Lerp(b.Max, 0.5)
		if centre.DistTo(eye) > g.cullRange()+b.Max.Sub(b.Min).Len()/2 {
			continue
		}
		// The pavement slab, raised to make a kerb, with its corners rounded
		// off for the junctions they face.
		kerb := g.outline(i)
		surface := g.look.pave
		if g.town.verges {
			// Grass to the kerb, and round the houses.
			surface = g.look.park
		}
		field := fieldOf(b, g.opts.Map)
		switch {
		case b.Park:
			surface = fieldShade(field, g.look)
		case b.Beach:
			surface = colSand
		}
		groundWall(centre, kerb, -0.02, city.KerbHeight, colKerb)
		groundFan(centre, kerb, city.KerbHeight, surface)
		switch {
		case field != fieldNone:
			drawField(b, field)
		case b.Beach:
			drawBeach(b)
		default:
			drawPark(b)
		}
	}
}

func (g *Game) drawRoundabouts(eye mathx.Vec) {
	// Puts the island in the middle of each roundabout. The blocks around it
	// are already scooped out to its outer kerb, so the carriageway is simply
	// what is left between them and this.
	for _, n := range g.world.City.Roundabouts() {
		if n.Pos.DistTo(eye) > g.cullRange() {
			continue
		}
		island := n.RingInner()
		if island < 2 {
			continue
		}
		top := city.KerbHeight
		pillar(n.Pos, -0.02, top, island, island, 32, colKerb)
		pillar(n.Pos, top, top+0.02, island-0.5, island-0.5, 32, g.look.park)
		// A low mound so it reads as an island from the driver's seat.
		pillar(n.Pos, top, top+1.0, island*0.6, island*0.25, 20, g.look.park)
		g.look.drawTree(tree{at: n.Pos, height: 5.2, spread: 2.1, shade: n.ID}, top+0.9)
	}
}

func (g *Game) drawMarkings(eye mathx.Vec) {
	c := g.world.City
	for _, r := range c.Roads {
		if r.Ring {
			continue // a circle has no centre line to paint
		}
		mid := c.Nodes[r.A].Pos.Lerp(c.Nodes[r.B].Pos, 0.5)
		if mid.DistTo(eye) > cullMarkings+r.Length/2 {
			continue
		}
		a := c.Nodes[r.A].Pos.Add(r.Dir.Mul(c.Nodes[r.A].Radius))
		b := c.Nodes[r.B].Pos.Sub(r.Dir.Mul(c.Nodes[r.B].Radius))
		length := b.Sub(a).Dot(r.Dir)
		if length <= 1 {
			continue
		}
		drawRoadPaint(r, a, b, length, mid.DistTo(eye))
	}

	// Stop lines and pedestrian crossings on controlled approaches.
	for _, l := range c.Lanes {
		if l.Control == city.ControlNone || l.B.DistTo(eye) > cullMarkings {
			continue
		}
		drawJunctionPaint(l, c.Roads[l.Road].Axis, l.B.DistTo(eye))
	}
}

func drawRoadPaint(r *city.Road, a, b mathx.Vec, length, eyeDist float32) {
	centre := a.Lerp(b, 0.5)
	drawCentreLine(r, a, centre, length, eyeDist)

	// Double yellow lines against both kerbs: no waiting at any time.
	for _, s := range [2]float32{-1, 1} {
		for _, off := range [2]float32{0.26, 0.46} {
			p := centre.Add(r.Dir.Right().Mul(s * (r.HalfWidth - off)))
			drawStripe(p, r.Axis, length, 0.1, colLineWarm)
		}
	}

	// Dashed dividers between same-direction lanes on multi-lane roads.
	if r.Class.Lanes() > 1 && eyeDist < cullDashes {
		for _, s := range [2]float32{-1, 1} {
			off := s * city.LaneWidth
			for t := float32(2); t < length-2; t += 6.5 {
				p := a.Add(r.Dir.Mul(t + 1.6)).Add(r.Dir.Right().Mul(off))
				drawStripe(p, r.Axis, 3.2, 0.14, colLinePale)
			}
		}
	}
}

func drawCentreLine(r *city.Road, a, centre mathx.Vec, length, eyeDist float32) {
	// The centre line dividing opposing traffic is white here: solid and
	// doubled on the main roads, a long broken line elsewhere.
	if r.Class == city.Arterial {
		for _, off := range [2]float32{-0.18, 0.18} {
			p := centre.Add(r.Dir.Right().Mul(off))
			drawStripe(p, r.Axis, length, 0.14, colLinePale)
		}
		return
	}
	if eyeDist >= cullDashes {
		return
	}
	for t := float32(2); t < length-2; t += 9 {
		p := a.Add(r.Dir.Mul(t + 2))
		drawStripe(p, r.Axis, 4, 0.14, colLinePale)
	}
}

func drawJunctionPaint(l *city.Lane, axis int, eyeDist float32) {
	bar := l.B.Sub(l.Fwd.Mul(0.5))
	if l.Control == city.ControlGiveWay {
		// A give way line is broken, and there are two of them.
		for _, back := range [2]float32{0.0, 0.55} {
			p := bar.Sub(l.Fwd.Mul(back))
			for i := range 4 {
				off := (float32(i) - 1.5) * 0.85
				drawStripe(p.Add(l.Fwd.Right().Mul(off)), 1-axis, 0.5, 0.28, colLinePale)
			}
		}
	} else {
		drawStripe(bar, 1-axis, city.LaneWidth-0.3, 0.5, colLinePale)
	}

	if l.Index != 0 || eyeDist >= cullCrossing || l.Control != city.ControlSignal {
		return
	}
	// Zebra stripes just beyond the stop line, running with the direction of
	// travel and spaced across the lane.
	base := l.B.Add(l.Fwd.Mul(1.7))
	for i := range 5 {
		off := (float32(i) - 2) * 0.8
		drawStripe(base.Add(l.Fwd.Right().Mul(off)), axis, 2.6, 0.4, colLinePale)
	}
}

func drawStripe(p mathx.Vec, axis int, length, width float32, col rl.Color) {
	// Road paint sits just above the carriageway so it does not z-fight with it.
	const y = 0.015
	// Paints a flat rectangle on the road. axis 0 runs along X, axis 1
	// along Z; length follows the axis and width crosses it.
	if axis == 0 {
		rl.DrawCube(vec3(p.X, y, p.Z), length, 0.02, width, col)
		return
	}
	rl.DrawCube(vec3(p.X, y, p.Z), width, 0.02, length, col)
}

func (g *Game) drawBuildings(eye mathx.Vec) {
	for _, b := range g.world.City.Buildings {
		if b.Center.DistTo(eye) > g.cullRange() {
			continue
		}
		drawBuilding(b, g.town, g.light.night, g.look.snow)
	}
}

func scaleColor(c rl.Color, f float32) rl.Color {
	// Dims a colour, delegating the arithmetic to crucible's paint
	// package so brightness scaling matches the rest of the family.
	v := paint.Scale(c, float64(f))
	return rl.NewColor(v.R, v.G, v.B, v.A)
}

func (g *Game) drawProps(eye mathx.Vec) {
	for _, p := range g.world.City.Props {
		if p.Pos.DistTo(eye) > cullProps {
			continue
		}
		switch p.Kind {
		case city.PropTrafficLight:
			g.drawTrafficLight(p)
		case city.PropStopSign:
			drawStopSign(p)
		case city.PropGiveWaySign:
			drawGiveWaySign(p)
		case city.PropSpeedSign:
			drawSpeedSign(p)
		case city.PropStreetLamp:
			g.drawLamp(p)
		}
	}
}

// post stands a square pole at a prop's position.
func post(p city.Prop, height, thick float32) {
	rl.DrawCube(vec3(p.Pos.X, height/2, p.Pos.Z), thick, height, thick, colPole)
}

// facing pushes a matrix that puts a prop's face at the origin: +X is the way
// the traffic it governs is travelling, so its face looks back along -X.
func facing(p city.Prop) {
	rl.PushMatrix()
	rl.Translatef(p.Pos.X, 0, p.Pos.Z)
	rl.Rotatef(-p.Heading*rad2deg, 0, 1, 0)
}

func (g *Game) drawTrafficLight(p city.Prop) {
	state := g.world.Signals.State(p.Node, p.Group)
	post(p, p.Height-0.9, 0.14)

	facing(p)
	head := p.Height - 0.35
	rl.DrawCube(vec3(0, head, 0), 0.24, 1.08, 0.38, colSignBack)
	// A backing board, which is what makes the head readable against the sky.
	rl.DrawCube(vec3(0.02, head, 0), 0.05, 1.3, 0.62, scaleColor(colSignBack, 0.7))
	// Three bulbs, only the active ones lit.
	bulbs := [3]rl.Color{bulbDark, bulbDark, bulbDark}
	switch state {
	case sim.SignalGreen:
		bulbs[2] = as(bulbGreen, matGlow)
	case sim.SignalAmber:
		bulbs[1] = as(bulbAmber, matGlow)
	case sim.SignalRedAmber:
		bulbs[0], bulbs[1] = as(bulbRed, matGlow), as(bulbAmber, matGlow)
	default:
		bulbs[0] = as(bulbRed, matGlow)
	}
	for i, col := range bulbs {
		rl.DrawCube(vec3(-0.13, head+0.32-float32(i)*0.32, 0), 0.06, 0.24, 0.24, col)
	}
	rl.PopMatrix()
}

func drawStopSign(p city.Prop) {
	const size float32 = 0.42
	post(p, p.Height-size, 0.1)
	facing(p)
	// An octagon, near enough: a square with a second square turned on it.
	rl.DrawCube(vec3(-0.06, p.Height-size, 0), 0.06, size*2, size*2, colStopFace)
	rl.DrawCube(vec3(-0.07, p.Height-size, 0), 0.06, size*2.3, size*1.2, colStopFace)
	rl.DrawCube(vec3(-0.07, p.Height-size, 0), 0.06, size*1.2, size*2.3, colStopFace)
	rl.DrawCube(vec3(-0.1, p.Height-size, 0), 0.03, size*0.34, size*1.5, colSignFace)
	rl.PopMatrix()
}

func drawSpeedSign(p city.Prop) {
	const size float32 = 0.4
	post(p, p.Height-size, 0.1)
	facing(p)
	// The roundel: a red ring round a white face.
	rl.DrawCube(vec3(-0.06, p.Height-size, 0), 0.05, size*2, size*2, colStopFace)
	rl.DrawCube(vec3(-0.08, p.Height-size, 0), 0.05, size*1.5, size*1.5, colSignFace)
	rl.PopMatrix()
}

func drawGiveWaySign(p city.Prop) {
	// Renders the British inverted triangle: a white face with a
	// red border, pointing down.
	const size float32 = 0.5
	post(p, p.Height-size, 0.1)
	facing(p)
	// Three tapering slabs approximate the downward triangle.
	for i := range 3 {
		f := 1 - float32(i)*0.32
		rl.DrawCube(vec3(-0.06, p.Height-size*float32(i)*0.34, 0),
			0.05, size*0.36, size*2*f, colStopFace)
		rl.DrawCube(vec3(-0.09, p.Height-size*float32(i)*0.34, 0),
			0.04, size*0.24, size*1.6*f, colSignFace)
	}
	rl.PopMatrix()
}

// lampReach is how far out over the road a street lamp's head hangs, in
// metres. The pool of light it throws is centred under it.
const lampReach float32 = 2.1

func (g *Game) drawLamp(p city.Prop) {
	post(p, p.Height, 0.16)
	facing(p)
	rl.DrawCube(vec3(0, p.Height, -lampReach/2), 0.1, 0.1, lampReach, colPole)
	head := colLampHead
	if g.light.night > 0.25 {
		head = as(colLampLit, matGlow)
	}
	rl.DrawCube(vec3(0, p.Height-0.12, -lampReach), 0.42, 0.14, 0.8, head)
	rl.PopMatrix()
}

// tree is one tree: where it stands and how big it is.
type tree struct {
	at     mathx.Vec
	height float32
	spread float32
	shade  int
}

// Tree planting, in metres.
const (
	treeSpacing float32 = 21  // along a street
	treeSetback float32 = 1.9 // in from the kerb
	parkPerTree float32 = 230 // square metres of park to each tree
)

// plantTrees adds the trees for whatever part of the city has appeared since
// it was last called. Where each one stands is worked out from the block or
// road it belongs to, so the same city always grows the same trees.
func (g *Game) plantTrees() {
	c := g.world.City
	for ; g.treedBlocks < len(c.Blocks); g.treedBlocks++ {
		g.plantPark(c.Blocks[g.treedBlocks])
	}
	for ; g.treedRoads < len(c.Roads); g.treedRoads++ {
		g.plantStreet(c, c.Roads[g.treedRoads])
	}
}

// plantPark scatters trees over a park, keeping them off its paths.
func (g *Game) plantPark(b city.Block) {
	if !b.Park {
		return
	}
	if fieldOf(b, g.opts.Map) != fieldNone {
		// A field is kept clear, and has its trees in the hedge round it.
		g.trees = append(g.trees, hedgerow(b)...)
		return
	}
	size := b.Max.Sub(b.Min)
	n := int(size.X * size.Z / parkPerTree)
	for i := range n {
		h := scatter(b.Min.X, b.Min.Z, i)
		at := mathx.V(b.Min.X+6+h[0]*(size.X-12), b.Min.Z+6+h[1]*(size.Z-12))
		if onParkPath(b, at) {
			continue
		}
		g.trees = append(g.trees, tree{
			at: at, height: 4.5 + h[2]*4, spread: 1.6 + h[2]*1.5, shade: i,
		})
	}
}

// plantStreet lines a side street with trees, leaving the parking bays clear.
func (g *Game) plantStreet(c *city.City, r *city.Road) {
	if r.Ring || r.Class == city.Arterial {
		return // the main roads are lined with lamps instead
	}
	from := c.Nodes[r.A].Radius + 9
	to := r.Length - c.Nodes[r.B].Radius - 9
	bays := c.Bays(r)
	for d, i := from, 0; d < to; d, i = d+treeSpacing, i+1 {
		for _, s := range [2]float32{-1, 1} {
			h := scatter(float32(r.ID), d, int(s)+2)
			if h[0] < 0.25 {
				continue // gaps, so it is not a row of soldiers
			}
			if inBay(bays, d+h[1]*4, s) {
				continue // nobody plants a tree in a parking space
			}
			at := c.Nodes[r.A].Pos.Add(r.Dir.Mul(d + h[1]*4)).
				Add(r.Dir.Right().Mul(s * (r.HalfWidth + treeSetback)))
			g.trees = append(g.trees, tree{
				at: at, height: 3.8 + h[2]*2.4, spread: 1.2 + h[2]*0.7, shade: i + int(s),
			})
		}
	}
}

// scatter returns three numbers in [0, 1) for a place and an index, the same
// every time, which is all a tree needs to decide where exactly to stand.
func scatter(x, z float32, i int) [3]float32 {
	h := uint32(int32(x*7.3))*0x9e3779b1 ^ uint32(int32(z*5.1))*0x85ebca6b ^ uint32(i+1)*0xc2b2ae35
	var out [3]float32
	for k := range out {
		h ^= h >> 15
		h *= 0x2c1b3c6d
		h ^= h >> 12
		out[k] = float32(h>>8&0xffff) / 65536
	}
	return out
}

func (g *Game) drawTrees(eye mathx.Vec) {
	g.plantTrees()
	for _, t := range g.trees {
		d := t.at.DistTo(eye)
		if d > cullTrees {
			continue
		}
		g.look.drawTree(t, city.KerbHeight)
		if g.look.litter && d < litterRange {
			g.look.drawLitter(t, city.KerbHeight)
		}
	}
}

func drawBoxes(boxes []annotate.Box, labels bool) {
	// Paints the annotator's boxes over whatever is already rendered.
	// It must run outside the lighting shader so the colours land in the
	// framebuffer exactly as specified: the scanner matches them exactly.
	for _, b := range boxes {
		v := vision.RGBA(b.Class)
		col := rl.NewColor(v.R, v.G, v.B, v.A)
		rl.DrawRectangleLinesEx(
			rl.NewRectangle(b.MinX, b.MinY, b.Width(), b.Height()),
			annotate.Thickness, col)
		if !labels || b.Width() <= 26 {
			continue
		}
		y := b.MinY - 11
		if y < 1 {
			y = b.MaxY + 2
		}
		rl.DrawText(b.Class.String(), int32(b.MinX), int32(y), 10, col)
	}
}

// PaletteBases returns the world's key colours, which a GIF palette ramps
// through light and shade to reproduce the city without banding. The
// detection boxes are not among them: they are flat, and go in exactly as
// they are, from [BoxColours].
func PaletteBases() []color.RGBA {
	// The media is made in the city, so it is the city's season that the
	// palette has to carry.
	lk := lookFor(city.MapCity.Season())
	key := [...]rl.Color{
		colSky, lk.ground, colAsphalt, colSidewalk, colKerb, lk.park,
		colLinePale, colLineWarm, colPole, colStopFace, colSignFace, colGlass,
		colTyre, hudPanel, hudText, colPoliceBody, colBeaconBlue, colBeaconRed,
		colShopLit, colWood, colStone,
	}
	sets := [][]rl.Color{key[:], buildingShades[:], carPaints[:], lk.leaves[:], roofShades[:]}
	var out []color.RGBA
	for _, set := range sets {
		for _, c := range set {
			out = append(out, toRGBAColor(c))
		}
	}
	return out
}

// BoxColours returns the vision system's annotation colours, which a GIF
// palette needs exactly: the boxes are drawn flat, and a shade of one is
// never seen.
func BoxColours() []color.RGBA {
	out := make([]color.RGBA, 0, int(vision.ClassLaneOncoming))
	for cl := vision.ClassVehicle; cl <= vision.ClassLaneOncoming; cl++ {
		out = append(out, vision.RGBA(cl))
	}
	return out
}

func toRGBAColor(c rl.Color) color.RGBA {
	return color.RGBA{R: c.R, G: c.G, B: c.B, A: 255}
}
