package game

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// Under a level place the ground is two flat sheets: grass, and over the
// built-up part of it asphalt, which shows as road wherever no pavement
// covers it. Under a place on hills the same two sheets have to follow the
// land, so each is cut into tiles, made as the eye comes near and given back
// once it has gone.
//
// Near the eye the asphalt is cut fine, because the paint on a road lies a
// finger's width above it and has to stay there over every crest. Past that
// there is only grass, seen through fog from a long way off, and it is cut
// coarse and laid a little low so that its flat pieces never show through
// the fine ones.

// Tiles of ground, in metres.
const (
	// roadTile is the side of a tile of asphalt, and roadCell of a square
	// within it.
	roadTile float32 = 64
	roadCell float32 = 8
	// farTile and farCell are the same for the grass beyond, farReach how
	// far off it is laid, and farDrop how far below the land.
	farTile  float32 = 512
	farCell  float32 = 64
	farReach float32 = 1100
	farDrop  float32 = 0.9
	// tilesKept is how many tiles are held before the far ones are given
	// back.
	tilesKept = 420
)

// tile names a tile of ground by where it is and which sheet it is of.
type tile struct {
	x, z int32
	far  bool
}

// drawTerrain draws the ground of a place on hills.
func (g *Game) drawTerrain(eye mathx.Vec) {
	_, sea, coast := g.world.City.Shoreline()
	limit := float32(1e9)
	if coast {
		limit = sea
	}
	g.sheet(eye, true, farTile, farCell, farReach, -farDrop, limit, g.look.ground)
	r := min(g.cullRange()+groundEdge, sim.GrowRadius)
	g.sheet(eye, false, roadTile, roadCell, r, -0.02, limit, as(colAsphalt, matRoad))
	g.shed(eye)
}

// sheet draws the tiles of one sheet within reach of the eye, making any
// that are not yet made. The sheet stops at limit, which is the sea.
func (g *Game) sheet(eye mathx.Vec, far bool, size, cell, reach, y, limit float32, col rl.Color) {
	if g.tiles == nil {
		g.tiles = map[tile]*patch{}
	}
	lo := func(v float32) int32 { return int32(math.Floor(float64((v - reach) / size))) }
	hi := func(v float32) int32 { return int32(math.Floor(float64((v + reach) / size))) }
	for tz := lo(eye.Z); tz <= hi(eye.Z); tz++ {
		for tx := lo(eye.X); tx <= hi(eye.X); tx++ {
			at := mathx.V(float32(tx)*size, float32(tz)*size)
			if at.Add(mathx.V(size/2, size/2)).DistTo(eye) > reach+size*0.71 || at.Z >= limit {
				continue
			}
			k := tile{x: tx, z: tz, far: far}
			p, ok := g.tiles[k]
			if !ok {
				p = g.cut(at, size, cell, y, limit, col)
				g.tiles[k] = p
			}
			p.draw(g.scene.material)
		}
	}
}

// cut makes one tile of ground: a square of squares, each corner as high as
// the land is there.
func (g *Game) cut(at mathx.Vec, size, cell, y, limit float32, col rl.Color) *patch {
	n := int(size / cell)
	var w patchwork
	c := g.world.City
	for j := 0; j <= n; j++ {
		for i := 0; i <= n; i++ {
			p := mathx.V(at.X+float32(i)*cell, min(at.Z+float32(j)*cell, limit))
			s := c.Slope(p)
			up := rl.Vector3Normalize(vec3(-s.X, 1, -s.Z))
			w.corner(vec3(p.X, c.Height(p)+y, p.Z), up, col)
		}
	}
	for j := range n {
		for i := range n {
			a := uint16(j*(n+1) + i)
			w.quad(a, a+uint16(n+1), a+uint16(n+2), a+1)
		}
	}
	return w.upload()
}

// shed gives back the tiles furthest from the eye once too many are held.
func (g *Game) shed(eye mathx.Vec) {
	if len(g.tiles) <= tilesKept {
		return
	}
	for k, p := range g.tiles {
		size, reach := roadTile, min(g.cullRange()+groundEdge, sim.GrowRadius)
		if k.far {
			size, reach = farTile, farReach
		}
		mid := mathx.V((float32(k.x)+0.5)*size, (float32(k.z)+0.5)*size)
		if mid.DistTo(eye) > reach+2*size {
			p.free()
			delete(g.tiles, k)
		}
	}
}

// freeGround gives back every piece of ground the graphics card holds.
func (g *Game) freeGround() {
	for k, p := range g.tiles {
		p.free()
		delete(g.tiles, k)
	}
	for _, r := range g.risen {
		if r != nil {
			r.ground.free()
			r.ground = nil
		}
	}
}

// blockFar is how far off a block's ground is given back, as a multiple of
// how far off it stops being drawn.
const blockFar = 2.5

// footing is how a car stands on the ground: how high, and tipped how.
type footing struct {
	land        land
	y           float32
	pitch, roll float32
}

// footingOf returns how a car at a pose stands on the ground.
func (g *Game) footingOf(pos mathx.Vec, yaw float32) footing {
	if g.land == nil {
		return footing{}
	}
	s := g.world.City.StandAt(pos, yaw)
	return footing{land: g.land, y: s.At.Y, pitch: s.Pitch(), roll: s.Roll()}
}

// plinth is how far below its doorstep a building on a hillside is carried
// down, in metres, so that the low end of it does not hang in the air.
const plinth float32 = 2.2

// doorstep returns where the middle of a building's front meets the street:
// the point it is stood level with, so that its door opens on the pavement
// whatever the hill does behind it. as is the building as planned and on as
// it lies.
func doorstep(as, on city.Building) mathx.Vec {
	half := mathx.Abs(as.Front.X)*on.W/2 + mathx.Abs(as.Front.Z)*on.D/2
	return on.Center.Add(mathx.FromAngle(as.Front.Angle() + on.Heading).Mul(half))
}
