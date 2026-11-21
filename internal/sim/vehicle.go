// Package sim contains the driving simulation: vehicle dynamics, the traffic
// agents that populate the city, signal timing and the road-rule judging that
// scores both the player and the autopilot.
//
// Nothing in this package renders. It has no dependency on raylib, so the
// simulation can be stepped headlessly in tests.
package sim

import (
	"github.com/danielriddell21/autobahn/internal/city"
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
	// Boost burns nitrous for as long as there is any left.
	Boost bool
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

// winterGrip is how much of its grip a tyre keeps on a winter road.
const winterGrip float32 = 0.7

// SeasonGrip returns how much of a tyre's dry grip the road gives in a
// season, from 0 to 1. Only winter takes any away.
func SeasonGrip(s city.Season) float32 {
	if s == city.Winter {
		return winterGrip
	}
	return 1
}

// OnSurface returns the profile of the same vehicle on a road giving that
// share of full grip. The tyres hold less sideways and the brakes can use
// less of what they have, so it runs wide and takes longer to stop; the
// engine is left alone, which is what makes it easy to spin up.
func (s Spec) OnSurface(grip float32) Spec {
	grip = mathx.Clamp(grip, 0.2, 1)
	s.GripFront *= grip
	s.GripRear *= grip
	s.MaxLateral *= grip
	// Braking loses a quarter of what cornering does. The roads are gritted,
	// so a car going straight has most of its grip; it is in a bend, on the
	// snow the tyres have pushed aside, that it finds out.
	s.BrakeForce *= (3 + grip) / 4
	return s
}

// seasoned is a vehicle's profile on the roads of a particular city.
func seasoned(s Spec, c *city.City) Spec {
	return s.OnSurface(SeasonGrip(c.Map().Season()))
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

	// Grade is how steeply the road climbs under the car the way it faces:
	// the height gained for each metre forward. The world sets it; a car on
	// the level has none.
	Grade float32

	// Slip is the magnitude of the rear slip angle from the last step, used by
	// the renderer to decide when to lay down skid marks.
	Slip float32
	// Odometer accumulates distance travelled, in metres.
	Odometer float32
	// Nitro is how much nitrous is left, from 0 to 1. Traffic carries none.
	Nitro float32
	// Boosting is set while it is being burned.
	Boosting bool
}

// Nitrous tuning.
const (
	nitroBurn   float32 = 3.6  // seconds a full bottle lasts
	nitroRefill float32 = 16   // seconds to refill one, driving normally
	nitroDrift  float32 = 4.5  // seconds to refill one, held in a slide
	nitroPower  float32 = 1.6  // engine force while burning
	nitroSpeed  float32 = 1.16 // top speed while burning
	nitroSlide  float32 = 0.2  // rear slip that counts as a slide, radians
	// nitroFloor is what an empty bottle is left holding, so that a car which
	// carries nitrous can be told from one that never did.
	nitroFloor float32 = 1e-4
)

// burn spends or refills the nitrous, and returns how much the engine and the
// top speed are raised by this step.
func (v *Vehicle) burn(c Controls, dt float32) (power, top float32) {
	if v.Nitro <= 0 {
		v.Boosting = false
		return 1, 1 // traffic carries none
	}
	v.Boosting = c.Boost && v.Nitro > nitroFloor && c.Throttle > 0 && !c.Reverse
	if v.Boosting {
		v.Nitro = max(v.Nitro-dt/nitroBurn, nitroFloor)
		return nitroPower, nitroSpeed
	}
	if !c.Boost {
		rate := 1 / nitroRefill
		if v.Slip > nitroSlide && v.Speed() > 10 {
			// A slide earns it back far faster than driving in a straight line.
			rate = 1 / nitroDrift
		}
		v.Nitro = min(v.Nitro+dt*rate, 1)
	}
	return 1, 1
}

// gravity is what pulls a car down a hill, in metres per second squared.
const gravity float32 = 9.81

// held returns the force of a brake on a car on a hill. On the level a brake
// only has the car's own motion to stop. On a hill it has the hill's pull as
// well, and has to go on holding it once the car has stopped: a brake that
// only ever opposed motion would let the car go a hair's breadth each step
// and then fetch it back, and it would creep down the hill with its brakes
// on, over a stop line in the time a light takes to change.
func held(brake, vLong, pull, mass, dt float32) float32 {
	// What it would take to have the car at rest when the step ends.
	stop := -(vLong*mass/dt + pull)
	if mathx.Abs(stop) <= brake {
		return stop
	}
	if vLong != 0 {
		return -mathx.Sign(vLong) * brake
	}
	return -mathx.Sign(pull) * brake
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
		power, top := v.burn(c, dt)
		drive = c.Throttle * s.EngineForce * power * mathx.Clamp(1-vLong/(s.MaxSpeed*top), 0, 1)
	}
	brake := c.Brake * s.BrakeForce
	if v.Handbrake {
		brake += s.BrakeForce * 0.55
	}
	// Braking opposes motion and must not drag a stationary car backwards.
	braking := -mathx.Sign(vLong) * min(brake, mathx.Abs(vLong)*s.Mass/dt)
	resist := -s.Drag*vLong*mathx.Abs(vLong) - s.Rolling*vLong
	// A hill pulls the car back down it, and a brake has that to hold too.
	pull := -s.Mass * gravity * v.Grade
	if pull != 0 && brake > 0 {
		braking = held(brake, vLong, pull, s.Mass, dt)
	}
	fLong := drive + braking + resist + pull

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
	// The hill is left out: it is the car that is tipped there, not its
	// weight thrown forward or back by the driving.
	transfer := mathx.Clamp((fLong-pull)/(s.Mass*gravity), -0.45, 0.45)
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
