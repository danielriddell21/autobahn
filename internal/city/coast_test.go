package city_test

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

func seaside(seed uint64) *city.City {
	c := city.NewGrowing(city.ParamsFor(city.MapSeaside, seed))
	c.EnsureAround(mathx.V(0, 0), 900)
	return c
}

func TestOnlyTheSeasideHasACoast(t *testing.T) {
	if _, _, ok := build(7).Shoreline(); ok {
		t.Error("the city has a coast")
	}
	if build(7).AtSea(mathx.V(0, 1e6)) {
		t.Error("somewhere in the city is at sea")
	}
	front, sea, ok := seaside(7).Shoreline()
	if !ok || sea <= front {
		t.Fatalf("the seaside's front is at %.1f and its sea at %.1f, ok %v", front, sea, ok)
	}
	if beach := sea - front; beach < 40 || beach > 140 {
		t.Errorf("the beach is %.0f m deep", beach)
	}
	// A drive begins inland, not on the sand.
	if front < 100 {
		t.Errorf("the front is only %.0f m from where a drive begins", front)
	}
}

// Nothing is built past the seafront: no road, no lane, no building, nothing
// parked, no sign.
func TestNothingIsBuiltInTheSea(t *testing.T) {
	for _, seed := range []uint64{3, 7, 21} {
		c := seaside(seed)
		front, sea, _ := c.Shoreline()
		if len(c.Lanes) == 0 || len(c.Buildings) == 0 {
			t.Fatalf("seed %d: the seaside town has nothing in it", seed)
		}
		for _, l := range c.Lanes {
			if l.A.Z > front+20 || l.B.Z > front+20 {
				t.Fatalf("seed %d: a lane runs from %v to %v, past the front at %.1f", seed, l.A, l.B, front)
			}
		}
		for _, b := range c.Buildings {
			if b.Center.Z+b.D/2 > front {
				t.Fatalf("seed %d: a building at %v stands beyond the front at %.1f", seed, b.Center, front)
			}
		}
		for _, p := range c.Props {
			if c.AtSea(p.Pos) {
				t.Fatalf("seed %d: a sign stands in the sea at %v", seed, p.Pos)
			}
		}
		for _, p := range c.Parked {
			if p.Pos.Z > front {
				t.Fatalf("seed %d: a car is parked on the beach at %v", seed, p.Pos)
			}
		}
		beaches := 0
		for _, b := range c.Blocks {
			if b.Beach {
				beaches++
				if b.Min.Z < front || mathx.Abs(b.Max.Z-sea) > 0.01 {
					t.Fatalf("seed %d: a beach runs from %.1f to %.1f, with the front at %.1f and the sea at %.1f",
						seed, b.Min.Z, b.Max.Z, front, sea)
				}
				continue
			}
			if b.Max.Z > front {
				t.Fatalf("seed %d: a block at %v..%v is beyond the front", seed, b.Min, b.Max)
			}
		}
		if beaches < 4 {
			t.Errorf("seed %d: only %d stretches of beach", seed, beaches)
		}
	}
}

// The front is a road that can be driven end to end, and every junction on it
// is finished: the growing city never leaves one waiting on a street that is
// not coming.
func TestTheSeafrontIsARoadAndItsJunctionsAreFinished(t *testing.T) {
	c := seaside(7)
	front, _, _ := c.Shoreline()
	along := 0
	for _, r := range c.Roads {
		if r.Ring {
			continue // a roundabout on the front is not a stretch of it
		}
		a, b := c.Nodes[r.A].Pos, c.Nodes[r.B].Pos
		if mathx.Abs(a.Z-front) < 0.01 && mathx.Abs(b.Z-front) < 0.01 {
			along++
			if r.Class != city.Arterial {
				t.Errorf("a stretch of the front is a %v road, want a main road", r.Class)
			}
		}
	}
	if along < 4 {
		t.Fatalf("only %d stretches of road run along the front", along)
	}
	for _, n := range c.Nodes {
		if mathx.Abs(n.Pos.Z-front) > 0.01 || n.Pos.Len() > 500 {
			continue
		}
		if !c.Complete(n) {
			t.Errorf("the junction on the front at %v was never finished", n.Pos)
		}
	}
	// Past the front the lattice runs on under the water, with nothing at
	// any of its junctions.
	for _, n := range c.Nodes {
		if n.Pos.Z > front+1 && len(n.Roads) != 0 {
			t.Fatalf("the junction at %v, out to sea, has %d roads", n.Pos, len(n.Roads))
		}
	}
}

func TestTheSeasideIsALowTown(t *testing.T) {
	town, big := seaside(7), build(7)
	tallest := func(c *city.City) float32 {
		var h float32
		for _, b := range c.Buildings {
			h = max(h, b.Height)
		}
		return h
	}
	if tallest(town) >= 40 || tallest(town) >= tallest(big) {
		t.Errorf("the seaside's tallest building is %.0f m and the city's %.0f", tallest(town), tallest(big))
	}
}
