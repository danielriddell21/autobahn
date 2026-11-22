package sim

import (
	"math/rand/v2"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/works"
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
	// Body is the shape of the car, which sets its size, how it drives, and
	// how the renderer builds it.
	Body Body
	// Style is the driver's personality, and supplies every parameter the
	// car-following model uses.
	Style Style
	// Role separates ambient traffic from police units.
	Role Role
	// Unit is the kind of police unit this is, for an agent that is one.
	Unit UnitKind

	lane    *city.Lane
	s       float32
	next    city.Turn
	hasNext bool
	inTurn  bool
	u       float32

	// stoppedAt records the node where this agent has already made its
	// mandatory stop, so it does not stop again on the same approach.
	stoppedAt int
	// ringSkip is how many exits the agent still passes before it leaves the
	// roundabout it is on or about to join.
	ringSkip int
	// waited is how long the agent has been held at a line waiting for a gap.
	// A driver who has waited long enough edges out, which is what stops a
	// junction where everybody is being polite from staying shut for ever.
	waited float32
	// yielding is set while the agent is being held at a line by other traffic
	// rather than by a signal or a sign.
	yielding bool
	// dazed counts down after a hard knock. A driver who has just been hit
	// does nothing but brake, and is not dragged back into lane as if nothing
	// had happened.
	dazed float32
	// pursuing is set on a police unit running to a call, and rank is its
	// place among the units on that call, nearest first.
	pursuing bool
	rank     int
	// free is set while a unit has left the lane graph to drive straight at
	// the car it is chasing.
	free bool
	// sidestep is how far to the right of its route the agent is currently
	// driving, in metres, and sidestepWant where that is heading. It is how a
	// driver gets round something stopped in the road, pulls in for a siren,
	// or overtakes on a call.
	sidestep, sidestepWant float32
	// backing is how much longer a unit in close pursuit will reverse for.
	backing float32
	// stuck is how long the agent has been held up by something that is not
	// moving. Once that is long enough it commits to going round: passing is
	// how much longer it stays out, and passSide which side it went.
	stuck    float32
	passing  float32
	passSide float32
	rng      *rand.Rand

	// Lane changes and overtakes; see overtake.go. heldUp is how long the
	// driver has been behind somebody slower than they want to go. pending is
	// the lane they are signalling to move into, and signalFor how long they
	// have been signalling. changing counts down while a change is under way,
	// toward changeSide. outside is how long they have sat in an outer lane
	// with nothing to pass. laneCool holds off the next change.
	heldUp, laneCool, signalFor float32
	pending                     *city.Lane
	changing, changeSide        float32
	outside                     float32
	// overtaking is the car being passed on a single carriageway, passFor how
	// much longer the pass may take, and aborting is set once it has been
	// given up and the driver is dropping back behind.
	overtaking *Vehicle
	passFor    float32
	aborting   bool
	// wantKind is the turn the driver means to make at the end of this road,
	// which on a wider road may need a different lane from the one they are
	// in. hasWant is false when they have no preference.
	wantKind city.TurnKind
	hasWant  bool

	// The human side; see temper.go. seen is the car in front as last
	// registered and seenLead the speed it was seen doing; distracted counts
	// down a glance away from the road. horn and flash count down a horn or a
	// flash of the lights, lastAhead is who was in front last step, and
	// greenWait how long the car in front has sat at a green. letting is a car
	// being let out of a junction, for letFor more seconds, and asked the last
	// one considered. atLine and pause hold a driver at the line for a moment
	// after being released.
	seen, lastAhead      *Vehicle
	seenLead, distracted float32
	horn, flash          float32
	greenWait            float32
	letting, asked       *Agent
	letFor               float32
	atLine               bool
	pause                float32

	// Manual hands the car to a human. A driven agent takes its controls from
	// Input and is left alone otherwise: no route to follow, no lane to be
	// nudged back onto, and no opinion about junctions.
	Manual bool
	// Input is the driver's controls while Manual is set.
	Input Controls
	// Blocking marks a unit parked across the carriageway as part of a
	// roadblock. It holds its position and drives nowhere.
	Blocking bool
	// surface is the share of full grip the roads it drives on give, so a
	// car it is changed to gets the same.
	surface float32
	// works is the roadworks, which no turn chosen may run into.
	works *works.Index
}

// Pursuing reports whether this unit is currently on a blue-light run.
func (a *Agent) Pursuing() bool { return a.pursuing }

func (a *Agent) style() Style {
	// Returns the style currently in force, which is the pursuit style while
	// a police unit is running to a call.
	st := a.Style
	if a.pursuing {
		st = Pursuit()
	}
	return st.onSurface(a.surface)
}

// Indicator returns the turn signal implied by the agent's next manoeuvre.
func (a *Agent) Indicator() Indicator {
	if ind, ok := a.laneIndicator(); ok {
		return ind
	}
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
	body := RandomBody(rng, c.Map())
	paint := rng.IntN(8)
	if body.Livery >= 0 {
		paint = body.Livery
	}
	a := &Agent{
		ID: id, V: NewVehicle(seasoned(body.Spec, c), lane.Point(s), lane.HeadingAt(s)),
		Paint: paint, Body: body, Style: RandomStyle(rng), Role: RoleCivilian,
		lane: lane, s: s, stoppedAt: -1, rng: rng,
		surface: SeasonGrip(c.Map().Season()),
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
	a.Body = BodyOf(Estate) // a marked traffic car, not a hatchback
	a.Body.Spec = PoliceSpec()
	a.V.Spec = a.Body.Spec.OnSurface(a.surface)
	return a
}

// SetUnit makes a police unit one of the given kind, with the car that goes
// with it.
func (a *Agent) SetUnit(k UnitKind) {
	a.Unit = k
	switch k {
	case UnitInterceptor:
		a.Body = BodyOf(Saloon)
	case UnitHeavy:
		a.Body = BodyOf(Van)
		a.Body.RoofFrac, a.Body.RoofRise = 0.62, 0.62
	default:
		a.Body = BodyOf(Estate)
	}
	a.Body.Spec = UnitSpec(k)
	a.V.Spec = a.Body.Spec.OnSurface(a.surface)
}

func (a *Agent) chooseNext(c *city.City) {
	a.chooseNextToward(c, mathx.Vec{}, false)
}

func (a *Agent) chooseNextToward(c *city.City, target mathx.Vec, chasing bool) {
	// A unit on a call heads for its target; everyone else prefers to carry
	// straight on, so traffic forms recognisable flows along the arterials
	// instead of turning at random.
	a.waited, a.hasWant = 0, false
	if chasing {
		if t, ok := a.pursuitTurn(c, target); ok {
			a.next, a.hasNext = t, true
			return
		}
	}
	turns := a.openTurns(a.lane)
	if len(turns) == 0 {
		a.hasNext = false
		return
	}
	a.hasNext = true
	if c.Roads[a.lane.Road].Ring {
		a.next = a.ringTurn()
		return
	}
	if c.Nodes[a.lane.ToNode].IsRoundabout() {
		// The exit is chosen on the way in, as a driver chooses it, so the car
		// goes round with a purpose instead of rolling a die at every exit.
		a.ringSkip = a.rng.IntN(max(c.Nodes[a.lane.ToNode].Degree()-1, 1))
		a.next = turns[0]
		return
	}
	// On a wider road the choice is made from every lane's turns, not just
	// this one's, and the driver changes lane for it on the way.
	cands := turns
	if lanes := c.Roads[a.lane.Road].Class.Lanes(); lanes > 1 {
		cands = nil
		for i := range lanes {
			if l := sibling(c, a.lane, i); l != nil {
				cands = append(cands, a.openTurns(l)...)
			}
		}
	}
	var straight []city.Turn
	for _, t := range cands {
		if t.Kind == city.TurnStraight {
			straight = append(straight, t)
		}
	}
	pick := cands[a.rng.IntN(len(cands))]
	if len(straight) > 0 && a.rng.Float32() < 0.62 {
		pick = straight[a.rng.IntN(len(straight))]
	}
	a.wantKind, a.hasWant = pick.Kind, true
	for _, t := range turns {
		if t == pick {
			a.next = t
			return
		}
	}
	// Not from this lane: carry on until there is a chance to move over.
	a.repickTurn()
	a.waited, a.hasWant = 0, true
}

// ringTurn carries on round a roundabout until the chosen exit comes up.
func (a *Agent) ringTurn() city.Turn {
	turns := a.openTurns(a.lane)
	round := turns[0]
	var exits []city.Turn
	for _, t := range turns {
		if t.Kind == city.TurnStraight {
			round = t
		} else {
			exits = append(exits, t)
		}
	}
	if len(exits) == 0 {
		return round
	}
	if a.ringSkip > 0 {
		a.ringSkip--
		return round
	}
	return exits[a.rng.IntN(len(exits))]
}

// routeHops bounds how many lanes a lookahead along an agent's route may
// cross. A roundabout is cut into chords a few metres long, so a lookahead
// that stopped at the next lane would see no further than the bonnet there.
const routeHops = 12

func (a *Agent) routePoint(c *city.City, ahead float32) mathx.Vec {
	// Walk forward along the route, crossing from the current lane onto its
	// connector and then onto the lanes that follow. Past the turn already
	// chosen the walk carries straight on, which is the best guess there is.
	lane, turn, hasTurn := a.lane, a.next, a.hasNext
	if a.inTurn {
		remain := (1 - a.u) * max(turn.Len, 0.01)
		if ahead <= remain {
			return c.TurnPoint(lane, turn, a.u+ahead/max(turn.Len, 0.01))
		}
		ahead -= remain
		lane = c.Lanes[turn.Lane]
		turn, hasTurn = onwardOpen(a.works, lane)
	} else {
		remain := lane.Length - a.s
		if ahead <= remain {
			return lane.Point(a.s + ahead)
		}
		ahead -= remain
	}
	for range routeHops {
		// Here the walk stands at the end of lane, about to take turn.
		if !hasTurn {
			return lane.Point(lane.Length)
		}
		if ahead <= turn.Len {
			return c.TurnPoint(lane, turn, ahead/max(turn.Len, 0.01))
		}
		ahead -= turn.Len
		lane = c.Lanes[turn.Lane]
		if ahead <= lane.Length {
			return lane.Point(ahead)
		}
		ahead -= lane.Length
		turn, hasTurn = onwardOpen(a.works, lane)
	}
	return lane.Point(lane.Length)
}

// onward is the successor a lookahead takes past a lane nobody has planned
// for yet: straight on where that is offered, and the first turn otherwise. It
// reads only the lane, so the answer is the same every time it is asked.
func onward(l *city.Lane) (city.Turn, bool) {
	if len(l.Succ) == 0 {
		return city.Turn{}, false
	}
	for _, t := range l.Succ {
		if t.Kind == city.TurnStraight {
			return t, true
		}
	}
	return l.Succ[0], true
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
	// Odometry alone drifts: a car that has been shunted, or has cut a corner,
	// is further along than its wheels say. Where it actually is wins, or the
	// route would trail behind the car and drag it backwards.
	a.s = max(a.s+step, a.lane.Along(a.V.Pos))
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
	if a.idle(c, dt) || (a.pursuing && a.engage(w, dt)) {
		return
	}
	if a.closedTurn() {
		// The street ahead has been shut since the turn was chosen.
		a.chooseNext(c)
	}

	st := a.style()
	speed := max(a.V.ForwardSpeed(), 0)
	limit := a.cruise(w, st)

	if a.pursuing && a.hasNext && !a.inTurn && a.lane.Length-a.s < 45 {
		a.chooseNextToward(c, w.interceptOf(a), true)
	}

	gap, lead, ahead, limit := a.lookAhead(w, st, speed, limit, dt)
	accel := idmFree(st, speed, limit)

	// Slow for the turn before reaching it, not half-way round.
	if !a.inTurn && a.hasNext {
		accel = min(accel, slowFor(st, speed, turnSpeed(a.next, st), a.lane.Length-a.s))
	}

	// The car in front as the driver sees it, which is not quite as it is.
	seen := a.perceive(st, ahead, gap, lead, speed, dt)
	a.express(w, st, ahead, gap, lead, speed, dt)
	if ahead != nil {
		accel = min(accel, idmFollow(a.pressed(st), speed, seen, gap))
	}
	if a.sidestepWant == 0 {
		a.sidestepWant = a.wander(st, w.Time, speed)
	}
	a.goRound(ahead, gap, lead, speed, dt)
	accel = a.atJunction(w, st, speed, accel, dt)

	a.applyLongitudinal(st, accel, speed)
	a.applySteering(c, speed)
	a.V.Update(Controls{
		Throttle: a.V.Throttle, Brake: a.V.Brake, Steer: a.V.Steer / a.V.Spec.MaxSteer,
	}, dt)
	a.advanceRoute(c, w.Player.Pos, dt)
	a.snapToRoute(c, dt)
}

// atJunction applies the junction rules, which produce a virtual obstacle at the
// stop line, along with a driver's own hesitation there and any courtesy to
// somebody waiting to pull out.
func (a *Agent) atJunction(w *World, st Style, speed, accel, dt float32) float32 {
	a.yielding = false
	d, stop := a.junctionStop(w)
	if stop {
		accel = min(accel, idmFollow(st, speed, 0, max(d, 0.05)))
		// Only waiting for a gap wears a driver's patience down. Nobody edges
		// out through a red light because it has been red a long time.
		if a.yielding && speed < 0.5 {
			a.waited += dt
		}
	}
	if a.hesitate(st, stop, speed, dt) {
		accel = min(accel, -st.Decel)
	}
	if d, ok := w.courtesyStop(a, st, speed, dt); ok {
		accel = min(accel, idmFollow(st, speed, 0, max(d, 0.05)))
	}
	return accel
}

// lookAhead finds whatever is directly in front, whether that is another agent
// or the player, and decides whether to stay behind it. It returns the car to
// follow, if any, and the speed the driver now wants.
func (a *Agent) lookAhead(w *World, st Style, speed, limit, dt float32) (gap, lead float32, ahead *Vehicle, want float32) {
	gap, lead, ahead = w.leaderFor(a)
	a.manoeuvre(w, st, ahead, gap, lead, speed, limit, dt)
	if a.overtaking == nil {
		return gap, lead, ahead, limit
	}
	if ahead == a.overtaking && !a.aborting {
		ahead = nil // pulling out round it, not queueing behind it
	}
	return gap, lead, ahead, a.passSpeed(limit)
}

// Getting round things, in metres and seconds.
const (
	// stuckTime is how long a driver sits behind something stationary before
	// deciding it is not going to move.
	stuckTime float32 = 3.5
	// passWidth is how far out a driver pulls to get round it, and
	// overtakeWidth how far a unit on a call pulls out to pass traffic.
	passWidth     float32 = 2.7
	overtakeWidth float32 = 3.5
	// sidestepRate is how fast a driver moves across the road, in m/s.
	sidestepRate float32 = 2.4
	// passTime is how long a driver stays out once committed to going round
	// something, in seconds: long enough to get past a car from a standstill.
	passTime float32 = 7
)

// goRound decides whether to pull out round whatever is in front.
//
// Ordinary traffic does it only for something that has plainly stopped for
// good — a crashed car, somebody parked in the road — and only after sitting
// behind it for a while. A unit on a call does it for anything slower than it
// wants to be going, at once.
func (a *Agent) goRound(ahead *Vehicle, gap, lead, speed, dt float32) {
	switch {
	case a.overtaking != nil:
		a.sidestepWant = a.passSidestep()
	case a.passing > 0:
		// Committed: hold the line out until the obstruction is behind.
		a.passing -= dt
		a.sidestepWant = a.passSide * passWidth
	case ahead != nil && a.pursuing && gap < 30 && lead < max(speed-2, 3):
		a.sidestepWant = overtakeWidth
	case ahead != nil && !a.inTurn && !a.yielding && lead < 0.3 && speed < 0.5 && gap < 9:
		a.waitBehind(ahead, dt)
	default:
		a.stuck = 0
	}
	// A car that is barely moving can only edge across.
	rate := mathx.Clamp(0.8+speed*0.5, 0.8, sidestepRate)
	if a.changing > 0 {
		// Drifting across into another lane, unhurried.
		a.changing -= dt
		rate = min(rate, laneShift)
	}
	a.sidestep = mathx.MoveToward(a.sidestep, a.sidestepWant, rate*dt)
}

// waitBehind counts the time spent sat behind something stationary, and
// commits to pulling out round it once that has gone on long enough.
func (a *Agent) waitBehind(ahead *Vehicle, dt float32) {
	if a.stuck += dt; a.stuck <= stuckTime {
		return
	}
	// Pull out on whichever side the obstruction is not.
	a.passSide = 1
	if ahead.Pos.Sub(a.V.Pos).Cross(a.V.Forward()) < -0.3 {
		a.passSide = -1
	}
	a.passing, a.stuck = passTime, 0
}

// pathPoint is routePoint moved across the road by the agent's sidestep.
func (a *Agent) pathPoint(c *city.City, ahead float32) mathx.Vec {
	p := a.routePoint(c, ahead)
	if a.sidestep == 0 {
		return p
	}
	along := a.routePoint(c, ahead+1.5).Sub(p)
	if along.Len() < 0.1 {
		return p
	}
	return p.Add(along.Norm().Right().Mul(a.sidestep))
}

// idle handles an agent that is not driving its route this step, and reports
// whether that was the case: one parked in a roadblock, one a person is
// driving, or one that has just been hit.
func (a *Agent) idle(c *city.City, dt float32) bool {
	switch {
	case a.Blocking:
		// Parked across the road, and staying there.
		a.V.Update(Controls{Brake: 1}, dt)
	case a.Manual:
		a.V.Update(a.Input, dt)
	case a.lane == nil:
	case a.dazed > 0:
		// Just been hit: stand on the brake and let the car go where the
		// impact sent it, then pick the road up again from wherever that is.
		a.V.Update(Controls{Brake: 0.7}, dt)
		if a.dazed -= dt; a.dazed <= 0 {
			a.rejoin(c)
		}
	default:
		return false
	}
	return true
}

// cruise is the speed a driver wants on the stretch of road they are on, with
// nothing in the way.
func (a *Agent) cruise(w *World, st Style) float32 {
	c := w.City
	limit := st.TargetSpeed(a.lane.SpeedLimit)
	if a.pursuing {
		// A unit on a call is not held to the posted limit.
		limit = min(st.TargetSpeed(mathx.MPH(52)), a.V.Spec.MaxSpeed)
	}
	a.sidestepWant = 0
	if w.sirenBehind(a) {
		// Lights in the mirror: pull in to the kerb and stop.
		limit, a.sidestepWant = 0, -1.7
	}
	if c.Roads[a.lane.Road].Ring {
		limit = min(limit, ringSpeed(c.Nodes[a.lane.ToNode], st))
	}
	if a.inTurn {
		limit = min(limit, turnSpeed(a.next, st))
	}
	return min(limit, bendSpeed(a.lane, a.s, st))
}

// bendAhead is how far up the road a driver reads the bends, in metres.
const bendAhead float32 = 45

// bendSpeed is how fast a driver takes the road ahead, from how sharply it
// turns at its sharpest within sight. A straight road is no constraint.
func bendSpeed(l *city.Lane, s float32, st Style) float32 {
	bend := l.Bend(s, bendAhead)
	if bend < 1e-4 {
		return 1e6
	}
	return mathx.Sqrt(cornerGrip/bend) * st.CornerBias
}

// dazeTime is how long a driver is out of it after a hard knock, in seconds,
// and dazeImpact the closing speed that counts as one, in m/s. A police driver
// is expecting it, and takes a good deal more.
const (
	dazeTime         float32 = 2.2
	dazeImpact       float32 = 4.0
	dazeImpactPolice float32 = 10.0
)

// Knock tells an agent it has been hit at the given closing speed.
func (a *Agent) knock(impact float32) {
	limit := dazeImpact
	if a.Role == RolePolice {
		limit = dazeImpactPolice
	}
	if impact >= limit && !a.Manual && !a.Blocking {
		a.dazed = dazeTime
	}
}

// rejoin puts an agent back on the lane graph from wherever it has ended up.
func (a *Agent) rejoin(c *city.City) {
	proj := c.ProjectAlong(a.V.Pos, a.V.Forward())
	if !proj.Valid {
		if proj = c.Project(a.V.Pos); !proj.Valid {
			return
		}
	}
	a.lane, a.s = proj.Lane, proj.S
	a.inTurn, a.u = false, 0
	a.stoppedAt = -1
	a.sidestep, a.sidestepWant, a.stuck, a.passing = 0, 0, 0, 0
	a.clearManoeuvre()
	a.chooseNext(c)
}

// cornerGrip is the sideways acceleration traffic takes a bend at, in m/s^2.
const cornerGrip float32 = 3.0

// turnSpeed is how fast a driver takes a connector, from how tightly it curves.
func turnSpeed(t city.Turn, st Style) float32 {
	if t.Kind == city.TurnStraight {
		return 1e6
	}
	// A quarter circle of this length has this radius, near enough.
	radius := t.Len * 2 / 3.14159
	return min(mathx.Sqrt(cornerGrip*radius)*st.CornerBias, st.CornerSpeed())
}

// ringSpeed is how fast a driver goes round a roundabout.
func ringSpeed(n *city.Node, st Style) float32 {
	return mathx.Sqrt(cornerGrip*n.RingRadius()) * st.CornerBias
}

// slowFor is the acceleration that brings a driver down to a target speed by
// the time a point ahead is reached, and no constraint until braking for it
// needs to begin.
func slowFor(st Style, speed, target, dist float32) float32 {
	if speed <= target {
		return 1e6
	}
	need := (speed*speed - target*target) / (2 * max(dist, 0.5))
	if need < st.Decel*st.brakePoint() {
		return 1e6 // plenty of room yet
	}
	return -need
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
	target := a.pathPoint(c, look)
	rel := target.Sub(a.V.Pos)
	fwd := a.V.Forward()
	alpha := mathx.Atan2(rel.Dot(fwd.Right()), rel.Dot(fwd))
	wb := a.V.Spec.Wheelbase()
	angle := mathx.Atan2(2*wb*mathx.Sin(alpha), max(rel.Len(), 1))
	a.V.Steer = mathx.Clamp(angle, -a.V.Spec.MaxSteer, a.V.Spec.MaxSteer)
}

func (a *Agent) snapToRoute(c *city.City, dt float32) {
	// Ambient traffic must not drift out of lane over time. Nudge the car back
	// onto its route line gently enough that the correction is invisible, and
	// ease its nose round to match, so it comes out of a turn pointing down
	// the road rather than across it.
	//
	// Only the sideways error is corrected. Pulling the car along the route as
	// well would move it with its brakes on, and a car held at a red light
	// would be drawn over the line a few millimetres at a time.
	here := a.routePoint(c, 0)
	along := a.routePoint(c, 2.2).Sub(here)
	if along.Len() < 0.5 {
		return
	}
	dir := along.Norm()
	off := here.Add(dir.Right().Mul(a.sidestep)).Sub(a.V.Pos)
	off = off.Sub(dir.Mul(off.Dot(dir)))
	if d := off.Len(); d > 0.05 {
		a.V.Pos = a.V.Pos.Add(off.Mul(min(1, 3.5*dt)))
	}
	if a.V.Speed() > 0.5 {
		err := mathx.AngleDiff(dir.Angle(), a.V.Yaw)
		a.V.Yaw = mathx.WrapPi(a.V.Yaw + err*mathx.Clamp(2.5*dt, 0, 1))
	}
}

// Junction approach ranges, in metres.
const (
	// yieldRange is how far from the line a driver starts judging the gap.
	yieldRange float32 = 28
	// waitLimit is how long a driver waits for one before edging out, in
	// seconds.
	waitLimit float32 = 14
)

func (a *Agent) junctionStop(w *World) (dist float32, stop bool) {
	if a.inTurn || !a.hasNext {
		return 0, false
	}
	// Traffic already on a roundabout has priority over everything joining it,
	// so it has nothing to stop for but the car in front.
	if w.City.Roads[a.lane.Road].Ring {
		return 0, false
	}
	d := a.lane.Length - a.s
	if d > 60 {
		return 0, false
	}

	style := a.style()
	// A unit on a blue-light run may pass a red, but slows to do it safely.
	if a.pursuing {
		if d < 18 && w.boxConflict(a, w.City.Nodes[a.lane.ToNode]) {
			return d, true
		}
		return 0, false
	}

	if a.controlStop(w, style, d) {
		return d, true
	}

	// Even with the right to go, do not drive into something: wait for a gap
	// in whatever has priority, and for the way out to be clear.
	if d < a.yieldFrom(style) && w.mustYield(a) {
		a.yielding = true
		return d, true
	}
	return 0, false
}

// fastRoad is the limit above which a driver has to start judging a junction
// from further back than yieldRange, in m/s: anything over forty.
var fastRoad = mathx.MPH(41)

// yieldFrom is how far from the line a driver starts judging the gap. On a
// town street that is yieldRange, which is room enough to stop from any speed
// a town street allows. On a fast road it is not, so there it is however far
// the driver would take to stop, and a little over.
func (a *Agent) yieldFrom(st Style) float32 {
	if a.lane == nil || a.lane.SpeedLimit < fastRoad {
		return yieldRange
	}
	v := a.V.Speed()
	return max(yieldRange, v*v/(2*st.Decel)+8)
}

func (a *Agent) controlStop(w *World, style Style, d float32) bool {
	switch a.lane.Control {
	case city.ControlSignal:
		return a.signalStop(w, style, d)
	case city.ControlStop:
		return a.signStop(style, d)
	}
	return false
}

func (a *Agent) signalStop(w *World, style Style, d float32) bool {
	switch w.Signals.LaneState(a.lane) {
	case SignalRed, SignalRedAmber:
		return true
	case SignalAmber:
		return style.StopsForAmber(d, a.V.Speed())
	}
	return false
}

func (a *Agent) signStop(style Style, d float32) bool {
	if a.stoppedAt == a.lane.ToNode {
		return false
	}
	// Some drivers count a crawl past the sign as stopping.
	rolled := style.RollingStop && d < 4.5 && a.V.Speed() < 2.5
	if (d < 3.2 && a.V.Speed() < 0.5) || rolled {
		a.stoppedAt = a.lane.ToNode // stop completed
		return false
	}
	return true
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
