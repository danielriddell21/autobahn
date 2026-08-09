package mathx_test

import (
	"math"
	"testing"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

const tol = 1e-4

func close(a, b float32) bool { return mathx.Abs(a-b) < tol }

func TestRightIsPerpendicularAndClockwise(t *testing.T) {
	// Looking along +X with +Y up, the right hand points along +Z.
	r := mathx.V(1, 0).Right()
	if !close(r.X, 0) || !close(r.Z, 1) {
		t.Errorf("Right of +X = %v, want (0,1)", r)
	}
	for _, v := range []mathx.Vec{{X: 1, Z: 0}, {X: 0, Z: 1}, {X: 3, Z: -4}} {
		if d := v.Dot(v.Right()); !close(d, 0) {
			t.Errorf("%v is not perpendicular to its right: dot = %v", v, d)
		}
	}
}

func TestAngleRoundTrips(t *testing.T) {
	for _, a := range []float32{0, 0.7, 1.9, -2.6, 3.0} {
		if got := mathx.FromAngle(a).Angle(); !close(mathx.WrapPi(got-a), 0) {
			t.Errorf("FromAngle(%v).Angle() = %v", a, got)
		}
	}
}

func TestWrapPiFoldsIntoRange(t *testing.T) {
	for _, a := range []float32{0, 3, -3, 7, -7, 100, -100} {
		got := mathx.WrapPi(a)
		if got < -math.Pi-tol || got > math.Pi+tol {
			t.Errorf("WrapPi(%v) = %v, outside [-pi, pi]", a, got)
		}
	}
}

func TestSpeedConversionsRoundTrip(t *testing.T) {
	for _, mph := range []float32{20, 30, 40, 70} {
		if got := mathx.ToMPH(mathx.MPH(mph)); !close(got, mph) {
			t.Errorf("round trip of %v mph gave %v", mph, got)
		}
	}
	// 30 mph is a shade over 13.4 m/s.
	if got := mathx.MPH(30); got < 13.4 || got > 13.5 {
		t.Errorf("30 mph = %v m/s, want about 13.41", got)
	}
}

func TestOverlappingBoxesReportPenetration(t *testing.T) {
	a := mathx.OBB{Center: mathx.V(0, 0), HalfW: 1, HalfL: 2}
	b := mathx.OBB{Center: mathx.V(0, 1.5), HalfW: 1, HalfL: 2}

	axis, depth, hit := a.Penetration(b)
	if !hit {
		t.Fatal("overlapping boxes reported no penetration")
	}
	if depth <= 0 {
		t.Errorf("depth = %v, want positive", depth)
	}
	// The axis must point from a toward b so a collision response separates them.
	if b.Center.Sub(a.Center).Dot(axis) <= 0 {
		t.Errorf("axis %v does not point from a toward b", axis)
	}
	// Pushing apart by the reported depth must resolve the overlap.
	b.Center = b.Center.Add(axis.Mul(depth + tol))
	if a.Overlaps(b) {
		t.Error("boxes still overlap after separating by the reported depth")
	}
}

func TestDistantBoxesDoNotOverlap(t *testing.T) {
	a := mathx.OBB{Center: mathx.V(0, 0), HalfW: 1, HalfL: 2}
	b := mathx.OBB{Center: mathx.V(40, 40), HalfW: 1, HalfL: 2}
	if a.Overlaps(b) {
		t.Error("boxes 56m apart reported as overlapping")
	}
}

// A long box rotated across another catches cases a circle test would miss.
func TestRotatedBoxesUseSeparatingAxes(t *testing.T) {
	a := mathx.OBB{Center: mathx.V(0, 0), HalfW: 0.95, HalfL: 2.25}
	across := mathx.OBB{Center: mathx.V(0, 2.6), HalfW: 0.95, HalfL: 2.25, Heading: math.Pi / 2}
	if !a.Overlaps(across) {
		t.Error("a car broadside across another was not detected as a collision")
	}
	clear := mathx.OBB{Center: mathx.V(0, 3.6), HalfW: 0.95, HalfL: 2.25, Heading: math.Pi / 2}
	if a.Overlaps(clear) {
		t.Error("a car alongside with a gap was reported as a collision")
	}
}

func TestApproachConvergesRegardlessOfStepSize(t *testing.T) {
	// Smoothing to the same target over the same total time should land in
	// nearly the same place whatever the frame rate.
	coarse, fine := float32(0), float32(0)
	for range 10 {
		coarse = mathx.Approach(coarse, 10, 4, 0.1)
	}
	for range 100 {
		fine = mathx.Approach(fine, 10, 4, 0.01)
	}
	if mathx.Abs(coarse-fine) > 0.15 {
		t.Errorf("coarse = %v, fine = %v; smoothing is frame-rate dependent", coarse, fine)
	}
}

func TestClampAndSign(t *testing.T) {
	if got := mathx.Clamp(5, 0, 3); got != 3 {
		t.Errorf("Clamp(5,0,3) = %v", got)
	}
	if got := mathx.Clamp(-5, 0, 3); got != 0 {
		t.Errorf("Clamp(-5,0,3) = %v", got)
	}
	if mathx.Sign(-2) != -1 || mathx.Sign(2) != 1 || mathx.Sign(0) != 0 {
		t.Error("Sign returned an unexpected value")
	}
}

func TestBezierPassesThroughItsEnds(t *testing.T) {
	p0, p1, p2 := mathx.V(0, 0), mathx.V(5, 0), mathx.V(5, 5)
	if got := mathx.Bezier(p0, p1, p2, 0); got != p0 {
		t.Errorf("Bezier at t=0 = %v, want %v", got, p0)
	}
	if got := mathx.Bezier(p0, p1, p2, 1); got != p2 {
		t.Errorf("Bezier at t=1 = %v, want %v", got, p2)
	}
	// The tangent at the start points toward the control point.
	if tan := mathx.BezierTangent(p0, p1, p2, 0).Norm(); !close(tan.X, 1) {
		t.Errorf("tangent at t=0 = %v, want (1,0)", tan)
	}
}
