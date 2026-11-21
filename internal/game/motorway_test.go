package game

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
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
