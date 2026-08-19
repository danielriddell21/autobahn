package sim

import (
	"math/rand/v2"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Indicator is the turn signal an agent is showing.
type Indicator int

// The indicator states.
const (
	IndicatorOff Indicator = iota
	IndicatorLeft
	IndicatorRight
)

// Car-following model ranges, after the Intelligent Driver Model. The gaps and
// rates themselves come from each driver's [Style]; only the search geometry is
// shared.
const (
	idmScanRange   float32 = 70
	idmLaneCoridor float32 = 2.3 // half-width of the "same lane" corridor, m
)

// Agent is a car the simulation drives: ambient traffic, or a police unit. It
// navigates the lane graph, follows the vehicle in front, and obeys the same
// signals and signs as the player.
type Agent struct {
	ID    int
	V     *Vehicle
	Paint int // index into the renderer's car palette
	// Style is the driver's personality, and supplies every parameter the
	// car-following model uses.
	Style Style
	// Role separates ambient traffic from police units.
	Role Role

	lane    *city.Lane
	s       float32
	next    city.Turn
	hasNext bool
	inTurn  bool
	u       float32

	// stoppedAt records the node where this agent has already made its
	// mandatory stop, so it does not stop again on the same approach.
	stoppedAt int
	// pursuing is set on a police unit running to a call.
	pursuing bool
	rng      *rand.Rand
}

// Pursuing reports whether this unit is currently on a blue-light run.
func (a *Agent) Pursuing() bool { return a.pursuing }

func (a *Agent) style() Style {
	// Returns the style currently in force, which is the pursuit style while
	// a police unit is running to a call.
	if a.pursuing {
		return Pursuit()
	}
	return a.Style
}

// Indicator returns the turn signal implied by the agent's next manoeuvre.
func (a *Agent) Indicator() Indicator {
	if !a.hasNext {
		return IndicatorOff
	}
	// Only indicate once the junction is close enough to matter.
	if !a.inTurn && a.lane != nil && a.lane.Length-a.s > 34 {
		return IndicatorOff
	}
	switch a.next.Kind {
	case city.TurnLeft:
		return IndicatorLeft
	case city.TurnRight:
		return IndicatorRight
	default:
		return IndicatorOff
	}
}

// Lane returns the lane the agent is currently travelling along. While it is
// crossing a junction this is the lane it is leaving.
func (a *Agent) Lane() *city.Lane { return a.lane }

// InJunction reports whether the agent is currently on a turn connector.
func (a *Agent) InJunction() bool { return a.inTurn }

// SpeedLimit returns the limit applying to the agent's current lane.
func (a *Agent) SpeedLimit() float32 {
	if a.lane == nil {
		return mathx.MPH(30)
	}
	return a.lane.SpeedLimit
}

// NewAgent spawns a traffic car at the given distance along a lane.
func NewAgent(id int, c *city.City, lane *city.Lane, s float32, rng *rand.Rand) *Agent {
	a := &Agent{
		ID: id, V: NewVehicle(TrafficSpec(), lane.Point(s), lane.Heading),
		Paint: rng.IntN(8), Style: RandomStyle(rng), Role: RoleCivilian,
		lane: lane, s: s, stoppedAt: -1, rng: rng,
	}
	a.chooseNext(c)
	return a
}

// NewPoliceUnit spawns a marked police car, which patrols like any other
// vehicle until it is called to a pursuit.
func NewPoliceUnit(id int, c *city.City, lane *city.Lane, s float32, rng *rand.Rand) *Agent {
	a := NewAgent(id, c, lane, s, rng)
	a.Role = RolePolice
	a.Style = Normal()
	a.V.Spec = PoliceSpec()
	return a
}

func (a *Agent) chooseNext(c *city.City) {
	a.chooseNextToward(c, mathx.Vec{}, false)
}

func (a *Agent) chooseNextToward(c *city.City, target mathx.Vec, chasing bool) {
	// A unit on a call heads for its target; everyone else prefers to carry
	// straight on, so traffic forms recognisable flows along the arterials
	// instead of turning at random.
	if chasing {
		if t, ok := a.pursuitTurn(c, target); ok {
			a.next, a.hasNext = t, true
			return
		}
	}
	if len(a.lane.Succ) == 0 {
		a.hasNext = false
		return
	}
	var straight []city.Turn
	for _, t := range a.lane.Succ {
		if t.Kind == city.TurnStraight {
			straight = append(straight, t)
		}
	}
	if len(straight) > 0 && a.rng.Float32() < 0.62 {
		a.next = straight[a.rng.IntN(len(straight))]
	} else {
		a.next = a.lane.Succ[a.rng.IntN(len(a.lane.Succ))]
	}
	a.hasNext = true
}

func (a *Agent) routePoint(c *city.City, ahead float32) mathx.Vec {
	// Walk forward along the route, crossing from the current lane onto its
	// connector and then onto the following lane as needed.
	if a.inTurn {
		remain := (1 - a.u) * max(a.next.Len, 0.01)
		if ahead <= remain {
			return c.TurnPoint(a.lane, a.next, a.u+ahead/max(a.next.Len, 0.01))
		}
		nl := c.Lanes[a.next.Lane]
		return nl.Point(ahead - remain)
	}
	remain := a.lane.Length - a.s
	if ahead <= remain {
		return a.lane.Point(a.s + ahead)
	}
	ahead -= remain
	if !a.hasNext {
		return a.lane.Point(a.lane.Length)
	}
	if ahead <= a.next.Len {
		return c.TurnPoint(a.lane, a.next, ahead/max(a.next.Len, 0.01))
	}
	nl := c.Lanes[a.next.Lane]
	return nl.Point(ahead - a.next.Len)
}

func (a *Agent) advanceRoute(c *city.City, chase mathx.Vec, dt float32) {
	// Progress along the route is driven by how far the car actually moved, so
	// the route state stays glued to the physics.
	step := max(a.V.ForwardSpeed(), 0) * dt
	if a.inTurn {
		a.u += step / max(a.next.Len, 0.01)
		if a.u >= 1 {
			a.lane = c.Lanes[a.next.Lane]
			a.s = 0
			a.inTurn, a.u = false, 0
			a.chooseNextToward(c, chase, a.pursuing)
		}
		return
	}
	a.s += step
	if a.s >= a.lane.Length {
		if !a.hasNext {
			// Nowhere to go: park the agent for the respawner to recycle.
			a.s = a.lane.Length
			return
		}
		a.inTurn, a.u = true, 0
		a.stoppedAt = -1
	}
}

func (a *Agent) drive(w *World, dt float32) {
	c := w.City
	if a.lane == nil {
		return
	}

	st := a.style()
	speed := max(a.V.ForwardSpeed(), 0)
	limit := st.TargetSpeed(a.lane.SpeedLimit)
	if a.pursuing {
		// A unit on a call is not held to the posted limit.
		limit = min(st.TargetSpeed(mathx.MPH(45)), a.V.Spec.MaxSpeed)
	}
	if a.inTurn && a.next.Kind != city.TurnStraight {
		limit = min(limit, st.CornerSpeed())
	}

	if a.pursuing && a.hasNext && !a.inTurn && a.lane.Length-a.s < 45 {
		a.chooseNextToward(c, w.Player.Pos, true)
	}

	accel := idmFree(st, speed, limit)

	// Yield to whatever is directly in front, whether that is another agent or
	// the player.
	if gap, lead, ok := w.leaderFor(a); ok {
		accel = min(accel, idmFollow(st, speed, lead, gap))
	}

	// Junction rules produce a virtual obstacle at the stop line.
	if d, stop := a.junctionStop(w); stop {
		accel = min(accel, idmFollow(st, speed, 0, max(d, 0.05)))
	}

	a.applyLongitudinal(st, accel, speed)
	a.applySteering(c, speed)
	a.V.Update(Controls{
		Throttle: a.V.Throttle, Brake: a.V.Brake, Steer: a.V.Steer / a.V.Spec.MaxSteer,
	}, dt)
	a.advanceRoute(c, w.Player.Pos, dt)
	a.snapToRoute(c, dt)
}

func (a *Agent) applyLongitudinal(st Style, accel, speed float32) {
	switch {
	case accel >= 0:
		a.V.Throttle = mathx.Clamp(accel/st.Accel, 0, 1)
		a.V.Brake = 0
	default:
		a.V.Throttle = 0
		a.V.Brake = mathx.Clamp(-accel/st.Decel, 0, 1)
		if speed < 0.25 {
			a.V.Brake = 1 // hold still at the line
		}
	}
}

func (a *Agent) applySteering(c *city.City, speed float32) {
	// Pure pursuit toward a point further along the route; the lookahead grows
	// with speed so fast cars steer smoothly.
	look := mathx.Clamp(4.5+0.85*speed, 5.5, 20)
	target := a.routePoint(c, look)
	rel := target.Sub(a.V.Pos)
	fwd := a.V.Forward()
	alpha := mathx.Atan2(rel.Dot(fwd.Right()), rel.Dot(fwd))
	wb := a.V.Spec.Wheelbase()
	angle := mathx.Atan2(2*wb*mathx.Sin(alpha), max(rel.Len(), 1))
	a.V.Steer = mathx.Clamp(angle, -a.V.Spec.MaxSteer, a.V.Spec.MaxSteer)
}

func (a *Agent) snapToRoute(c *city.City, dt float32) {
	// Ambient traffic must not drift out of lane over time. Nudge the car back
	// onto its route line gently enough that the correction is invisible.
	want := a.routePoint(c, 0.1)
	off := want.Sub(a.V.Pos)
	if d := off.Len(); d > 0.05 {
		a.V.Pos = a.V.Pos.Add(off.Norm().Mul(min(d, d*3.5*dt)))
	}
}

func (a *Agent) junctionStop(w *World) (dist float32, stop bool) {
	if a.inTurn || !a.hasNext {
		return 0, false
	}
	d := a.lane.Length - a.s
	if d > 60 {
		return 0, false
	}

	style := a.style()
	// A unit on a blue-light run may pass a red, but slows to do it safely.
	if a.pursuing {
		if d < 18 && w.junctionBlocked(a) {
			return d, true
		}
		return 0, false
	}

	switch a.lane.Control {
	case city.ControlSignal:
		switch w.Signals.LaneState(a.lane) {
		case SignalRed, SignalRedAmber:
			return d, true
		case SignalAmber:
			if style.StopsForAmber(d, a.V.Speed()) {
				return d, true
			}
		}
	case city.ControlStop:
		if a.stoppedAt != a.lane.ToNode {
			if d < 3.2 && a.V.Speed() < 0.5 {
				a.stoppedAt = a.lane.ToNode // stop completed
			} else {
				return d, true
			}
		}
	case city.ControlGiveWay:
		// No halt is required, but the approach is taken slowly and the car
		// waits for a gap when the junction is not clear.
		if d < 26 && w.junctionBlocked(a) {
			return d, true
		}
	}

	// Even on a green light, do not enter a junction that is already occupied,
	// and give way when turning across oncoming traffic.
	if d < 14 && w.junctionBlocked(a) {
		return d, true
	}
	return 0, false
}

func idmFree(s Style, v, v0 float32) float32 {
	if v0 <= 0.1 {
		return -s.Decel
	}
	r := v / v0
	return s.Accel * (1 - r*r*r*r)
}

func idmFollow(s Style, v, lead, gap float32) float32 {
	// Desired dynamic gap grows with speed and with the closing rate, and how
	// much of it the driver insists on is their own business.
	dv := v - lead
	want := s.MinGap + max(0, v*s.Headway+v*dv/(2*mathx.Sqrt(s.Accel*s.Decel)))
	ratio := want / max(gap, 0.35)
	return s.Accel * (1 - ratio*ratio)
}
