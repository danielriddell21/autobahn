package sim

import (
	"math/rand/v2"

	"github.com/danielriddell21/crucible/rng"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
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
	// restitution is the bounciness of a vehicle impact.
	restitution float32 = 0.22
)

// Config describes a world to build.
type Config struct {
	Seed    uint64
	Traffic int
	// Police is how many marked units patrol the city. Zero disables the
	// police entirely, which leaves the judge still scoring but nobody
	// responding.
	Police int
}

// DefaultConfig returns a populated city configuration.
func DefaultConfig(seed uint64) Config {
	return Config{Seed: seed, Traffic: 70, Police: 5}
}

// World is the whole simulation: the city, its traffic, the signal controller
// and the judge watching the player's car.
type World struct {
	City    *city.City
	Signals *Signals
	Player  *Vehicle
	Agents  []*Agent
	Judge   *Judge
	// Wanted tracks the police response to how the player has been driving.
	Wanted Wanted
	// Route is the player car's navigation plan, used to draw lane guidance
	// into the annotated camera image.
	Route *Route

	// Time is the elapsed simulation time in seconds.
	Time float32

	rng       *rand.Rand
	buildings map[[2]int][]int
	cellSize  float32
	// spawn is where the player restarts.
	spawnPos mathx.Vec
	spawnYaw float32
}

// NewWorld generates a city and populates it with traffic.
func NewWorld(cfg Config) *World {
	traffic := rng.Stream(cfg.Seed, streamTraffic)
	c := city.Generate(city.DefaultParams(cfg.Seed))

	w := &World{
		City: c, Signals: NewSignals(c, rng.Stream(cfg.Seed, streamSignals)),
		Judge: NewJudge(c),
		rng:   traffic, buildings: map[[2]int][]int{}, cellSize: 48,
	}
	w.indexBuildings()

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
	w.spawnYaw = start.Heading
	w.Player = NewVehicle(CarSpec(), w.spawnPos, w.spawnYaw)
	w.Route = NewRoute(c, rng.Stream(cfg.Seed, streamRoute))
	w.Route.Update(w.Player.Pos, w.Player.Forward())

	for i := range cfg.Traffic {
		lane := c.RandomLane(traffic, 30)
		a := NewAgent(i, c, lane, traffic.Float32()*lane.Length, traffic)
		if w.tooClose(a.V.Pos, 9) {
			continue
		}
		w.Agents = append(w.Agents, a)
	}
	for i := range cfg.Police {
		lane := c.RandomLane(traffic, 30)
		u := NewPoliceUnit(cfg.Traffic+i, c, lane, traffic.Float32()*lane.Length, traffic)
		if w.tooClose(u.V.Pos, 9) {
			continue
		}
		w.Agents = append(w.Agents, u)
	}
	w.watchForOffences()
	return w
}

// watchForOffences subscribes the police to the judge. An offence only draws
// attention when a unit was close enough to witness it, so driving badly on an
// empty street is its own affair.
func (w *World) watchForOffences() {
	w.Judge.Events.Subscribe(judgeWatcher(func(in Infraction) {
		if w.witnessed() {
			w.Wanted.witness(in.Points)
		}
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

// Respawn returns the player to the starting pose.
func (w *World) Respawn() {
	w.Player.Place(w.spawnPos, w.spawnYaw)
}

// PlaceOnNearestLane drops the player onto the nearest lane facing the right
// way, which recovers a car that has been beached on a pavement.
func (w *World) PlaceOnNearestLane() {
	proj := w.City.Project(w.Player.Pos)
	if !proj.Valid {
		w.Respawn()
		return
	}
	w.Player.Place(proj.Lane.Point(proj.S), proj.Lane.Heading)
}

// Update advances the whole simulation by dt seconds, driving the player's car
// with the given controls.
func (w *World) Update(c Controls, dt float32) {
	w.Time += dt
	w.Signals.Update(dt)
	w.Player.Update(c, dt)
	for _, a := range w.Agents {
		a.drive(w, dt)
	}
	w.resolveCollisions()
	w.updateWanted(dt)
	w.recycleAgents()
	w.Route.Update(w.Player.Pos, w.Player.Forward())
	w.Judge.Update(w.Player, w.Signals, w.Time, dt)
}

func (w *World) indexBuildings() {
	for i, b := range w.City.Buildings {
		lo := mathx.V(b.Center.X-b.W/2, b.Center.Z-b.D/2)
		hi := mathx.V(b.Center.X+b.W/2, b.Center.Z+b.D/2)
		for gx := int(lo.X / w.cellSize); gx <= int(hi.X/w.cellSize); gx++ {
			for gz := int(lo.Z / w.cellSize); gz <= int(hi.Z/w.cellSize); gz++ {
				key := [2]int{gx, gz}
				w.buildings[key] = append(w.buildings[key], i)
			}
		}
	}
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

func (w *World) leaderFor(a *Agent) (gap, leadSpeed float32, ok bool) {
	// Find the closest thing directly ahead within a lane-width corridor. The
	// player counts as traffic, so agents queue behind the human too.
	fwd := a.V.Forward()
	right := fwd.Right()
	best := idmScanRange

	consider := func(pos mathx.Vec, vel mathx.Vec, halfLen float32) {
		rel := pos.Sub(a.V.Pos)
		along := rel.Dot(fwd)
		if along <= 0 || along > best {
			return
		}
		if mathx.Abs(rel.Dot(right)) > idmLaneCoridor {
			return
		}
		best = along
		gap = along - a.V.Spec.HalfLength - halfLen
		leadSpeed = max(vel.Dot(fwd), 0)
		ok = true
	}

	consider(w.Player.Pos, w.Player.Vel, w.Player.Spec.HalfLength)
	for _, o := range w.Agents {
		if o == a {
			continue
		}
		if o.V.Pos.DistTo(a.V.Pos) > idmScanRange {
			continue
		}
		consider(o.V.Pos, o.V.Vel, o.V.Spec.HalfLength)
	}
	if ok {
		gap = max(gap, 0.2)
	}
	return gap, leadSpeed, ok
}

func (w *World) junctionBlocked(a *Agent) bool {
	node := w.City.Nodes[a.lane.ToNode]
	if node.IsRoundabout() {
		return w.ringBlocked(a, node)
	}
	clearance := node.Radius + 3

	for _, o := range w.Agents {
		if o == a {
			continue
		}
		d := o.V.Pos.DistTo(node.Pos)
		if d > clearance {
			continue
		}
		// Something is already occupying the box. Only wait for it if it is
		// not simply following us out of the junction.
		if o.inTurn || o.V.Speed() < 1.2 {
			if o.V.Pos.Sub(a.V.Pos).Dot(a.V.Forward()) > 0 {
				return true
			}
		}
	}

	// Turning across oncoming traffic must give way to it.
	if a.hasNext && a.next.Kind == turnAcrossTraffic() {
		for _, o := range w.Agents {
			if o == a || o.lane == nil || o.inTurn {
				continue
			}
			if o.lane.ToNode != node.ID || o.V.Speed() < 2 {
				continue
			}
			// Oncoming means heading roughly opposite to us.
			if o.lane.Fwd.Dot(a.lane.Fwd) > -0.7 {
				continue
			}
			if o.lane.Length-o.s < 32 {
				return true
			}
		}
	}

	// Where several cars are waiting at the same priority junction, let the
	// lowest-numbered one go first so they never sit staring at each other.
	if a.lane.Control == city.ControlStop || a.lane.Control == city.ControlGiveWay {
		for _, o := range w.Agents {
			if o == a || o.lane == nil || o.inTurn || o.ID >= a.ID {
				continue
			}
			if o.lane.ToNode == node.ID && o.lane.Length-o.s < 6 && o.V.Speed() < 1.5 {
				return true
			}
		}
	}
	return false
}

// ringBlocked reports whether a driver waiting to join a roundabout should
// give way. The rule is to yield to whatever is already circulating and about
// to reach the entry, which on a British roundabout is the traffic coming from
// the right. How small a gap the driver will take is their own business, so it
// scales with their style.
func (w *World) ringBlocked(a *Agent, node *city.Node) bool {
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

func (w *World) resolveCollisions() {
	// Player against traffic.
	pb := w.Player.Box()
	for _, a := range w.Agents {
		if w.Player.Pos.DistTo(a.V.Pos) > 8 {
			continue
		}
		if axis, depth, hit := pb.Penetration(a.V.Box()); hit {
			impact := w.separate(w.Player, a.V, axis, depth)
			w.Judge.ReportCollision(w.Player, "hit a vehicle", impact, w.Time)
			pb = w.Player.Box()
		}
	}

	// Traffic against traffic, so queues do not interpenetrate.
	for i, a := range w.Agents {
		for _, b := range w.Agents[i+1:] {
			if a.V.Pos.DistTo(b.V.Pos) > 8 {
				continue
			}
			if axis, depth, hit := a.V.Box().Penetration(b.V.Box()); hit {
				w.separate(a.V, b.V, axis, depth)
			}
		}
	}

	w.collideBuildings(w.Player, true)
	for _, a := range w.Agents {
		w.collideBuildings(a.V, false)
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

func (w *World) collideBuildings(v *Vehicle, report bool) {
	key := [2]int{int(v.Pos.X / w.cellSize), int(v.Pos.Z / w.cellSize)}
	box := v.Box()
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			for _, bi := range w.buildings[[2]int{key[0] + dx, key[1] + dz}] {
				b := w.City.Buildings[bi]
				wall := mathx.OBB{Center: b.Center, HalfW: b.W / 2, HalfL: b.D / 2}
				axis, depth, hit := box.Penetration(wall)
				if !hit {
					continue
				}
				// A building is immovable, so the car absorbs all of it.
				v.Pos = v.Pos.Sub(axis.Mul(depth))
				closing := v.Vel.Dot(axis)
				if closing > 0 {
					v.Vel = v.Vel.Sub(axis.Mul(closing * (1 + restitution)))
					if report {
						w.Judge.ReportCollision(v, "hit a building", closing, w.Time)
					}
				}
				box = v.Box()
			}
		}
	}
}

func (w *World) recycleAgents() {
	for _, a := range w.Agents {
		stranded := !a.hasNext && a.s >= a.lane.Length-0.5
		if a.V.Pos.DistTo(w.Player.Pos) < keepRadius && !stranded {
			continue
		}
		if a.pursuing && !stranded {
			continue // never teleport a unit out of an active pursuit
		}
		w.relocate(a)
	}
}

func (w *World) relocate(a *Agent) {
	// Drop the car back into the world on a lane at a comfortable distance:
	// close enough to be part of the scene, far enough not to pop into view.
	for range 24 {
		lane := w.City.RandomLane(w.rng, 30)
		s := w.rng.Float32() * lane.Length
		p := lane.Point(s)
		d := p.DistTo(w.Player.Pos)
		if d < spawnNear || d > spawnFar {
			continue
		}
		if w.tooClose(p, 11) {
			continue
		}
		a.lane, a.s = lane, s
		a.inTurn, a.u = false, 0
		a.stoppedAt = -1
		a.V.Place(p, lane.Heading)
		a.chooseNext(w.City)
		return
	}
}
