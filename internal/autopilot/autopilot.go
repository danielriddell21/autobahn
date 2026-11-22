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
	// Indicator is the turn signal for a lane change or overtake.
	Indicator Indicator
}

// defaultLimit is the national built-up limit of 30 mph, assumed until a sign
// is actually read.
const defaultLimit float32 = 30 * 0.44704

// Vehicle geometry and performance the autopilot knows about its own car. This
// is calibration, the same thing a real controller is told about the vehicle
// it is fitted to, and none of it is read from the simulation.
const (
	wheelbase float32 = 2.7
	maxSteer  float32 = 0.60
	// lensToAxle is how far the camera sits ahead of the rear axle. Pure
	// pursuit steers the rear axle onto an arc, so a point ranged from the lens
	// is this much further away than it looks.
	lensToAxle float32 = 3.05
	// topSpeed sets how much steering authority the car loses with speed.
	topSpeed float32 = 62
	// brakeDecel is what full brake pressure achieves, in m/s^2.
	brakeDecel float32 = 10.7

	comfortStop float32 = 2.7 // comfortable deceleration, m/s^2
	hardStop    float32 = 6.5
)

// Behaviour tuning.
const (
	stopBuffer   float32 = 2.2 // stop with the lens this far before the line, metres
	followGap    float32 = 4.5 // standstill gap to the car in front, from the lens
	followDecel  float32 = 3.0 // the braking a queue ahead is planned around
	stopWaitTime float32 = 1.1 // pause at a stop sign, seconds
	giveWaySpeed float32 = 4.0 // crawl speed at a give way line, m/s
	// aspectMemory is how long a signal aspect is trusted once no head is in
	// view, and aspectHold how long a stopping aspect is trusted by a car
	// already standing at the line, which has nothing to gain by guessing.
	aspectMemory float32 = 2.5
	aspectHold   float32 = 30
	// junctionCreep is the fastest a junction is approached when the signal
	// ahead has not been confirmed green. A light can change while the car is
	// closing on it, so arriving at a speed it cannot stop from is what causes
	// a car to be caught out in the dilemma zone.
	junctionCreep float32 = 9.5 // m/s, about 21 mph
	junctionWatch float32 = 38  // start being careful this far out, metres
	// junctionClear is how far past a stop line a signal with no line of its
	// own is disregarded. Turning through a junction swings the camera across
	// the far-side heads of the road being joined, and those govern traffic
	// that is waiting, not a car already through.
	junctionClear float32 = 30
	// patience is how long the car waits for a gap at a give way before it
	// edges out, and nudge how long it then keeps going. Without it a queue on
	// the far side of the junction holds it for ever.
	patience float32 = 7
	nudge    float32 = 3

	// cornerGrip is the sideways acceleration a bend is taken at, m/s^2.
	cornerGrip  float32 = 2.6
	cornerDecel float32 = 3.4
	cornerFloor float32 = 3.6 // never plan a bend slower than this, m/s
	steerGain   float32 = 1.1

	blindCreep float32 = 3.5 // m/s with no lane in view
	blindStop  float32 = 3.0 // seconds blind before giving up and stopping
)

// Mode is how the autopilot drives. It is a setting, not something the
// camera can tell it, so it does not cross the perception boundary.
type Mode int

// The driving modes, named for the traffic styles in package sim they match.
const (
	ModeNormal Mode = iota
	ModeCautious
	ModeBrisk
	ModeAggressive
	// ModeGetaway ignores the posted limit and brakes late. It still stops for
	// signals, signs and the car in front.
	ModeGetaway
	NumModes
)

// String returns the mode's name.
func (m Mode) String() string {
	switch m {
	case ModeCautious:
		return "cautious"
	case ModeBrisk:
		return "brisk"
	case ModeAggressive:
		return "aggressive"
	case ModeGetaway:
		return "getaway"
	default:
		return "normal"
	}
}

// ParseMode returns the mode with the given name.
func ParseMode(name string) (Mode, bool) {
	for m := range NumModes {
		if m.String() == name {
			return m, true
		}
	}
	return ModeNormal, false
}

// profile is what a mode changes.
type profile struct {
	speed       float32 // multiplies the speed it is aiming for
	followGap   float32 // standstill gap to the car in front, metres
	followDecel float32 // braking a queue ahead is planned around
	grip        float32 // sideways acceleration bends are taken at
	cornerDecel float32 // braking for a bend
	comfort     float32 // braking for everything else
	creep       float32 // fastest approach to an unconfirmed junction, m/s

	// How it gets past people, after the traffic styles in package sim.
	overtake  float32 // keenness to pass, 0 to 1: how much slower a car has to be
	patience  float32 // seconds sat behind a slower car before passing it
	keepLeft  float32 // seconds in an outer lane with nothing to pass before moving back
	laneGap   float32 // scales the room wanted to move into a lane
	oncoming  bool    // passes on a single carriageway
	undertake bool    // passes on the inside
	// Bending the rules, which the normal mode never does.
	rolling bool // treats a stop sign as a give way when it is clear
	press   bool // closes up on a car it has been held up behind
}

func (m Mode) profile() profile {
	switch m {
	case ModeCautious:
		return profile{
			speed: 0.85, followGap: 6.5, followDecel: 2.4, grip: 2.0, cornerDecel: 2.6, comfort: 2.2, creep: 8,
			overtake: 0.15, patience: 20, keepLeft: 3, laneGap: 1.5,
		}
	case ModeBrisk:
		return profile{
			speed: 1.15, followGap: 3.8, followDecel: 3.4, grip: 3.0, cornerDecel: 3.8, comfort: 3.1, creep: 10.5,
			overtake: 0.75, patience: 4, keepLeft: 9, laneGap: 0.85, oncoming: true,
		}
	case ModeAggressive:
		return profile{
			speed: 1.3, followGap: 3.0, followDecel: 4.0, grip: 3.5, cornerDecel: 4.4, comfort: 3.6, creep: 12,
			overtake: 1, patience: 1.5, keepLeft: 14, laneGap: 0.65, oncoming: true,
			undertake: true, rolling: true, press: true,
		}
	case ModeGetaway:
		return profile{
			speed: 1.8, followGap: 2.6, followDecel: 4.6, grip: 4.2, cornerDecel: 5.0, comfort: 4.2, creep: 13.5,
			overtake: 1, patience: 0.5, keepLeft: 20, laneGap: 0.55, oncoming: true,
			undertake: true, rolling: true, press: true,
		}
	default:
		return profile{
			speed: 1, followGap: followGap, followDecel: followDecel, grip: cornerGrip,
			cornerDecel: cornerDecel, comfort: comfortStop, creep: junctionCreep,
			overtake: 0.45, patience: 8, keepLeft: 5, laneGap: 1,
		}
	}
}

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
	StateChangingLane
	StateOvertaking
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
	case StateChangingLane:
		return "CHANGING LANE"
	case StateOvertaking:
		return "OVERTAKING"
	default:
		return "CRUISING"
	}
}

// Driver is the autopilot controller. It keeps a little memory between frames,
// because signs pass out of view and a stop sign has to be remembered long
// enough to be obeyed.
type Driver struct {
	cam vision.Camera
	// mirrors are the cameras watching behind and beside the car, and around
	// what they saw on their last look.
	mirrors [vision.NumMirrors]vision.Camera
	around  [vision.NumMirrors][]Neighbour

	// Mode is how the autopilot is driving, and may be changed at any time.
	Mode Mode

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
	// Sight is how much road the last frames showed. It holds the car to a speed
	// it can stop from within that, which in clear weather is never.
	Sight Sight

	steer float32
	// curve is the curvature of the arc currently being steered, 1/m.
	curve float32

	// stopDist dead-reckons the stop line's range once it is no longer visible,
	// using the car's own odometry, exactly as a tracker would.
	stopDist     float32
	haveStopDist bool
	// aspect is the last signal aspect seen governing the junction ahead, and
	// aspectAge how long ago that was.
	aspect    vision.Class
	aspectAge float32
	// sign is the priority sign seen governing the junction ahead. It is
	// remembered until the line is crossed, because the sign itself passes out
	// of the lens on the final approach.
	sign vision.Class
	// clearRun is how much further the car has to travel before a signal with
	// no stop line of its own is believed again.
	clearRun float32

	stopTimer  float32
	stopHeld   bool
	stopDone   bool
	waited     float32
	blindTimer float32

	// Lane changes and overtakes; see manoeuvre.go. shift is how far to the
	// right of its lane markers the car is steering, and shiftWant where that
	// is heading. lastLane is where the markers were last frame, to notice
	// them jump when the route follows the car into another lane. path is
	// the shifted line actually being driven.
	shift, shiftWant float32
	lastLane         float32
	haveLast         bool
	shifted, path    []lanePoint
	move             move
	moveSide         float32 // offset of the lane being moved to
	signalTimer      float32
	moveTimer        float32
	passing          bool
	heldUp, outside  float32
	// The car in front, as tracked from frame to frame.
	leadDist, leadSpeed float32
	leadSeen            bool
	speed               float32 // the car's own, as of the last frame
	// reach is how far ahead the lane to the left, the lane to the right and
	// the oncoming lane were last known to go on.
	reach [3]float32
	// coned is set while what is in the way is a cone or a barrier, which is not
	// going anywhere and has to be gone round.
	coned bool
	// odo is how far the car has gone, and divEnd where a diversion sign said
	// to take care until.
	odo, divEnd float32
}

// New returns a driver calibrated to the given camera, with mirrors at the
// default size.
func New(cam vision.Camera) *Driver {
	return &Driver{
		cam: cam, mirrors: vision.MirrorCameras(MirrorWidth, MirrorHeight),
		SeenLimit: defaultLimit, State: StateSearching,
	}
}

// MirrorWidth and MirrorHeight are the size of the mirror images. A mirror is
// only for knowing what is around the car, so it is coarser than the camera
// the car steers by.
const (
	MirrorWidth  = 256
	MirrorHeight = 144
)

// Mirrors returns the mirror cameras the driver is calibrated to, which are
// the ones the world should be shown through.
func (d *Driver) Mirrors() [vision.NumMirrors]vision.Camera { return d.mirrors }

// Reset clears the driver's memory.
func (d *Driver) Reset() {
	*d = Driver{cam: d.cam, mirrors: d.mirrors, Mode: d.Mode, SeenLimit: defaultLimit, State: StateSearching}
}

// Neighbour is a vehicle seen in a mirror, placed on the road around the car:
// how far ahead of the middle of the car its nearest point is, negative behind,
// and how far to the right.
type Neighbour struct {
	Ahead, Right float32
}

// Watch takes one look through a mirror. The vehicles in it replace whatever
// that mirror showed last time.
func (d *Driver) Watch(m vision.Mirror, dets []vision.Detection) {
	cam := d.mirrors[m]
	seen := d.around[m][:0]
	for _, det := range dets {
		if !vision.IsVehicle(det.Class) {
			continue
		}
		// The middle of the bottom edge is where the near side of the car
		// meets the road.
		lo, hi := det.Base()
		ahead, right, ok := cam.Ground((lo+hi)/2, det.MaxY)
		if !ok {
			continue
		}
		seen = append(seen, Neighbour{Ahead: ahead, Right: right})
	}
	d.around[m] = seen
}

// Around returns every vehicle the mirrors showed on their last look.
func (d *Driver) Around() []Neighbour {
	var out []Neighbour
	for _, seen := range d.around {
		out = append(out, seen...)
	}
	return out
}

// targetKind is why the car may have to slow down.
type targetKind int

const (
	targetControl targetKind = iota // a signal, sign or line
	targetVehicle
	targetBend
)

// target is one thing the car may have to slow down or stop for.
type target struct {
	kind  targetKind
	dist  float32 // metres ahead
	speed float32 // the speed to be doing on arrival
	decel float32 // the braking it is planned around; zero means comfortable
	why   string
}

// Drive converts one frame of detections into a driving command. speed is the
// car's own road speed in metres per second.
func (d *Driver) Drive(dets []vision.Detection, speed, dt float32) Command {
	d.Detections = len(dets)
	d.odo += max(speed, 0) * dt

	raw := d.lanePoints(dets)
	lane := d.shiftLane(raw, dt)
	d.readSigns(dets, lane)
	d.Sight.Update(d.cam, dets, dt)

	cmd := Command{}
	cmd.Steer = d.steerFor(lane, speed, dt)

	pf := d.Mode.profile()
	cruise := d.SeenLimit * pf.speed
	d.State = StateCruise
	d.Reason = "clear road"
	if len(lane) == 0 {
		// Without lane markers the car has no idea where the road goes. It
		// creeps for a moment in case they come back, then gives up and stops.
		d.blindTimer += dt
		if d.blindTimer > 0.3 {
			d.State = StateSearching
			d.Reason = "no lane markers in view"
			cruise = min(cruise, blindCreep)
		}
		if d.blindTimer > blindStop {
			cruise = 0
		}
	} else {
		d.blindTimer = 0
	}

	cruise = d.takeCareAtWorks(dets, lane, cruise)

	cruise = d.manoeuvre(dets, raw, cruise, speed, dt, pf)
	d.path = lane

	// Collect everything that constrains speed, then obey the tightest.
	targets := d.bendTargets(lane, pf)
	targets = append(targets, d.vehicleTargets(dets, lane, pf)...)
	targets = append(targets, d.closedTargets(dets, lane)...)
	targets = append(targets, d.junctionTargets(dets, lane, speed, dt, pf)...)

	governed, need := cruise, float32(0)
	for _, t := range targets {
		decel := t.decel
		if decel == 0 {
			decel = pf.comfort
		}
		if speed > t.speed {
			need = max(need, (speed*speed-t.speed*t.speed)/(2*max(t.dist, 0.3)))
		}
		v := approachSpeed(t.dist, t.speed, decel)
		if v >= governed {
			continue
		}
		governed = v
		d.Reason = t.why
		switch {
		case t.kind == targetBend:
			d.State = StateCruise
		case t.dist < 6 && speed < 0.6:
			d.State = StateHalted
		case t.kind == targetVehicle:
			d.State = StateFollow
		default:
			d.State = StateSlowing
		}
	}
	if sightCap := d.Sight.Cap(brakeDecel * 0.5); sightCap < governed {
		governed = sightCap
		d.Reason = "limited sight"
	}
	if d.stopHeld {
		d.State = StateWaiting
		d.Reason = "waiting at the line"
		governed = 0
	}
	d.describeMove()

	d.TargetSpeed = max(governed, 0)
	applyLongitudinal(&cmd, speed, d.TargetSpeed, need)
	cmd.Indicator = d.indicator()
	return cmd
}

// lanePoint is a lane marker resolved into vehicle-relative ground coordinates.
type lanePoint struct {
	dist    float32 // metres ahead of the lens
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
	return chain(pts)
}

func chain(pts []lanePoint) []lanePoint {
	// Puts lane markers into the order they run along the road, by starting
	// at the nearest and repeatedly taking whichever is closest to the last one.
	//
	// Sorting them by range instead would be wrong on anything tightly curved. Most
	// of the way round a roundabout is nearer in a straight line than a point just
	// ahead on the ring, so a range-ordered lookahead picks a target across the
	// island and steers at it. Following the chain keeps the path a path.
	if len(pts) < 2 {
		return pts
	}
	// Start from whichever marker is nearest the car.
	start := 0
	for i, p := range pts {
		if p.dist < pts[start].dist {
			start = i
		}
	}
	pts[0], pts[start] = pts[start], pts[0]

	for i := 1; i < len(pts); i++ {
		best, bestGap := i, float32(1e9)
		for j := i; j < len(pts); j++ {
			if g := gap(pts[i-1], pts[j]); g < bestGap {
				best, bestGap = j, g
			}
		}
		pts[i], pts[best] = pts[best], pts[i]
	}
	return pts
}

func gap(a, b lanePoint) float32 {
	// Is the distance between two lane markers on the ground.
	dx := b.lateral - a.lateral
	dz := b.dist - a.dist
	return sqrt(dx*dx + dz*dz)
}

// Speed sign reading limits, in metres.
const (
	signRange    float32 = 45 // a sign further off than this is not ours yet
	signCorridor float32 = 14 // nor is one this far to the side of the lane
)

func (d *Driver) readSigns(dets []vision.Detection, lane []lanePoint) {
	// Take the nearest speed sign standing beside the road being driven, and
	// keep its limit after it leaves the frame. A sign a long way off, or well
	// to one side, belongs to a road the car is not on: the same camera that
	// reads this street's sign can see the next street's across a park.
	best := signRange
	for _, det := range dets {
		v, ok := det.Class.SpeedValue()
		if !ok {
			continue
		}
		dist := d.cam.RangeAtHeight(det.MinY, vision.SpeedSignTopHeight)
		lateral := dist * tan(d.cam.Bearing(det.CenterX()))
		if dist >= best || abs(lateral-laneLateralAt(lane, dist)) > signCorridor {
			continue
		}
		best, d.SeenLimit = dist, v
	}
}

func (d *Driver) steerFor(lane []lanePoint, speed, dt float32) float32 {
	if len(lane) == 0 {
		// Keep turning the way the road was last seen to go. The markers are
		// usually lost off the side of the frame in a tight turn, and
		// straightening up is the one thing guaranteed not to find them again.
		return d.steer
	}
	look := clamp(3.5+0.6*speed, 6.5, 20)
	tgt := lookahead(lane, look)

	// Pure pursuit: the arc through the rear axle and the lookahead point,
	// tangent to the car's heading, and the wheel angle that drives it.
	x, y := tgt.dist+lensToAxle, tgt.lateral
	d.curve = 2 * y / (x*x + y*y)
	angle := float32(math.Atan(float64(wheelbase * d.curve)))

	// The car turns its wheels less at speed, so ask for more to get the same.
	authority := clamp(1-speed/(topSpeed*1.5), 0.32, 1)

	d.LaneOffset = lane[0].lateral
	want := clamp(angle*steerGain/(maxSteer*authority), -1, 1)
	// Smooth so the wheel does not chatter on noisy detections.
	d.steer += (want - d.steer) * clamp(12*dt, 0, 1)
	return d.steer
}

// lookahead returns the point a given distance along the chain of markers,
// measured from the rear axle and interpolated between markers so the target
// slides smoothly instead of jumping from one to the next.
func lookahead(lane []lanePoint, look float32) lanePoint {
	prev := lanePoint{dist: -lensToAxle}
	var travelled float32
	for _, p := range lane {
		step := gap(prev, p)
		if travelled+step >= look && step > 0.01 {
			t := (look - travelled) / step
			return lanePoint{
				dist:    prev.dist + (p.dist-prev.dist)*t,
				lateral: prev.lateral + (p.lateral-prev.lateral)*t,
			}
		}
		travelled += step
		prev = p
	}
	return lane[len(lane)-1]
}

// Bend detection limits.
const (
	bendRange   float32 = 48   // markers beyond this are too coarsely ranged to trust
	bendSegment float32 = 2.5  // the shortest run between markers worth measuring
	bendAngle   float32 = 0.11 // the smallest change of direction that is a bend, rad
)

func (d *Driver) bendTargets(lane []lanePoint, pf profile) []target {
	// Reads how sharply the lane turns from the chain of markers, and plans to
	// be going slowly enough by the time each bend arrives. A junction turn and
	// a roundabout both show up this way; nothing else tells the car one is
	// coming.
	var out []target
	if c := abs(d.curve); c > 0.02 && len(lane) > 0 {
		// The arc being steered right now is a bend too.
		out = append(out, target{
			kind: targetBend, speed: bendSpeed(c, pf), decel: pf.cornerDecel, why: "in a bend",
		})
	}
	var travelled float32
	for i := 1; i+1 < len(lane); i++ {
		a, b, c := lane[i-1], lane[i], lane[i+1]
		in, onward := gap(a, b), gap(b, c)
		travelled += in
		if b.dist > bendRange {
			break
		}
		if in < bendSegment || onward < bendSegment {
			continue
		}
		turn := abs(wrapPi(heading(b, c) - heading(a, b)))
		if turn < bendAngle {
			continue
		}
		out = append(out, target{
			kind: targetBend, dist: max(lane[0].dist+travelled-in/2, 0),
			speed: bendSpeed(turn/((in+onward)/2), pf), decel: pf.cornerDecel,
			why: "bend ahead",
		})
	}
	return out
}

func bendSpeed(curvature float32, pf profile) float32 {
	return max(sqrt(pf.grip/curvature), cornerFloor)
}

func heading(a, b lanePoint) float32 {
	return float32(math.Atan2(float64(b.lateral-a.lateral), float64(b.dist-a.dist)))
}

// clearance is how far to the side of the lane centre the nearest edge of a
// vehicle may be and still be in the way, in metres: half the car's own width
// and a margin. It opens slightly with range, where bearings are coarser, but
// never as far as the oncoming lane.
func clearance(dist float32) float32 {
	if dist < 8 {
		return 1.6
	}
	return 1.45 + dist*0.02
}

// Closing up on a car that is in the way, for the modes that do: how much of
// the usual gap is kept, and the slowest it is done at. In a crawling queue
// there is nothing to gain by it.
const (
	pressGap   float32 = 0.85
	pressSpeed float32 = 6
)

// Vehicle box interpretation, in metres.
const (
	endOnWidth  float32 = 2.2 // the widest a car looks from directly behind
	sideOnDepth float32 = 3.0 // how much further off the far corners of one seen side-on are
)

// inPath reports whether a vehicle detection overlaps the car's own path, and
// how far away it is.
//
// The test is on the edges of the box rather than its centre. A car seen at an
// angle, or cut off by the side of the frame, has a box whose centre says very
// little about where the car is, but the edge nearest the lane is always the
// part that matters.
func (d *Driver) inPath(det vision.Detection, lane []lanePoint) (float32, bool) {
	dist := d.cam.GroundDistance(det.MaxY)
	if dist <= 0 || dist > 75 {
		return 0, false
	}
	lo, hi := det.Base()
	tl, tr := tan(d.cam.Bearing(lo)), tan(d.cam.Bearing(hi))
	// The bottom of the box ranges the nearest corner, but its sides are set
	// by whichever corners stick out furthest, and on a car seen side-on those
	// are further away. Ranging the sides from the near corner would pull a
	// car in the next lane across into this one, so a box too wide to be a
	// car seen end-on, or one cut off by the frame, has its sides pushed back.
	depth := clamp(dist*(tr-tl)-endOnWidth, 0, sideOnDepth)
	if lo <= 1 || hi >= float32(d.cam.Width-2) {
		depth = sideOnDepth
	}
	left, right := (dist+depth)*tl, (dist+depth)*tr
	centre := laneLateralAt(lane, dist)
	room := clearance(dist)
	if d.move != moveNone && d.move != moveSignal {
		room = tightPath // moving across: only what is actually in the way
	}
	return dist, left < centre+room && right > centre-room
}

func (d *Driver) vehicleTargets(dets []vision.Detection, lane []lanePoint, pf profile) []target {
	var out []target
	gap := pf.followGap
	if pf.press && d.heldUp > pf.patience && d.speed > pressSpeed {
		gap *= pressGap // sat on the bumper of somebody in the way
	}
	// The car in front. A vehicle box's bottom edge is where its tyres meet
	// the road, which is the usable range cue.
	for _, det := range dets {
		if !vision.IsObstacle(det.Class) {
			continue
		}
		dist, blocking := d.inPath(det, lane)
		if !blocking {
			continue
		}
		// With no tracking between frames there is no telling how fast the car
		// ahead is going, so plan to be able to stop short of where it is now.
		// Behind moving traffic that settles into a gap that grows with speed;
		// behind a queue it stops.
		out = append(out, target{
			kind: targetVehicle, dist: max(dist-gap, 0), decel: pf.followDecel,
			why: fmt.Sprintf("vehicle %.0fm ahead", dist),
		})
	}
	return out
}

// Roadworks signs, in metres and as a share of the speed.
const (
	// diversionSlow is how much of its speed the car keeps after a diversion
	// sign, for diversionRun metres.
	diversionSlow float32 = 0.8
	diversionRun  float32 = 150
	// worksSignRange is how far off a roadworks sign is read, and
	// closedStopShort how far short of a road closed sign the car stops.
	worksSignRange  float32 = 60
	closedStopShort float32 = 8
)

// takeCareAtWorks slows the speed the car is aiming for while a diversion sign
// read a short way back still holds.
func (d *Driver) takeCareAtWorks(dets []vision.Detection, lane []lanePoint, cruise float32) float32 {
	if d.readDiversion(dets, lane) && d.State == StateCruise {
		d.Reason = "diversion ahead"
	}
	if d.odo < d.divEnd {
		return cruise * diversionSlow
	}
	return cruise
}

// readDiversion takes note of a diversion sign beside the road being driven,
// and reports whether it saw one. The warning holds for a stretch of road
// after it, because the sign itself passes out of the lens.
func (d *Driver) readDiversion(dets []vision.Detection, lane []lanePoint) bool {
	seen := false
	for _, det := range dets {
		if det.Class != vision.ClassDiversion {
			continue
		}
		dist := d.cam.RangeAtHeight(det.MinY, vision.WorksSignTopHeight)
		lateral := dist * tan(d.cam.Bearing(det.CenterX()))
		if dist > worksSignRange || abs(lateral-laneLateralAt(lane, dist)) > signCorridor {
			continue
		}
		d.divEnd, seen = d.odo+diversionRun, true
	}
	return seen
}

// closedTargets stops the car for a road closed sign it is steering at: one
// the lane markers go on past. The route never leads into a closed street, so
// this is only a net under it.
func (d *Driver) closedTargets(dets []vision.Detection, lane []lanePoint) []target {
	if len(lane) == 0 {
		return nil
	}
	var out []target
	for _, det := range dets {
		if det.Class != vision.ClassRoadClosed {
			continue
		}
		dist := d.cam.RangeAtHeight(det.MinY, vision.WorksSignTopHeight)
		lateral := dist * tan(d.cam.Bearing(det.CenterX()))
		if dist > worksSignRange || lane[len(lane)-1].dist < dist || abs(lateral-laneLateralAt(lane, dist)) > 6 {
			continue
		}
		out = append(out, target{
			kind: targetControl, dist: max(dist-closedStopShort, 0),
			why: fmt.Sprintf("road closed %.0fm ahead", dist),
		})
	}
	return out
}

func (d *Driver) junctionTargets(dets []vision.Detection, lane []lanePoint, speed, dt float32, pf profile) []target {
	stopAt, haveStop := d.locateLine(dets, speed, dt)
	stopAt, haveStop = d.readSignal(dets, stopAt, haveStop, speed, dt)
	stopAt, haveStop = d.readPrioritySign(dets, stopAt, haveStop)
	if !haveStop {
		d.stopHeld = false
		return nil
	}

	stop := max(stopAt-stopBuffer, 0)
	var out []target
	switch {
	case d.aspect == vision.ClassLightRed || d.aspect == vision.ClassLightRedAmber:
		// Red and amber together still means stop: it only warns that green is
		// coming.
		out = append(out, target{dist: stop, why: "red light"})
	case d.aspect == vision.ClassLightAmber:
		if !committed(speed, stopAt) {
			out = append(out, target{dist: stop, why: "amber light"})
		}
	case d.aspect == vision.ClassLightGreen:
		return nil
	case d.sign == vision.ClassStopSign && !d.stopDone:
		d.haltAtStopSign(stopAt, speed, dt, pf)
		if !d.stopHeld {
			out = append(out, target{dist: stop, why: "stop sign"})
		}
	case d.sign != vision.ClassNone:
		out = append(out, d.giveWay(dets, lane, stop, stopAt, speed, dt)...)
	}

	// Approach an unconfirmed junction at a speed the car can stop from, unless
	// the line is already too close to stop for at all.
	if stopAt < junctionWatch && !committed(speed, stopAt) {
		out = append(out, target{dist: stop, speed: pf.creep, why: "approaching a junction"})
	}
	return out
}

// locateLine finds the stop line ahead, by sight while it is in view and by
// odometry once it is not, and notices when the car has driven over it.
func (d *Driver) locateLine(dets []vision.Detection, speed, dt float32) (float32, bool) {
	if d.clearRun > 0 {
		d.clearRun -= speed * dt
	}
	if d.haveStopDist {
		// Carry the line forward on odometry, so it survives being hidden by
		// the car in front or passing under the bonnet.
		if d.stopDist -= speed * dt; d.stopDist < -1 {
			d.crossedLine()
		}
	}
	if dd, ok := d.stopLineDistance(dets); ok {
		d.stopDist, d.haveStopDist = dd, true
	}
	return d.stopDist, d.haveStopDist
}

// crossedLine discharges everything remembered about the junction just entered.
func (d *Driver) crossedLine() {
	d.haveStopDist = false
	d.aspect, d.sign = vision.ClassNone, vision.ClassNone
	d.stopHeld, d.stopDone = false, false
	d.waited = 0
	d.clearRun = junctionClear
}

// Signal association limits, in metres either side of the stop line. The
// primary head stands at the line and the secondary across the junction.
const (
	signalBefore float32 = 5
	signalBeyond float32 = 36
)

// nearestHead finds the nearest signal head that could govern the junction
// ahead, and how far off it is. With a stop line to pin it to, a head has to
// stand at that line or across the junction from it to count.
func (d *Driver) nearestHead(dets []vision.Detection, stopAt float32, haveStop bool) (vision.Class, float32) {
	best, bestDist := vision.ClassNone, float32(1e9)
	for _, det := range dets {
		if !det.Class.StopsTraffic() && det.Class != vision.ClassLightGreen {
			continue
		}
		dd := d.cam.RangeAtHeight(det.MaxY, vision.SignalHeadHeight)
		if dd <= 0 || dd > 95 || dd >= bestDist {
			continue
		}
		if haveStop && (dd < stopAt-signalBefore || dd > stopAt+signalBeyond) {
			continue // a head at some other junction
		}
		best, bestDist = det.Class, dd
	}
	return best, bestDist
}

// readSignal finds the signal governing the junction ahead and remembers its
// aspect. With no stop line to pin it to, the nearest head is taken to stand at
// its own.
func (d *Driver) readSignal(dets []vision.Detection, stopAt float32, haveStop bool, speed, dt float32) (float32, bool) {
	best, bestDist := d.nearestHead(dets, stopAt, haveStop)
	switch {
	case best != vision.ClassNone && haveStop:
		d.aspect, d.aspectAge = best, 0
	case best != vision.ClassNone && d.clearRun <= 0:
		// A stop line is often hidden by the car waiting on it, so range the
		// signal head itself, whose mounting height is known.
		d.aspect, d.aspectAge = best, 0
		stopAt, haveStop = bestDist+1, true
	default:
		d.forgetAspect(haveStop, speed, dt)
	}
	return stopAt, haveStop
}

// forgetAspect ages the aspect last seen while no head is in view. What was
// last seen still stands for a moment, and for much longer if the car is
// already waiting at the line.
func (d *Driver) forgetAspect(haveStop bool, speed, dt float32) {
	d.aspectAge += dt
	limit := aspectMemory
	if speed < 0.6 && d.aspect.StopsTraffic() {
		limit = aspectHold
	}
	if d.aspectAge > limit || !haveStop {
		d.aspect = vision.ClassNone
	}
}

// signReach is how far either side of the stop line a priority sign may stand
// and still be the one governing it, in metres.
const signReach float32 = 8

// readPrioritySign notices a stop or give way sign at the junction ahead.
func (d *Driver) readPrioritySign(dets []vision.Detection, stopAt float32, haveStop bool) (float32, bool) {
	for _, det := range dets {
		if det.Class != vision.ClassStopSign && det.Class != vision.ClassGiveWay {
			continue
		}
		dd := d.cam.RangeAtHeight(det.MinY, vision.SignTopHeight)
		if dd <= 0 || dd > 70 {
			continue
		}
		switch {
		case haveStop && abs(dd-stopAt) < signReach:
			d.sign = det.Class
		case !haveStop && d.clearRun <= 0:
			// The line is hidden, so the sign stands in for it.
			d.sign = det.Class
			stopAt, haveStop = dd+1, true
		}
	}
	if !haveStop {
		d.sign = vision.ClassNone
	}
	return stopAt, haveStop
}

func (d *Driver) haltAtStopSign(stopAt, speed, dt float32, pf profile) {
	// A stop sign has to be obeyed with a genuine halt, then a short pause
	// before moving off again, unless the driver is one who counts a crawl
	// over the line as stopping.
	if pf.rolling && !d.stopHeld && stopAt < stopBuffer+3 && speed < 2.2 {
		d.stopDone = true
		return
	}
	if !d.stopHeld && stopAt < stopBuffer+1.2 && speed < 0.4 {
		d.stopHeld, d.stopTimer = true, 0
	}
	if d.stopHeld {
		if d.stopTimer += dt; d.stopTimer >= stopWaitTime {
			d.stopHeld, d.stopDone = false, true
		}
	}
}

func (d *Driver) giveWay(dets []vision.Detection, lane []lanePoint, stop, stopAt, speed, dt float32) []target {
	// A give way needs no halt, only a slow approach and a stop if something
	// is crossing. A stop sign that has been obeyed is the same thing.
	out := make([]target, 0, 2)
	out = append(out, target{dist: stop, speed: giveWaySpeed, why: "give way"})
	if !d.crossingTraffic(dets, lane, stopAt) {
		d.waited = 0
		return out
	}
	if speed < 0.6 {
		d.waited += dt
	}
	if d.waited > patience {
		// Nothing has cleared, so it is probably a queue rather than traffic.
		if d.waited > patience+nudge {
			d.waited = 0
		}
		return out
	}
	return append(out, target{dist: stop, why: "giving way to traffic"})
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

// crossReach is how far beyond a give way line traffic is watched for, in
// metres. It has to take in the far side of a roundabout, because that is
// where the car that will arrive at the entry is coming from.
const crossReach float32 = 34

func (d *Driver) crossingTraffic(dets []vision.Detection, lane []lanePoint, lineDist float32) bool {
	// Anything detected off to the side at roughly the distance of the give way
	// line is traffic on the road being joined.
	for _, det := range dets {
		if !vision.IsVehicle(det.Class) {
			continue
		}
		dist, blocking := d.inPath(det, lane)
		if dist <= 0 || dist < lineDist-4 || dist > lineDist+crossReach {
			continue
		}
		if !blocking {
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

func approachSpeed(dist, final, decel float32) float32 {
	// The fastest speed from which the car can still reach `final` by the time
	// it has travelled `dist`, braking at the given rate.
	if dist <= 0.05 {
		return final
	}
	return sqrt(final*final + 2*decel*dist)
}

func committed(speed, lineDist float32) bool {
	// Reports whether the car is too close to the line to stop safely,
	// which is the only case in which crossing on amber is permitted. Anything
	// short of that gets stopped for, so the car is not still crossing when the
	// aspect turns red.
	d := max(lineDist-stopBuffer, 0.1)
	return speed*speed/(2*d) > hardStop
}

func applyLongitudinal(cmd *Command, speed, target, need float32) {
	err := target - speed
	switch {
	case err > 0.4:
		cmd.Throttle = clamp(err/5, 0.12, 1)
	case err < -0.25:
		// Scale braking with how far over the target we are.
		cmd.Brake = clamp(-err/(hardStop*0.6), 0.06, 1)
	default:
		cmd.Throttle = clamp(err, 0, 0.25)
	}
	// Riding the braking curve down to a stop, the error stays near zero while
	// the brakes still have real work to do, so feed in what the curve needs.
	if err < 0.3 && need > 0.6 {
		cmd.Throttle = 0
		cmd.Brake = max(cmd.Brake, clamp(need/brakeDecel, 0, 1))
	}
	if target < 0.3 && speed < 1.2 {
		cmd.Throttle, cmd.Brake = 0, 1 // hold firmly at a standstill
	}
}

func wrapPi(a float32) float32 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a < -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

func clamp(v, lo, hi float32) float32 { return min(max(v, lo), hi) }
func abs(v float32) float32           { return float32(math.Abs(float64(v))) }
func sqrt(v float32) float32          { return float32(math.Sqrt(float64(v))) }
func tan(v float32) float32           { return float32(math.Tan(float64(v))) }
