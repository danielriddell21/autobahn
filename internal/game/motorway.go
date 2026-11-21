package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// The motorway town is the city's generator asked for fast roads a long way
// apart. What makes it look the part is here: sheds in steel and render
// rather than streets of shops, and a crash barrier along every fast road.

var (
	// shedShades are profiled steel, concrete and render.
	shedShades = [6]rl.Color{
		rl.NewColor(150, 156, 164, 255),
		rl.NewColor(176, 180, 186, 255),
		rl.NewColor(128, 140, 156, 255),
		rl.NewColor(190, 186, 176, 255),
		rl.NewColor(110, 122, 138, 255),
		rl.NewColor(206, 208, 212, 255),
	}
	colBarrier = rl.NewColor(168, 172, 178, 255)
)

// Crash barrier layout, in metres.
const (
	// barrierSetback is how far in from the kerb the barrier stands, and
	// barrierMargin how far short of each junction it stops, so that nothing
	// is fenced off from a road that crosses.
	barrierSetback float32 = 0.7
	barrierMargin  float32 = 12
	barrierHeight  float32 = 0.7
	barrierPitch   float32 = 4
	// barrierLimit is the speed limit a road has to carry before it is
	// thought worth fencing, in miles per hour.
	barrierLimit float32 = 50
)

// barrier is one length of crash barrier beside a road, from a to b.
type barrier struct{ a, b mathx.Vec }

// barriersFor returns the crash barriers along a road: one down each side of
// any road fast enough to need them.
func barriersFor(c *city.City, r *city.Road) []barrier {
	if r.Ring || len(r.LanesAB) == 0 {
		return nil
	}
	if mathx.ToMPH(c.Lanes[r.LanesAB[0]].SpeedLimit) < barrierLimit {
		return nil
	}
	na, nb := c.Nodes[r.A], c.Nodes[r.B]
	from, to := na.Radius+barrierMargin, r.Length-nb.Radius-barrierMargin
	if to-from < 10 {
		return nil
	}
	out := make([]barrier, 0, 2)
	for _, side := range [2]float32{-1, 1} {
		off := r.Dir.Right().Mul(side * (r.HalfWidth + barrierSetback))
		out = append(out, barrier{
			a: na.Pos.Add(r.Dir.Mul(from)).Add(off),
			b: na.Pos.Add(r.Dir.Mul(to)).Add(off),
		})
	}
	return out
}

// barrierStand is one post of a crash barrier, with the length of rail that
// runs on from it to the next.
type barrierStand struct {
	at, run mathx.Vec // where the plan puts it, and the way the rail runs on
	on      mathx.Vec // where it stands on the ground
	turn    float32
	last    bool // the end of the barrier, with no rail beyond it
}

// postsOf returns the posts of a barrier, a fixed distance apart.
func postsOf(b barrier) []barrierStand {
	span := b.b.Sub(b.a)
	length := span.Len()
	dir := span.Norm()
	n := int(length / barrierPitch)
	out := make([]barrierStand, 0, n+1)
	for i := range n + 1 {
		out = append(out, barrierStand{at: b.a.Add(dir.Mul(float32(i) * barrierPitch)), run: dir, last: i == n})
	}
	return out
}

// barrierPost is the distance between posts as a rail length.
// drawBarriers draws the crash barriers near the eye, working out those of
// any road that has appeared since it last looked. A barrier is stood a post
// at a time, each with its length of rail, so it follows the road round.
func (g *Game) drawBarriers(eye mathx.Vec) {
	c := g.world.City.Plan()
	for ; g.barrieredRoads < len(c.Roads); g.barrieredRoads++ {
		for _, b := range barriersFor(c, c.Roads[g.barrieredRoads]) {
			for _, p := range postsOf(b) {
				p.on, p.turn = g.world.City.Lay(p.at)
				g.barriers = append(g.barriers, p)
			}
		}
	}
	ground := city.KerbHeight
	for _, p := range g.barriers {
		if p.on.DistTo(eye) > cullProps {
			continue
		}
		g.standOn(p.at, p.on, p.turn)
		rl.DrawCube(vec3(p.at.X, ground+barrierHeight/2, p.at.Z), 0.12, barrierHeight, 0.12, colPole)
		if !p.last {
			mid := p.at.Add(p.run.Mul(barrierPitch / 2))
			rl.DrawCube(vec3(mid.X, ground+barrierHeight, mid.Z),
				max(mathx.Abs(p.run.X)*barrierPitch, 0.08), 0.3, max(mathx.Abs(p.run.Z)*barrierPitch, 0.08), colBarrier)
		}
		rl.PopMatrix()
	}
}
