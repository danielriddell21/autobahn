package city_test

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

func TestTheCityHasCarsParkedInIt(t *testing.T) {
	c := build(7)
	if len(c.Parked) < 20 {
		t.Fatalf("only %d cars parked in a whole city", len(c.Parked))
	}
	same := build(7)
	if len(same.Parked) != len(c.Parked) {
		t.Fatalf("the same seed parked %d cars and then %d", len(c.Parked), len(same.Parked))
	}
	for i := range c.Parked {
		if c.Parked[i] != same.Parked[i] {
			t.Fatalf("parked car %d differs between two builds of the same seed", i)
		}
	}
}

// A parked car is scenery, so it has to be out of every lane: beyond the kerb
// of the nearest road, and clear of every junction.
func TestParkedCarsStandOffTheCarriageway(t *testing.T) {
	for _, seed := range []uint64{3, 7, 21} {
		c := build(seed)
		for _, p := range c.Parked {
			pr := c.Project(p.Pos)
			if pr.Lane == nil {
				t.Fatalf("seed %d: a car is parked at %v, nowhere near a road", seed, p.Pos)
			}
			r := c.Roads[pr.Lane.Road]
			mid := c.Nodes[r.A].Pos
			off := mathx.Abs(p.Pos.Sub(mid).Dot(r.Dir.Right()))
			if off < r.HalfWidth+0.9 {
				t.Errorf("seed %d: a car parked at %v is %.2f m from the centre line of a road %.2f m to the kerb",
					seed, p.Pos, off, r.HalfWidth)
			}
			for _, n := range c.Nodes {
				if n.Settled() && n.Pos.DistTo(p.Pos) < n.Radius+12 {
					t.Errorf("seed %d: a car is parked %.1f m from a junction", seed, n.Pos.DistTo(p.Pos))
				}
			}
		}
	}
}

func TestParkedCarsStandClearOfTheBuildings(t *testing.T) {
	c := build(7)
	for _, p := range c.Parked {
		for _, b := range c.Buildings {
			if mathx.Abs(p.Pos.X-b.Center.X) < b.W/2+0.9 && mathx.Abs(p.Pos.Z-b.Center.Z) < b.D/2+0.9 {
				t.Fatalf("a car is parked at %v, inside the building at %v", p.Pos, b.Center)
			}
		}
	}
}

func TestTheMainRoadsHaveNoBays(t *testing.T) {
	c := build(7)
	bays := 0
	for _, r := range c.Roads {
		got := c.Bays(r)
		bays += len(got)
		if (r.Ring || r.Class == city.Arterial) && len(got) != 0 {
			t.Errorf("road %d is a main road or a roundabout and has %d bays", r.ID, len(got))
		}
		for _, b := range got {
			if b.Along < 20 || b.Along+city.BayLength*city.BaySlots > r.Length-20 {
				t.Errorf("a bay on road %d runs from %.1f on a road %.1f long", r.ID, b.Along, r.Length)
			}
			// Every space is on the pavement side of the kerb.
			for i := range city.BaySlots {
				if in := b.Slot(i).Sub(b.Start).Dot(b.Out); in <= 0 || in >= city.BayDepth {
					t.Errorf("space %d of a bay is %.2f m in from the kerb", i, in)
				}
			}
		}
	}
	if bays == 0 {
		t.Error("no road in the city has a parking bay")
	}
}

func TestEveryBuildingFacesAStreet(t *testing.T) {
	c := build(7)
	for _, b := range c.Buildings {
		if l := b.Front.Len(); l < 0.99 || l > 1.01 {
			t.Fatalf("the building at %v faces %v, which is no direction", b.Center, b.Front)
		}
		if b.Front.X != 0 && b.Front.Z != 0 {
			t.Fatalf("the building at %v faces %v, across the grid", b.Center, b.Front)
		}
	}
}
