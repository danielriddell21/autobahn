package game

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// A map is driven in one season, and the season is most of what tells one
// place from another at a glance: the colour of the trees and the ground,
// what is coming down out of the sky, and the light.

// seasonLook is how a season dresses the scene.
type seasonLook struct {
	// ground is the open land beyond the streets, and park the grass within
	// them.
	ground, park rl.Color
	// leaves are the shades a tree in leaf may be. bare is the share of
	// trees with none, and blossom the share in flower instead.
	leaves  [4]rl.Color
	bare    float32
	blossom float32
	// fall is what drifts down: how many pieces are in the air round the
	// camera, how fast they come down in metres a second, how big they are
	// and what colours. None at all in a season with nothing falling.
	fall      int
	fallSpeed float32
	fallSize  float32
	fallShade [3]rl.Color
	// litter leaves fallen leaves round the foot of each tree, and snow caps
	// the roofs and the trees in white.
	litter, snow bool
}

var (
	colSnow     = rl.NewColor(226, 230, 236, 255)
	colSnowPark = rl.NewColor(214, 220, 228, 255)
	colBlossom  = [2]rl.Color{
		rl.NewColor(232, 190, 204, 255),
		rl.NewColor(238, 230, 226, 255),
	}
	colEvergreen = rl.NewColor(46, 82, 62, 255)
)

// lookFor returns how a season dresses the scene.
func lookFor(s city.Season) seasonLook {
	switch s {
	case city.Summer:
		return seasonLook{
			ground: rl.NewColor(116, 128, 78, 255), park: rl.NewColor(96, 138, 70, 255),
			leaves: [4]rl.Color{
				rl.NewColor(62, 118, 58, 255), rl.NewColor(78, 132, 62, 255),
				rl.NewColor(54, 106, 64, 255), rl.NewColor(92, 138, 66, 255),
			},
		}
	case city.Spring:
		return seasonLook{
			ground: rl.NewColor(98, 132, 80, 255), park: rl.NewColor(100, 150, 78, 255),
			leaves: [4]rl.Color{
				rl.NewColor(108, 158, 82, 255), rl.NewColor(124, 170, 88, 255),
				rl.NewColor(96, 148, 90, 255), rl.NewColor(140, 176, 96, 255),
			},
			blossom: 0.34,
			fall:    60, fallSpeed: 0.9, fallSize: 0.07,
			fallShade: [3]rl.Color{colBlossom[0], colBlossom[1], colBlossom[0]},
		}
	case city.Winter:
		return seasonLook{
			ground: colSnow, park: colSnowPark,
			leaves: [4]rl.Color{colEvergreen, colEvergreen, colEvergreen, colEvergreen},
			bare:   0.72,
			fall:   260, fallSpeed: 2.2, fallSize: 0.06,
			fallShade: [3]rl.Color{colSnow, colSnow, colSnowPark},
			snow:      true,
		}
	default: // autumn
		return seasonLook{
			ground: rl.NewColor(112, 110, 74, 255), park: rl.NewColor(104, 118, 68, 255),
			leaves: [4]rl.Color{
				rl.NewColor(190, 122, 52, 255), rl.NewColor(168, 82, 48, 255),
				rl.NewColor(204, 164, 66, 255), rl.NewColor(132, 110, 58, 255),
			},
			bare: 0.14,
			fall: 90, fallSpeed: 1.3, fallSize: 0.12,
			fallShade: [3]rl.Color{
				rl.NewColor(190, 122, 52, 255), rl.NewColor(168, 82, 48, 255), rl.NewColor(204, 164, 66, 255),
			},
			litter: true,
		}
	}
}

// colours returns every colour a season puts in the scene, for whoever has to
// be sure none of them is one the vision system has reserved.
func (lk seasonLook) colours() []rl.Color {
	out := []rl.Color{lk.ground, lk.park}
	out = append(out, lk.leaves[:]...)
	out = append(out, lk.fallShade[:]...)
	return append(out, colSnow, colSnowPark, colBlossom[0], colBlossom[1], colEvergreen)
}

// relight turns the light for an hour into the light for that hour in a
// season: warmer and hazier in autumn, harder in summer, and flat and close
// under a winter sky.
func relight(l lighting, s city.Season) lighting {
	scale := func(v rl.Vector3, r, g, b float32) rl.Vector3 { return vec3(v.X*r, v.Y*g, v.Z*b) }
	switch s {
	case city.Summer:
		l.sun = scale(l.sun, 1.1, 1.08, 1.02)
		l.fogRange *= 1.25
	case city.Spring:
		l.sun = scale(l.sun, 1.02, 1.04, 1.02)
		l.fogRange *= 1.08
	case city.Winter:
		l.sun = scale(l.sun, 0.8, 0.84, 0.94)
		// Snow throws the light back up, so what is in shade is not dark.
		l.ground = scale(l.ground, 1.3, 1.32, 1.38)
		l.fog = mix3(l.fog, vec3(0.82, 0.85, 0.9), 0.55*(1-l.night))
		l.fogRange *= 0.62
		l.skyTop = mixColour(l.skyTop, rl.NewColor(150, 160, 174, 255), 0.6*(1-l.night))
		l.skyLow = mixColour(l.skyLow, rl.NewColor(206, 212, 220, 255), 0.6*(1-l.night))
	default:
		l.sun = scale(l.sun, 1.08, 1.0, 0.88)
		l.fog = mix3(l.fog, vec3(0.84, 0.76, 0.64), 0.3*(1-l.night))
		l.fogRange *= 0.88
	}
	return l
}

func mix3(a, b rl.Vector3, f float32) rl.Vector3 {
	return vec3(a.X+(b.X-a.X)*f, a.Y+(b.Y-a.Y)*f, a.Z+(b.Z-a.Z)*f)
}

func mixColour(a, b rl.Color, f float32) rl.Color {
	ch := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*f) }
	return rl.NewColor(ch(a.R, b.R), ch(a.G, b.G), ch(a.B, b.B), a.A)
}

// treeState is what a tree is doing this season.
type treeState int

const (
	treeLeaf treeState = iota
	treeBare
	treeBlossom
)

// treeIn decides what one tree is doing, from where it stands. In any season
// the same tree always does the same thing.
func (lk seasonLook) treeIn(at mathx.Vec) treeState {
	v := scatter(at.X, at.Z, 3)[0]
	switch {
	case v < lk.bare:
		return treeBare
	case v > 1-lk.blossom:
		return treeBlossom
	}
	return treeLeaf
}

// drawTree draws a tree as the season has it.
func (lk seasonLook) drawTree(t tree, ground float32) {
	n := len(lk.leaves)
	leaf := lk.leaves[((t.shade%n)+n)%n]
	state := lk.treeIn(t.at)
	if state == treeBlossom {
		leaf = colBlossom[((t.shade%2)+2)%2]
	}
	trunk := t.height * 0.42
	rl.DrawCube(vec3(t.at.X, ground+trunk/2, t.at.Z), 0.32, trunk, 0.32, colTrunk)
	crown := t.height - trunk
	y := ground + trunk
	if state == treeBare {
		// The trunk carries on up, with a few boughs off it.
		rl.DrawCube(vec3(t.at.X, y+crown*0.4, t.at.Z), 0.2, crown*0.8, 0.2, colTrunk)
		for i, up := range [3]float32{0.25, 0.48, 0.68} {
			reach := t.spread * (1.5 - 0.4*float32(i))
			if i%2 == 0 {
				rl.DrawCube(vec3(t.at.X, y+crown*up, t.at.Z), reach, 0.12, 0.12, colTrunk)
			} else {
				rl.DrawCube(vec3(t.at.X, y+crown*up, t.at.Z), 0.12, 0.12, reach, colTrunk)
			}
			if lk.snow {
				rl.DrawCube(vec3(t.at.X, y+crown*up+0.09, t.at.Z),
					max(reach*float32((i+1)%2), 0.14), 0.06, max(reach*float32(i%2), 0.14), colSnow)
			}
		}
		return
	}
	// Three stacked blocks, shrinking toward the top.
	for i, f := range [3]float32{1, 0.78, 0.48} {
		h := crown * [3]float32{0.42, 0.34, 0.24}[i]
		w := t.spread * 2 * f
		rl.DrawCube(vec3(t.at.X, y+h/2, t.at.Z), w, h, w, crownShade(leaf, i))
		y += h
	}
	if lk.snow {
		w := t.spread * 2 * 0.5
		rl.DrawCube(vec3(t.at.X, y+0.06, t.at.Z), w, 0.14, w, colSnow)
	}
}

// crownShade is the colour of one tier of a tree's crown, the lowest darkest.
// It only ever darkens: brightening a pale colour would run a channel past
// full, and a colour that wraps round could land on one the vision system
// has reserved.
func crownShade(leaf rl.Color, tier int) rl.Color {
	return scaleColor(leaf, min(0.8+0.1*float32(tier), 1))
}

// litterRange is how near the eye a tree has to be before the leaves under
// it are worth drawing, in metres.
const litterRange float32 = 85

// drawLitter scatters fallen leaves round the foot of a tree. They keep
// within the tree's own patch of pavement, so none hangs out over the road.
func (lk seasonLook) drawLitter(t tree, ground float32) {
	for k := range 3 {
		h := scatter(t.at.X, t.at.Z, 40+k)
		at := t.at.Add(mathx.FromAngle(h[0] * 2 * math.Pi).Mul(0.4 + h[1]*0.8))
		pillar(at, ground+0.012, ground+0.02, 0.35+h[2]*0.4, 0.35+h[2]*0.4, 6, lk.fallShade[k%3])
	}
}

// Falling pieces are kept in a box round the camera, in metres.
const (
	fallReach  float32 = 30 // either side of the eye
	fallHeight float32 = 15
)

// fallAt returns where the i-th falling piece is at a moment, near an eye.
//
// Nothing is stored for a piece. Where it is comes from its number and the
// time, on a lattice fixed to the ground that repeats every so often, so the
// pieces stay where they are as the camera moves through them and there are
// always the same number in view.
func fallAt(i int, now float32, eye mathx.Vec, speed float32) (x, y, z float32) {
	h := scatter(float32(i)*1.7, float32(i)*0.9, 77)
	span := 2 * fallReach
	wrap := func(home, about float32) float32 {
		d := float32(math.Mod(float64(home-about), float64(span)))
		if d < 0 {
			d += span
		}
		return about + d - fallReach
	}
	drift := float32(math.Sin(float64(now*1.3 + float32(i))))
	x = wrap(h[0]*span+drift*0.5, eye.X)
	z = wrap(h[1]*span+drift*0.3, eye.Z)
	fallen := float32(math.Mod(float64(now*speed*(0.7+0.6*h[2])+h[2]*fallHeight*7), float64(fallHeight)))
	return x, fallHeight - fallen, z
}

// drawFall draws whatever the season has coming down: leaves, petals, snow.
func (g *Game) drawFall(eye mathx.Vec) {
	lk := g.look
	if lk.fall == 0 {
		return
	}
	rl.Normal3f(0, 1, 0)
	for i := range lk.fall {
		x, y, z := fallAt(i, g.world.Time, eye, lk.fallSpeed)
		s := lk.fallSize
		rl.DrawCube(vec3(x, y, z), s, s*0.4, s*0.8, lk.fallShade[i%3])
	}
}
