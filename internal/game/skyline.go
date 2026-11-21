package game

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// A place goes on for as far as it is driven, so there is no edge of town to
// see hills beyond. The hills on the horizon are painted on the sky instead:
// too far off to come any nearer, and the same whichever street they are
// seen from. Each map has its own, and the city, down on its plain, has none.

// Painting the horizon.
const (
	// ridges is how many lines of hills stand one behind another.
	ridges = 2
	// skylineStep is the angle between the points a ridge is drawn through.
	skylineStep = math.Pi / 90
	// skylineFar is how far off the hills are taken to be when they are put
	// on the screen, in metres. It only has to be a long way.
	skylineFar float32 = 900
	// skylineFoot is how far below the horizon a ridge is filled to, as a
	// slope, so that no sky shows between it and the ground.
	skylineFoot float32 = 0.04
	// seaward is the bearing of the open sea from a seaside town, and
	// seaGap the angle either side of it with no land in sight.
	seaward float32 = math.Pi / 2
	seaGap  float32 = 0.95
)

// ridgeShape is what one ridge of a map's horizon looks like: how high it
// stands, as a slope up from the eye, and how much of that comes and goes
// along it.
type ridgeShape struct{ height, roll float32 }

// horizonOf returns the ridges of a map's horizon, the furthest first.
func horizonOf(m city.Map) (shape [ridges]ridgeShape, ok bool) {
	switch m {
	case city.MapVillage:
		// Downs, rolling all the way round.
		return [ridges]ridgeShape{{0.078, 0.03}, {0.042, 0.022}}, true
	case city.MapSeaside:
		// Lower, and ending in a headland either side of the bay.
		return [ridges]ridgeShape{{0.062, 0.022}, {0.035, 0.016}}, true
	case city.MapMotorway:
		// Moors: higher and longer in the back.
		return [ridges]ridgeShape{{0.095, 0.034}, {0.044, 0.02}}, true
	}
	return shape, false
}

// skyline returns how high the land stands on the horizon in a direction, as
// a slope up from the eye: the tangent of the angle it is seen at. A bearing
// is measured as the map measures one, from +X round toward +Z, and the
// ridges are numbered from the furthest. It is the same a whole turn on.
func skyline(m city.Map, ridge int, bearing float32) float32 {
	shapes, ok := horizonOf(m)
	if !ok {
		return 0
	}
	s := shapes[ridge]
	k := float32(ridge)
	// Whole numbers of waves to the turn, so the horizon meets itself.
	w := 0.5*mathx.Sin(3*bearing+1.7+2.3*k) + 0.3*mathx.Sin(7*bearing+0.4+1.1*k) +
		0.2*mathx.Sin(13*bearing+2.9+4.7*k)
	h := s.height + s.roll*w
	if m == city.MapSeaside {
		// Nothing out to sea, and the land climbing steeply from it either
		// side: the nearer ridge reaching further round the bay.
		off := mathx.Abs(wrapAngle(bearing - seaward))
		h *= climb((off - seaGap + 0.12*k) / 0.3)
	}
	return max(h, 0)
}

// wrapAngle brings an angle into the half turn either side of nothing.
func wrapAngle(a float32) float32 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a < -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

// ridgeShade is the colour of a ridge: the land's own, as the light has it,
// gone most of the way to the colour of the sky behind it, and further for a
// ridge further off. No channel of it is ever full, which keeps it off every
// colour the vision system looks for.
func ridgeShade(land rl.Color, l lighting, ridge int) rl.Color {
	lit := 1 - 0.78*l.night
	haze := float32(0.4)
	if ridge == 0 {
		haze = 0.62
	}
	mix := func(c, sky uint8) uint8 {
		v := float32(c)*lit*(1-haze) + float32(sky)*haze
		return uint8(min(v, 250))
	}
	return rl.NewColor(mix(land.R, l.skyLow.R), mix(land.G, l.skyLow.G), mix(land.B, l.skyLow.B), 255)
}

// drawSkyline paints the horizon's hills on the sky as a camera sees them.
// It is drawn flat, before the scene, and the scene covers what it should.
func (g *Game) drawSkyline(cam rl.Camera3D) {
	if _, ok := horizonOf(g.opts.Map); !ok || g.viewW == 0 {
		return
	}
	eye := cam.Position
	ahead := mathx.V(cam.Target.X-eye.X, cam.Target.Z-eye.Z)
	if ahead.Len() < 1e-3 {
		return // looking straight down, at no horizon at all
	}
	ahead = ahead.Norm()
	put := func(dir mathx.Vec, slope float32) rl.Vector2 {
		at := vec3(eye.X+dir.X*skylineFar, eye.Y+slope*skylineFar, eye.Z+dir.Z*skylineFar)
		return rl.GetWorldToScreenEx(at, cam, int32(g.viewW), int32(g.viewH))
	}
	for ridge := range ridges {
		col := ridgeShade(g.look.park, g.light, ridge)
		for a := float32(-math.Pi); a < math.Pi; a += skylineStep {
			b := a + skylineStep
			da, db := mathx.FromAngle(a), mathx.FromAngle(b)
			// Only what is in front: a point behind the eye lands on the
			// screen somewhere it has no business.
			if da.Dot(ahead) < 0.2 || db.Dot(ahead) < 0.2 {
				continue
			}
			ha, hb := skyline(g.opts.Map, ridge, a), skyline(g.opts.Map, ridge, b)
			if ha <= 0 && hb <= 0 {
				continue
			}
			ta, tb := put(da, ha), put(db, hb)
			fa, fb := put(da, -skylineFoot), put(db, -skylineFoot)
			// Both ways round, since which way is the front depends on
			// which way the camera is looking.
			rl.DrawTriangle(ta, fa, fb, col)
			rl.DrawTriangle(ta, fb, fa, col)
			rl.DrawTriangle(ta, fb, tb, col)
			rl.DrawTriangle(ta, tb, fb, col)
		}
	}
}
