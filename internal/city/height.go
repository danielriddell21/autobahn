package city

import (
	"math"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

// A place may stand on hills. How high the ground is at a point is a
// function of where the point is and nothing else, the same as everything
// else about a place, so a street climbs the same hill whichever end it is
// reached from; and it is a function of where the point is on the ground,
// not on the plan, so asking costs a few sines and no search.
//
// The hills are long and low on purpose. A car's camera judges how far off
// something is from how far down the picture it sits, which is right on any
// slope so long as the slope is the same under the car and under the thing:
// what throws it is the road curving up or down between the two. So the land
// is held to a curve of a kilometre and more, which a street's length never
// shows as anything but a steady climb.

// heightSalt is the salt behind where the hills fall.
const heightSalt = 0x4d1b

// heightWaves are the swells the land is made of: how long each is as a
// share of the longest, how much of the height it carries, and which way it
// runs, as a turn from +X in radians. The long one gives a place its shape.
// A short one carries little, because the curve of the ground goes as the
// height over the square of the length.
var heightWaves = [3]struct{ length, share, turn float64 }{
	{1, 0.6, 0.5}, {0.64, 0.25, 2.2}, {1.5, 0.15, 3.9},
}

// The land by the sea, in metres.
const (
	// shoreFlat is how far inland of the water the ground is still level:
	// the beach, the front and the first streets behind it.
	shoreFlat = 190
	// shoreRun is how far the land takes to climb from there to its height.
	shoreRun = 560
)

// relief is the height of the land a city stands on.
type relief struct {
	// hills is the height of the swells either side of the middle, and
	// bluff what the land climbs to inland of a coast.
	hills, bluff float64
	// k and phase are each swell's wave vector and where it starts.
	k     [3][2]float64
	phase [3]float64
	// sea is the line the water starts at, if there is any.
	sea    float64
	hasSea bool
}

// reliefFor returns the land a city of these parameters stands on. The sea
// is the line the water starts at.
func reliefFor(p Params, sea float64, hasSea bool) *relief {
	if p.Hills <= 0 && p.Bluff <= 0 {
		return nil
	}
	r := &relief{hills: float64(p.Hills), bluff: float64(p.Bluff), sea: sea, hasSea: hasSea}
	span := float64(p.HillSpan)
	if span <= 0 {
		span = 1000
	}
	for i, w := range heightWaves {
		n := 2 * math.Pi / (span * w.length)
		r.k[i] = [2]float64{n * math.Cos(w.turn), n * math.Sin(w.turn)}
		r.phase[i] = 2 * math.Pi * float64(cellValue(p.Seed, heightSalt, 0, 0, i))
	}
	return r
}

// at returns the height of the land at a point on the ground.
func (r *relief) at(x, z float64) float64 {
	h := 0.0
	for i, w := range heightWaves {
		h += r.hills * w.share * math.Sin(x*r.k[i][0]+z*r.k[i][1]+r.phase[i])
	}
	if !r.hasSea {
		return h
	}
	// Level by the water, then up to the bluff, with the swells growing on
	// it as it climbs.
	f := mathx.Clamp(float32((r.sea-z-shoreFlat)/shoreRun), 0, 1)
	f = f * f * (3 - 2*f)
	return float64(f) * (r.bluff + h)
}

// Hilly reports whether the ground stands at any height at all. A place
// that is not is level everywhere, and nothing need ask.
func (c *City) Hilly() bool { return c.relief != nil }

// Height returns how high the ground stands at a point, in metres. The sea,
// where there is one, is at nothing.
func (c *City) Height(p mathx.Vec) float32 {
	if c.relief == nil {
		return 0
	}
	return float32(c.relief.at(float64(p.X), float64(p.Z)))
}

// slopeStep is how far either side of a point the ground is measured to
// find which way it slopes, in metres.
const slopeStep = 1.5

// Slope returns how steeply the ground climbs at a point: the height gained
// for each metre gone along +X, and along +Z.
func (c *City) Slope(p mathx.Vec) mathx.Vec {
	if c.relief == nil {
		return mathx.Vec{}
	}
	x, z := float64(p.X), float64(p.Z)
	return mathx.V(
		float32((c.relief.at(x+slopeStep, z)-c.relief.at(x-slopeStep, z))/(2*slopeStep)),
		float32((c.relief.at(x, z+slopeStep)-c.relief.at(x, z-slopeStep))/(2*slopeStep)),
	)
}

// Grade returns how steeply the ground climbs at a point in a direction: the
// height gained for each metre gone that way, so a hill ahead is above
// nothing and one falling away is below it.
func (c *City) Grade(p, dir mathx.Vec) float32 {
	if c.relief == nil {
		return 0
	}
	return c.Slope(p).Dot(dir)
}

// Stand is how something stands on the ground at a point, facing a way: the
// point itself, and its own forward, right and up, tilted as the ground is.
type Stand struct {
	At             mathx.Vec3
	Fwd, Right, Up mathx.Vec3
}

// StandAt returns how something at a point and facing along yaw stands on
// the ground there.
func (c *City) StandAt(p mathx.Vec, yaw float32) Stand {
	f := mathx.FromAngle(yaw)
	r := f.Right()
	s := c.Slope(p)
	fwd := mathx.V3(f.X, s.Dot(f), f.Z).Norm()
	right := mathx.V3(r.X, s.Dot(r), r.Z).Norm()
	// Up is square to both, and the right way up.
	up := right.Cross(fwd).Norm()
	if up.Y < 0 {
		up = up.Mul(-1)
	}
	return Stand{At: mathx.V3(p.X, c.Height(p), p.Z), Fwd: fwd, Right: right, Up: up}
}

// Point returns the point so far ahead of where the thing stands, so far to
// its right and so far above it, each measured its own way.
func (s Stand) Point(ahead, right, up float32) mathx.Vec3 {
	return s.At.Add(s.Fwd.Mul(ahead)).Add(s.Right.Mul(right)).Add(s.Up.Mul(up))
}

// Pitch returns how far the thing is tipped nose up, in radians, and Roll
// how far it leans to its right.
func (s Stand) Pitch() float32 {
	return mathx.Atan2(s.Fwd.Y, mathx.Sqrt(s.Fwd.X*s.Fwd.X+s.Fwd.Z*s.Fwd.Z))
}

// Roll returns how far the thing leans over to its right, in radians.
func (s Stand) Roll() float32 {
	return -mathx.Atan2(s.Right.Y, mathx.Sqrt(s.Right.X*s.Right.X+s.Right.Z*s.Right.Z))
}
