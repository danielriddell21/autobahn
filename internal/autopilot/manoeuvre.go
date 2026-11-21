package autopilot

import "github.com/danielriddell21/autobahn/internal/vision"

// Lane changes and overtakes, from the camera and the mirrors alone.
//
// The lane markers the car steers by are its own lane. To move over, it steers
// along them with a sideways shift, and once the planned route has followed it
// into the next lane the markers jump across by a lane's width: the shift is
// taken back by the same amount, so the car carries on exactly where it was.
// Passing on a single carriageway is the same shift with nothing to jump to,
// held until the car passed is behind, then let go.
//
// What it moves into is judged from what it can see: the markers for the lanes
// alongside, vehicles ahead in the bonnet camera, and vehicles behind and
// beside in the mirrors.

// laneWidth is the width of a British lane, which is calibration: a driver
// knows roughly how far across the next lane is.
const laneWidth float32 = 3.6

// Lane change timing and room, in seconds and metres.
const (
	// signalFor is how long the car indicates before it moves across.
	signalFor float32 = 1.0
	// shiftRate is how fast the car drifts across into the next lane, m/s, and
	// passRate how fast it pulls out to overtake, which is done more briskly.
	shiftRate float32 = 1.5
	passRate  float32 = 2.3
	// tightPath is how far either side of the path being driven another car's
	// edge has to be to be out of the way while moving across: a car's
	// half-width and a little. The ordinary margin is wider, and would keep
	// the car braking for the very one it is pulling out round.
	tightPath float32 = 1.25
	// laneHalf is how far from a lane's middle the middle of a car may be and
	// still be in it, and laneEdge how far into a lane from its middle a car's
	// edge has to reach to be in the way there: a car in the next lane over
	// may come up to the line between them.
	laneHalf float32 = 1.9
	laneEdge float32 = 1.4
	// sideAhead and sideBehind are the room wanted in a lane before moving
	// into it, ahead and behind, before speed and the driver's own margin.
	sideAhead  float32 = 14
	sideBehind float32 = 9
	// straightFor is how far ahead the road has to be seen running straight
	// for a move to be started. The car being followed hides the markers
	// beyond it, so this is short; sideReach, how far the lane alongside has to
	// be seen to go on, says the road continues. Its markers stop where the
	// road does. junctionClear is the nearest a known stop line may be.
	straightFor   float32 = 12
	sideReach     float32 = 30
	junctionAhead float32 = 30
	// moveTime is roughly how long a lane change takes from the first flash of
	// the indicator to being in the next lane, which is how much road it needs.
	moveTime float32 = 2.8
	// sideLost is how little of the lane alongside may be left before a move
	// into it is called off.
	sideLost float32 = 10
	// moveSpeed is the slowest the car changes lane at. In a queue the gaps
	// alongside open and shut faster than the mirrors can be trusted with.
	moveSpeed float32 = 5
	// heldRange is how close a slower car has to be to hold the driver up.
	heldRange float32 = 40
	// jumpAt is how far the lane markers have to move at once to count as the
	// route having followed the car into another lane.
	jumpAt float32 = 2.4
)

// Overtaking on a single carriageway, in metres and m/s.
const (
	// passBoost is how much quicker than the car being passed the autopilot
	// goes while passing it.
	passBoost float32 = 4.5
	// passClearRange is as far as the bonnet camera can be trusted to see an
	// oncoming car, which bounds how long an overtake may take.
	passClearRange float32 = 82
	// passFrom is the furthest back from the car in front an overtake is
	// planned from: anyone further back closes up first.
	passFrom float32 = 12
	// passBack is how far behind the car passed has to be before pulling in.
	passBack float32 = 7
)

// move is a lane change or overtake in progress.
type move int

const (
	moveNone move = iota
	moveSignal
	moveShift
	movePass
	moveAbort
)

// Indicator is the turn signal the autopilot wants shown.
type Indicator int

// The indicator states.
const (
	IndicateOff Indicator = iota
	IndicateLeft
	IndicateRight
)

// sideLanes is what the bonnet camera shows of the lanes beside the car's own:
// how far across each one's middle is, and how far ahead it can be seen.
type sideLanes struct {
	left, right         float32 // offset of a lane alongside going our way; zero if none
	leftReach, rghReach float32
	oncoming, onReach   float32 // offset of the oncoming lane; zero if none
}

// readSides finds the lanes alongside from their markers, relative to the
// car's own lane.
func (d *Driver) readSides(dets []vision.Detection, raw []lanePoint) sideLanes {
	var s sideLanes
	for _, det := range dets {
		if det.Class != vision.ClassLaneSide && det.Class != vision.ClassLaneOncoming {
			continue
		}
		dist := d.cam.GroundDistance(det.MaxY)
		if dist <= 0.5 || dist > 60 {
			continue
		}
		off := dist*tan(d.cam.Bearing(det.CenterX())) - laneLateralAt(raw, dist)
		if abs(off) < laneWidth*0.6 || abs(off) > laneWidth*1.5 {
			continue
		}
		switch {
		case det.Class == vision.ClassLaneOncoming:
			s.oncoming, s.onReach = off, max(s.onReach, dist)
		case off > 0:
			s.right, s.rghReach = off, max(s.rghReach, dist)
		default:
			s.left, s.leftReach = off, max(s.leftReach, dist)
		}
	}
	return s
}

// rememberSides carries how far each lane alongside goes on from one look to
// the next, by the car's own progress. A marker is easily hidden for a frame by
// the outline of the car in front, and the end of a lane does not move.
func (d *Driver) rememberSides(s sideLanes, speed, dt float32) sideLanes {
	carry := func(mem *float32, seen float32) float32 {
		*mem = max(*mem-speed*dt, seen, 0)
		return *mem
	}
	s.leftReach = carry(&d.reach[0], s.leftReach)
	s.rghReach = carry(&d.reach[1], s.rghReach)
	s.onReach = carry(&d.reach[2], s.onReach)
	return s
}

// shiftLane follows the lane markers with the car's sideways shift, and notices
// when the route has followed the car into the next lane.
func (d *Driver) shiftLane(raw []lanePoint, dt float32) []lanePoint {
	if len(raw) == 0 {
		return raw
	}
	here := laneLateralAt(raw, 6)
	if jump := here - d.lastLane; d.haveLast && abs(jump) > jumpAt && abs(jump) < laneWidth*1.6 &&
		abs(d.shift) > 0.5 && (jump > 0) == (d.shift > 0) {
		// The markers are now the lane moved into. Keep the car where it is.
		d.shift -= jump
		d.shiftWant -= jump
		if d.move == moveShift {
			d.shiftWant = 0
			d.move = moveNone
		}
	}
	d.lastLane, d.haveLast = here, true
	rate := shiftRate
	if d.passing {
		rate = passRate
	}
	d.shift = toward(d.shift, d.shiftWant, rate*dt)
	if d.shift == 0 {
		return raw
	}
	out := d.shifted[:0]
	for _, p := range raw {
		out = append(out, lanePoint{dist: p.dist, lateral: p.lateral + d.shift})
	}
	d.shifted = out
	return out
}

// trackLead keeps an estimate of how fast the car in front is going, from how
// its range changes against the car's own speed. Frames repeat between looks,
// so the estimate is smoothed over several.
func (d *Driver) trackLead(dist float32, seen bool, speed, dt float32) {
	if !seen {
		d.leadSeen = false
		return
	}
	if d.leadSeen && abs(dist-d.leadDist) < 6 && dt > 0 {
		est := speed + (dist-d.leadDist)/dt
		d.leadSpeed += (est - d.leadSpeed) * clamp(dt/0.6, 0, 1)
	} else {
		d.leadSpeed = speed
	}
	d.leadDist, d.leadSeen = dist, true
}

// nearestInPath is the range to the nearest vehicle on the path the car is
// steering along.
func (d *Driver) nearestInPath(dets []vision.Detection, lane []lanePoint) (float32, bool) {
	best, found := float32(1e9), false
	for _, det := range dets {
		if !vision.IsVehicle(det.Class) {
			continue
		}
		if dist, in := d.inPath(det, lane); in && dist < best {
			best, found = dist, true
		}
	}
	return best, found
}

// laneOccupied reports whether the bonnet camera shows a vehicle in the lane
// whose middle is off metres across from the car's own, between near and far.
func (d *Driver) laneOccupied(dets []vision.Detection, raw []lanePoint, off, near, far float32) bool {
	for _, det := range dets {
		if !vision.IsVehicle(det.Class) {
			continue
		}
		dist := d.cam.GroundDistance(det.MaxY)
		if dist < near || dist > far {
			continue
		}
		lo, hi := det.Base()
		mid := laneLateralAt(raw, dist) + off
		left, right := dist*tan(d.cam.Bearing(lo)), dist*tan(d.cam.Bearing(hi))
		if left < mid+laneEdge && right > mid-laneEdge {
			return true
		}
	}
	return false
}

// mirrorOccupied reports whether the mirrors show a vehicle in the lane whose
// middle is at the given offset to the right of the car, from behind metres
// back to ahead metres forward.
func (d *Driver) mirrorOccupied(mid, behind, ahead float32) bool {
	for _, seen := range d.around {
		for _, n := range seen {
			if abs(n.Right-mid) < laneHalf && n.Ahead > -behind && n.Ahead < ahead {
				return true
			}
		}
	}
	return false
}

// roomIn reports whether a lane alongside has room to move into now.
func (d *Driver) roomIn(dets []vision.Detection, raw []lanePoint, off, speed float32, pf profile) bool {
	ahead := (sideAhead + speed*0.4) * pf.laneGap
	behind := (sideBehind + speed*0.9) * pf.laneGap
	// The middle of that lane, measured from the car rather than its own lane.
	mid := laneLateralAt(raw, 0) + off
	return !d.laneOccupied(dets, raw, off, 0, ahead) && !d.mirrorOccupied(mid, behind, 6)
}

// straightAhead reports whether the lane runs straight for long enough to
// start a manoeuvre, with no junction known to be coming.
func (d *Driver) straightAhead(raw []lanePoint) bool {
	if len(raw) < 2 || raw[len(raw)-1].dist < straightFor {
		return false
	}
	if d.haveStopDist && d.stopDist < junctionAhead {
		return false
	}
	for _, p := range raw {
		if p.dist <= straightFor*3 && abs(p.lateral-raw[0].lateral) > 1.2 {
			return false
		}
	}
	return true
}

// manoeuvre decides on lane changes and overtakes, and returns the speed the
// car should now aim for.
func (d *Driver) manoeuvre(dets []vision.Detection, raw []lanePoint, cruise, speed, dt float32, pf profile) float32 {
	d.speed = speed
	dist, ahead := d.nearestInPath(dets, d.path)
	d.trackLead(dist, ahead, speed, dt)
	held := ahead && dist < heldRange && d.leadSpeed < cruise*(1-mixf(0.5, 0.08, pf.overtake))
	if held {
		d.heldUp += dt
	} else {
		d.heldUp = max(d.heldUp-2*dt, 0)
	}
	sides := d.rememberSides(d.readSides(dets, raw), speed, dt)

	switch d.move {
	case moveSignal:
		d.signalled(dets, raw, sides, speed, dt, pf)
		return cruise
	case moveShift:
		// The route should have followed the car over by now. If it has not,
		// or the lane alongside is running out, the lane did not go where it
		// looked like going: drift back.
		d.moveTimer += dt
		if d.moveTimer > 6 || d.sideReach(sides, d.moveSide) < sideLost {
			d.endMove()
		}
		return cruise
	case movePass, moveAbort:
		return d.carryOnPassing(dets, raw, cruise, pf)
	}

	if len(raw) == 0 || pf.overtake <= 0 || speed < moveSpeed || !d.straightAhead(raw) {
		return cruise
	}
	d.chooseLane(dets, raw, sides, held, cruise, dt, pf)
	return cruise
}

// signalled counts down the indicator, then moves across if there is room.
func (d *Driver) signalled(dets []vision.Detection, raw []lanePoint, sides sideLanes, speed, dt float32, pf profile) {
	if d.signalTimer += dt; d.signalTimer < signalFor {
		return
	}
	if speed < moveSpeed || !d.roomIn(dets, raw, d.moveSide, speed, pf) {
		if d.signalTimer > signalFor+5 {
			d.endMove()
		}
		return
	}
	if d.sideReach(sides, d.moveSide) < speed*moveTime*0.6 {
		d.endMove() // too late now: the lane alongside is ending
		return
	}
	d.shiftWant, d.move, d.moveTimer = d.moveSide, moveShift, 0
	if d.passing {
		d.move = movePass
	}
}

// chooseLane picks a lane change or overtake to start, if any.
func (d *Driver) chooseLane(dets []vision.Detection, raw []lanePoint, sides sideLanes, held bool, cruise, dt float32, pf profile) {
	fedUp := held && d.heldUp > pf.patience
	// Enough of the lane alongside left to get into it.
	room := max(sideReach, d.speed*moveTime)
	left := sides.left != 0 && sides.leftReach > room
	if fedUp && d.passBy(dets, raw, sides, room, cruise, pf) {
		return
	}
	if !left || held {
		d.outside = 0
		return
	}
	// Sat out in an outer lane with nothing to pass: back in, in time.
	if d.outside += dt; d.outside > pf.keepLeft {
		d.startMove(sides.left, false)
	}
}

// passBy starts getting past the car in front, by whichever way this driver
// will take and the road offers, and reports whether it did.
func (d *Driver) passBy(dets []vision.Detection, raw []lanePoint, sides sideLanes, room, cruise float32, pf profile) bool {
	switch {
	case sides.right != 0 && sides.rghReach > room:
		d.startMove(sides.right, false)
	case pf.undertake && sides.left != 0 && sides.leftReach > room &&
		!d.laneOccupied(dets, raw, sides.left, 0, heldRange):
		d.startMove(sides.left, false)
	case pf.oncoming && sides.oncoming != 0 && sides.onReach > room &&
		d.passIsClear(dets, raw, sides.oncoming, d.passSpeed(cruise), pf):
		d.startMove(sides.oncoming, true)
	default:
		return false
	}
	return true
}

// passIsClear reports whether the other side of a single carriageway is empty
// for as far as an overtake of the car in front would need it.
func (d *Driver) passIsClear(dets []vision.Detection, raw []lanePoint, off, pass float32, pf profile) bool {
	cover := min(d.leadDist, passFrom) + 2*4.5 + passBack
	takes := cover/max(pass-d.leadSpeed, 1) + 1
	// Whatever is coming the other way is taken to be doing the limit. How
	// much of a margin on top is enough is the driver's own business.
	need := (takes*(pass+d.SeenLimit) + 10) * pf.laneGap
	if need > passClearRange {
		return false
	}
	// Nothing coming, and nobody already out there passing us.
	return !d.laneOccupied(dets, raw, off, 0, need) && !d.mirrorOccupied(laneLateralAt(raw, 0)+off, 20, 6)
}

// sideReach is how far ahead the lane on the given side can be seen to go.
func (d *Driver) sideReach(s sideLanes, side float32) float32 {
	switch {
	case d.passing:
		return s.onReach
	case side > 0:
		return s.rghReach
	default:
		return s.leftReach
	}
}

// startMove signals for a move to the lane on the side of off. The lane is
// taken to be a lane's width across, whatever the markers made it: their
// range is coarse, and the width of a lane is not.
func (d *Driver) startMove(off float32, passing bool) {
	side := laneWidth
	if off < 0 {
		side = -laneWidth
	}
	d.move, d.moveSide, d.signalTimer, d.passing = moveSignal, side, 0, passing
	d.outside = 0
}

// endMove forgets the manoeuvre and lets the car settle where it is.
func (d *Driver) endMove() {
	d.move, d.passing, d.shiftWant = moveNone, false, 0
	d.heldUp, d.outside = 0, 0
}

// carryOnPassing holds an overtake on a single carriageway until the car passed
// is well behind, and gives it up if something appears coming the other way.
func (d *Driver) carryOnPassing(dets []vision.Detection, raw []lanePoint, cruise float32, pf profile) float32 {
	home := -d.shift // where the car's own lane is, from here
	passedDist, stillAhead := d.passedCar(dets, home)
	alongside := d.mirrorOccupied(home, passBack, 4)
	if !stillAhead && !alongside {
		d.endMove()
		return cruise
	}
	// Something coming the other way, beyond the car being passed, or a
	// junction turning up: either way there is no longer room to finish.
	near := float32(4)
	if stillAhead {
		near = passedDist + 5
	}
	blocked := d.laneOccupied(dets, raw, d.moveSide, near, passClearRange*0.75) ||
		d.haveStopDist && d.stopDist < 20
	if d.move == movePass && blocked && stillAhead {
		d.move = moveAbort
	}
	if d.move == moveAbort {
		// Drop back behind the car that was being passed, and pull in once
		// there is room.
		if stillAhead && passedDist > pf.followGap {
			d.shiftWant = 0
			d.endMove()
			return cruise
		}
		return max(d.leadSpeed-3, 0)
	}
	return d.passSpeed(cruise)
}

// passedCar finds the car being passed while the bonnet camera still has it:
// the nearest one in the car's own lane, which is home metres across.
func (d *Driver) passedCar(dets []vision.Detection, home float32) (float32, bool) {
	best, found := float32(30), false
	for _, det := range dets {
		if !vision.IsVehicle(det.Class) {
			continue
		}
		dist := d.cam.GroundDistance(det.MaxY)
		lo, hi := det.Base()
		mid := laneLateralAt(d.path, dist) + home
		if dist < best && dist*tan(d.cam.Bearing(lo)) < mid+laneEdge && dist*tan(d.cam.Bearing(hi)) > mid-laneEdge {
			best, found = dist, true
		}
	}
	return best, found
}

// passSpeed is the speed an overtake is made at: quick enough to get by, but
// not much over what the driver would do anyway.
func (d *Driver) passSpeed(cruise float32) float32 {
	return min(max(cruise, d.leadSpeed+passBoost), cruise*1.25)
}

// describeMove reports a manoeuvre in progress on the HUD.
func (d *Driver) describeMove() {
	switch {
	case d.move == movePass:
		d.State, d.Reason = StateOvertaking, "passing on the other side"
	case d.move == moveAbort:
		d.State, d.Reason = StateOvertaking, "dropping back in"
	case d.move == moveSignal && d.passing:
		d.State, d.Reason = StateOvertaking, "looking to pass"
	case d.move == moveSignal, d.move == moveShift:
		d.State, d.Reason = StateChangingLane, "changing lane"
	}
}

// indicator is the signal for whatever manoeuvre is in progress.
func (d *Driver) indicator() Indicator {
	side := d.moveSide
	switch {
	case d.move == moveNone && abs(d.shift) > 0.8:
		side = -d.shift // pulling back in
	case d.move == moveNone, d.move == moveAbort:
		return IndicateOff
	}
	if side > 0 {
		return IndicateRight
	}
	return IndicateLeft
}

func toward(v, want, step float32) float32 {
	switch {
	case v < want:
		return min(v+step, want)
	case v > want:
		return max(v-step, want)
	}
	return v
}

func mixf(a, b, t float32) float32 { return a + (b-a)*t }
