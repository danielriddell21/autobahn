package sim

import (
	"math/rand/v2"

	"github.com/danielriddell21/crucible/rng"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/weather"
	"github.com/danielriddell21/autobahn/internal/works"
)

// The crucible random streams this package draws from. Each subsystem has its
// own, so changing how one of them consumes randomness never disturbs another.
const (
	streamTraffic uint64 = 2
	streamSignals uint64 = 3
	streamRoute   uint64 = 4
)

// Simulation tuning.
const (
	// keepRadius is how far from the player a traffic car may stray before it
	// is recycled somewhere closer, which keeps density high where it is seen.
	keepRadius float32 = 420
	spawnNear  float32 = 120
	spawnFar   float32 = 300
	// popRange is how close a car may be put down in plain view of the
	// player. Nearer than this it has to be round a corner.
	popRange float32 = 210
	// restitution is the bounciness of a vehicle impact.
	restitution float32 = 0.22
)

// Config describes a world to build.
type Config struct {
	Seed uint64
	// Map is which place to build. The zero value is the city.
	Map     city.Map
	Traffic int
	// Police is how many marked units patrol the city. Zero disables the
	// police entirely, which leaves the judge still scoring but nobody
	// responding.
	Police int
	// PlayerBody is the player's car. Left empty, it is the usual one.
	PlayerBody Body
	// Works is how much of the town is dug up. The zero value is the normal
	// amount, so roadworks are part of the world unless somebody turns them off.
	Works works.Level
	// Weather is how the sky is chosen. The zero value drifts through the spells
	// the seed gives it; hold it at weather.Clear for the dry runs the gate scores.
	Weather weather.Setting
}

// DefaultConfig returns a populated city configuration.
func DefaultConfig(seed uint64) Config {
	return Config{Seed: seed, Traffic: 70, Police: 7}
}

// World is the whole simulation: the city, its traffic, the signal controller
// and the judge watching the player's car.
type World struct {
	City    *city.City
	Signals *Signals
	Player  *Vehicle
	// PlayerBody is the shape of the player's car, for whoever draws it.
	PlayerBody Body
	Agents     []*Agent
	Judge      *Judge
	// Wanted tracks the police response to how the player has been driving.
	Wanted Wanted
	// Route is the player car's navigation plan, used to draw lane guidance
	// into the annotated camera image.
	Route *Route
	// Works is the roadworks in force, brought up to Time every step.
	Works *works.Index
	// Climate is the weather the world is driven in, applied every step.
	Climate Climate

	// Time is the elapsed simulation time in seconds.
	Time float32
	// Damage is how battered the player's car is, from 0 to 1. Every hard
	// knock adds to it, and a car at 1 has very little left to give.
	Damage float32

	// solid is every cone and barrier in the road at the moment, and stubs the
	// stopped stand-ins traffic queues behind at the start of each; see works.go.
	solid    []mathx.OBB
	stubs    []*Vehicle
	stubPool []*Vehicle

	rng       *rand.Rand
	buildings map[[2]int][]int
	cellSize  float32
	// indexed is how many buildings have been filed, so growth only indexes
	// what has just appeared.
	indexed int
	// parked files the parked cars the same way, and parkedSeen is how many
	// of them have been.
	parked     map[[2]int][]int
	parkedSeen int
	blocked    bool
	sinceBlock float32
	// units is scratch space for the police units that could answer a call,
	// nearest first, and after setPursuit the ones on it lead the slice.
	units []*Agent
	// blockPos and blockDir are where the current roadblock stands and which
	// way the road runs through it; blockRun is set once it has been passed.
	blockPos, blockDir mathx.Vec
	blockRun           bool
	// wedged and backing are how long the reckless driver has been stuck
	// against something, and how much longer it will reverse away from it.
	wedged, backing float32
	// near is scratch space for the vehicles around whichever agent is looking
	// for the car in front, reused so the search does not allocate.
	near []*Vehicle
	// spawn is where the player restarts.
	spawnPos mathx.Vec
	spawnYaw float32
}

// NewWorld generates a city and populates it with traffic.
func NewWorld(cfg Config) *World {
	traffic := rng.Stream(cfg.Seed, streamTraffic)
	c := city.Generate(city.ParamsFor(cfg.Map, cfg.Seed))

	w := &World{
		City: c, Signals: NewSignals(c, cfg.Seed),
		Judge: NewJudge(c),
		rng:   traffic, buildings: map[[2]int][]int{}, cellSize: 48,
		parked:  map[[2]int][]int{},
		Climate: NewClimate(cfg.Seed, cfg.Map, cfg.Weather),
	}
	w.indexBuildings()
	w.Works = works.NewIndex(c, cfg.Seed, cfg.Works)
	w.syncWorks()

	// Put the player on a decent length of arterial near the middle.
	start := c.Lanes[0]
	best := float32(-1)
	for _, l := range c.Lanes {
		score := l.Length - l.A.Len()*0.35
		if l.Class == city.Arterial {
			score += 60
		}
		if score > best {
			best, start = score, l
		}
	}
	w.spawnPos = start.Point(min(18, start.Length*0.35))
	w.spawnYaw = start.HeadingAt(min(18, start.Length*0.35))
	w.PlayerBody = cfg.PlayerBody
	if w.PlayerBody.Spec.Mass == 0 {
		w.PlayerBody = PlayerBody()
	}
	w.Player = NewVehicle(seasoned(w.PlayerBody.Spec, c), w.spawnPos, w.spawnYaw)
	w.Player.Nitro = 1
	w.Route = NewRoute(c, rng.Stream(cfg.Seed, streamRoute))
	w.Route.works = w.Works
	w.Route.Update(w.Player.Pos, w.Player.Forward())

	for i := range cfg.Traffic {
		lane := c.RandomLane(traffic, 30)
		a := NewAgent(i, c, lane, traffic.Float32()*lane.Length, traffic)
		if w.tooClose(a.V.Pos, 9) || w.Works.Barred(lane.ID) {
			continue
		}
		a.works = w.Works
		if a.closedTurn() {
			a.chooseNext(c)
		}
		w.Agents = append(w.Agents, a)
	}
	for i := range cfg.Police {
		lane := c.RandomLane(traffic, 30)
		u := NewPoliceUnit(cfg.Traffic+i, c, lane, traffic.Float32()*lane.Length, traffic)
		u.SetUnit(unitRoster[i%len(unitRoster)])
		if w.tooClose(u.V.Pos, 9) || w.Works.Barred(lane.ID) {
			continue
		}
		u.works = w.Works
		if u.closedTurn() {
			u.chooseNext(c)
		}
		w.Agents = append(w.Agents, u)
	}
	w.watchForOffences()
	w.Climate.Apply(w)
	return w
}

func (w *World) watchForOffences() {
	// Subscribes the police to the judge. An offence a unit saw draws attention
	// at once; one nobody saw is remembered, and enough of them get called in.
	w.Judge.Events.Subscribe(judgeWatcher(func(in Infraction) {
		// Seen or phoned in, an offence says where the car was when it was
		// committed, which is where anybody sent after it starts looking.
		if w.Wanted.State != PursuitActive {
			w.Wanted.LastSeen = in.Pos
		}
		if w.witnessed() {
			w.Wanted.witness(in.Points)
			return
		}
		w.Wanted.report(in.Points)
	}))
}

// judgeWatcher adapts a function to the telemetry subscriber interface.
type judgeWatcher func(Infraction)

// OnEvent implements the telemetry subscriber interface.
func (f judgeWatcher) OnEvent(in Infraction) { f(in) }

// Police returns the marked units in the world.
func (w *World) Police() []*Agent {
	var out []*Agent
	for _, a := range w.Agents {
		if a.Role == RolePolice {
			out = append(out, a)
		}
	}
	return out
}

// Chaser returns the human-driven police unit, or nil if none has been
// assigned.
func (w *World) Chaser() *Agent {
	for _, a := range w.Agents {
		if a.Manual {
			return a
		}
	}
	return nil
}

// AssignChaser hands a police unit to a human and returns it, placing it on the
// road behind the runner so a chase starts with something to chase. Everything
// else about the unit — its livery, its lights, the fact that catching the
// runner ends the pursuit — is unchanged. Calling it again returns the unit
// already assigned.
func (w *World) AssignChaser() *Agent {
	if a := w.Chaser(); a != nil {
		return a
	}
	for _, a := range w.Agents {
		if a.Role != RolePolice {
			continue
		}
		a.Manual = true
		a.pursuing = true
		// Put the unit on the road behind the runner, so the chase starts with
		// something to chase rather than a search.
		behind := w.Player.Pos.Sub(w.Player.Forward().Mul(34))
		if p := w.City.Project(behind); p.Valid {
			a.V.Place(p.Lane.Point(p.S), p.Lane.HeadingAt(p.S))
		}
		return a
	}
	return nil
}

// Caught reports whether the human-driven unit has the runner stopped, which is
// the same test the AI units are held to.
func (w *World) Caught() bool {
	c := w.Chaser()
	if c == nil {
		return false
	}
	return c.V.Pos.DistTo(w.Player.Pos) < stopRange && w.Player.Speed() < 2.5
}

// Respawn returns the player to the starting pose, in a car that has been
// straightened out.
func (w *World) Respawn() {
	w.Player.Place(w.spawnPos, w.spawnYaw)
	w.Player.Nitro = 1
	w.Damage = 0
}

// PlaceOnNearestLane drops the player onto the nearest lane facing the right
// way, which recovers a car that has been beached on a pavement.
func (w *World) PlaceOnNearestLane() {
	proj := w.City.Project(w.Player.Pos)
	if !proj.Valid {
		w.Respawn()
		return
	}
	w.Player.Place(proj.Lane.Point(proj.S), proj.Lane.HeadingAt(proj.S))
}

// Update advances the whole simulation by dt seconds, driving the player's car
// with the given controls.
func (w *World) Update(c Controls, dt float32) {
	w.Time += dt
	w.Climate.Apply(w)
	w.grow()
	w.syncWorks()
	w.Signals.Update(dt)
	if w.Damage >= 1 {
		// A wrecked car still moves, but only just.
		c.Throttle *= wreckedPower
		c.Boost = false
	}
	w.tilt()
	w.Player.Update(c, dt)
	if w.City.AtSea(w.Player.Pos) {
		// There is nothing out there to drive on. The car is put back on
		// the nearest road, which is the front.
		w.PlaceOnNearestLane()
	}
	for _, a := range w.Agents {
		a.drive(w, dt)
	}
	w.resolveCollisions()
	w.updateWanted(dt)
	w.updateRoadblock(dt)
	w.recycleAgents()
	w.Route.Update(w.Player.Pos, w.Player.Forward())
	w.Judge.Update(w.Player, w.Signals, w.Time, dt)
}

// tilt tells every car how steeply the road climbs under it. A car is driven
// on the plane; the hill it is on reaches it as a pull, and nothing else.
func (w *World) tilt() {
	if !w.City.Hilly() {
		return
	}
	w.Player.Grade = w.City.Grade(w.Player.Pos, w.Player.Forward())
	for _, a := range w.Agents {
		a.V.Grade = w.City.Grade(a.V.Pos, a.V.Forward())
	}
}

// GrowRadius is how far ahead of the player the city is brought into
// existence, in metres. It is comfortably beyond keepRadius, so traffic
// recycling always has somewhere built to put a car.
//
// A renderer needs it too: it is the promise that ground within this distance
// of the player exists, and the point past which there is nothing to draw.
const GrowRadius float32 = 700

// grow extends the city around the player. Everything already there is left
// alone, so this costs nothing once the surroundings exist and only does work
// when the car reaches somewhere new.
func (w *World) grow() { w.EnsureBuilt(w.Player.Pos) }

// EnsureBuilt extends the city around a point, whoever is standing there.
//
// [World.Update] does this for the player, which covers a world this machine
// is simulating. A player who has joined somebody else's chase simulates
// nothing — they send controls and draw the poses that come back — so this is
// the only thing that extends their copy of the city as the chase leaves the
// ground it started on.
func (w *World) EnsureBuilt(at mathx.Vec) {
	w.City.EnsureAround(at, GrowRadius)
	w.indexBuildings()
}

// indexBuildings files the buildings that have appeared since it last ran.
func (w *World) indexBuildings() {
	for i, b := range w.City.Buildings {
		if i < w.indexed {
			continue
		}
		// Filed by the square it fits in whichever way it is turned.
		reach := (b.W + b.D) / 2
		lo := mathx.V(b.Center.X-reach, b.Center.Z-reach)
		hi := mathx.V(b.Center.X+reach, b.Center.Z+reach)
		for gx := int(lo.X / w.cellSize); gx <= int(hi.X/w.cellSize); gx++ {
			for gz := int(lo.Z / w.cellSize); gz <= int(hi.Z/w.cellSize); gz++ {
				key := [2]int{gx, gz}
				w.buildings[key] = append(w.buildings[key], i)
			}
		}
	}
	w.indexed = len(w.City.Buildings)

	// A parked car is small enough to file under the one cell its middle is
	// in; a collision looks in the cells either side as well.
	for i := w.parkedSeen; i < len(w.City.Parked); i++ {
		key := w.cellOf(w.City.Parked[i].Pos)
		w.parked[key] = append(w.parked[key], i)
	}
	w.parkedSeen = len(w.City.Parked)
}

func (w *World) tooClose(p mathx.Vec, r float32) bool {
	if w.Player.Pos.DistTo(p) < r {
		return true
	}
	for _, a := range w.Agents {
		if a.V.Pos.DistTo(p) < r {
			return true
		}
	}
	return false
}

// Leader search geometry, in metres.
const (
	// nearScan is how far ahead the route itself is searched. Within it
	// anything standing on the path counts, whichever way it is facing: a car
	// crossing a junction, a unit parked across the road, a wreck.
	nearScan float32 = 26
	nearStep float32 = 2.6
	// pathClearance is how far to the side of the path a vehicle's centre may
	// be and still be on it, beyond its own half-width.
	pathClearance float32 = 1.25
)

func (w *World) leaderFor(a *Agent) (gap, leadSpeed float32, lead *Vehicle) {
	// Find the closest thing ahead. The player counts as traffic, so agents
	// queue behind the human too.
	//
	// Close in, the search follows the agent's route, so it goes round a bend
	// with the road instead of straight on into the oncoming lane. Further out
	// it looks straight ahead down the lane, and only at vehicles going the
	// same way, which is all that is worth reacting to at that range.
	fwd := a.V.Forward()
	right := fwd.Right()

	near := w.near[:0]
	near = append(near, w.Player)
	for _, o := range w.Agents {
		if o != a && o.V.Pos.DistTo(a.V.Pos) <= idmScanRange {
			near = append(near, o.V)
		}
	}
	// A cone or a barrier is a stopped car, as far as queueing is concerned.
	for _, s := range w.stubs {
		if s.Pos.DistTo(a.V.Pos) <= idmScanRange {
			near = append(near, s)
		}
	}
	w.near = near

	prev := a.V.Pos
	for d := nearStep; d <= nearScan; d += nearStep {
		p := a.pathPoint(w.City, d)
		dir := p.Sub(prev).Norm()
		prev = p
		for _, v := range near {
			if v.Pos.DistTo(p) > v.Spec.HalfWidth+pathClearance {
				continue
			}
			// How much of the vehicle lies back along the path toward us.
			reach := mathx.Abs(v.Forward().Dot(dir))*(v.Spec.HalfLength-v.Spec.HalfWidth) + v.Spec.HalfWidth
			return max(d-a.V.Spec.HalfLength-reach, 0.2), max(v.Vel.Dot(dir), 0), v
		}
	}

	best := idmScanRange
	for _, v := range near {
		rel := v.Pos.Sub(a.V.Pos)
		along := rel.Dot(fwd)
		if along <= nearScan || along > best {
			continue
		}
		if mathx.Abs(rel.Dot(right)-a.sidestep) > idmLaneCoridor || v.Forward().Dot(fwd) < 0.5 {
			continue
		}
		best = along
		gap = max(along-a.V.Spec.HalfLength-v.Spec.HalfLength, 0.2)
		leadSpeed = max(v.Vel.Dot(fwd), 0)
		lead = v
	}
	return gap, leadSpeed, lead
}

// Gap judgement, in seconds and metres.
const (
	// gapMargin is the room an ordinary driver leaves beyond the time their
	// turn takes. A driver's own GapAcceptance scales it.
	gapMargin float32 = 2.2
	// pathConflict is how close two paths through a junction may come before
	// the cars on them are in each other's way.
	pathConflict float32 = 3.1
	// recklessGap is how soon a car has to be arriving for a driver to wait for
	// it whatever the rules say. It is what makes traffic brake for somebody
	// coming through a red light instead of driving into them.
	recklessGap float32 = 2.4
)

// pathSamples is how finely a path through a junction is compared with another.
const pathSamples = 6

// mustYield reports whether a driver at a junction should wait rather than go.
func (w *World) mustYield(a *Agent) bool {
	node := w.City.Nodes[a.lane.ToNode]
	if node.IsRoundabout() {
		return w.ringBlocked(a, node)
	}
	if w.exitBlocked(a) || w.boxConflict(a, node) || w.playerThreatens(a) {
		return true
	}
	if a.waited > waitLimit {
		// Waited long enough. Whatever has priority is not taking it.
		return false
	}
	return w.priorityConflict(a, node)
}

// turnPath samples the connector a driver is about to take, from u onward.
func (w *World) turnPath(l *city.Lane, t city.Turn, u float32) [pathSamples]mathx.Vec {
	var out [pathSamples]mathx.Vec
	for i := range out {
		out[i] = w.City.TurnPoint(l, t, u+(1-u)*float32(i)/(pathSamples-1))
	}
	return out
}

func pathsCross(a, b [pathSamples]mathx.Vec) bool {
	for _, p := range a {
		for _, q := range b {
			if p.DistTo(q) < pathConflict {
				return true
			}
		}
	}
	return false
}

// exitBlocked reports whether the lane a driver is heading for is backed up to
// the junction. Entering then would leave the car stranded in the box.
func (w *World) exitBlocked(a *Agent) bool {
	out := w.City.Lanes[a.next.Lane]
	// Standing in the mouth of that lane, and on it: the head of the queue
	// waiting in the next lane over is right alongside, and is not in the way.
	inMouth := func(v *Vehicle) bool {
		rel := v.Pos.Sub(out.A)
		along := rel.Dot(out.StartDir())
		return along > -2 && along < 9 && mathx.Abs(rel.Cross(out.StartDir())) < 1.6 && v.Speed() < 1
	}
	if inMouth(w.Player) {
		return true
	}
	for _, o := range w.Agents {
		if o != a && inMouth(o.V) {
			return true
		}
	}
	return false
}

// boxConflict reports whether something already in the junction is in the way
// of the turn a driver is about to make.
func (w *World) boxConflict(a *Agent, node *city.Node) bool {
	mine := w.turnPath(a.lane, a.next, 0)
	for _, o := range w.Agents {
		if o == a || o.V.Pos.DistTo(node.Pos) > node.Radius+2 {
			continue
		}
		if o.inTurn && o.lane.ToNode == node.ID && o.dazed <= 0 {
			if o.lane == a.lane {
				continue // the car in front, which is followed, not waited for
			}
			if pathsCross(mine, w.turnPath(o.lane, o.next, o.u)) {
				return true
			}
			continue
		}
		// Anything else in the box is not going anywhere predictable: a car
		// that has been hit, or one parked across it.
		if o.lane != a.lane && nearPath(mine, o.V.Pos, o.V.Spec.HalfLength+1.2) {
			return true
		}
	}
	return false
}

func nearPath(path [pathSamples]mathx.Vec, p mathx.Vec, within float32) bool {
	for _, q := range path {
		if q.DistTo(p) < within {
			return true
		}
	}
	return false
}

// playerThreatens reports whether the player's car is about to be where this
// driver's turn goes. The player is not on the lane graph and obeys nothing, so
// the question is asked of where the car is and where it is heading.
func (w *World) playerThreatens(a *Agent) bool {
	p := w.Player
	node := w.City.Nodes[a.lane.ToNode]
	if p.Pos.DistTo(node.Pos) > node.Radius+p.Speed()*recklessGap+4 {
		return false
	}
	mine := w.turnPath(a.lane, a.next, 0)
	// Sweep the player forward over the next few seconds.
	for t := float32(0); t <= recklessGap; t += 0.3 {
		at := p.Pos.Add(p.Vel.Mul(t))
		if !nearPath(mine, at, p.Spec.HalfLength+1.4) {
			continue
		}
		// The player sitting in the queue behind is not a threat.
		if at.Sub(a.V.Pos).Dot(a.V.Forward()) > a.V.Spec.HalfLength {
			return true
		}
	}
	return false
}

// priorityConflict reports whether traffic with priority over this driver is
// close enough that pulling out would be in its way.
func (w *World) priorityConflict(a *Agent, node *city.Node) bool {
	minor := a.lane.Control == city.ControlStop || a.lane.Control == city.ControlGiveWay
	across := a.next.Kind == turnAcrossTraffic()
	if !minor && !across {
		return false
	}
	need := a.gapNeeded()
	mine := w.turnPath(a.lane, a.next, 0)

	for _, o := range w.Agents {
		if o == a || !w.approaching(o, a, node) || !w.outranks(o, a, minor, across) {
			continue
		}
		if !arrivesWithin(o.lane.Length-o.s, o.V.Speed(), need) {
			continue
		}
		if !o.hasNext || pathsCross(mine, w.turnPath(o.lane, o.next, 0)) {
			return true
		}
	}

	return w.playerOutranks(a, node, minor, across, need)
}

// approaching reports whether o is on its way into a junction by some road
// other than the one a is on, which is what makes it something a might have to
// wait for.
func (w *World) approaching(o, a *Agent, node *city.Node) bool {
	if o.inTurn || o.lane == nil || o.lane.ToNode != node.ID || o.dazed > 0 {
		return false
	}
	// Alongside and going the same way is not in the way.
	return o.lane.Road != a.lane.Road || o.lane.Dir != a.lane.Dir
}

// playerOutranks reports whether the player is close enough, and has enough
// priority, that this driver should wait for them. The player has the same
// priority any other car would have on the lane they are driving along, and
// nobody knows which way they mean to go.
func (w *World) playerOutranks(a *Agent, node *city.Node, minor, across bool, need float32) bool {
	p := w.City.ProjectAlong(w.Player.Pos, w.Player.Forward())
	if !p.Valid || p.Dist > 4 || p.Lane.ToNode != node.ID {
		return false
	}
	if p.Lane.Road == a.lane.Road && p.Lane.Dir == a.lane.Dir {
		return false // alongside, going the same way
	}
	pMinor := p.Lane.Control == city.ControlStop || p.Lane.Control == city.ControlGiveWay
	ranks := (minor && !pMinor) || (across && p.Lane.Fwd.Dot(a.lane.Fwd) < -0.7)
	return ranks && arrivesWithin(p.Lane.Length-p.S, w.Player.Speed(), need)
}

// arrivesWithin reports whether a car this far from the line, at this speed,
// will be at the junction inside the gap a driver needs.
func arrivesWithin(dist, speed, need float32) bool {
	if dist > 110 || (speed < 0.5 && dist > 9) {
		return false // too far off, or sat in a queue
	}
	return dist/max(speed, 2.5) <= need
}

// gapNeeded is how long a driver needs the road to stay clear for the turn
// they are about to make: the time it takes to get through, plus a margin that
// is theirs to choose.
func (a *Agent) gapNeeded() float32 {
	st := a.style()
	through := (a.next.Len*0.6 + a.V.Spec.HalfLength*2) / min(turnSpeed(a.next, st), 9)
	return 1 + through + gapMargin*st.GapAcceptance
}

// outranks reports whether o has priority over a at the junction both are
// approaching.
func (w *World) outranks(o, a *Agent, minor, across bool) bool {
	oMinor := o.lane.Control == city.ControlStop || o.lane.Control == city.ControlGiveWay
	switch {
	case minor && !oMinor:
		// On the major road, which runs through.
		return true
	case across && o.lane.Fwd.Dot(a.lane.Fwd) < -0.7 && o.next.Kind != turnAcrossTraffic():
		// Oncoming, and not itself turning across: it comes first, provided it
		// is actually coming and not held at its own line.
		return o.V.Speed() > 1.5 || (!oMinor && w.Signals.LaneState(o.lane) == SignalGreen)
	case minor && oMinor:
		// Two drivers both waiting to pull out: the lower number goes first,
		// so they do not sit staring at each other.
		return o.ID < a.ID && o.lane.Length-o.s < 8 && o.V.Speed() < 1.5
	}
	return false
}

func (w *World) ringBlocked(a *Agent, node *city.Node) bool {
	// Reports whether a driver waiting to join a roundabout should
	// give way. The rule is to yield to whatever is already circulating and about
	// to reach the entry, which on a British roundabout is the traffic coming from
	// the right. How small a gap the driver will take is their own business, so it
	// scales with their style.
	entry := node.EntryAngle(a.lane)
	return node.RingOccupied(entry, func(yield func(mathx.Vec, float32) bool) {
		for _, o := range w.Agents {
			if o == a {
				continue
			}
			if !yield(o.V.Pos, o.V.Speed()) {
				return
			}
		}
		yield(w.Player.Pos, w.Player.Speed())
	}, a.style().GapAcceptance)
}

func turnAcrossTraffic() city.TurnKind {
	// Where traffic keeps left it is the right turn that crosses the oncoming
	// stream, and vice versa.
	if city.DriveOnLeft {
		return city.TurnRight
	}
	return city.TurnLeft
}

// inReach reports whether two vehicles are near enough to be worth testing
// for a collision. It goes by their lengths rather than a fixed distance: two
// buses nose to tail have their middles further apart than two cars could
// ever be while touching.
func inReach(a, b *Vehicle) bool {
	return a.Pos.DistTo(b.Pos) <= a.Spec.HalfLength+b.Spec.HalfLength+a.Spec.HalfWidth+b.Spec.HalfWidth
}

func (w *World) resolveCollisions() {
	// Player against traffic.
	pb := w.Player.Box()
	for _, a := range w.Agents {
		if !inReach(w.Player, a.V) {
			continue
		}
		if axis, depth, hit := pb.Penetration(a.V.Box()); hit {
			impact := w.separate(w.Player, a.V, axis, depth)
			w.Judge.ReportCollision(w.Player, "hit a vehicle", impact, w.Time)
			a.knock(impact)
			w.batter(impact)
			if a.Role == RolePolice && impact > 3 && w.Wanted.Active() && w.Wanted.sinceRam > ramPause {
				// Hitting a police car is its own offence, and its own reward.
				w.Wanted.sinceRam = 0
				w.Wanted.raise(heatPerRam)
				w.Wanted.earn(float32(bountyPerRam * w.Wanted.Level))
			}
			pb = w.Player.Box()
		}
	}

	// Traffic against traffic, so queues do not interpenetrate.
	for i, a := range w.Agents {
		for _, b := range w.Agents[i+1:] {
			if !inReach(a.V, b.V) {
				continue
			}
			if axis, depth, hit := a.V.Box().Penetration(b.V.Box()); hit {
				impact := w.separate(a.V, b.V, axis, depth)
				a.knock(impact)
				b.knock(impact)
			}
		}
	}

	w.collideBuildings(w.Player, true)
	for _, a := range w.Agents {
		w.collideBuildings(a.V, false)
	}
	w.collideWorks(w.Player, true)
	for _, a := range w.Agents {
		w.collideWorks(a.V, false)
	}
}

func (w *World) separate(a, b *Vehicle, axis mathx.Vec, depth float32) float32 {
	// Push the pair apart, then exchange momentum along the contact normal.
	push := axis.Mul(depth / 2)
	a.Pos = a.Pos.Sub(push)
	b.Pos = b.Pos.Add(push)

	rel := b.Vel.Sub(a.Vel)
	closing := rel.Dot(axis)
	if closing >= 0 {
		return 0
	}
	invA := 1 / a.Spec.Mass
	invB := 1 / b.Spec.Mass
	jn := -(1 + restitution) * closing / (invA + invB)
	impulse := axis.Mul(jn)
	a.Vel = a.Vel.Sub(impulse.Mul(invA))
	b.Vel = b.Vel.Add(impulse.Mul(invB))
	// A knock also unsettles the car's rotation.
	a.YawRate -= jn * 0.00018
	b.YawRate += jn * 0.00018
	return mathx.Abs(closing)
}

// footprint is the ground a building stands on, as a box to collide with. A
// box with no heading runs its length along X, which is the way a building's
// width runs.
func footprint(b city.Building) mathx.OBB {
	return mathx.OBB{Center: b.Center, HalfL: b.W / 2, HalfW: b.D / 2, Heading: b.Heading}
}

func (w *World) collideBuildings(v *Vehicle, report bool) {
	key := [2]int{int(v.Pos.X / w.cellSize), int(v.Pos.Z / w.cellSize)}
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			for _, bi := range w.buildings[[2]int{key[0] + dx, key[1] + dz}] {
				w.collideSolid(v, footprint(w.City.Buildings[bi]), "hit a building", report)
			}
		}
	}
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			for _, pi := range w.parked[[2]int{key[0] + dx, key[1] + dz}] {
				w.collideSolid(v, ParkedBox(w.City.Parked[pi]), "hit a parked car", report)
			}
		}
	}
}

// collideSolid pushes a vehicle out of something that does not move.
func (w *World) collideSolid(v *Vehicle, solid mathx.OBB, what string, report bool) {
	axis, depth, hit := v.Box().Penetration(solid)
	if !hit {
		return
	}
	// It is immovable, so the car absorbs all of it.
	v.Pos = v.Pos.Sub(axis.Mul(depth))
	closing := v.Vel.Dot(axis)
	if closing > 0 {
		v.Vel = v.Vel.Sub(axis.Mul(closing * (1 + restitution)))
		if report {
			w.Judge.ReportCollision(v, what, closing, w.Time)
			w.batter(closing)
		}
	}
}

func (w *World) recycleAgents() {
	for _, a := range w.Agents {
		if a.Manual {
			continue // somebody is driving it
		}
		stranded := !a.hasNext && a.s >= a.lane.Length-0.5 && !a.free
		keep := keepRadius
		if a.Role == RolePolice && w.Wanted.Active() {
			// With a pursuit on, units a long way off are no use to it. Bring
			// them in, so there is somebody to send when it escalates.
			keep = backupRadius
		}
		if a.V.Pos.DistTo(w.Player.Pos) < keep && !stranded {
			continue
		}
		if (a.pursuing || a.Blocking) && !stranded {
			continue // never teleport a unit out of an active pursuit
		}
		w.relocate(a)
	}
}

// backupRadius is how far from the driver a spare unit may be while a pursuit
// is on before it is brought closer, in metres.
const backupRadius float32 = 330

// Damage tuning.
const (
	// batterFloor is the closing speed below which a knock does no damage, in
	// m/s, and batterScale how much above it writes the car off.
	batterFloor float32 = 3
	batterScale float32 = 55
	// wreckedPower is how much of its engine a wrecked car has left.
	wreckedPower float32 = 0.3
)

// batter adds the damage from one impact to the player's car.
func (w *World) batter(impact float32) {
	if impact > batterFloor {
		w.Damage = min(w.Damage+(impact-batterFloor)/batterScale, 1)
	}
}

// cellOf returns the building index cell a point falls in.
func (w *World) cellOf(p mathx.Vec) [2]int {
	return [2]int{int(p.X / w.cellSize), int(p.Z / w.cellSize)}
}

func (w *World) relocate(a *Agent) {
	// Drop the car back into the world on a lane at a comfortable distance:
	// close enough to be part of the scene, far enough not to pop into view.
	for range 24 {
		lane := w.City.RandomLane(w.rng, 30)
		s := w.rng.Float32() * lane.Length
		if w.laneBarred(lane) {
			continue
		}
		p := lane.Point(s)
		d := p.DistTo(w.Player.Pos)
		if d < spawnNear || d > spawnFar {
			continue
		}
		if d < popRange && w.LineOfSight(p, w.Player.Pos) {
			continue // would appear out of thin air in full view
		}
		if w.tooClose(p, 11) {
			continue
		}
		a.lane, a.s = lane, s
		a.inTurn, a.u = false, 0
		a.stoppedAt = -1
		a.dazed, a.waited = 0, 0
		a.free, a.sidestep, a.sidestepWant, a.stuck, a.passing = false, 0, 0, 0, 0
		a.clearManoeuvre()
		a.V.Place(p, lane.HeadingAt(s))
		a.chooseNext(w.City)
		return
	}
}

// Weather returns the weather now.
func (w *World) Weather() weather.State { return w.Climate.Weather(w.Time) }

// SetPlayerBody changes the player's car in place, wherever it is, keeping its
// speed and heading.
func (w *World) SetPlayerBody(b Body) {
	w.PlayerBody = b
	w.Player.Spec = b.Spec.OnSurface(w.Climate.Grip())
}

// ApplyPose moves a vehicle to a pose supplied by the host of a network game.
// A joining machine does not simulate: it is told where everything is, and this
// is how it is told.
func ApplyPose(v *Vehicle, x, z, yaw, speed float32) {
	v.Pos = mathx.V(x, z)
	v.Yaw = yaw
	// The velocity is reconstructed from the heading, which is enough for the
	// renderer and for the wheels to look right.
	v.Vel = mathx.FromAngle(yaw).Mul(speed)
}

// SetPursuing marks a unit as running to a call, for a joining machine that is
// told rather than deciding.
func (a *Agent) SetPursuing(on bool) { a.pursuing = on }

// SetInput supplies a human-driven agent's controls.
func (a *Agent) SetInput(c Controls) { a.Input = c }

// Controls returns a human-driven agent's current controls.
func (a *Agent) Controls() Controls { return a.Input }
