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

// PoliceSpec returns the handling profile of a marked police car: quicker and
// better planted than ambient traffic, so a pursuit is not hopeless either way.
func PoliceSpec() Spec {
	s := CarSpec()
	s.EngineForce = 10600
	s.MaxSpeed = 58
	s.GripFront, s.GripRear = 86000, 92000
	return s
}

// Wanted tuning.
const (
	// heatPerPoint converts a penalty point into wanted heat.
	heatPerPoint float32 = 0.9
	// heatPerLevel is how much heat each wanted level costs.
	heatPerLevel float32 = 45
	// maxWanted is the highest level the response escalates to.
	maxWanted = 4
	// noticeRange is how close a unit must be to see an offence committed.
	noticeRange float32 = 95
	// pursuitRange is how far a unit will chase before it gives up.
	pursuitRange float32 = 520
	// escapeRange is the distance at which the driver counts as out of sight.
	escapeRange float32 = 190
	// escapeTime is how long they must stay out of sight to shed a level.
	escapeTime float32 = 7.5
	// stopRange and stopTime are what it takes to be pulled over: a unit
	// alongside, and the car stationary.
	stopRange float32 = 9.5
	stopTime  float32 = 2.4
	// coolTime is how long without a new offence before heat starts to fade.
	coolTime float32 = 6
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
		return "PULLED OVER"
	default:
		return "CLEAR"
	}
}

// Wanted tracks how much attention the driver has attracted and what the police
// are doing about it.
//
// Heat accumulates from the judge's infractions — the same events the HUD shows
// — but only when the police actually notice: a unit has to be close enough to
// see the offence, or a pursuit has to be under way already. Heat fades once the
// driver settles down, and a level is shed by staying out of sight.
type Wanted struct {
	// Level is the current wanted level, from zero to maxWanted.
	Level int
	// State is what the response is doing.
	State PursuitState
	// Heat is the raw attention score behind the level.
	Heat float32
	// Witnessed counts offences a unit actually saw.
	Witnessed int
	// Stops counts how many times the driver has been pulled over.
	Stops int

	sinceOffence float32
	outOfSight   float32
	heldStill    float32
	stoppedFor   float32
}

// Active reports whether the police are currently interested in the driver.
func (w *Wanted) Active() bool { return w.Level > 0 }

// Clear drops the wanted level and ends any pursuit.
func (w *Wanted) Clear() {
	w.Level, w.Heat, w.State = 0, 0, PursuitClear
	w.outOfSight, w.heldStill = 0, 0
}

func (w *Wanted) witness(points int) {
	// Adds heat for an offence a unit saw, and is what the telemetry
	// subscription calls.
	w.Witnessed++
	w.Heat += float32(points) * heatPerPoint
	w.sinceOffence = 0
	w.Level = min(1+int(w.Heat/heatPerLevel), maxWanted)
}

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

func (w *World) updateWanted(dt float32) {
	wa := &w.Wanted
	if wa.stoppedFor > 0 {
		// Hold the "pulled over" state briefly so it can be read, then clear.
		if wa.stoppedFor -= dt; wa.stoppedFor <= 0 {
			wa.Clear()
		}
		return
	}
	if !wa.Active() {
		wa.State = PursuitClear
		w.setPursuit(false)
		return
	}

	dist, haveUnit := w.nearestUnit()
	wa.sinceOffence += dt

	// Being caught: a unit alongside a car that has given up running.
	if haveUnit && dist < stopRange && w.Player.Speed() < 2.5 {
		if wa.heldStill += dt; wa.heldStill >= stopTime {
			wa.Stops++
			wa.State = PursuitStopped
			wa.stoppedFor = 3.5
			w.setPursuit(false)
			return
		}
	} else {
		wa.heldStill = 0
	}

	// Getting away: stay clear of every unit for long enough and a level goes.
	if haveUnit && dist < escapeRange {
		wa.outOfSight = 0
		wa.State = PursuitActive
	} else {
		wa.State = PursuitEvading
		if wa.outOfSight += dt; wa.outOfSight >= escapeTime {
			wa.outOfSight = 0
			wa.Level--
			wa.Heat = float32(max(wa.Level-1, 0)) * heatPerLevel
			if wa.Level <= 0 {
				wa.Clear()
				return
			}
		}
	}

	// Heat fades once the driver settles down, which lets a single mistake
	// blow over without being chased across the city.
	if wa.sinceOffence > coolTime {
		wa.Heat = max(wa.Heat-dt*4, float32(max(wa.Level-1, 0))*heatPerLevel)
	}
	w.setPursuit(true)
}

func (w *World) setPursuit(on bool) {
	// Puts every unit within range onto the call, or stands them down.
	// More of the response commits as the level rises, so a driver who keeps
	// offending finds units arriving from further away.
	reach := pursuitRange * (0.55 + 0.15*float32(w.Wanted.Level))
	for _, a := range w.Agents {
		if a.Role != RolePolice || a.Manual {
			continue
		}
		a.pursuing = on && a.V.Pos.DistTo(w.Player.Pos) < reach
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
	best, bestDist := a.lane.Succ[0], float32(1e9)
	for _, t := range a.lane.Succ {
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
	// Aim where the runner will be, not where it is.
	return InterceptPoint(w.Player.Pos, w.Player.Vel, a.V.Pos, a.V.Speed())
}
