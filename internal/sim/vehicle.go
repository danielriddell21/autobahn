// Package sim contains the driving simulation: vehicle dynamics, the traffic
// agents that populate the city, signal timing and the road-rule judging that
// scores both the player and the autopilot.
//
// Nothing in this package renders. It has no dependency on raylib, so the
// simulation can be stepped headlessly in tests.
package sim

import (
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Controls is one frame of driver input. The same struct is filled by the
// keyboard, by a traffic agent and by the autopilot, so every driver in the
// world is subject to identical dynamics.
type Controls struct {
	Throttle  float32 // 0..1
	Brake     float32 // 0..1
	Steer     float32 // -1 (full left) .. +1 (full right)
	Handbrake bool
	Reverse   bool
}

// Spec is the fixed physical description of a vehicle.
type Spec struct {
	Mass       float32 // kg
	Inertia    float32 // yaw moment of inertia, kg m^2
	FrontAxle  float32 // distance from centre of mass to front axle, m
	RearAxle   float32 // distance from centre of mass to rear axle, m
	HalfWidth  float32
	HalfLength float32
	Height     float32
	MaxSteer   float32 // radians at full lock

	EngineForce float32 // peak driving force, N
	BrakeForce  float32 // peak braking force, N
	ReverseFrac float32 // reverse force as a fraction of EngineForce
	MaxSpeed    float32 // m/s

	Drag       float32 // quadratic air drag coefficient
	Rolling    float32 // linear rolling resistance coefficient
	GripFront  float32 // cornering stiffness, N/rad
	GripRear   float32
	MaxLateral float32 // per-axle lateral force limit, N
}

// CarSpec returns the handling profile of the standard road car. It is tuned
// for arcade weight rather than strict realism: quick to rotate, forgiving on
// the limit, and able to break traction with the handbrake.
func CarSpec() Spec {
	return Spec{
		Mass: 1400, Inertia: 2200,
		FrontAxle: 1.35, RearAxle: 1.35,
		HalfWidth: 0.95, HalfLength: 2.25, Height: 1.45,
		MaxSteer:    0.60,
		EngineForce: 9800, BrakeForce: 15000, ReverseFrac: 0.45,
		MaxSpeed: 62,
		Drag:     0.42, Rolling: 14,
		GripFront: 78000, GripRear: 84000, MaxLateral: 8600,
	}
}

// TrafficSpec returns a slightly softer profile for ambient traffic, which
// keeps NPC cars stable without them driving like race cars.
func TrafficSpec() Spec {
	s := CarSpec()
	s.EngineForce = 7200
	s.MaxSpeed = 40
	s.GripFront, s.GripRear = 90000, 96000
	return s
}

// Wheelbase returns the distance between the axles.
func (s Spec) Wheelbase() float32 { return s.FrontAxle + s.RearAxle }

// Vehicle is a rigid body driven by [Controls]. Position and velocity are in
// world space on the XZ plane; Yaw is the heading in radians.
type Vehicle struct {
	Spec    Spec
	Pos     mathx.Vec
	Yaw     float32
	Vel     mathx.Vec
	YawRate float32

	// Steer is the current front wheel angle in radians, smoothed toward the
	// commanded input so the car cannot snap to full lock instantly.
	Steer     float32
	Throttle  float32
	Brake     float32
	Handbrake bool

	// Slip is the magnitude of the rear slip angle from the last step, used by
	// the renderer to decide when to lay down skid marks.
	Slip float32
	// Odometer accumulates distance travelled, in metres.
	Odometer float32
}

// NewVehicle returns a vehicle at rest at the given pose.
func NewVehicle(spec Spec, pos mathx.Vec, yaw float32) *Vehicle {
	return &Vehicle{Spec: spec, Pos: pos, Yaw: yaw}
}

// Forward returns the unit heading vector.
func (v *Vehicle) Forward() mathx.Vec { return mathx.FromAngle(v.Yaw) }

// Speed returns the magnitude of the velocity in metres per second.
func (v *Vehicle) Speed() float32 { return v.Vel.Len() }

// ForwardSpeed returns the signed speed along the heading, so reversing gives
// a negative value.
func (v *Vehicle) ForwardSpeed() float32 { return v.Vel.Dot(v.Forward()) }

// Box returns the vehicle's oriented bounding box for collision tests.
func (v *Vehicle) Box() mathx.OBB {
	return mathx.OBB{
		Center: v.Pos, HalfW: v.Spec.HalfWidth,
		HalfL: v.Spec.HalfLength, Heading: v.Yaw,
	}
}

// Place teleports the vehicle to a pose and brings it to a stop.
func (v *Vehicle) Place(pos mathx.Vec, yaw float32) {
	v.Pos, v.Yaw = pos, yaw
	v.Vel, v.YawRate, v.Steer, v.Slip = mathx.Vec{}, 0, 0, 0
}

// Update advances the vehicle by dt seconds under the given controls.
func (v *Vehicle) Update(c Controls, dt float32) {
	if dt <= 0 {
		return
	}
	s := v.Spec

	// Steering authority falls off with speed, which stops the car from
	// spinning itself at motorway pace.
	speed := v.Speed()
	authority := mathx.Clamp(1-speed/(s.MaxSpeed*1.5), 0.32, 1)
	target := mathx.Clamp(c.Steer, -1, 1) * s.MaxSteer * authority
	v.Steer = mathx.Approach(v.Steer, target, 9, dt)
	v.Throttle, v.Brake, v.Handbrake = c.Throttle, c.Brake, c.Handbrake

	fwd := v.Forward()
	right := fwd.Right()
	vLong := v.Vel.Dot(fwd)
	vLat := v.Vel.Dot(right)

	// Longitudinal forces. Engine output tapers as the car approaches its
	// maximum speed so it settles rather than accelerating forever.
	var drive float32
	switch {
	case c.Reverse:
		drive = -c.Throttle * s.EngineForce * s.ReverseFrac
	default:
		drive = c.Throttle * s.EngineForce * mathx.Clamp(1-vLong/s.MaxSpeed, 0, 1)
	}
	brake := c.Brake * s.BrakeForce
	if v.Handbrake {
		brake += s.BrakeForce * 0.55
	}
	// Braking opposes motion and must not drag a stationary car backwards.
	braking := -mathx.Sign(vLong) * min(brake, mathx.Abs(vLong)*s.Mass/dt)
	resist := -s.Drag*vLong*mathx.Abs(vLong) - s.Rolling*vLong
	fLong := drive + braking + resist

	gripRear := s.GripRear
	latLimit := s.MaxLateral
	if v.Handbrake {
		// Locking the rear axle destroys its lateral grip, which is what lets
		// the car rotate into a slide.
		gripRear *= 0.22
	}

	var fLatFront, fLatRear, torque float32
	if mathx.Abs(vLong) < 1.8 {
		// The slip-angle model is ill-conditioned near standstill, so blend to
		// a kinematic bicycle: the car simply follows where the wheels point.
		if wb := s.Wheelbase(); wb > 0 {
			v.YawRate = vLong * mathx.Tan(v.Steer) / wb
		}
		v.Slip = 0
		v.Vel = fwd.Mul(vLong + fLong/s.Mass*dt).Add(right.Mul(vLat * 0.85))
		v.Yaw = mathx.WrapPi(v.Yaw + v.YawRate*dt)
		v.advance(dt)
		return
	}

	absLong := mathx.Abs(vLong)
	slipFront := mathx.Atan2(vLat+v.YawRate*s.FrontAxle, absLong) - v.Steer*mathx.Sign(vLong)
	slipRear := mathx.Atan2(vLat-v.YawRate*s.RearAxle, absLong)
	v.Slip = mathx.Abs(slipRear)

	fLatFront = mathx.Clamp(-s.GripFront*slipFront, -latLimit, latLimit)
	fLatRear = mathx.Clamp(-gripRear*slipRear, -latLimit, latLimit)

	// Weight transfer under acceleration and braking shifts grip between the
	// axles, so a trailing throttle tightens the line and power loosens it.
	transfer := mathx.Clamp(fLong/(s.Mass*9.81), -0.45, 0.45)
	fLatFront *= 1 - transfer
	fLatRear *= 1 + transfer

	aLong := fLong/s.Mass + vLat*v.YawRate
	aLat := (fLatFront*mathx.Cos(v.Steer)+fLatRear)/s.Mass - vLong*v.YawRate
	torque = fLatFront*mathx.Cos(v.Steer)*s.FrontAxle - fLatRear*s.RearAxle

	vLong += aLong * dt
	vLat += aLat * dt
	v.YawRate += torque / s.Inertia * dt
	// Bleed off yaw rate so the car recovers from a slide instead of spinning.
	v.YawRate -= v.YawRate * 1.6 * dt

	v.Yaw = mathx.WrapPi(v.Yaw + v.YawRate*dt)
	fwd = v.Forward()
	v.Vel = fwd.Mul(vLong).Add(fwd.Right().Mul(vLat))
	v.advance(dt)
}

func (v *Vehicle) advance(dt float32) {
	step := v.Vel.Mul(dt)
	v.Pos = v.Pos.Add(step)
	v.Odometer += step.Len()
}

// ApplyImpulse changes the velocity directly, used by collision response.
func (v *Vehicle) ApplyImpulse(j mathx.Vec) { v.Vel = v.Vel.Add(j) }
