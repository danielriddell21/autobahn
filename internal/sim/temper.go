package sim

import (
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// The human side of driving: nobody reacts the instant the car in front
// brakes, holds the dead centre of the lane, or treats everyone else on the
// road the same way. How much of each a driver has comes from their [Style].

// Perception and attention.
const (
	// distractRate is how often a driver with no attention at all looks away
	// from the road, per second; an attentive one does it proportionally less.
	distractRate float32 = 0.25
	// distractShortest and distractLongest bound how long a glance away lasts.
	distractShortest float32 = 0.5
	distractLongest  float32 = 1.4
	// wanderReach is how far an entirely inattentive driver drifts either side
	// of the middle of their lane, in metres.
	wanderReach float32 = 1.1
	// closeCall is the time to collision at which any driver, however
	// distracted, reacts at once.
	closeCall float32 = 1.6
)

// Temper and courtesy.
const (
	// pressHeadway is how much of their usual time gap the angriest driver
	// gives up once they have been held up past their patience.
	pressHeadway float32 = 0.4
	// letOutRoom is how far short of the junction a courteous driver stops to
	// leave a gap for somebody waiting to pull out, and letOutFor how long
	// they will hold it.
	letOutRoom float32 = 12
	letOutFor  float32 = 8
	// letOutQueue is how slow the traffic has to be moving for a driver to
	// think of letting somebody out: nobody stops in free-flowing traffic.
	letOutQueue float32 = 7
	// hornLength and flashLength are how long a horn or a flash of the
	// headlights lasts.
	hornLength  float32 = 0.7
	flashLength float32 = 0.5
	// slowStart is how long a stopped car in front may sit at a green before an
	// impatient driver behind sounds the horn, at the shortest.
	slowStart float32 = 1.8
)

// Hooting reports whether the driver is sounding their horn.
func (a *Agent) Hooting() bool { return a.horn > 0 }

// Flashing reports whether the driver is flashing their headlights.
func (a *Agent) Flashing() bool { return a.flash > 0 }

// perceive is the speed of the car in front as the driver has registered it.
// It trails the real one by their reaction time, and holds still altogether
// while they are looking somewhere else, except that anybody notices a car
// they are about to hit.
func (a *Agent) perceive(st Style, ahead *Vehicle, gap, lead, speed, dt float32) float32 {
	a.lookAway(st, dt)
	if ahead != a.seen {
		// Somebody new to follow: their speed is seen for what it is.
		a.seen, a.seenLead = ahead, lead
		return lead
	}
	if ahead == nil {
		return lead
	}
	closing := speed - lead
	if closing > 0 && gap/closing < closeCall {
		a.seenLead, a.distracted = lead, 0
		return lead
	}
	if a.distracted > 0 {
		return a.seenLead
	}
	a.seenLead += (lead - a.seenLead) * min(dt/max(st.Reaction, 0.05), 1)
	return a.seenLead
}

// lookAway decides whether the driver glances away from the road this step,
// and counts down a glance already under way.
func (a *Agent) lookAway(st Style, dt float32) {
	if a.distracted > 0 {
		a.distracted -= dt
		return
	}
	if a.rng.Float32() < (1-st.Attention)*distractRate*dt {
		a.distracted = mathx.Lerp(distractShortest, distractLongest, a.rng.Float32())
	}
}

// wander is how far off the middle of the lane the driver has drifted, from
// how well they keep their eyes on the road. Each driver drifts at their own
// pace, so a line of cars does not sway in step.
func (a *Agent) wander(st Style, now, speed float32) float32 {
	if speed < 3 || a.inTurn {
		return 0
	}
	reach := (1 - st.Attention) * wanderReach
	phase := float32(a.ID) * 1.7
	rate := 0.35 + float32(a.ID%7)*0.05
	return reach * (0.7*mathx.Sin(now*rate+phase) + 0.3*mathx.Sin(now*rate*2.3+phase*0.6))
}

// pressed is the driver's style once they have lost patience with the car in
// front: they close up on it.
func (a *Agent) pressed(st Style) Style {
	if a.heldUp <= st.Patience || a.pursuing {
		return st
	}
	st.Headway *= 1 - pressHeadway*st.Temper
	st.MinGap *= 1 - pressHeadway*0.5*st.Temper
	return st
}

// brakePoint is the share of their comfortable braking at which a driver
// starts slowing for something ahead. A calm driver lifts early and brakes
// gently; a hot-headed one leaves it late and stands on it.
func (s Style) brakePoint() float32 {
	return mathx.Lerp(0.35, 0.8, s.Temper)
}

// express lets the driver's temper show: the horn for somebody who cuts in or
// sits at a green, and the headlights for somebody they have been stuck behind
// too long.
func (a *Agent) express(w *World, st Style, ahead *Vehicle, gap, lead, speed, dt float32) {
	a.horn = max(a.horn-dt, 0)
	a.flash = max(a.flash-dt, 0)
	cutIn := ahead != nil && ahead != a.lastAhead && a.lastAhead != nil &&
		gap < st.MinGap+speed*st.Headway*0.5 && speed > 4
	a.lastAhead = ahead
	if a.pursuing || a.Role == RolePolice {
		return
	}
	switch {
	case cutIn && a.rng.Float32() < st.Temper:
		a.horn = hornLength
	case a.heldUp > st.Patience*2+2 && a.rng.Float32() < st.Temper*0.08*dt:
		a.flash = flashLength
	}
	if a.stuckAtGreen(w, ahead, gap, lead, speed) {
		a.greenWait += dt
	} else {
		a.greenWait = 0
	}
	if a.greenWait > slowStart+(1-st.Temper)*6 && a.horn <= 0 {
		a.horn, a.greenWait = hornLength, 0
	}
}

// stuckAtGreen reports whether the driver is sat behind somebody who has not
// noticed the lights change.
func (a *Agent) stuckAtGreen(w *World, ahead *Vehicle, gap, lead, speed float32) bool {
	if ahead == nil || speed > 0.3 || lead > 0.3 || gap > 8 || a.yielding || a.inTurn || a.lane == nil {
		return false
	}
	return a.lane.Control == city.ControlSignal && w.Signals.LaneState(a.lane) == SignalGreen
}

// courtesyStop is where a driver in slow traffic stops short of a junction to
// let somebody waiting there pull out, if they are inclined to.
func (w *World) courtesyStop(a *Agent, st Style, speed, dt float32) (float32, bool) {
	if a.pursuing || a.inTurn || a.lane == nil || a.lane.Control != city.ControlNone ||
		w.City.Nodes[a.lane.ToNode].IsRoundabout() {
		a.letting = nil
		return 0, false
	}
	d := a.lane.Length - a.s
	if a.letting != nil {
		return a.holdForLetting(d, dt)
	}
	// Only in slow traffic, and only with room to stop without braking hard.
	if speed > letOutQueue || d > 45 || d-letOutRoom < speed*speed/(2*st.Decel)+1 {
		return 0, false
	}
	o := w.waitingToPullOut(a)
	if o == nil || o == a.asked {
		return 0, false
	}
	// Only ever make the decision once for each car waiting.
	a.asked = o
	if a.rng.Float32() >= st.Courtesy {
		return 0, false
	}
	a.letting, a.letFor, a.flash = o, letOutFor, flashLength
	return d - letOutRoom, true
}

// holdForLetting keeps a gap open for the car being let out until it has gone,
// or has had long enough.
func (a *Agent) holdForLetting(d, dt float32) (float32, bool) {
	o := a.letting
	a.letFor -= dt
	if a.letFor <= 0 || o.inTurn || o.lane == nil || o.lane.ToNode != a.lane.ToNode || d < letOutRoom-1 {
		a.letting = nil
		return 0, false
	}
	return d - letOutRoom, true
}

// waitingToPullOut finds a car that has been held at the line of a side road
// into the junction a is approaching, or nil.
func (w *World) waitingToPullOut(a *Agent) *Agent {
	for _, o := range w.Agents {
		if o != a && o.yielding && o.waited >= 1 && o.lane != nil &&
			o.lane.ToNode == a.lane.ToNode && o.lane.Road != a.lane.Road {
			return o
		}
	}
	return nil
}

// hesitate holds a driver at the line for a moment after whatever they were
// waiting for has gone: nobody pulls away the instant the light changes.
func (a *Agent) hesitate(st Style, stopped bool, speed, dt float32) bool {
	switch {
	case stopped:
		a.atLine = speed < 0.5
		a.pause = 0
		return false
	case a.atLine:
		a.atLine = false
		a.pause = st.Reaction * 0.8
	}
	if a.pause > 0 {
		a.pause -= dt
		return true
	}
	return false
}
