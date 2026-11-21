package sim

import (
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Lane changes and overtakes.
//
// On a road with more than one lane each way a driver picks a lane: the one
// their next turn is made from, the outer one to get past somebody slower, and
// back to the nearside once nothing is holding them out there. On a single
// carriageway the bold ones pass by borrowing the other side of the road, when
// it is straight, empty and long enough.
//
// Every change is signalled first, and only made into a gap the driver is happy
// with. How long they signal, how big a gap they want and how soon they go back
// in are all theirs: see [Style].

// Lane-change geometry and timing, in metres and seconds.
const (
	// laneChangeEnd is how close to the end of a lane a driver stops changing
	// lane for anything but the turn, and turnLaneEnd for the turn too: the
	// last stretch before a junction is for being in the lane already.
	laneChangeEnd float32 = 24
	turnLaneEnd   float32 = 10
	// laneShift is how fast a driver drifts across into the new lane, in m/s.
	laneShift float32 = 1.6
	// laneSettle is how long a lane change is treated as under way.
	laneSettle float32 = 2.6
	// laneCooldown is how long after one change a driver waits before
	// considering another, so nobody weaves.
	laneCooldown float32 = 4
	// laneScan is how far along the target lane, either way, a driver looks
	// before moving into it.
	laneScan float32 = 70
	// signalGiveUp is how long a driver signals for a gap before giving up.
	signalGiveUp float32 = 5
)

// Overtaking on a single carriageway.
const (
	// passReturnGap is how far ahead of the car passed a driver gets before
	// pulling back in, beyond the two cars' lengths.
	passReturnGap float32 = 5
	// passLongest is the longest an overtake may be planned to take. Anything
	// that would take longer is not worth starting.
	passLongest float32 = 8
	// passMargin is the clear road wanted beyond what the pass itself needs,
	// in seconds of closing on oncoming traffic.
	passMargin float32 = 3
	// passBoost is how much quicker than the car being passed a driver goes,
	// in m/s, however low the limit.
	passBoost float32 = 4.5
)

// sibling returns the lane beside l going the same way with the given index,
// or nil.
func sibling(c *city.City, l *city.Lane, idx int) *city.Lane {
	r := c.Roads[l.Road]
	ids := r.LanesAB
	if l.Dir < 0 {
		ids = r.LanesBA
	}
	for _, id := range ids {
		if c.Lanes[id].Index == idx {
			return c.Lanes[id]
		}
	}
	return nil
}

// oncoming returns the lane nearest the centreline going the other way along
// the same road, or nil.
func oncoming(c *city.City, l *city.Lane) *city.Lane {
	r := c.Roads[l.Road]
	ids := r.LanesBA
	if l.Dir < 0 {
		ids = r.LanesAB
	}
	for _, id := range ids {
		if c.Lanes[id].Index == 0 {
			return c.Lanes[id]
		}
	}
	return nil
}

// serves reports whether a turn of the given kind can be made from a lane.
// A lane whose junction is not wired yet has no successors to ask, and is
// taken to serve anything.
func serves(l *city.Lane, k city.TurnKind) bool {
	if len(l.Succ) == 0 {
		return true
	}
	for _, t := range l.Succ {
		if t.Kind == k {
			return true
		}
	}
	return false
}

// towardCentre is the side the centreline is on, as a sidestep sign: positive
// is to the driver's right.
func towardCentre() float32 {
	if city.DriveOnLeft {
		return 1
	}
	return -1
}

// sideIndicator returns the indicator for a move toward the given side, as a
// sidestep sign.
func sideIndicator(side float32) Indicator {
	if side > 0 {
		return IndicatorRight
	}
	return IndicatorLeft
}

// signalTime is how long a driver indicates before moving across.
func (s Style) signalTime() float32 {
	return mathx.Lerp(1.6, 0.4, s.Temper)
}

// keepLeftDelay is how long a driver sits in an outer lane with nothing to
// pass before moving back in.
func (s Style) keepLeftDelay() float32 {
	return 2 + (1-s.KeepLeft)*14
}

// manoeuvre decides on lane changes and overtakes. It runs once a step, after
// the car in front has been found.
func (a *Agent) manoeuvre(w *World, st Style, ahead *Vehicle, gap, lead, speed, want, dt float32) {
	a.laneCool = max(a.laneCool-dt, 0)
	a.trackHeldUp(st, ahead != nil && gap < 45, lead, want, dt)
	switch {
	case a.pursuing:
		a.clearManoeuvre()
		return
	case a.overtaking != nil:
		a.carryOnPassing(w, speed, dt)
		return
	}
	c := w.City
	if a.inTurn || c.Roads[a.lane.Road].Ring {
		a.pending = nil
		return
	}
	left := a.lane.Length - a.s
	if a.pending != nil {
		a.signalLane(w, st, speed, left, dt)
		return
	}
	if a.passing > 0 || a.changing > 0 || a.laneCool > 0 || a.s < 4 {
		return
	}
	if lanes := c.Roads[a.lane.Road].Class.Lanes(); lanes > 1 {
		a.chooseLane(w, st, ahead, lead, want, left, lanes, dt)
		return
	}
	if st.PassesOncoming && ahead != nil && a.heldUp > st.Patience {
		a.tryPassOncoming(w, st, ahead, gap, lead, speed, want, left)
	}
}

// trackHeldUp keeps count of how long the driver has been stuck behind
// somebody slower than they want to go.
func (a *Agent) trackHeldUp(st Style, close bool, lead, want, dt float32) {
	if close && st.WantsPast(want, lead) {
		a.heldUp += dt
	} else {
		a.heldUp = max(a.heldUp-2*dt, 0)
	}
}

// moveForTurn signals for the lane the next turn is made from, if this is not
// it, and reports whether it did.
func (a *Agent) moveForTurn(c *city.City, left float32, lanes int) bool {
	if !a.hasWant || serves(a.lane, a.wantKind) || left <= turnLaneEnd {
		return false
	}
	for i := range lanes {
		if to := sibling(c, a.lane, i); to != nil && serves(to, a.wantKind) {
			a.requestLane(c, i)
			return true
		}
	}
	return false
}

// chooseLane picks the lane to be in on a road with more than one each way.
func (a *Agent) chooseLane(w *World, st Style, ahead *Vehicle, lead, want, left float32, lanes int, dt float32) {
	c := w.City
	idx, kerb := a.lane.Index, lanes-1
	// The lane for the turn comes first, and may be moved into later than
	// anything else.
	if a.moveForTurn(c, left, lanes) || left < laneChangeEnd {
		return
	}
	held := ahead != nil && a.heldUp > st.Patience
	switch {
	case held && idx > 0 && a.keepsTurn(c, idx-1):
		// Out to pass.
		if a.laneLeadSpeed(w, sibling(c, a.lane, idx-1)) > lead+1 {
			a.requestLane(c, idx-1)
		}
	case held && idx < kerb && st.Undertakes && a.keepsTurn(c, idx+1):
		// The outer lane is the slow one: go by on the inside.
		if a.laneLeadSpeed(w, sibling(c, a.lane, idx+1)) > lead+2 {
			a.requestLane(c, idx+1)
		}
	case idx < kerb:
		a.keepLeft(c, st, ahead != nil && st.WantsPast(want, lead), dt)
	}
}

// keepLeft moves a driver back toward the nearside once they have sat out
// with nothing to pass for as long as their discipline allows.
func (a *Agent) keepLeft(c *city.City, st Style, passing bool, dt float32) {
	if passing {
		a.outside = 0
		return
	}
	if a.outside += dt; a.outside > st.keepLeftDelay() && a.keepsTurn(c, a.lane.Index+1) {
		a.requestLane(c, a.lane.Index+1)
	}
}

// keepsTurn reports whether moving to lane idx still lets the driver make the
// turn they mean to.
func (a *Agent) keepsTurn(c *city.City, idx int) bool {
	to := sibling(c, a.lane, idx)
	return to != nil && (!a.hasWant || serves(to, a.wantKind))
}

// laneLeadSpeed is the speed of the nearest car ahead in another lane, or a
// large number if the lane is clear.
func (a *Agent) laneLeadSpeed(w *World, to *city.Lane) float32 {
	if to == nil {
		return 0
	}
	best, speed := laneScan, float32(1e6)
	w.eachVehicle(a, func(v *Vehicle) {
		along, lat := laneFrame(to, a.V.Pos, v.Pos)
		if along > 0 && along < best && mathx.Abs(lat) < LaneHalf {
			best, speed = along, max(v.Vel.Dot(to.DirAt(to.Along(v.Pos))), 0)
		}
	})
	return speed
}

// LaneHalf is how far either side of a lane's line a car counts as in it.
const LaneHalf = city.LaneWidth * 0.6

// laneFrame measures q against a lane: how far ahead of p it is along the
// lane, and how far to the right of the lane's line it sits.
func laneFrame(l *city.Lane, p, q mathx.Vec) (along, lat float32) {
	_, lat = l.Locate(q)
	return l.Run(q) - l.Run(p), lat
}

// eachVehicle calls f with the player's car and every agent but a.
func (w *World) eachVehicle(a *Agent, f func(*Vehicle)) {
	f(w.Player)
	for _, o := range w.Agents {
		if o != a {
			f(o.V)
		}
	}
}

// requestLane starts signalling for a move to lane idx.
func (a *Agent) requestLane(c *city.City, idx int) {
	if to := sibling(c, a.lane, idx); to != nil {
		a.pending, a.signalFor = to, 0
	}
}

// clearManoeuvre forgets any lane change or overtake, for a car that has been
// put somewhere new or taken off its route.
func (a *Agent) clearManoeuvre() {
	a.pending, a.overtaking, a.aborting = nil, nil, false
	a.changing, a.outside, a.heldUp, a.passFor = 0, 0, 0, 0
	a.seen, a.lastAhead, a.letting, a.asked = nil, nil, nil, nil
	a.distracted, a.greenWait, a.pause, a.atLine = 0, 0, 0, false
}

// signalLane counts down the signal, then moves across if the gap is there.
func (a *Agent) signalLane(w *World, st Style, speed, left, dt float32) {
	to := a.pending
	if to.Road != a.lane.Road || to.Dir != a.lane.Dir || left < turnLaneEnd {
		a.pending = nil
		return
	}
	a.signalFor += dt
	if a.signalFor < st.signalTime() {
		return
	}
	if w.laneClear(a, to, st, speed) {
		a.switchLane(to)
		return
	}
	if a.signalFor > st.signalTime()+signalGiveUp {
		a.pending, a.laneCool = nil, laneCooldown/2
	}
}

// laneClear reports whether lane to has room for this driver to move into it
// now: nothing alongside, and enough of a gap to whatever is ahead and behind
// that nobody has to brake hard. How much counts as enough is the driver's.
func (w *World) laneClear(a *Agent, to *city.Lane, st Style, speed float32) bool {
	clear := true
	w.eachVehicle(a, func(v *Vehicle) {
		if !clear {
			return
		}
		along, lat := laneFrame(to, a.V.Pos, v.Pos)
		if mathx.Abs(lat) > LaneHalf+v.Spec.HalfWidth*0.5 || mathx.Abs(along) > laneScan {
			return
		}
		lengths := a.V.Spec.HalfLength + v.Spec.HalfLength
		vs := v.Vel.Dot(to.DirAt(to.Along(v.Pos)))
		var need float32
		if along >= 0 {
			need = st.MinGap + max(speed-vs, 0)*1.2 + speed*0.3
		} else {
			// Behind: whoever is there must not have to brake for us.
			need = st.MinGap + max(vs-speed, 0)*1.8 + max(vs, 0)*0.5
		}
		if mathx.Abs(along) < need*st.LaneGap+lengths {
			clear = false
		}
	})
	return clear
}

// switchLane moves the agent onto another lane of the same carriageway. The
// car stays where it is: its sidestep takes up the difference, and eases away
// to nothing as it drifts across.
func (a *Agent) switchLane(to *city.Lane) {
	_, lat := laneFrame(to, a.V.Pos, a.V.Pos)
	// A car to the right of its new lane is moving left, and the other way.
	a.changeSide = 1
	if lat > 0 {
		a.changeSide = -1
	}
	a.lane = to
	a.s = to.Along(a.V.Pos)
	a.sidestep = lat
	a.changing, a.pending, a.laneCool = laneSettle, nil, laneCooldown
	a.outside, a.stuck = 0, 0
	a.repickTurn()
}

// repickTurn chooses the turn from a lane just changed into: the one meant all
// along where this lane makes it, and otherwise carrying on.
func (a *Agent) repickTurn() {
	a.waited = 0
	if len(a.lane.Succ) == 0 {
		a.hasNext = false
		return
	}
	a.hasNext = true
	if a.hasWant {
		for _, t := range a.lane.Succ {
			if t.Kind == a.wantKind {
				a.next = t
				return
			}
		}
	}
	a.next, _ = onward(a.lane)
}

// tryPassOncoming starts an overtake on a single carriageway if the road is
// long enough and the other side of it clear enough.
func (a *Agent) tryPassOncoming(w *World, st Style, ahead *Vehicle, gap, lead, speed, want, left float32) {
	c := w.City
	if lead < 1.5 || oncoming(c, a.lane) == nil {
		return // a queue, or a stopped car: goRound deals with those
	}
	pass := min(max(want, lead+passBoost), a.V.Spec.MaxSpeed)
	closing := pass - lead
	if closing < 2 {
		return
	}
	cover := gap + 2*(a.V.Spec.HalfLength+ahead.Spec.HalfLength) + passReturnGap
	// Starting from the speed of the car in front, so a little longer than the
	// closing speed alone says.
	takes := cover/closing + (pass-speed)/(2*st.Accel)
	if takes > passLongest {
		return
	}
	travel := takes * (speed + pass) / 2
	if left < travel+laneChangeEnd {
		return
	}
	if !w.otherSideClear(a, travel, takes+passMargin, a.lane.SpeedLimit) {
		return
	}
	a.overtaking, a.passFor = ahead, takes*1.8+3
	a.heldUp = 0
}

// otherSideClear reports whether the other side of the road ahead is empty for
// as far as an overtake needs it: the distance the pass covers, plus however
// far anything coming the other way would travel meanwhile.
func (w *World) otherSideClear(a *Agent, travel, takes, limit float32) bool {
	fwd, right := a.V.Forward(), a.V.Forward().Right()
	side := towardCentre()
	clear := true
	w.eachVehicle(a, func(v *Vehicle) {
		if !clear {
			return
		}
		rel := v.Pos.Sub(a.V.Pos)
		along, lat := rel.Dot(fwd), rel.Dot(right)*side
		if lat < 1.4 || lat > city.LaneWidth*1.9 {
			return
		}
		toward := max(-v.Vel.Dot(fwd), 0)
		if v.Forward().Dot(fwd) < 0 {
			toward = max(toward, limit) // assume it is doing the limit
		}
		if along > -a.V.Spec.HalfLength*2 && along < travel+toward*takes+10 {
			clear = false
		}
	})
	return clear
}

// carryOnPassing holds an overtake on a single carriageway until the car being
// passed is behind, or gives it up if the road stops being clear.
func (a *Agent) carryOnPassing(w *World, speed, dt float32) {
	o := a.overtaking
	a.passFor -= dt
	fwd := a.V.Forward()
	along := o.Pos.Sub(a.V.Pos).Dot(fwd)
	lengths := a.V.Spec.HalfLength + o.Spec.HalfLength
	if along < -(lengths + passReturnGap) {
		// Past, with room: back in.
		a.endPass(false)
		return
	}
	left := a.lane.Length - a.s
	if a.inTurn || left < laneChangeEnd/2 || a.passFor <= 0 {
		a.endPass(along > -lengths)
		return
	}
	// Something has appeared coming the other way. Ahead of the car being
	// passed there is no room to get back in, so the pass is abandoned and the
	// driver drops back behind it.
	remain := max(along+lengths+passReturnGap, 0)
	takes := remain/max(speed-max(o.Vel.Dot(fwd), 0), 1.5) + 1
	if !a.aborting && !w.otherSideClear(a, takes*speed, takes+1, a.lane.SpeedLimit) && along > 0 {
		a.aborting = true
	}
	if a.aborting && along > lengths+1.5 {
		a.endPass(false)
	}
}

// endPass finishes an overtake. A driver who is still alongside cannot pull in
// yet, so they fall back first.
func (a *Agent) endPass(alongside bool) {
	if alongside {
		a.aborting = true
		return
	}
	a.overtaking, a.aborting = nil, false
	a.changing, a.changeSide = laneSettle, -towardCentre()
	a.laneCool, a.heldUp = laneCooldown, 0
}

// passSpeed is the speed a driver wants while overtaking: brisk enough to get
// by, and well back off it while dropping back behind.
func (a *Agent) passSpeed(want float32) float32 {
	o := a.overtaking
	lead := max(o.Vel.Dot(a.V.Forward()), 0)
	if a.aborting {
		return max(lead-3, 0)
	}
	return min(max(want, lead+passBoost), a.V.Spec.MaxSpeed)
}

// passSidestep is how far out a driver sits while overtaking on a single
// carriageway: the middle of the other lane.
func (a *Agent) passSidestep() float32 {
	return towardCentre() * city.LaneWidth
}

// laneIndicator is the indicator for a lane change or overtake in progress, if
// one is.
func (a *Agent) laneIndicator() (Indicator, bool) {
	switch {
	case a.overtaking != nil && !a.aborting:
		return sideIndicator(towardCentre()), true
	case a.pending != nil && a.lane != nil:
		side := towardCentre()
		if a.pending.Index > a.lane.Index {
			side = -side
		}
		return sideIndicator(side), true
	case a.changing > laneSettle/2:
		return sideIndicator(a.changeSide), true
	}
	return IndicatorOff, false
}
