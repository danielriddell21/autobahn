package taxi

// What shakes a load, in metres, seconds and metres a second. The tracker sees
// only the car's speed from frame to frame, so acceleration and jerk are
// worked out from successive speeds.
const (
	// BrakeLimit is the deceleration a load takes without complaint.
	BrakeLimit float32 = 4
	// JerkLimit is the rate of change of acceleration it takes without
	// complaint: a smooth stop is under it and a stamp on the pedal is not.
	JerkLimit float32 = 15
	// CollisionDecel is a deceleration no brake can make. Slowing faster than
	// that is a collision, and counts in full.
	CollisionDecel float32 = 14
	// The damage done for each unit of excess braking, of excess jerk and of
	// speed lost to a collision.
	brakeK     float32 = 1
	jerkK      float32 = 0.5
	collisionK float32 = 2
	// smoothing is how long, in seconds, acceleration is averaged over before
	// its rate of change is taken: a pedal going down is not a jolt in itself,
	// and a frame of rounding is not either.
	smoothing float32 = 0.15
	// maxAccel is the most a car gains speed, past which a frame is a jump and
	// not driving.
	maxAccel float32 = 40
	// maxGap is the longest step trusted: a pause is not a jolt.
	maxGap float32 = 0.25
	// MaxPoints is how many penalty points a load destroyed is worth.
	MaxPoints = 20
)

// Cargo tracks the damage done to a fragile load by the car's own motion.
// Feed it the time and speed each frame.
type Cargo struct {
	// Fragility is the damage the load stands before it is destroyed.
	Fragility float32
	// Damage is what it has taken so far.
	Damage float32

	seeded, haveA       bool
	prevT, prevV, prevA float32
	applied             int
}

// NewCargo returns a load that stands fragility of damage.
func NewCargo(fragility float32) *Cargo { return &Cargo{Fragility: max(fragility, 1)} }

// Skip notes the car's motion without damaging the load, for the time before
// it is on board.
func (c *Cargo) Skip(now, speed float32) {
	c.seeded, c.haveA, c.prevT, c.prevV, c.prevA = true, false, now, speed, 0
}

// Feed takes one frame of the car's motion and returns the damage it did: hard
// braking counts for how far it is beyond [BrakeLimit] and how long it lasts,
// a jolt for how far its jerk is beyond [JerkLimit], and a collision, which is
// slowing faster than [CollisionDecel], for the speed it took off, in full.
func (c *Cargo) Feed(now, speed float32) float32 {
	dt := now - c.prevT
	if !c.seeded || dt <= 0 || dt > maxGap {
		c.Skip(now, speed)
		return 0
	}
	raw := (speed - c.prevV) / dt
	if raw > maxAccel {
		// No car gains speed like that: the car was put somewhere, not driven.
		c.Skip(now, speed)
		return 0
	}
	accel := raw
	if c.haveA {
		accel = c.prevA + (raw-c.prevA)*dt/(smoothing+dt)
	}
	var dmg float32
	switch {
	case -raw >= CollisionDecel:
		dmg = (c.prevV - speed) * collisionK
		accel = 0 // the same blow must not be counted again as a jolt
	default:
		if -accel > BrakeLimit {
			dmg += (-accel - BrakeLimit) * dt * brakeK
		}
		if c.haveA {
			if jerk := abs((accel - c.prevA) / dt); jerk > JerkLimit {
				dmg += (jerk - JerkLimit) * dt * jerkK
			}
		}
	}
	c.haveA = true
	c.prevT, c.prevV, c.prevA = now, speed, accel
	c.Damage += dmg
	return dmg
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// Destroyed reports whether the load has taken all it can.
func (c *Cargo) Destroyed() bool { return c.Damage >= c.Fragility }

// Points is the damage as penalty points, from none to [MaxPoints].
func (c *Cargo) Points() int {
	return min(int(float32(MaxPoints)*c.Damage/c.Fragility), MaxPoints)
}

// Take returns the penalty points that have come due since it was last asked.
func (c *Cargo) Take() int {
	p := c.Points()
	d := p - c.applied
	c.applied = p
	return max(d, 0)
}
