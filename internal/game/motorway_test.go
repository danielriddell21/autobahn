package game

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

func TestOnlyFastRoadsHaveCrashBarriers(t *testing.T) {
	grow := func(m city.Map) *city.City {
		c := city.NewGrowing(city.ParamsFor(m, 7))
		c.EnsureAround(mathx.V(0, 0), 1200)
		return c
	}
	for _, m := range []city.Map{city.MapCity, city.MapSeaside, city.MapVillage} {
		c := grow(m)
		for _, r := range c.Roads {
			if len(barriersFor(c, r)) != 0 {
				t.Fatalf("a road in the %s has a crash barrier", m.Name())
			}
		}
	}

	c := grow(city.MapMotorway)
	fenced := 0
	for _, r := range c.Roads {
		got := barriersFor(c, r)
		if r.Ring || r.Class != city.Arterial {
			if len(got) != 0 {
				t.Fatalf("road %d is not a fast road and has a barrier", r.ID)
			}
			continue
		}
		if len(got) == 0 {
			continue // too short between its junctions to fence
		}
		fenced++
		if len(got) != 2 {
			t.Fatalf("road %d has %d barriers, want one each side", r.ID, len(got))
		}
		for _, b := range got {
			// Beyond the kerb, on the pavement, and clear of both junctions.
			for _, p := range []mathx.Vec{b.a, b.b} {
				rel := p.Sub(c.Nodes[r.A].Pos)
				off, along := mathx.Abs(rel.Dot(r.Dir.Right())), rel.Dot(r.Dir)
				if off <= r.HalfWidth || off >= r.HalfWidth+city.PavementWidth {
					t.Errorf("road %d: a barrier %.2f m from the centre line of a road %.2f m to the kerb",
						r.ID, off, r.HalfWidth)
				}
				if along < c.Nodes[r.A].Radius+barrierMargin-0.01 || along > r.Length-c.Nodes[r.B].Radius-barrierMargin+0.01 {
					t.Errorf("road %d: a barrier end %.1f m along a road %.1f long", r.ID, along, r.Length)
				}
			}
		}
	}
	if fenced == 0 {
		t.Error("no road in the motorway town has a crash barrier")
	}
}

func TestABarrierIsPostsAFixedDistanceApart(t *testing.T) {
	posts := postsOf(barrier{a: mathx.V(10, 5), b: mathx.V(10, 5+barrierPitch*6+1)})
	if len(posts) != 7 {
		t.Fatalf("%d posts, want 7", len(posts))
	}
	for i, p := range posts {
		if want := mathx.V(10, 5+barrierPitch*float32(i)); p.at.DistTo(want) > 0.001 {
			t.Errorf("post %d stands at %v, want %v", i, p.at, want)
		}
		if p.last != (i == len(posts)-1) {
			t.Errorf("post %d: last = %v", i, p.last)
		}
		if p.run != mathx.V(0, 1) {
			t.Errorf("post %d's rail runs %v", i, p.run)
		}
	}
}

// A block's outline is laid with points all along its sides where the ground
// wanders, so each side follows its street, and left alone where it does not.
func TestAnOutlineFollowsTheGround(t *testing.T) {
	plain := &Game{world: sim.NewWorld(sim.Config{Seed: 7, Traffic: 0})}
	square := plain.world.City.Blocks[0].Outline()
	if got := plain.layOutline(square); len(got) != len(square) || got[2] != square[2] {
		t.Errorf("an outline in the city was changed: %v", got)
	}

	bent := &Game{world: sim.NewWorld(sim.Config{Seed: 7, Map: city.MapVillage, Traffic: 0})}
	c := bent.world.City
	if !c.Laid() {
		t.Skip("the village is laid as drawn")
	}
	for _, b := range c.Blocks[:min(len(c.Blocks), 40)] {
		planned := b.Outline()
		var round float32
		for i, p := range planned {
			round += p.DistTo(planned[(i+1)%len(planned)])
		}
		got := bent.layOutline(planned)
		if len(got) < int(round/layPiece)/2 {
			t.Fatalf("an outline %.0f m round was laid in %d points", round, len(got))
		}
		first, _ := c.Lay(planned[0])
		if got[0].DistTo(first) > 0.001 {
			t.Fatalf("the outline starts at %v, want where its first corner lies, %v", got[0], first)
		}
		// A side follows its street: no two points far apart.
		for i, p := range got {
			if d := p.DistTo(got[(i+1)%len(got)]); d > layPiece*1.6 {
				t.Fatalf("two points of an outline are %.1f m apart", d)
			}
		}
	}
}
