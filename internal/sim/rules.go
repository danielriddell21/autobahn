package sim

import (
	"fmt"

	"github.com/danielriddell21/crucible/ring"
	"github.com/danielriddell21/crucible/telemetry"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// HistoryDepth is how many recent infractions the judge keeps. The log is a
// rolling buffer, so a long session cannot grow it without bound.
const HistoryDepth = 32

// InfractionKind identifies a road rule that was broken.
type InfractionKind int

// The road rules the judge enforces.
const (
	RedLight InfractionKind = iota
	StopSignViolation
	Speeding
	WrongWay
	OffRoad
	CollisionInfraction
)

// String returns a short label for the infraction.
func (k InfractionKind) String() string {
	switch k {
	case RedLight:
		return "RAN RED LIGHT"
	case StopSignViolation:
		return "FAILED TO STOP"
	case Speeding:
		return "SPEEDING"
	case WrongWay:
		return "WRONG WAY"
	case OffRoad:
		return "OFF ROAD"
	default:
		return "COLLISION"
	}
}

// Penalty returns the points added to the driver's score for this kind.
func (k InfractionKind) Penalty() int {
	switch k {
	case RedLight:
		return 60
	case StopSignViolation:
		return 35
	case Speeding:
		return 25
	case WrongWay:
		return 40
	case OffRoad:
		return 10
	default:
		return 30
	}
}

// Infraction is one recorded rule violation.
type Infraction struct {
	Kind   InfractionKind
	At     float32 // simulation time in seconds
	Pos    mathx.Vec
	Detail string
	Points int
}

// Judge watches one vehicle and records the road rules it breaks. The player
// and the autopilot are both scored by an identical judge, so the two are
// directly comparable.
type Judge struct {
	// Points is the running penalty total.
	Points int
	// Distance is how far the judged vehicle has driven, in metres.
	Distance float32
	// Faults is the number of infractions recorded, including any that have
	// since rolled out of the bounded history.
	Faults int
	// Events publishes every infraction as it happens, so the HUD can react
	// without polling. The bus is generic over this game's own event type.
	Events *telemetry.Bus[Infraction]

	history *ring.Ring[Infraction]

	city *city.City

	// Current state, refreshed every step for the HUD.
	lane      *city.Lane
	limit     float32
	onRoad    bool
	signal    SignalState
	hasSignal bool

	approach    *city.Lane
	judgedPass  bool
	stopDone    bool
	speedTimer  float32
	wrongTimer  float32
	offTimer    float32
	cooldown    map[InfractionKind]float32
	lastOdo     float32
	initialised bool
}

// Judged state timing, in seconds.
const (
	speedGraceTime  float32 = 1.4
	wrongWayTime    float32 = 0.9
	offRoadTime     float32 = 1.2
	speedTolerance  float32 = 5 * mathx.MetresPerSecondPerMPH // 5 mph of leeway
	infractionPause float32 = 5.0
)

// NewJudge returns a judge for vehicles driving in c.
func NewJudge(c *city.City) *Judge {
	return &Judge{
		city:     c,
		cooldown: map[InfractionKind]float32{},
		history:  ring.New[Infraction](HistoryDepth),
		Events:   telemetry.NewBus[Infraction](),
	}
}

// Reset clears the log and score. Subscribers stay attached.
func (j *Judge) Reset() {
	j.history = ring.New[Infraction](HistoryDepth)
	j.Faults = 0
	j.Points = 0
	j.Distance = 0
	j.approach = nil
	j.judgedPass = false
	j.stopDone = false
	j.speedTimer, j.wrongTimer, j.offTimer = 0, 0, 0
	clear(j.cooldown)
	j.initialised = false
}

// Lane returns the lane the judged vehicle is currently nearest to, which may
// be nil before the first update.
func (j *Judge) Lane() *city.Lane { return j.lane }

// SpeedLimit returns the limit applying at the vehicle's position, in m/s.
func (j *Judge) SpeedLimit() float32 { return j.limit }

// OnRoad reports whether the vehicle is within the carriageway.
func (j *Judge) OnRoad() bool { return j.onRoad }

// Signal returns the aspect facing the vehicle and whether one applies.
func (j *Judge) Signal() (SignalState, bool) { return j.signal, j.hasSignal }

// Log returns the retained infraction history, oldest first.
func (j *Judge) Log() []Infraction { return j.history.Slice() }

// Recent returns up to n of the most recent infractions, newest first.
func (j *Judge) Recent(n int) []Infraction {
	all := j.history.Slice()
	n = min(n, len(all))
	out := make([]Infraction, 0, n)
	for i := len(all) - 1; i >= len(all)-n; i-- {
		out = append(out, all[i])
	}
	return out
}

// Update evaluates one simulation step for the given vehicle.
func (j *Judge) Update(v *Vehicle, sig *Signals, now, dt float32) {
	for k, t := range j.cooldown {
		if t -= dt; t <= 0 {
			delete(j.cooldown, k)
		} else {
			j.cooldown[k] = t
		}
	}
	if !j.initialised {
		j.lastOdo, j.initialised = v.Odometer, true
	}
	j.Distance += v.Odometer - j.lastOdo
	j.lastOdo = v.Odometer

	proj := j.city.Project(v.Pos)
	if !proj.Valid {
		return
	}
	j.lane = proj.Lane
	j.limit = proj.Lane.SpeedLimit
	j.onRoad = proj.Dist <= j.city.RoadHalfWidth(proj.Lane)+0.4
	j.signal, j.hasSignal = SignalGreen, false
	if proj.Lane.Control == city.ControlSignal {
		j.signal, j.hasSignal = sig.LaneState(proj.Lane), true
	}

	speed := v.Speed()
	j.checkSpeed(v, speed, now, dt)
	j.checkDirection(v, proj, speed, now, dt)
	j.checkOffRoad(v, proj, speed, now, dt)
	j.checkJunction(v, proj, sig, speed, now)
}

func (j *Judge) checkSpeed(v *Vehicle, speed, now, dt float32) {
	if speed > j.limit+speedTolerance {
		j.speedTimer += dt
		if j.speedTimer >= speedGraceTime {
			j.record(Infraction{
				Kind: Speeding, At: now, Pos: v.Pos,
				Detail: fmt.Sprintf("%.0f mph in a %.0f zone",
					mathx.ToMPH(speed), mathx.ToMPH(j.limit)),
			})
			j.speedTimer = 0
		}
		return
	}
	j.speedTimer = 0
}

func (j *Judge) checkDirection(v *Vehicle, proj city.LaneProjection, speed, now, dt float32) {
	// The nearest lane is the one the car occupies. Travelling against it means
	// the car is on the wrong side of the road.
	if speed > 3 && v.Vel.Dot(proj.Lane.Fwd) < -0.5*speed && j.onRoad {
		j.wrongTimer += dt
		if j.wrongTimer >= wrongWayTime {
			j.record(Infraction{
				Kind: WrongWay, At: now, Pos: v.Pos,
				Detail: "driving against the flow",
			})
			j.wrongTimer = 0
		}
		return
	}
	j.wrongTimer = 0
}

func (j *Judge) checkOffRoad(v *Vehicle, proj city.LaneProjection, speed, now, dt float32) {
	if !j.onRoad && speed > 1.5 {
		j.offTimer += dt
		if j.offTimer >= offRoadTime {
			j.record(Infraction{
				Kind: OffRoad, At: now, Pos: v.Pos,
				Detail: fmt.Sprintf("%.1fm from the carriageway",
					proj.Dist-j.city.RoadHalfWidth(proj.Lane)),
			})
			j.offTimer = 0
		}
		return
	}
	j.offTimer = 0
}

func (j *Judge) checkJunction(v *Vehicle, proj city.LaneProjection, sig *Signals, speed, now float32) {
	l := proj.Lane
	// Track the approach lane while the car is still short of the stop line.
	beyond := v.Pos.Sub(l.B).Dot(l.Fwd)
	if beyond < -0.5 && l.Control != city.ControlNone && v.Vel.Dot(l.Fwd) > 0 {
		if j.approach != l {
			j.approach, j.judgedPass, j.stopDone = l, false, false
		}
		// A full stop anywhere near the line satisfies a stop sign.
		if beyond > -4.5 && speed < 0.7 {
			j.stopDone = true
		}
		return
	}

	// The moment the car crosses the line, judge the approach it came from.
	if j.approach == nil || j.judgedPass || j.approach != l {
		return
	}
	if beyond < 0 || speed < 0.6 {
		return
	}
	j.judgedPass = true
	switch l.Control {
	case city.ControlSignal:
		if sig.LaneState(l).MustStop() {
			j.record(Infraction{
				Kind: RedLight, At: now, Pos: v.Pos,
				Detail: fmt.Sprintf("entered on red at %.0f mph", mathx.ToMPH(speed)),
			})
		}
	case city.ControlStop:
		if !j.stopDone {
			j.record(Infraction{
				Kind: StopSignViolation, At: now, Pos: v.Pos,
				Detail: fmt.Sprintf("rolled the line at %.0f mph", mathx.ToMPH(speed)),
			})
		}
	}
}

// ReportCollision records a collision against the judged vehicle. Severity is
// the impact speed in metres per second.
func (j *Judge) ReportCollision(v *Vehicle, with string, severity, now float32) {
	if severity < 2.2 {
		return // kerb scrapes and gentle nudges are not worth logging
	}
	j.record(Infraction{
		Kind: CollisionInfraction, At: now, Pos: v.Pos,
		Detail: fmt.Sprintf("%s at %.0f km/h", with, mathx.ToKPH(severity)),
	})
}

func (j *Judge) record(in Infraction) {
	if _, muted := j.cooldown[in.Kind]; muted {
		return
	}
	in.Points = in.Kind.Penalty()
	j.Points += in.Points
	j.Faults++
	j.history.Push(in)
	j.Events.Publish(in)
	j.cooldown[in.Kind] = infractionPause
}
