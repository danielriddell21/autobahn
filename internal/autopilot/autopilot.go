// Package autopilot drives the car from camera detections alone.
//
// The only project package it imports is vision, and the only world state it
// ever receives is a slice of [vision.Detection] recovered from pixels plus its
// own road speed, which is odometry every real car already has. It cannot read
// the simulation, so it genuinely has to see a red light to stop for one.
//
// The controller is a conventional two-axis stack: pure pursuit on the lane
// markers for steering, and a target-speed governor feeding a longitudinal
// controller for throttle and brake.
package autopilot

import (
	"fmt"
	"math"

	"github.com/danielriddell21/autobahn/internal/vision"
)

// Command is one frame of driving output.
type Command struct {
	Throttle float32
	Brake    float32
	Steer    float32 // -1 full left .. +1 full right
}

// defaultLimit is the national built-up limit of 30 mph, assumed until a sign
// is actually read.
const defaultLimit float32 = 30 * 0.44704

// Vehicle geometry the autopilot knows about its own car.
const (
	wheelbase   float32 = 2.7
	maxSteer    float32 = 0.60
	comfortStop float32 = 2.7 // comfortable deceleration, m/s^2
	hardStop    float32 = 6.5
)

// Behaviour tuning.
const (
	laneCorridor float32 = 2.4 // half-width for "in my lane", metres
	stopBuffer   float32 = 3.4 // stop this far before the line, metres
	followGap    float32 = 6.0 // standstill gap to the car in front
	headway      float32 = 1.5 // desired time gap, seconds
	stopWaitTime float32 = 1.1 // pause at a stop sign, seconds
	giveWaySpeed float32 = 4.0 // crawl speed at a give way line, m/s
	aspectMemory float32 = 2.5 // how long a signal aspect is trusted, seconds
	// junctionCreep is the fastest a junction is approached when the signal
	// ahead has not been confirmed green. A light can change while the car is
	// closing on it, so arriving at a speed it cannot stop from is what causes
	// a car to be caught out in the dilemma zone.
	junctionCreep float32 = 9.5 // m/s, about 21 mph
	junctionWatch float32 = 38  // start being careful this far out, metres

	cornerSlowdown float32 = 2.1
	steerGain      float32 = 1.15
)

// State is what the controller is currently doing, surfaced for the HUD.
type State int

// The controller states.
const (
	StateCruise State = iota
	StateFollow
	StateSlowing
	StateHalted
	StateWaiting
	StateSearching
)

// String returns a short label for the state.
func (s State) String() string {
	switch s {
	case StateFollow:
		return "FOLLOWING"
	case StateSlowing:
		return "SLOWING"
	case StateHalted:
		return "STOPPED"
	case StateWaiting:
		return "WAITING"
	case StateSearching:
		return "NO LANE"
	default:
		return "CRUISING"
	}
}

// Driver is the autopilot controller. It keeps a little memory between frames,
// because signs pass out of view and a stop sign has to be remembered long
// enough to be obeyed.
type Driver struct {
	cam vision.Camera

	// State is the controller's current behaviour, for display.
	State State
	// Reason explains the current longitudinal decision, for display.
	Reason string
	// TargetSpeed is the speed the governor is aiming for, in m/s.
	TargetSpeed float32
	// SeenLimit is the most recent speed limit read from a sign, in m/s.
	SeenLimit float32
	// LaneOffset is the estimated lateral error from the lane centre, metres.
	LaneOffset float32
	// Detections is how many boxes the last frame produced.
	Detections int

	steer float32
	// redMemory holds a stopping aspect briefly after the signal leaves the
	// camera's view, which happens on the final approach because the head is
	// mounted at the kerb and passes outside the lens.
	redMemory float32
	// stopDist dead-reckons the stop line's range once it is no longer visible,
	// using the car's own odometry, exactly as a tracker would.
	stopDist     float32
	haveStopDist bool
	stopTimer    float32
	stopArmed    bool
	stopHeld     bool
	blindTimer   float32
}

// New returns a driver calibrated to the given camera.
func New(cam vision.Camera) *Driver {
	return &Driver{cam: cam, SeenLimit: defaultLimit, State: StateSearching}
}

// Reset clears the driver's memory.
func (d *Driver) Reset() {
	d.steer, d.stopTimer = 0, 0
	d.stopArmed, d.stopHeld = false, false
	d.redMemory, d.stopDist, d.haveStopDist = 0, 0, false
	d.SeenLimit = defaultLimit
}

// target is one thing the car may have to slow down or stop for.
type target struct {
	dist  float32 // metres ahead
	speed float32 // the speed to be doing on arrival
	why   string
}

// Drive converts one frame of detections into a driving command. speed is the
// car's own road speed in metres per second.
func (d *Driver) Drive(dets []vision.Detection, speed, dt float32) Command {
	d.Detections = len(dets)

	lane := d.lanePoints(dets)
	d.readSigns(dets)

	cmd := Command{}
	cmd.Steer = d.steerFor(lane, speed, dt)

	limit := d.SeenLimit
	governed := limit
	if c := cornerLimit(lane); c < governed {
		governed = c
	}

	// Collect everything that constrains speed, then obey the tightest.
	targets := d.hazards(dets, lane, speed, dt)
	d.State = StateCruise
	d.Reason = "clear road"
	if len(lane) == 0 {
		// Without lane markers the car has no idea where the road goes.
		d.blindTimer += dt
		if d.blindTimer > 0.3 {
			d.State = StateSearching
			d.Reason = "no lane markers in view"
			governed = min(governed, 4)
		}
	} else {
		d.blindTimer = 0
	}

	for _, t := range targets {
		if v := approachSpeed(t.dist, t.speed); v < governed {
			governed = v
			d.Reason = t.why
			switch {
			case t.speed > 0.1:
				d.State = StateFollow
			case t.dist < 6 && speed < 0.6:
				d.State = StateHalted
			default:
				d.State = StateSlowing
			}
		}
	}
	if d.stopHeld {
		d.State = StateWaiting
		d.Reason = "waiting at the line"
		governed = 0
	}

	d.TargetSpeed = max(governed, 0)
	applyLongitudinal(&cmd, speed, d.TargetSpeed)
	return cmd
}

// lanePoint is a lane marker resolved into vehicle-relative ground coordinates.
type lanePoint struct {
	dist    float32 // metres ahead
	lateral float32 // metres right of straight ahead
}

func (d *Driver) lanePoints(dets []vision.Detection) []lanePoint {
	// Each lane marker box sits on the road surface, so the bottom edge of the
	// box is where it meets the ground and gives the range.
	var pts []lanePoint
	for _, det := range dets {
		if !vision.IsLaneMarker(det.Class) {
			continue
		}
		dist := d.cam.GroundDistance(det.MaxY)
		if dist <= 0.5 || dist > 90 {
			continue
		}
		bearing := d.cam.Bearing(det.CenterX())
		pts = append(pts, lanePoint{dist: dist, lateral: dist * tan(bearing)})
	}
	// Nearest first, so the lookahead search can stop early.
	for i := 1; i < len(pts); i++ {
		for j := i; j > 0 && pts[j].dist < pts[j-1].dist; j-- {
			pts[j], pts[j-1] = pts[j-1], pts[j]
		}
	}
	return pts
}

func (d *Driver) readSigns(dets []vision.Detection) {
	// Take the largest speed sign in view: the nearest one dominates, and the
	// limit persists after the sign leaves the frame.
	best := 0
	for _, det := range dets {
		v, ok := det.Class.SpeedValue()
		if !ok {
			continue
		}
		if det.Pixels > best {
			best, d.SeenLimit = det.Pixels, v
		}
	}
}

func (d *Driver) steerFor(lane []lanePoint, speed, dt float32) float32 {
	if len(lane) == 0 {
		// Hold the last command briefly rather than snapping straight.
		d.steer *= 0.9
		return d.steer
	}
	look := clamp(5.5+0.85*speed, 7, 24)
	tgt := lane[len(lane)-1]
	for _, p := range lane {
		if p.dist >= look {
			tgt = p
			break
		}
	}
	// Pure pursuit: the steer angle that puts the car on an arc through the
	// lookahead point.
	alpha := float32(math.Atan2(float64(tgt.lateral), float64(tgt.dist)))
	angle := float32(math.Atan2(
		float64(2*wheelbase*sin(alpha)), float64(max(tgt.dist, 1))))

	d.LaneOffset = lane[0].lateral
	want := clamp(angle*steerGain/maxSteer, -1, 1)
	// Smooth so the wheel does not chatter on noisy detections.
	d.steer += (want - d.steer) * clamp(12*dt, 0, 1)
	return d.steer
}

func (d *Driver) hazards(dets []vision.Detection, lane []lanePoint, speed, dt float32) []target {
	var out []target

	// The car in front. A vehicle box's bottom edge is where its tyres meet
	// the road, which is the usable range cue.
	for _, det := range dets {
		if !vision.IsVehicle(det.Class) {
			continue
		}
		dist := d.cam.GroundDistance(det.MaxY)
		if dist <= 0 || dist > 70 {
			continue
		}
		lateral := dist * tan(d.cam.Bearing(det.CenterX()))
		// Only cars roughly in our own lane matter, and the corridor widens
		// with distance to allow for curvature.
		if abs(lateral-laneLateralAt(lane, dist)) > laneCorridor+dist*0.05 {
			continue
		}
		gap := dist - followGap
		out = append(out, target{
			dist: max(gap, 0), speed: max(speed-closingEstimate(gap, speed), 0),
			why: fmt.Sprintf("vehicle %.0fm ahead", dist),
		})
	}

	// Signals and signs, positioned by the stop line when it is visible.
	lineDist, haveLine := d.stopLineDistance(dets)
	var red, amber, green, stopSign, giveWay bool
	for _, det := range dets {
		switch det.Class {
		case vision.ClassLightRed, vision.ClassLightRedAmber:
			// Red and amber together still means stop: it only warns that
			// green is coming.
			red = true
		case vision.ClassLightAmber:
			amber = true
		case vision.ClassLightGreen:
			green = true
		case vision.ClassStopSign, vision.ClassGiveWay:
			if det.Class == vision.ClassGiveWay {
				giveWay = true
			} else {
				stopSign = true
			}
			if !haveLine {
				if dd := d.cam.GroundDistance(det.MaxY); dd > 0 && dd < 60 {
					lineDist, haveLine = dd, true
				}
			}
		}
	}

	// Carry the stop line forward on odometry while it is in view, so it
	// survives being hidden by the car in front or leaving the frame.
	if d.haveStopDist {
		d.stopDist -= speed * dt
		if d.stopDist < -2 {
			d.haveStopDist = false
		}
	}
	if haveLine {
		d.stopDist, d.haveStopDist = lineDist, true
	}

	// A signal seen a moment ago still governs this junction even once it has
	// slipped out of frame. Seeing green cancels the memory at once.
	switch {
	case green:
		d.redMemory = 0
	case red || amber:
		d.redMemory = aspectMemory
	case d.redMemory > 0:
		d.redMemory -= dt
		if !haveLine && d.haveStopDist && d.stopDist > 0 {
			lineDist, haveLine = d.stopDist, true
		}
		red = red || haveLine
	}

	// A stop line painted on the road is often hidden by the car waiting on it.
	// When a signal is showing but its line cannot be seen, range the signal
	// head itself, whose mounting height is known.
	if !haveLine {
		if dd, ok := d.signalDistance(dets); ok {
			lineDist, haveLine = dd, true
		}
	}

	switch {
	case haveLine && red:
		out = append(out, target{dist: max(lineDist-stopBuffer, 0), why: "red light"})
	case haveLine && amber && !committed(speed, lineDist):
		out = append(out, target{dist: max(lineDist-stopBuffer, 0), why: "amber light"})
	case haveLine && stopSign:
		d.handleStopSign(lineDist, speed, dt)
		if !d.stopHeld && d.stopArmed {
			out = append(out, target{dist: max(lineDist-stopBuffer, 0), why: "stop sign"})
		}
	case haveLine && giveWay:
		// A give way sign needs no halt, only a slow approach and a stop if
		// something is crossing.
		out = append(out, target{
			dist: max(lineDist-stopBuffer, 0), speed: giveWaySpeed,
			why: "give way",
		})
		if crossingTraffic(d.cam, dets, lineDist) {
			out = append(out, target{
				dist: max(lineDist-stopBuffer, 0), why: "giving way to traffic",
			})
		}
	}
	// Approach an unconfirmed junction at a speed the car can stop from, unless
	// the line is already too close to stop for at all.
	if haveLine && !green && lineDist < junctionWatch && !committed(speed, lineDist) {
		out = append(out, target{
			dist: max(lineDist-stopBuffer, 0), speed: junctionCreep,
			why: "approaching a junction",
		})
	}

	if green || (!red && !amber && !stopSign && !giveWay) {
		// Once the way is clear the memory of a stop is discharged.
		if !stopSign {
			d.stopArmed = false
		}
	}
	return out
}

func (d *Driver) handleStopSign(lineDist, speed, dt float32) {
	// A stop sign has to be obeyed with a genuine halt, then a short pause
	// before moving off again.
	if !d.stopArmed && !d.stopHeld {
		d.stopArmed = true
	}
	if d.stopArmed && lineDist < stopBuffer+2.2 && speed < 0.6 {
		d.stopArmed, d.stopHeld, d.stopTimer = false, true, 0
	}
	if d.stopHeld {
		if d.stopTimer += dt; d.stopTimer >= stopWaitTime {
			d.stopHeld = false
		}
	}
}

func (d *Driver) signalDistance(dets []vision.Detection) (float32, bool) {
	best, found := float32(1e9), false
	for _, det := range dets {
		if !det.Class.StopsTraffic() && det.Class != vision.ClassLightGreen {
			continue
		}
		dd := d.cam.RangeAtHeight(det.MaxY, vision.SignalHeadHeight)
		if dd > 0 && dd < 95 && dd < best {
			best, found = dd, true
		}
	}
	return best, found
}

func (d *Driver) stopLineDistance(dets []vision.Detection) (float32, bool) {
	best, found := float32(1e9), false
	for _, det := range dets {
		if det.Class != vision.ClassStopLine {
			continue
		}
		if dd := d.cam.GroundDistance(det.MaxY); dd > 0 && dd < best {
			best, found = dd, true
		}
	}
	return best, found
}

func crossingTraffic(cam vision.Camera, dets []vision.Detection, lineDist float32) bool {
	// Anything detected off to the side at roughly the distance of the give way
	// line is traffic on the road being joined.
	for _, det := range dets {
		if !vision.IsVehicle(det.Class) {
			continue
		}
		dist := cam.GroundDistance(det.MaxY)
		if dist <= 0 || dist > lineDist+22 {
			continue
		}
		if abs(dist*tan(cam.Bearing(det.CenterX()))) > laneCorridor {
			return true
		}
	}
	return false
}

func laneLateralAt(lane []lanePoint, dist float32) float32 {
	// Where the lane centre is expected to be at a given range, so a car on a
	// bend is still recognised as being in our lane.
	if len(lane) == 0 {
		return 0
	}
	prev := lane[0]
	for _, p := range lane {
		if p.dist >= dist {
			if p.dist-prev.dist < 0.01 {
				return p.lateral
			}
			t := (dist - prev.dist) / (p.dist - prev.dist)
			return prev.lateral + (p.lateral-prev.lateral)*t
		}
		prev = p
	}
	return prev.lateral
}

func cornerLimit(lane []lanePoint) float32 {
	// Estimate how sharply the lane bends ahead and cap the speed accordingly.
	if len(lane) < 3 {
		return 1e6
	}
	far := lane[len(lane)-1]
	if far.dist < 6 {
		return 1e6
	}
	// Lateral displacement over distance approximates the turn's tightness.
	curve := abs(far.lateral) / far.dist
	if curve < 0.06 {
		return 1e6
	}
	return max(4.5, 22/(1+cornerSlowdown*curve*10))
}

func approachSpeed(dist, final float32) float32 {
	// The fastest speed from which the car can still reach `final` by the time
	// it has travelled `dist`, at a comfortable rate.
	if dist <= 0.05 {
		return final
	}
	return sqrt(final*final + 2*comfortStop*dist)
}

// committed reports whether the car is too close to the line to stop safely,
// which is the only case in which crossing on amber is permitted. Anything
// short of that gets stopped for, so the car is not still crossing when the
// aspect turns red.
func committed(speed, lineDist float32) bool {
	d := max(lineDist-stopBuffer, 0.1)
	return speed*speed/(2*d) > hardStop
}

func closingEstimate(gap, speed float32) float32 {
	// Without frame-to-frame tracking, assume a lead car that is close is
	// slower than us, which biases the controller toward caution.
	if gap > 25 {
		return 0
	}
	return min(speed, (25-gap)/25*speed*0.5)
}

func applyLongitudinal(cmd *Command, speed, target float32) {
	err := target - speed
	switch {
	case err > 0.4:
		cmd.Throttle = clamp(err/6, 0.12, 1)
	case err < -0.25:
		// Scale braking with how far over the target we are.
		cmd.Brake = clamp(-err/(hardStop*0.6), 0.06, 1)
		if target < 0.3 && speed < 1.2 {
			cmd.Brake = 1 // hold firmly at a standstill
		}
	default:
		cmd.Throttle = clamp(err, 0, 0.25)
	}
}

func clamp(v, lo, hi float32) float32 { return min(max(v, lo), hi) }
func abs(v float32) float32           { return float32(math.Abs(float64(v))) }
func sqrt(v float32) float32          { return float32(math.Sqrt(float64(v))) }
func sin(v float32) float32           { return float32(math.Sin(float64(v))) }
func tan(v float32) float32           { return float32(math.Tan(float64(v))) }
