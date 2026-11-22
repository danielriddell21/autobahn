package sim

import (
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/works"
)

// Roadworks, as the simulation sees them.
//
// Package works says which lanes are barred and where the cones and barriers
// stand, as a function of the clock. This file is the only place the simulation
// acts on it: a turn into a barred lane is not offered to anyone choosing one,
// what is solid in the road is collided with, and traffic meets the start of a
// coned lane as it would a stopped car, which is how the lane-change logic it
// already has gets it over to the next lane.

// openTurns returns the turns from a lane that a driver may choose between at
// the moment: all of them but those into a lane that roadworks have barred.
// It never returns nothing for a lane that has something, so nobody is
// stranded by a closure.
func openTurns(ix *works.Index, l *city.Lane) []city.Turn {
	if ix == nil || len(l.Succ) == 0 {
		return l.Succ
	}
	shut := false
	for _, t := range l.Succ {
		if ix.Barred(t.Lane) {
			shut = true
			break
		}
	}
	if !shut {
		return l.Succ
	}
	out := make([]city.Turn, 0, len(l.Succ))
	for _, t := range l.Succ {
		if !ix.Barred(t.Lane) {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return l.Succ
	}
	return out
}

// onwardOpen is [onward] over the turns that are open.
func onwardOpen(ix *works.Index, l *city.Lane) (city.Turn, bool) {
	turns := openTurns(ix, l)
	if len(turns) == 0 {
		return city.Turn{}, false
	}
	for _, t := range turns {
		if t.Kind == city.TurnStraight {
			return t, true
		}
	}
	return turns[0], true
}

// stubSize is the footprint of the stand-in for a closure's first cone or
// barrier: a stopped thing, as far as anybody following the lane is concerned.
const (
	coneStubHalf    float32 = 0.3
	barrierStubHalf float32 = 1.2
)

// syncWorks brings the roadworks to the clock: what is closed, what is solid,
// and the stopped stand-ins at the start of each stretch of cones and at each
// barrier.
func (w *World) syncWorks() {
	if w.Works == nil {
		return
	}
	w.Works.Update(w.Time)
	w.solid = w.Works.Solid(w.solid[:0])
	w.stubs = w.stubs[:0]
	for _, cl := range w.Works.Active() {
		if len(cl.Cones) > 0 {
			w.stubs = append(w.stubs, w.stub(len(w.stubs), cl.Cones[0], coneStubHalf))
		}
		for _, b := range cl.Barriers {
			w.stubs = append(w.stubs, w.stub(len(w.stubs), b, barrierStubHalf))
		}
	}
}

// stub is the stopped stand-in at the given box, reusing the one made for the
// same place last time round.
func (w *World) stub(i int, at mathx.OBB, half float32) *Vehicle {
	for len(w.stubPool) <= i {
		w.stubPool = append(w.stubPool, &Vehicle{})
	}
	v := w.stubPool[i]
	v.Pos, v.Yaw, v.Vel = at.Center, at.Heading, mathx.Vec{}
	v.Spec = Spec{HalfWidth: half, HalfLength: coneStubHalf, Height: 0.8}
	return v
}

// collideWorks pushes a vehicle out of the cones and barriers that are in the
// road.
func (w *World) collideWorks(v *Vehicle, report bool) {
	for _, b := range w.solid {
		if v.Pos.DistTo(b.Center) > v.Spec.HalfLength+v.Spec.HalfWidth+b.Radius() {
			continue
		}
		if b.HalfW <= coneStubHalf && b.HalfL <= coneStubHalf {
			w.collideCone(v, b, report)
			continue
		}
		w.collideSolid(v, b, "hit roadworks", report)
	}
}

// collideCone pushes a vehicle out of a cone, which is narrower than the car
// that hits it. A box wholly inside another's width is pushed out by less than
// it takes by the usual test, and slides round the cone; this treats the cone
// as a post, and stops the car against it.
func (w *World) collideCone(v *Vehicle, cone mathx.OBB, report bool) {
	fwd := v.Forward()
	right := fwd.Right()
	rel := cone.Center.Sub(v.Pos)
	cx, cy := rel.Dot(fwd), rel.Dot(right)
	half, wide := v.Spec.HalfLength, v.Spec.HalfWidth
	dx, dy := cx-mathx.Clamp(cx, -half, half), cy-mathx.Clamp(cy, -wide, wide)
	r := cone.HalfW + 0.05
	d := mathx.Sqrt(dx*dx + dy*dy)
	if d >= r {
		return
	}
	var nx, ny, depth float32
	switch {
	case d > 1e-4:
		nx, ny, depth = dx/d, dy/d, r-d
	case half-mathx.Abs(cx) < wide-mathx.Abs(cy):
		nx, depth = mathx.Sign(cx), half-mathx.Abs(cx)+r
	default:
		ny, depth = mathx.Sign(cy), wide-mathx.Abs(cy)+r
	}
	axis := fwd.Mul(nx).Add(right.Mul(ny))
	v.Pos = v.Pos.Sub(axis.Mul(depth))
	if closing := v.Vel.Dot(axis); closing > 0 {
		v.Vel = v.Vel.Sub(axis.Mul(closing * (1 + restitution)))
		if report {
			w.judgeFor(v).ReportCollision(v, "hit roadworks", closing, w.Time)
			if v == w.Player {
				w.batter(closing)
			}
		}
	}
}

// laneBarred reports whether roadworks have shut a lane to anyone entering it.
func (w *World) laneBarred(l *city.Lane) bool { return w.Works.Barred(l.ID) }

// conedUntil returns how far along a lane its cones run on, plus the room a
// driver leaves before pulling back in, and whether it has any.
func conedUntil(ix *works.Index, l *city.Lane) (float32, bool) {
	cl, ok := ix.Coned(l.ID)
	if !ok || len(cl.Cones) == 0 {
		return 0, false
	}
	return l.Along(cl.Cones[len(cl.Cones)-1].Center) + coneReturn, true
}

// coneReturn is how far past the last cone a driver goes before moving back into
// the lane that was coned.
const coneReturn float32 = 8

// openTurns is the turns open to this agent from a lane.
func (a *Agent) openTurns(l *city.Lane) []city.Turn { return openTurns(a.works, l) }

// serves reports whether a turn of the given kind can be made from a lane.
// A lane whose junction is not wired yet has no successors to ask, and is
// taken to serve anything.
func (a *Agent) serves(l *city.Lane, k city.TurnKind) bool {
	if len(l.Succ) == 0 {
		return true
	}
	for _, t := range a.openTurns(l) {
		if t.Kind == k {
			return true
		}
	}
	return false
}

// restOfCones reports whether moving into lane to would put the agent among
// cones that are still ahead of it.
func (a *Agent) restOfCones(to *city.Lane) bool {
	until, ok := conedUntil(a.works, to)
	return ok && a.V.Pos != (mathx.Vec{}) && to.Along(a.V.Pos) < until
}

// closedTurn reports whether the turn an agent has chosen has since been shut.
func (a *Agent) closedTurn() bool {
	return a.hasNext && !a.inTurn && a.works.Barred(a.next.Lane)
}
