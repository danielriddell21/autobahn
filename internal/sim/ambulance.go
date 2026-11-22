package sim

import (
	"math/rand/v2"

	"github.com/danielriddell21/crucible/rng"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// The emergency pack's ambulance in ambient traffic. It is a role, like
// police, with a count of its own in [Config.Ambulances] and a stream of its
// own: it is never drawn from a map's traffic mix, so a world with none is the
// world it always was.
//
// An ambulance drives like anybody else until it goes out on a call, which it
// does now and then: blue lights and siren for twenty seconds or so, quicker
// than the limit, pulling out round whatever is slow in front of it. Traffic
// pulls in for it, as it does for a police car on a call; so must the player,
// and a player who sits in its way is judged for it.

// streamAmbulance is the crucible stream ambulances are placed from.
const streamAmbulance uint64 = 5

// The timetable and the rules of the road for an ambulance on a run, in
// seconds and metres.
const (
	// runShortest and runLongest bound how long a run lasts, and restShortest
	// and restLongest how long an ambulance goes about its business between.
	runShortest, runLongest   float32 = 18, 30
	restShortest, restLongest float32 = 6, 16
	// obstructRange is how far ahead of an ambulance with its lights on a
	// vehicle is in its way, and obstructTime how long it may be before that
	// is a fault.
	obstructRange float32 = 25
	obstructTime  float32 = 3
	// obstructWidth is how far to the side of the ambulance's line a vehicle's
	// middle may be and still be in the way.
	obstructWidth float32 = 2.2
	// haltSpeed is the speed, in m/s, below which a vehicle is as good as
	// stopped in an ambulance's way, however fast the ambulance is coming.
	haltSpeed float32 = 3
	// runSpeed is the speed, in m/s, an ambulance on a run drives at when the
	// road allows.
	runSpeed = 45 * mathx.MetresPerSecondPerMPH
	// crawlSpeed is the speed, in m/s, traffic eases to while it pulls in for an
	// ambulance. It does not stop dead in the road: the cars behind it have to
	// stop for whatever it does, and the ambulance goes round a crawl.
	crawlSpeed float32 = 2.5
)

func newAmbulance(id int, c *city.City, lane *city.Lane, s float32, rng *rand.Rand, pl place) *Agent {
	a := newAgent(id, c, lane, s, rng, pl)
	a.Role = RoleAmbulance
	a.Style = Normal()
	a.Body = BodyOf(Ambulance)
	a.Paint = 0
	a.V.Spec = a.Body.Spec.OnSurface(a.surface)
	a.rest = mathx.Lerp(restShortest, restLongest, rng.Float32())
	return a
}

// spawnAmbulances puts the ambulances a config asks for on the road, from their
// own stream, after everything else is in place.
func (w *World) spawnAmbulances(cfg Config, pl place) {
	if cfg.Ambulances <= 0 {
		return
	}
	c := w.City
	amb := rng.Stream(cfg.Seed, streamAmbulance)
	for i := range cfg.Ambulances {
		lane := c.RandomLane(amb, 30)
		u := newAmbulance(cfg.Traffic+cfg.Police+i, c, lane, amb.Float32()*lane.Length, amb, pl)
		if w.tooClose(u.V.Pos, 9) || w.Works.Barred(lane.ID) {
			continue
		}
		u.works = w.Works
		if u.closedTurn() {
			u.chooseNext(c)
		}
		w.Agents = append(w.Agents, u)
		w.ambulances = append(w.ambulances, u)
	}
}

// NewAmbulance spawns an ambulance, which goes about its business like any
// other vehicle until it is out on a call.
func NewAmbulance(id int, c *city.City, lane *city.Lane, s float32, rng *rand.Rand) *Agent {
	return newAmbulance(id, c, lane, s, rng, placeOf(c))
}

// BlueLights reports whether an ambulance is on a run, with its lights and
// siren on. It is the one thing that makes it something to get out of the way
// of, and the one thing the camera marks it for.
func (a *Agent) BlueLights() bool { return a.Role == RoleAmbulance && a.blue > 0 }

// StartRun sends an ambulance out on a call for the given time.
func (a *Agent) StartRun(seconds float32) { a.blue, a.rest = seconds, 0 }

// onCall is whether the agent is a unit that wants to be past anybody slower
// than it is: a police car on a pursuit or an ambulance on a run.
func (a *Agent) onCall() bool { return a.pursuing || a.BlueLights() }

// runClock keeps an ambulance's timetable: out for a while, then back to
// ordinary driving for a while.
func (a *Agent) runClock(dt float32) {
	if a.Role != RoleAmbulance {
		return
	}
	if a.blue > 0 {
		if a.blue -= dt; a.blue <= 0 {
			a.blue, a.rest = 0, mathx.Lerp(restShortest, restLongest, a.rng.Float32())
		}
		return
	}
	if a.rest -= dt; a.rest <= 0 {
		a.blue = mathx.Lerp(runShortest, runLongest, a.rng.Float32())
	}
}

// Ambulances returns the ambulances in the world.
func (w *World) Ambulances() []*Agent { return w.ambulances }

// ambulanceBehind reports whether an ambulance on a run is behind a vehicle, within
// the range of its siren, on the same road and going the same way.
func (w *World) ambulanceBehind(pos, fwd mathx.Vec) bool {
	for _, u := range w.ambulances {
		if !u.BlueLights() {
			continue
		}
		rel := u.V.Pos.Sub(pos)
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

// obstructing reports whether the player is in the way of an ambulance on a
// run: within twenty-five metres ahead of it, in its line, going its way, and
// holding it up.
func (w *World) obstructing() bool {
	p := w.Player
	for _, u := range w.ambulances {
		if !u.BlueLights() {
			continue
		}
		fwd := u.V.Forward()
		rel := p.Pos.Sub(u.V.Pos)
		ahead := rel.Dot(fwd)
		if ahead <= 0 || ahead > obstructRange || mathx.Abs(rel.Cross(fwd)) > obstructWidth {
			continue
		}
		if p.Forward().Dot(fwd) < 0.3 {
			continue
		}
		// Slow enough to hold an ambulance up, and either stopped or not being
		// gained on: an ambulance coming up fast is about to be past.
		slow := p.ForwardSpeed() < runSpeed*0.9
		if slow && (p.ForwardSpeed() < haltSpeed || u.V.ForwardSpeed() <= p.ForwardSpeed()+1) {
			return !w.queued(p)
		}
	}
	return false
}

// queueGap is how close behind a stopped thing, bumper to bumper in metres, a
// vehicle is in a queue it cannot leave.
const queueGap float32 = 6

// queued reports whether a vehicle has a stopped vehicle, cone or barrier
// directly in front of it: it cannot make way for anybody, only wait.
func (w *World) queued(p *Vehicle) bool {
	fwd := p.Forward()
	behind := func(o *Vehicle) bool {
		rel := o.Pos.Sub(p.Pos)
		ahead := rel.Dot(fwd)
		if ahead <= 0 || o.Speed() >= haltSpeed {
			return false
		}
		gap := ahead - p.Spec.HalfLength - o.Spec.HalfLength
		return gap < queueGap && mathx.Abs(rel.Cross(fwd)) < p.Spec.HalfWidth+o.Spec.HalfWidth+0.3
	}
	for _, a := range w.Agents {
		if behind(a.V) {
			return true
		}
	}
	for _, s := range w.stubs {
		if behind(s) {
			return true
		}
	}
	return false
}

// heldByTheRoad reports whether the judged vehicle is somewhere it cannot
// simply move away from: held at a signal that is not green, standing at a stop
// or give-way line, inside a junction or at a roundabout's entry.
func (j *Judge) heldByTheRoad(v *Vehicle) bool {
	if j.hasSignal && j.signal != SignalGreen {
		return true
	}
	if j.inBox || onRoundabout(j.city, v.Pos) {
		return true
	}
	l := j.lane
	if l == nil {
		return false
	}
	if l.Control == city.ControlStop || l.Control == city.ControlGiveWay {
		beyond := v.Pos.Sub(l.B).Dot(l.Fwd)
		return beyond > -lineHold && beyond < 2
	}
	// A roundabout's approach carries no control of its own: it is the island
	// that is the junction.
	n := j.city.Nodes[l.ToNode]
	return n.Roundabout && v.Pos.DistTo(n.Pos) < n.RingOuter()+lineHold
}

// lineHold is how far short of a stop line, or of a roundabout, a vehicle is
// held by it, in metres.
const lineHold float32 = 8

// Obstruct is told each step whether the judged vehicle is in the way of an
// ambulance, and records the fault once it has been for long enough. A vehicle
// that cannot make way is excused: see [Judge.heldByTheRoad], and
// [World.obstructing] for one queued behind something stopped.
func (j *Judge) Obstruct(v *Vehicle, blocking bool, now, dt float32) {
	if !blocking || j.heldByTheRoad(v) {
		j.obstructTimer = 0
		return
	}
	if j.obstructTimer += dt; j.obstructTimer >= obstructTime {
		j.record(Infraction{
			Kind: ObstructedEmergency, At: now, Pos: v.Pos,
			Detail: "in the way of an ambulance with its lights on",
		})
		j.obstructTimer = 0
	}
}
