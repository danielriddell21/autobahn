package sim

import (
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Role separates ambient traffic from police units.
type Role int

// The agent roles.
const (
	RoleCivilian Role = iota
	RolePolice
)

// UnitKind is the sort of car a police unit drives, which decides how soon it
// is sent after somebody and what it does when it gets there.
type UnitKind int

// The kinds of police unit.
const (
	// UnitPatrol is the ordinary marked car. It answers every call.
	UnitPatrol UnitKind = iota
	// UnitInterceptor is a fast pursuit car, sent once a driver has shown
	// they are not going to stop.
	UnitInterceptor
	// UnitHeavy is a big 4x4 that ends pursuits by weight. It is the last
	// thing sent and the hardest to shrug off.
	UnitHeavy
)

// Name returns the kind's name.
func (k UnitKind) Name() string {
	switch k {
	case UnitInterceptor:
		return "interceptor"
	case UnitHeavy:
		return "heavy"
	default:
		return "patrol"
	}
}

// joinsAt is the wanted level at which a kind of unit is sent.
func (k UnitKind) joinsAt() int {
	switch k {
	case UnitInterceptor:
		return 2
	case UnitHeavy:
		return 3
	default:
		return 1
	}
}

// unitRoster is the kind each unit is, in the order they are created. The
// first few are always patrol cars, so a world with only a couple of units
// still has somebody to answer a call.
var unitRoster = [...]UnitKind{
	UnitPatrol, UnitPatrol, UnitInterceptor, UnitPatrol,
	UnitHeavy, UnitInterceptor, UnitPatrol, UnitHeavy,
}

// PoliceSpec returns the handling profile of a marked police car: quicker and
// better planted than ambient traffic, so a pursuit is not hopeless either way.
func PoliceSpec() Spec { return UnitSpec(UnitPatrol) }

// UnitSpec returns the handling profile of a kind of police unit.
func UnitSpec(k UnitKind) Spec {
	s := CarSpec()
	s.EngineForce = 10600
	s.MaxSpeed = 58
	s.GripFront, s.GripRear = 86000, 92000
	switch k {
	case UnitInterceptor:
		s.EngineForce, s.MaxSpeed = 13200, 68
		s.GripFront, s.GripRear = 92000, 98000
		s.HalfLength, s.Height = 2.3, 1.36
	case UnitHeavy:
		s.Mass, s.Inertia = 2500, 4200
		s.EngineForce, s.BrakeForce, s.MaxSpeed = 16500, 24000, 54
		s.HalfWidth, s.HalfLength, s.Height = 1.04, 2.5, 1.9
		s.MaxLateral = 14000
	}
	return s
}

// Wanted tuning.
const (
	// heatPerPoint converts a penalty point into wanted heat.
	heatPerPoint float32 = 0.55
	// heatPerLevel is how much heat each wanted level costs.
	heatPerLevel float32 = 45
	// heatPerSecond is how fast a pursuit escalates on its own. A driver who
	// keeps running in plain sight is a bigger problem every second.
	heatPerSecond float32 = 1.3
	// heatPerRam is the heat for driving into a police car.
	heatPerRam float32 = 14
	// maxWanted is the highest level the response escalates to.
	maxWanted = 5
	// noticeRange is how close a unit must be to see an offence committed.
	noticeRange float32 = 95
	// reportWeight is how much an offence nobody saw is worth against
	// reportThreshold. Being seen is worth more, and is immediate.
	reportWeight float32 = 0.5
	// reportThreshold is how much unseen bad driving it takes before somebody
	// phones it in, in the judge's own penalty points. It sits at roughly two
	// red lights, three wrong-way stretches, or a long run of wandering off
	// the carriageway — persistent bad driving, not one lapse.
	reportThreshold float32 = 55
	// reportFade is how fast a report is forgotten, in points per second. A
	// driver who settles down is off the hook in under a minute.
	reportFade float32 = 1.5
	// pursuitRange is how far a unit will chase before it gives up.
	pursuitRange float32 = 520
	// sightRange is how far a unit can keep a car in view down a clear street,
	// and closeRange how near it has to be to know where the car is without
	// seeing it.
	sightRange float32 = 170
	closeRange float32 = 28
	// coolBase and coolPerLevel are how long a driver has to stay out of sight
	// to be lost, in seconds. The more wanted, the longer they keep looking.
	coolBase     float32 = 7
	coolPerLevel float32 = 2.5
	// stopRange and stopTime are what it takes to be pulled over: a unit
	// alongside, and the car stationary.
	stopRange float32 = 9.5
	stopTime  float32 = 2.4
	// cornerRange and cornerRate are the slower way: stopped in plain sight of
	// a unit this close counts toward being pulled over at this fraction of the
	// usual rate, so a car wedged somewhere no unit can drive to is not safe.
	cornerRange float32 = 60
	cornerRate  float32 = 0.3
	// ramPause is how long after hitting a unit before hitting one counts
	// again, in seconds. Two cars locked together touch every step.
	ramPause float32 = 2.5
	// coolTime is how long without a new offence before heat starts to fade.
	coolTime float32 = 6
)

// unitsFor is how many units are committed to a pursuit at each wanted level.
var unitsFor = [maxWanted + 1]int{0, 2, 3, 4, 6, 8}

// Bounty tuning, in points.
const (
	bountyPerSecond = 9   // per wanted level, while in sight
	bountyPerRam    = 180 // per wanted level, for hitting a unit
	bountyPerBlock  = 450 // for going through a roadblock
)

// PursuitState is what the police response is currently doing.
type PursuitState int

// The pursuit states.
const (
	// PursuitClear means nobody is looking for the driver.
	PursuitClear PursuitState = iota
	// PursuitActive means units are chasing.
	PursuitActive
	// PursuitEvading means the driver is out of sight and the level is
	// counting down.
	PursuitEvading
	// PursuitStopped means the driver has been pulled over.
	PursuitStopped
)

// String returns a short label for the state.
func (s PursuitState) String() string {
	switch s {
	case PursuitActive:
		return "PURSUIT"
	case PursuitEvading:
		return "EVADING"
	case PursuitStopped:
		return "BUSTED"
	default:
		return "CLEAR"
	}
}

// Wanted tracks how much attention the driver has attracted and what the police
// are doing about it.
//
// Heat accumulates from the judge's infractions — the same events the HUD shows
// — but only when the police actually notice: a unit has to be close enough to
// see the offence, or a pursuit has to be under way already. Once one is, heat
// climbs for as long as the driver stays in sight, so a chase escalates on its
// own. The way out is to break the line of sight and keep it broken.
type Wanted struct {
	// Level is the current wanted level, from zero to maxWanted.
	Level int
	// State is what the response is doing.
	State PursuitState
	// Heat is the raw attention score behind the level.
	Heat float32
	// Witnessed counts offences a unit actually saw.
	Witnessed int
	// Reported counts offences nobody saw that were called in anyway.
	Reported int
	// Stops counts how many times the driver has been pulled over.
	Stops int
	// Escapes counts how many pursuits the driver has got away from.
	Escapes int
	// Bounty is what the pursuit in progress is worth, and Banked what has
	// been kept from the ones escaped. Being stopped forfeits the bounty.
	Bounty, Banked int
	// Busting runs from 0 to 1 as a unit holds the driver stopped. At 1 they
	// are pulled over.
	Busting float32
	// Cooldown runs from 0 to 1 while the driver is out of sight. At 1 they
	// have got away.
	Cooldown float32
	// LastSeen is where a unit last had eyes on the driver, which is where the
	// search goes once they lose them.
	LastSeen mathx.Vec

	sinceOffence float32
	sinceRam     float32
	unreported   float32 // unseen offences building toward a call
	hidden       float32
	heldStill    float32
	stoppedFor   float32
	accrued      float32 // bounty earned but not yet a whole point
}

// MaxWanted is the highest wanted level there is.
const MaxWanted = maxWanted

// Active reports whether the police are currently interested in the driver.
func (w *Wanted) Active() bool { return w.Level > 0 }

// Clear drops the wanted level and ends any pursuit.
func (w *Wanted) Clear() {
	w.Level, w.Heat, w.State = 0, 0, PursuitClear
	w.hidden, w.heldStill, w.unreported = 0, 0, 0
	w.Busting, w.Cooldown = 0, 0
}

func (w *Wanted) witness(points int) {
	// Adds heat for an offence a unit saw, and is what the telemetry
	// subscription calls.
	w.Witnessed++
	w.raise(float32(points) * heatPerPoint)
}

// raise adds the heat for a fresh offence, which also restarts the clock on
// how long the driver has been behaving.
func (w *Wanted) raise(heat float32) {
	w.escalate(heat)
	w.sinceOffence = 0
}

// escalate turns heat into a wanted level. A level is never lost by gaining
// heat.
func (w *Wanted) escalate(heat float32) {
	w.Heat = min(w.Heat+heat, heatPerLevel*maxWanted)
	w.Level = max(w.Level, min(1+int(w.Heat/heatPerLevel), maxWanted))
}

// report records an offence no unit saw.
//
// Without this a driver on a deserted street is untouchable: with a handful of
// units wandering a city a kilometre across, most offences happen with nobody
// in sight, and the response never starts however badly the car is driven.
// A witness is still worth more and acts at once — this is somebody watching
// from a window, and it takes a few offences before they pick up the phone.
func (w *Wanted) report(points int) {
	w.unreported += float32(points) * reportWeight
	if w.unreported < reportThreshold {
		return
	}
	// Somebody has called it in. From here it is an ordinary wanted level, so
	// units are dispatched from wherever they happen to be.
	w.unreported = 0
	w.Reported++
	w.raise(float32(points) * heatPerPoint)
}

// earn adds to the bounty of the pursuit in progress.
func (w *Wanted) earn(points float32) {
	w.accrued += points
	whole := int(w.accrued)
	w.Bounty += whole
	w.accrued -= float32(whole)
}

// coolFor is how long the driver has to stay hidden to be lost.
func (w *Wanted) coolFor() float32 { return coolBase + coolPerLevel*float32(w.Level) }

func (w *World) nearestUnit() (float32, bool) {
	// Returns the distance to the closest police unit, and whether
	// there is one at all.
	best, found := float32(1e9), false
	for _, a := range w.Agents {
		if a.Role != RolePolice {
			continue
		}
		if d := a.V.Pos.DistTo(w.Player.Pos); d < best {
			best, found = d, true
		}
	}
	return best, found
}

func (w *World) witnessed() bool {
	// Reports whether an offence draws police attention: either a unit
	// was close enough to see it, or a pursuit is already under way, in which case
	// they are watching by definition.
	if w.Wanted.State == PursuitActive {
		return true
	}
	d, ok := w.nearestUnit()
	return ok && d < noticeRange
}

// inSight reports whether any unit currently knows where the driver is: it
// can see the car down a clear street, or is close enough not to need to.
func (w *World) inSight() bool {
	for _, a := range w.Agents {
		if a.Role != RolePolice {
			continue
		}
		d := a.V.Pos.DistTo(w.Player.Pos)
		if d < closeRange || (d < sightRange && w.LineOfSight(a.V.Pos, w.Player.Pos)) {
			return true
		}
	}
	return false
}

// LineOfSight reports whether a straight line between two points on the
// ground is clear of buildings. It is what decides whether the police can see
// a car, so turning a corner hides it and an open boulevard does not.
func (w *World) LineOfSight(a, b mathx.Vec) bool {
	d := b.Sub(a)
	steps := int(d.Len()/sightStep) + 1
	for i := 1; i < steps; i++ {
		if w.insideBuilding(a.Add(d.Mul(float32(i) / float32(steps)))) {
			return false
		}
	}
	return true
}

// sightStep is how finely a sight line is checked, in metres. No building is
// narrower than this, so none slips between two checks.
const sightStep float32 = 4.5

func (w *World) insideBuilding(p mathx.Vec) bool {
	for _, bi := range w.buildings[w.cellOf(p)] {
		b := w.City.Buildings[bi]
		if mathx.Abs(p.X-b.Center.X) <= b.W/2 && mathx.Abs(p.Z-b.Center.Z) <= b.D/2 {
			return true
		}
	}
	return false
}

func (w *World) updateWanted(dt float32) {
	wa := &w.Wanted
	// A report is forgotten if the driving settles down, so one lapse on an
	// empty street does not summon a car five minutes later.
	wa.unreported = max(wa.unreported-dt*reportFade, 0)
	if wa.stoppedFor > 0 {
		// Hold the "pulled over" state briefly so it can be read, then clear.
		if wa.stoppedFor -= dt; wa.stoppedFor <= 0 {
			wa.Clear()
			w.Damage = 0
		}
		return
	}
	if !wa.Active() {
		wa.State = PursuitClear
		w.setPursuit(false)
		return
	}
	wa.sinceOffence += dt
	wa.sinceRam += dt

	if w.holdStill(dt) {
		return
	}
	if w.inSight() {
		wa.State = PursuitActive
		wa.LastSeen = w.Player.Pos
		// Being found again does not hand back all the ground gained, but it
		// goes quickly.
		wa.hidden = max(wa.hidden-dt*3, 0)
		wa.escalate(heatPerSecond * dt)
		wa.earn(float32(bountyPerSecond*wa.Level) * dt)
	} else {
		wa.State = PursuitEvading
		if wa.hidden += dt; wa.hidden >= wa.coolFor() {
			w.escaped()
			return
		}
	}
	wa.Cooldown = mathx.Clamp(wa.hidden/wa.coolFor(), 0, 1)

	// Heat fades once the driver settles down, though never below what the
	// level already reached is worth.
	if wa.sinceOffence > coolTime && wa.State != PursuitActive {
		wa.Heat = max(wa.Heat-dt*4, float32(wa.Level-1)*heatPerLevel)
	}
	w.setPursuit(true)
}

// holdStill counts a unit holding the driver stopped, and reports whether
// that has just ended the pursuit.
func (w *World) holdStill(dt float32) bool {
	wa := &w.Wanted
	dist, haveUnit := w.nearestUnit()
	switch {
	case haveUnit && dist < stopRange && w.Player.Speed() < 2.5:
		// A unit alongside and the car stopped: the ordinary way it ends.
		wa.heldStill += dt
	case haveUnit && dist < cornerRange && w.Player.Speed() < 1.2 && wa.State == PursuitActive:
		// Stopped in plain view with units close by, wherever the car has got
		// itself to. It takes them longer to walk over, but they will.
		wa.heldStill += dt * cornerRate
	default:
		wa.heldStill = max(wa.heldStill-dt*2, 0)
		wa.Busting = wa.heldStill / stopTime
		return false
	}
	wa.Busting = mathx.Clamp(wa.heldStill/stopTime, 0, 1)
	if wa.heldStill < stopTime {
		return false
	}
	wa.Stops++
	wa.State = PursuitStopped
	wa.stoppedFor = 3.5
	wa.Bounty, wa.accrued = 0, 0
	w.setPursuit(false)
	return true
}

// escaped ends a pursuit in the driver's favour.
func (w *World) escaped() {
	wa := &w.Wanted
	wa.Escapes++
	wa.Banked += wa.Bounty
	wa.Bounty, wa.accrued = 0, 0
	wa.Clear()
	w.Damage = 0
	w.setPursuit(false)
}

func (w *World) setPursuit(on bool) {
	// Commits the nearest units to the call, or stands everybody down. More
	// of the response commits as the level rises, and the faster and heavier
	// units are only sent once the ordinary ones have failed.
	level := w.Wanted.Level
	units := w.units[:0]
	for _, a := range w.Agents {
		if a.Role != RolePolice || a.Manual {
			continue
		}
		a.pursuing, a.rank = false, 0
		if on && !a.Blocking && level >= a.Unit.joinsAt() {
			units = append(units, a)
		}
	}
	w.units = units
	if !on {
		return
	}
	// Nearest first.
	for i := 1; i < len(units); i++ {
		for j := i; j > 0 && units[j].V.Pos.DistTo(w.Player.Pos) < units[j-1].V.Pos.DistTo(w.Player.Pos); j-- {
			units[j], units[j-1] = units[j-1], units[j]
		}
	}
	reach := pursuitRange * (0.55 + 0.15*float32(level))
	for i, a := range units {
		if i >= unitsFor[level] || a.V.Pos.DistTo(w.Player.Pos) > reach {
			break
		}
		a.pursuing, a.rank = true, i
	}
}

// InterceptPoint returns where a pursuing unit should aim for: not the target's
// current position, but where it will be by the time the unit gets there. It is
// a plain constant-velocity lead, which is enough to stop units trailing behind
// a car that is simply driving away from them.
func InterceptPoint(target, velocity mathx.Vec, from mathx.Vec, closing float32) mathx.Vec {
	lead := target.DistTo(from) / max(closing, 6)
	return target.Add(velocity.Mul(mathx.Clamp(lead, 0, 3.5)))
}

func (a *Agent) pursuitTurn(c *city.City, target mathx.Vec) (city.Turn, bool) {
	// Picks the exit from a junction that gets a unit closest to the
	// car it is chasing. It is greedy rather than a full search, which is enough
	// on a grid and costs nothing.
	if len(a.lane.Succ) == 0 {
		return city.Turn{}, false
	}
	turns := a.openTurns(a.lane)
	best, bestDist := turns[0], float32(1e9)
	for _, t := range turns {
		// Score on where the successor lane ends up, so a turn that heads the
		// right way is preferred even if it starts by going the wrong way.
		if d := c.Lanes[t.Lane].B.DistTo(target); d < bestDist {
			best, bestDist = t, d
		}
	}
	return best, true
}

// BluesAndTwos reports whether the unit should be showing its lights, which is
// whenever it is running to a call.
func (a *Agent) BluesAndTwos() bool { return a.Role == RolePolice && a.pursuing }

func (w *World) interceptOf(a *Agent) mathx.Vec {
	// Aim where the runner will be, not where it is. Out of sight, nobody
	// knows where that is, so the unit heads for where the car was last seen.
	if w.Wanted.State == PursuitEvading {
		return w.Wanted.LastSeen
	}
	return InterceptPoint(w.Player.Pos, w.Player.Vel, a.V.Pos, a.V.Speed())
}

// Close-quarters pursuit, in metres and m/s.
const (
	// engageRange is how close a unit has to be, with a clear run at the car,
	// before it stops following the road and goes straight for it.
	engageRange float32 = 60
	// tailGap is how far back a unit sits while it is only following.
	tailGap float32 = 9
	// boxSpeed is how slow the runner has to be going before the units stop
	// ramming and close in around it instead.
	boxSpeed float32 = 3.5
)

// engage drives a unit straight at the runner once it is close and has a clear
// run, and reports whether it did. Until then the unit follows the road like
// anything else, only faster.
//
// What it does when it gets there depends on how wanted the driver is. At the
// first level it sits behind with its lights on and waits for them to stop. After
// that it is trying to stop them: the lead unit goes for the back corner to spin
// the car, the next two come up either side to hem it in, and a heavy unit that
// finds itself ahead simply drives at it. Once the car is nearly stopped
// everybody closes in around it.
func (a *Agent) engage(w *World, dt float32) bool {
	p := w.Player
	dist := a.V.Pos.DistTo(p.Pos)
	if w.Wanted.State != PursuitActive || dist > engageRange || !w.LineOfSight(a.V.Pos, p.Pos) {
		if a.free {
			a.free = false
			a.rejoin(w.City)
		}
		return false
	}
	a.free = true

	target, want := a.tactic(w, dist)
	a.V.Update(a.closeControls(w, target, want, dt), dt)
	return true
}

// closeControls steers a unit in close pursuit at a point, at a speed.
func (a *Agent) closeControls(w *World, target mathx.Vec, want, dt float32) Controls {
	to := target.Sub(a.V.Pos)
	fwd := a.V.Forward()
	alpha := mathx.Atan2(to.Dot(fwd.Right()), to.Dot(fwd))
	speed := a.V.ForwardSpeed()

	c := Controls{}
	angle := mathx.Atan2(2*a.V.Spec.Wheelbase()*mathx.Sin(alpha), max(to.Len(), 2.5))
	c.Steer = mathx.Clamp(angle/a.V.Spec.MaxSteer*1.25, -1, 1)
	walled := w.insideBuilding(a.V.Pos.Add(fwd.Mul(4 + max(speed, 0)*0.45)))
	behind := mathx.Abs(alpha) > 2.1 && to.Len() < 18
	if (walled || behind) && speed < 4 && a.backing <= 0 {
		// Nose against a wall, or the car is behind: back round toward it
		// rather than sit there or drive a circle.
		a.backing = backTime
	}
	switch {
	case a.backing > 0:
		a.backing -= dt
		c.Reverse, c.Throttle, c.Steer = true, 0.85, -mathx.Sign(alpha)
	case walled:
		c.Brake = 1
	case mathx.Abs(alpha) > 0.85 && speed > 10:
		// Pointing well away from where it wants to go: scrub speed to turn.
		c.Brake = 0.55
	case speed < want-0.5:
		c.Throttle = mathx.Clamp((want-speed)/3.5, 0.25, 1)
	case speed > want+1:
		c.Brake = mathx.Clamp((speed-want)/6, 0.1, 1)
	}
	return c
}

// backTime is how long a unit reverses to get its nose off a wall or round
// toward a car behind it, in seconds.
const backTime float32 = 1.1

// tactic is where a unit in close pursuit is trying to put itself, and how
// fast it wants to be going when it gets there.
func (a *Agent) tactic(w *World, dist float32) (mathx.Vec, float32) {
	p := w.Player
	fwd := p.Forward()
	side := fwd.Right()
	speed := p.Speed()

	if speed < boxSpeed {
		// Nearly stopped: take a place around the car and hold it there.
		slot := boxSlots[a.rank%len(boxSlots)]
		at := p.Pos.Add(fwd.Mul(slot.X)).Add(side.Mul(slot.Z))
		return at, mathx.Clamp((a.V.Pos.DistTo(at)-1.2)*1.4, 0, 10)
	}
	if w.Wanted.Level <= 1 {
		// Lights on, sat behind, waiting for the driver to pull over.
		at := p.Pos.Sub(fwd.Mul(tailGap))
		return at, speed + mathx.Clamp((a.V.Pos.DistTo(at)-2)*0.7, -5, 9)
	}

	lead := mathx.Clamp(dist/max(a.V.Speed(), 12), 0, 1.1)
	ahead := p.Pos.Add(p.Vel.Mul(lead))
	chase := min(max(speed+10, 17), a.V.Spec.MaxSpeed)
	switch {
	case a.Unit == UnitHeavy && a.V.Pos.Sub(p.Pos).Dot(fwd) > 12:
		// Already in front of it: meet it head on.
		return p.Pos.Add(p.Vel.Mul(lead * 1.3)), chase
	case a.rank%3 == 1:
		return ahead.Add(side.Mul(2.7)).Add(fwd.Mul(2.2)), chase
	case a.rank%3 == 2:
		return ahead.Sub(side.Mul(2.7)).Add(fwd.Mul(2.2)), chase
	default:
		// The back corner, which is where a nudge turns a car round.
		corner := float32(1.1)
		if a.ID%2 == 0 {
			corner = -corner
		}
		return ahead.Sub(fwd.Mul(1.6)).Add(side.Mul(corner)), chase
	}
}

// boxSlots are the places units take around a stopped car, as distances ahead
// of it (X) and to its right (Z).
var boxSlots = [...]mathx.Vec{
	{X: -5.6, Z: 0},
	{X: 0.4, Z: 3.1},
	{X: 0.4, Z: -3.1},
	{X: 5.8, Z: 0},
	{X: -5.2, Z: 3.2},
	{X: -5.2, Z: -3.2},
	{X: 5.4, Z: 3.2},
	{X: 5.4, Z: -3.2},
}

// sirenRange is how far behind a car a unit on a call has to be for its
// driver to get out of the way, in metres.
const sirenRange float32 = 55

// sirenBehind reports whether a unit with its lights on is coming up behind
// this driver on the same road.
func (w *World) sirenBehind(a *Agent) bool {
	if a.Role == RolePolice {
		return false
	}
	fwd := a.V.Forward()
	for _, u := range w.units {
		if !u.pursuing {
			continue
		}
		rel := u.V.Pos.Sub(a.V.Pos)
		back := -rel.Dot(fwd)
		if back < -4 || back > sirenRange || mathx.Abs(rel.Cross(fwd)) > 9 {
			continue
		}
		if u.V.Forward().Dot(fwd) > 0.3 {
			return true
		}
	}
	return false
}

// Roadblock tuning.
const (
	// blockAt is the wanted level at which units start setting up ahead of the
	// driver instead of only chasing behind.
	blockAt = 2
	// blockAhead is how far up the road a block is set, in metres. Far enough
	// to be built before the car arrives, near enough to be on the route it is
	// actually taking.
	blockAhead float32 = 165
	// blockCars is how many units make up a block, and blockKeep how many are
	// always left to carry on the chase.
	blockCars = 3
	blockKeep = 2
	// blockRedeploy is how long before a block that has been passed is lifted
	// and set again further on.
	blockRedeploy float32 = 22
)

func (w *World) updateRoadblock(dt float32) {
	// Blocks are set ahead of a driver who keeps running, lifted the moment
	// they are no longer wanted, and moved on once they have been passed.
	if w.Wanted.Level < blockAt {
		if w.blocked {
			w.liftRoadblock()
		}
		return
	}
	w.sinceBlock += dt
	if w.blocked && !w.blockRun && w.Player.Pos.Sub(w.blockPos).Dot(w.blockDir) > 8 &&
		w.Player.Pos.DistTo(w.blockPos) < 40 {
		// Through it, or round it.
		w.blockRun = true
		w.Wanted.earn(bountyPerBlock)
	}
	if w.blocked && w.sinceBlock < blockRedeploy {
		return
	}
	w.deployRoadblock()
}

func (w *World) liftRoadblock() {
	for _, a := range w.Agents {
		if a.Blocking {
			a.Blocking = false
			// The unit was put here rather than driving here, so it has to
			// find the road again before it can drive off.
			a.rejoin(w.City)
		}
	}
	w.blocked = false
}

func (w *World) deployRoadblock() {
	// Set the block on the road the driver is actually heading down, which the
	// navigation route already knows.
	pts := w.Route.Centreline(blockAhead, 1, 1)
	if len(pts) == 0 {
		return
	}
	proj := w.City.Project(pts[0])
	if !proj.Valid || w.City.Roads[proj.Lane.Road].Ring {
		return // nowhere to put one, or a roundabout, which cannot be shut
	}
	lane := proj.Lane
	// The middle of the road, not of the lane the driver happens to be in, so
	// the block shuts the whole carriageway.
	across := lane.DirAt(proj.S).Right()
	at := lane.Point(proj.S).Add(across.Mul((0.5 + float32(lane.Index)) * city.LaneWidth))
	if at.DistTo(w.Player.Pos) < 110 {
		return // too close to be set up without being seen arriving
	}

	// Take the units furthest from the driver: the near ones are the pursuit,
	// and pulling those out of the chase would be perverse.
	cars := blockCars
	if hw := w.City.RoadHalfWidth(lane); hw < city.LaneWidth*1.5 {
		cars = 2 // a single carriageway takes two to shut
	}
	units := w.spareUnits(cars)
	if len(units) == 0 {
		return
	}

	w.liftRoadblock()
	hw := w.City.RoadHalfWidth(lane)
	for i, u := range units {
		// Spread them across the carriageway, angled to it as they are parked
		// in reality rather than left neatly in lane.
		off := (float32(i) - float32(len(units)-1)/2) * (hw * 2 / float32(len(units)))
		u.Blocking = true
		u.pursuing, u.free = false, false
		u.dazed = 0
		u.V.Place(at.Add(across.Mul(off)), lane.HeadingAt(proj.S)+1.35)
	}
	w.blocked = true
	w.sinceBlock = 0
	w.blockPos, w.blockDir, w.blockRun = at, lane.DirAt(proj.S), false
}

func (w *World) spareUnits(n int) []*Agent {
	var out []*Agent
	for _, a := range w.Agents {
		if a.Role != RolePolice || a.Manual {
			continue
		}
		out = append(out, a)
	}
	// A block is no use if setting it up leaves nobody to do the chasing, and
	// one car is not a block.
	n = min(n, len(out)-blockKeep)
	if n < 2 {
		return nil
	}
	// Furthest from the driver first, so the pursuit keeps its closest cars.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].V.Pos.DistTo(w.Player.Pos) > out[j-1].V.Pos.DistTo(w.Player.Pos); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out[:n]
}

// Roadblocked reports whether units are currently parked across the road ahead.
func (w *World) Roadblocked() bool { return w.blocked }
