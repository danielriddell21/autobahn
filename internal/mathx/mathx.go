// Package mathx provides the small 2D and scalar helpers shared across the
// simulation.
//
// The world is modelled on the XZ ground plane with +Y up, matching raylib's
// right-handed convention. Angles are in radians and headings are measured so
// that the vector (1, 0) has an angle of zero.
package mathx

import "math"

// Vec is a point or direction on the ground plane.
type Vec struct{ X, Z float32 }

// V returns the vector with the given components.
func V(x, z float32) Vec { return Vec{x, z} }

// Add returns the sum of a and b.
func (a Vec) Add(b Vec) Vec { return Vec{a.X + b.X, a.Z + b.Z} }

// Sub returns the difference a - b.
func (a Vec) Sub(b Vec) Vec { return Vec{a.X - b.X, a.Z - b.Z} }

// Mul returns a scaled by s.
func (a Vec) Mul(s float32) Vec { return Vec{a.X * s, a.Z * s} }

// Dot returns the dot product of a and b.
func (a Vec) Dot(b Vec) float32 { return a.X*b.X + a.Z*b.Z }

// Cross returns the scalar cross product of a and b, which is positive when b
// lies to the right of a.
func (a Vec) Cross(b Vec) float32 { return a.X*b.Z - a.Z*b.X }

// LenSq returns the squared length of a, avoiding a square root.
func (a Vec) LenSq() float32 { return a.X*a.X + a.Z*a.Z }

// Len returns the length of a.
func (a Vec) Len() float32 { return Sqrt(a.LenSq()) }

// DistTo returns the distance between a and b.
func (a Vec) DistTo(b Vec) float32 { return a.Sub(b).Len() }

// Norm returns a unit-length copy of a, or the zero vector if a is degenerate.
func (a Vec) Norm() Vec {
	l := a.Len()
	if l < 1e-6 {
		return Vec{}
	}
	return Vec{a.X / l, a.Z / l}
}

// Right returns the vector 90 degrees clockwise from a, which is the direction
// to the right of a heading in a right-handed Y-up system.
func (a Vec) Right() Vec { return Vec{-a.Z, a.X} }

// Angle returns the heading of a in radians.
func (a Vec) Angle() float32 { return Atan2(a.Z, a.X) }

// Lerp returns the linear blend from a to b at t.
func (a Vec) Lerp(b Vec, t float32) Vec {
	return Vec{a.X + (b.X-a.X)*t, a.Z + (b.Z-a.Z)*t}
}

// FromAngle returns the unit vector for a heading in radians.
func FromAngle(r float32) Vec { return Vec{Cos(r), Sin(r)} }

// Sqrt returns the square root of v.
func Sqrt(v float32) float32 { return float32(math.Sqrt(float64(v))) }

// Abs returns the absolute value of v.
func Abs(v float32) float32 { return float32(math.Abs(float64(v))) }

// Sin returns the sine of v radians.
func Sin(v float32) float32 { return float32(math.Sin(float64(v))) }

// Cos returns the cosine of v radians.
func Cos(v float32) float32 { return float32(math.Cos(float64(v))) }

// Tan returns the tangent of v radians.
func Tan(v float32) float32 { return float32(math.Tan(float64(v))) }

// Atan2 returns the arc tangent of y/x, using the signs of both to determine
// the quadrant.
func Atan2(y, x float32) float32 { return float32(math.Atan2(float64(y), float64(x))) }

// Exp returns e raised to the power of v.
func Exp(v float32) float32 { return float32(math.Exp(float64(v))) }

// Clamp constrains v to the range [lo, hi].
func Clamp(v, lo, hi float32) float32 { return min(max(v, lo), hi) }

// Sign returns -1, 0 or 1 according to the sign of v.
func Sign(v float32) float32 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}

// Lerp returns the linear blend from a to b at t.
func Lerp(a, b, t float32) float32 { return a + (b-a)*t }

// WrapPi folds an angle in radians into the range [-pi, pi].
func WrapPi(a float32) float32 {
	const twoPi = 2 * math.Pi
	a = float32(math.Mod(float64(a), twoPi))
	if a > math.Pi {
		a -= twoPi
	}
	if a < -math.Pi {
		a += twoPi
	}
	return a
}

// MoveToward steps current toward target by at most maxDelta.
func MoveToward(current, target, maxDelta float32) float32 {
	d := target - current
	if Abs(d) <= maxDelta {
		return target
	}
	return current + Sign(d)*maxDelta
}

// Approach smooths current toward target at the given rate, independently of
// the frame duration dt.
func Approach(current, target, rate, dt float32) float32 {
	return current + (target-current)*(1-Exp(-rate*dt))
}

// Bezier evaluates the quadratic curve through p0 and p2 with control point p1.
func Bezier(p0, p1, p2 Vec, t float32) Vec {
	u := 1 - t
	return p0.Mul(u * u).Add(p1.Mul(2 * u * t)).Add(p2.Mul(t * t))
}

// BezierTangent returns the unnormalised derivative of Bezier at t.
func BezierTangent(p0, p1, p2 Vec, t float32) Vec {
	return p1.Sub(p0).Mul(2 * (1 - t)).Add(p2.Sub(p1).Mul(2 * t))
}

// KPH converts kilometres per hour to metres per second.
func KPH(v float32) float32 { return v / 3.6 }

// ToKPH converts metres per second to kilometres per hour.
func ToKPH(v float32) float32 { return v * 3.6 }

// MetresPerSecondPerMPH is one mile per hour expressed in metres per second.
const MetresPerSecondPerMPH = 0.44704

// MPH converts miles per hour to metres per second. British speed limits are
// posted in miles per hour, so this is the unit signs and the speedometer use.
func MPH(v float32) float32 { return v * MetresPerSecondPerMPH }

// ToMPH converts metres per second to miles per hour.
func ToMPH(v float32) float32 { return v / MetresPerSecondPerMPH }

// OBB is an oriented bounding box on the ground plane, used for collision
// tests between vehicles and against static obstacles.
type OBB struct {
	Center  Vec
	HalfW   float32 // half extent across the heading
	HalfL   float32 // half extent along the heading
	Heading float32
}

// Corners returns the four corners of the box in world space.
func (o OBB) Corners() [4]Vec {
	f := FromAngle(o.Heading)
	r := f.Right().Mul(o.HalfW)
	fl := f.Mul(o.HalfL)
	return [4]Vec{
		o.Center.Add(fl).Add(r),
		o.Center.Add(fl).Sub(r),
		o.Center.Sub(fl).Sub(r),
		o.Center.Sub(fl).Add(r),
	}
}

// Radius returns the radius of the circle enclosing the box.
func (o OBB) Radius() float32 { return Sqrt(o.HalfW*o.HalfW + o.HalfL*o.HalfL) }

// Overlaps reports whether two boxes intersect.
func (o OBB) Overlaps(b OBB) bool {
	_, _, hit := o.Penetration(b)
	return hit
}

// Penetration reports whether two boxes intersect and, when they do, returns
// the axis of least overlap pointing from o toward b together with the depth
// along it. This is the separating-axis test, and the returned axis and depth
// are what a collision response needs to push the boxes apart.
func (o OBB) Penetration(b OBB) (axis Vec, depth float32, hit bool) {
	if o.Center.DistTo(b.Center) > o.Radius()+b.Radius() {
		return Vec{}, 0, false
	}
	ca, cb := o.Corners(), b.Corners()
	fa, fb := FromAngle(o.Heading), FromAngle(b.Heading)

	depth = math.MaxFloat32
	for _, ax := range [4]Vec{fa, fa.Right(), fb, fb.Right()} {
		minA, maxA := project(ca, ax)
		minB, maxB := project(cb, ax)
		if maxA < minB || maxB < minA {
			return Vec{}, 0, false
		}
		if ov := min(maxA, maxB) - max(minA, minB); ov < depth {
			depth = ov
			axis = ax
		}
	}
	// Orient the axis so it always points from o toward b.
	if b.Center.Sub(o.Center).Dot(axis) < 0 {
		axis = axis.Mul(-1)
	}
	return axis, depth, true
}

func project(pts [4]Vec, axis Vec) (lo, hi float32) {
	lo = pts[0].Dot(axis)
	hi = lo
	for _, p := range pts[1:] {
		d := p.Dot(axis)
		lo = min(lo, d)
		hi = max(hi, d)
	}
	return lo, hi
}

// Vec3 is a point or direction in world space, with +Y up.
type Vec3 struct{ X, Y, Z float32 }

// V3 returns the vector with the given components.
func V3(x, y, z float32) Vec3 { return Vec3{x, y, z} }

// Ground returns the XZ components of a, dropping its height.
func (a Vec3) Ground() Vec { return Vec{a.X, a.Z} }

// Add returns the sum of a and b.
func (a Vec3) Add(b Vec3) Vec3 { return Vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }

// Sub returns the difference a - b.
func (a Vec3) Sub(b Vec3) Vec3 { return Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }

// Mul returns a scaled by s.
func (a Vec3) Mul(s float32) Vec3 { return Vec3{a.X * s, a.Y * s, a.Z * s} }

// Dot returns the dot product of a and b.
func (a Vec3) Dot(b Vec3) float32 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }

// Cross returns the cross product of a and b.
func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{
		a.Y*b.Z - a.Z*b.Y,
		a.Z*b.X - a.X*b.Z,
		a.X*b.Y - a.Y*b.X,
	}
}

// Len returns the length of a.
func (a Vec3) Len() float32 { return Sqrt(a.Dot(a)) }

// Norm returns a unit-length copy of a, or the zero vector if a is degenerate.
func (a Vec3) Norm() Vec3 {
	l := a.Len()
	if l < 1e-6 {
		return Vec3{}
	}
	return Vec3{a.X / l, a.Y / l, a.Z / l}
}
