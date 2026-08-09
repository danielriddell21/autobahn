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

// Car-following model parameters, after the Intelligent Driver Model.
const (
	idmMinGap      float32 = 2.6  // bumper-to-bumper gap at a standstill, m
	idmHeadway     float32 = 1.35 // desired time headway, s
	idmAccel       float32 = 2.3  // comfortable acceleration, m/s^2
	idmDecel       float32 = 3.1  // comfortable deceleration, m/s^2
	idmScanRange   float32 = 70
	idmLaneCoridor float32 = 2.3 // half-width of the "same lane" corridor, m
)

// Agent is an ambient traffic car. It navigates the lane graph, follows the
// vehicle in front and obeys the same signals and signs as the player.
type Agent struct {
	ID    int
	V     *Vehicle
	Paint int // index into the renderer's car palette

	lane    *city.Lane
	s       float32
	next    city.Turn
	hasNext bool
	inTurn  bool
	u       float32

	// stoppedAt records the node where this agent has already made its
	// mandatory stop, so it does not stop again on the same approach.
	stoppedAt int
	// speedBias gives each driver a slightly different target speed.
	speedBias float32
	rng       *rand.Rand
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
		return mathx.KPH(50)
	}
	return a.lane.SpeedLimit
}

// NewAgent spawns a traffic car at the given distance along a lane.
func NewAgent(id int, c *city.City, lane *city.Lane, s float32, rng *rand.Rand) *Agent {
	pos := lane.Point(s)
	a := &Agent{
		ID: id, V: NewVehicle(TrafficSpec(), pos, lane.Heading),
		Paint: rng.IntN(8), lane: lane, s: s,
		stoppedAt: -1, speedBias: 0.86 + rng.Float32()*0.26, rng: rng,
	}
	a.chooseNext(c)
	return a
}

func (a *Agent) chooseNext(c *city.City) {
	// Prefer carrying straight on, so traffic forms recognisable flows along
	// the arterials instead of turning at random.
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

func (a *Agent) advanceRoute(c *city.City, dt float32) {
	// Progress along the route is driven by how far the car actually moved, so
	// the route state stays glued to the physics.
	step := max(a.V.ForwardSpeed(), 0) * dt
	if a.inTurn {
		a.u += step / max(a.next.Len, 0.01)
		if a.u >= 1 {
			a.lane = c.Lanes[a.next.Lane]
			a.s = 0
			a.inTurn, a.u = false, 0
			a.chooseNext(c)
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

	speed := max(a.V.ForwardSpeed(), 0)
	limit := a.lane.SpeedLimit * a.speedBias
	if a.inTurn && a.next.Kind != city.TurnStraight {
		limit = min(limit, mathx.KPH(24)) // slow for the corner
	}

	accel := idmFree(speed, limit)

	// Yield to whatever is directly in front, whether that is another agent or
	// the player.
	if gap, lead, ok := w.leaderFor(a); ok {
		accel = min(accel, idmFollow(speed, lead, gap))
	}

	// Junction rules produce a virtual obstacle at the stop line.
	if d, stop := a.junctionStop(w); stop {
		accel = min(accel, idmFollow(speed, 0, max(d, 0.05)))
	}

	a.applyLongitudinal(accel, speed)
	a.applySteering(c, speed)
	a.V.Update(Controls{
		Throttle: a.V.Throttle, Brake: a.V.Brake, Steer: a.V.Steer / a.V.Spec.MaxSteer,
	}, dt)
	a.advanceRoute(c, dt)
	a.snapToRoute(c, dt)
}

func (a *Agent) applyLongitudinal(accel, speed float32) {
	switch {
	case accel >= 0:
		a.V.Throttle = mathx.Clamp(accel/idmAccel, 0, 1)
		a.V.Brake = 0
	default:
		a.V.Throttle = 0
		a.V.Brake = mathx.Clamp(-accel/idmDecel, 0, 1)
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

	switch a.lane.Control {
	case city.ControlSignal:
		switch st := w.Signals.LaneState(a.lane); st {
		case SignalRed, SignalRedAmber:
			return d, true
		case SignalAmber:
			// Stop only if there is room to do so comfortably.
			if d > a.V.Speed()*a.V.Speed()/(2*idmDecel) {
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

func idmFree(v, v0 float32) float32 {
	if v0 <= 0.1 {
		return -idmDecel
	}
	r := v / v0
	return idmAccel * (1 - r*r*r*r)
}

func idmFollow(v, lead, gap float32) float32 {
	// Desired dynamic gap grows with speed and with the closing rate.
	dv := v - lead
	want := idmMinGap + max(0, v*idmHeadway+v*dv/(2*mathx.Sqrt(idmAccel*idmDecel)))
	ratio := want / max(gap, 0.35)
	return idmAccel * (1 - ratio*ratio)
}
