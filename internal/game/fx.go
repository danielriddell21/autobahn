package game

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// The things a hard-driven car leaves behind it and throws around it: rubber
// on the road, smoke off the tyres, and after dark the light from its lamps.
// None of it feeds back into the simulation. It is drawn, and that is all.

// skid is one stretch of rubber laid on the road by one tyre.
type skid struct {
	a, b mathx.Vec
}

// puff is one cloud of tyre smoke.
type puff struct {
	at     mathx.Vec
	height float32
	drift  mathx.Vec
	age    float32
}

// Effect limits.
const (
	maxSkids    = 700
	maxPuffs    = 90
	puffLife    = 1.1  // seconds
	skidSlip    = 0.19 // rear slip angle that lays rubber, radians
	skidWidth   = 0.21 // metres
	smokeSlip   = 0.3
	trailRange  = 110 // metres from the player beyond which nobody leaves marks
	skidSpacing = 0.7 // metres of travel between marks
)

// trail remembers where a car's rear tyres were when it last left a mark.
type trail struct {
	left, right mathx.Vec
	laying      bool
}

// effects is the renderer's own memory of what has happened on the road.
type effects struct {
	skids  []skid
	next   int // where the next skid goes once the buffer is full
	puffs  []puff
	trails map[*sim.Vehicle]*trail
	smoke  float32 // time since the last puff
	// shake is how hard the camera is being rattled, and fades by itself.
	shake      float32
	lastDamage float32
}

// slipping reports whether a car is laying rubber.
func slipping(v *sim.Vehicle) bool {
	speed := v.Speed()
	switch {
	case speed < 4:
		return false
	case v.Handbrake:
		return true
	case v.Slip > skidSlip:
		return true
	default:
		return v.Brake > 0.85 && speed > 13
	}
}

func (g *Game) updateEffects(dt float32) {
	fx := &g.fx
	if fx.trails == nil {
		fx.trails = map[*sim.Vehicle]*trail{}
	}
	p := g.world.Player
	g.layRubber(p)
	for _, a := range g.world.Agents {
		if a.Role == sim.RolePolice && a.V.Pos.DistTo(p.Pos) < trailRange {
			g.layRubber(a.V)
		}
	}

	// Smoke off the back tyres in a slide, and out of the exhaust on nitrous.
	fx.smoke += dt
	if fx.smoke > 0.035 && (p.Slip > smokeSlip && p.Speed() > 6 || p.Handbrake && p.Speed() > 5 || p.Boosting) {
		fx.smoke = 0
		back := p.Pos.Sub(p.Forward().Mul(p.Spec.RearAxle))
		side := p.Forward().Right().Mul(p.Spec.HalfWidth - 0.15)
		for _, at := range [2]mathx.Vec{back.Add(side), back.Sub(side)} {
			g.addPuff(puff{at: at, height: 0.25, drift: p.Vel.Mul(0.25)})
		}
	}
	live := fx.puffs[:0]
	for _, q := range fx.puffs {
		if q.age += dt; q.age < puffLife {
			q.at = q.at.Add(q.drift.Mul(dt))
			q.drift = q.drift.Mul(1 - 1.5*dt)
			q.height += 0.9 * dt
			live = append(live, q)
		}
	}
	fx.puffs = live

	// A knock rattles the camera. Damage is the one number that goes up by
	// exactly as much as the car has just been hit.
	if d := g.world.Damage - fx.lastDamage; d > 0 {
		fx.shake = min(fx.shake+d*7, 1)
	}
	fx.lastDamage = g.world.Damage
	fx.shake = max(fx.shake-dt*2.2, 0)
}

func (g *Game) addPuff(q puff) {
	if len(g.fx.puffs) < maxPuffs {
		g.fx.puffs = append(g.fx.puffs, q)
	}
}

// layRubber extends a car's skid marks if it is sliding, and lifts the pen if
// it is not.
func (g *Game) layRubber(v *sim.Vehicle) {
	fx := &g.fx
	t := fx.trails[v]
	if t == nil {
		t = &trail{}
		fx.trails[v] = t
	}
	back := v.Pos.Sub(v.Forward().Mul(v.Spec.RearAxle))
	side := v.Forward().Right().Mul(v.Spec.HalfWidth - 0.14)
	left, right := back.Sub(side), back.Add(side)
	if !slipping(v) {
		t.laying = false
		return
	}
	if !t.laying {
		t.left, t.right, t.laying = left, right, true
		return
	}
	if left.DistTo(t.left) < skidSpacing {
		return
	}
	// A car that has been put somewhere else has not skidded all the way there.
	if left.DistTo(t.left) < 6 {
		g.addSkid(skid{t.left, left})
		g.addSkid(skid{t.right, right})
	}
	t.left, t.right = left, right
}

func (g *Game) addSkid(s skid) {
	fx := &g.fx
	if len(fx.skids) < maxSkids {
		fx.skids = append(fx.skids, s)
		return
	}
	fx.skids[fx.next] = s
	fx.next = (fx.next + 1) % maxSkids
}

func (g *Game) drawSkids(eye mathx.Vec) {
	for _, s := range g.fx.skids {
		if s.a.DistTo(eye) > cullMarkings {
			continue
		}
		side := s.b.Sub(s.a).Norm().Right().Mul(skidWidth / 2)
		groundQuad(g.land, s.a.Sub(side), s.a.Add(side), s.b.Add(side), s.b.Sub(side), 0.012, colSkid)
	}
}

func (g *Game) drawSmoke() {
	if len(g.fx.puffs) == 0 {
		return
	}
	// Smoke has no surface to light, so it is given one normal pointing up
	// and takes whatever the sky gives it.
	rl.Normal3f(0, 1, 0)
	rl.DisableDepthMask()
	for _, q := range g.fx.puffs {
		f := q.age / puffLife
		alpha := uint8(58 * (1 - f))
		rl.DrawSphereEx(vec3(q.at.X, q.height+g.land.at(q.at), q.at.Z), 0.22+0.75*f, 6, 8,
			rl.NewColor(204, 204, 208, alpha))
	}
	rl.DrawRenderBatchActive()
	rl.EnableDepthMask()
}

// drawLightPools lays the light that lamps throw on the road: under the street
// lamps and ahead of every car once it is dark, and round a unit on a call at
// any time of day.
func (g *Game) drawLightPools(eye mathx.Vec, drawPlayer bool) {
	night := g.light.night
	// Flush what has been drawn so far, then add light on top of it without
	// writing depth, so one pool never cuts a hole in another.
	rl.DrawRenderBatchActive()
	rl.BeginBlendMode(rl.BlendAdditive)
	rl.DisableDepthMask()

	if night > 0.2 {
		lamp := rl.NewColor(250, 206, 132, uint8(118*night))
		for _, p := range g.world.City.Props {
			if p.Kind != city.PropStreetLamp || p.Pos.DistTo(eye) > cullPools {
				continue
			}
			under := p.Pos.Sub(mathx.FromAngle(p.Heading).Right().Mul(lampReach))
			groundDisc(g.land, under, 10.5, 0.03, lamp, 14)
		}
	}

	for _, a := range g.world.Agents {
		if a.V.Pos.DistTo(eye) > cullPools {
			continue
		}
		g.carLight(a.V, night)
		if a.BluesAndTwos() {
			// Blue and red sweeping over everything nearby.
			col := colBeaconBlue
			if g.blink {
				col = colBeaconRed
			}
			strength := 44 + 66*night
			groundDisc(g.land, a.V.Pos, 11, 0.035, rl.NewColor(col.R, col.G, col.B, uint8(strength)), 16)
		}
	}
	if drawPlayer {
		p := g.world.Player
		g.carLight(p, night)
		if p.Boosting {
			back := p.Pos.Sub(p.Forward().Mul(p.Spec.HalfLength + 0.8))
			groundDisc(g.land, back, 3.4, 0.035, rl.NewColor(96, 160, 250, 120), 12)
		}
	}

	rl.DrawRenderBatchActive()
	rl.EnableDepthMask()
	rl.EndBlendMode()
}

// carLight lays a car's own lamps on the road: beams ahead, and a red glow
// behind that brightens when it brakes.
func (g *Game) carLight(v *sim.Vehicle, night float32) {
	fwd := v.Forward()
	if night > 0.2 {
		nose := v.Pos.Add(fwd.Mul(v.Spec.HalfLength))
		groundBeam(g.land, nose, fwd, 21, 5.2, 0.03, rl.NewColor(246, 238, 208, uint8(120*night)))
	}
	glow := 46 * night
	if v.Brake > 0.05 {
		glow = 60 + 70*night
	}
	if glow > 4 {
		tail := v.Pos.Sub(fwd.Mul(v.Spec.HalfLength + 0.5))
		groundDisc(g.land, tail, 2.6, 0.03, rl.NewColor(236, 52, 44, uint8(glow)), 10)
	}
}

// rattle returns how far to knock the camera off its mark this frame, from
// how hard the car has just been hit and how fast it is going.
func (g *Game) rattle() (x, y float32) {
	p := g.world.Player
	amount := g.fx.shake*0.5 + mathx.Clamp((p.Speed()-34)/60, 0, 0.12)
	if p.Boosting {
		amount += 0.07
	}
	if amount <= 0.001 {
		return 0, 0
	}
	t := float64(g.world.Time)
	return float32(math.Sin(t*71.3)) * amount, float32(math.Sin(t*93.7+1.3)) * amount * 0.7
}
