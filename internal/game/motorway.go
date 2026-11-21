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
	barrierPost    float32 = 4
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

// drawBarriers draws the crash barriers near the eye, working out those of
// any road that has appeared since it last looked.
func (g *Game) drawBarriers(eye mathx.Vec) {
	c := g.world.City
	for ; g.barrieredRoads < len(c.Roads); g.barrieredRoads++ {
		g.barriers = append(g.barriers, barriersFor(c, c.Roads[g.barrieredRoads])...)
	}
	ground := city.KerbHeight
	for _, b := range g.barriers {
		mid := b.a.Lerp(b.b, 0.5)
		span := b.b.Sub(b.a)
		length := span.Len()
		if mid.DistTo(eye) > cullProps+length/2 {
			continue
		}
		// The rail, and the posts it is bolted to.
		rl.DrawCube(vec3(mid.X, ground+barrierHeight, mid.Z),
			max(mathx.Abs(span.X), 0.08), 0.3, max(mathx.Abs(span.Z), 0.08), colBarrier)
		dir := span.Norm()
		for d := float32(0); d <= length; d += barrierPost {
			p := b.a.Add(dir.Mul(d))
			if p.DistTo(eye) > cullProps {
				continue
			}
			rl.DrawCube(vec3(p.X, ground+barrierHeight/2, p.Z), 0.12, barrierHeight, 0.12, colPole)
		}
	}
}
