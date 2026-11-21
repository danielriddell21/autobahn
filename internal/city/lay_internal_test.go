package city

import (
	"testing"

	"github.com/danielriddell21/crucible/worldgen"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

func villageLand(t *testing.T) *lie {
	t.Helper()
	c := NewGrowing(ParamsFor(MapVillage, 7))
	if c.land == nil {
		t.Skip("the village is laid as drawn")
	}
	return c.land
}

// A street leaves each junction dead straight for the length of its arm, so
// a junction is laid exactly the size the plan made it; and it comes out a
// little longer than planned, never shorter than the gap it has to cross.
func TestAStreetIsStraightOutOfEachJunction(t *testing.T) {
	land := villageLand(t)
	for col := -4; col <= 4; col++ {
		for row := -4; row <= 4; row++ {
			for _, axis := range []worldgen.Axis{worldgen.AxisX, worldgen.AxisY} {
				c := land.curveOf(street{col, row, axis})
				if c.arm < armLeast-0.01 || c.arm > armLength+0.01 || 2*c.arm >= c.planned {
					t.Fatalf("a street planned %.1f m long has arms of %.1f m", c.planned, c.arm)
				}
				start, _ := c.at(0)
				end, _ := c.at(c.planned)
				if c.length < start.DistTo(end)-0.01 {
					t.Fatalf("a street is %.2f m long between ends %.2f m apart", c.length, start.DistTo(end))
				}
				for _, s := range []float32{c.arm * 0.3, c.arm * 0.9} {
					near, dir := c.at(s)
					_, out := c.at(0)
					if mathx.Abs(near.DistTo(start)-s) > 0.01 || dir.Dot(out) < 0.9999 {
						t.Fatalf("%.1f m out of a junction a street has gone %.2f m and turned", s, near.DistTo(start))
					}
					far, _ := c.at(c.planned - s)
					if mathx.Abs(far.DistTo(end)-s) > 0.01 {
						t.Fatalf("%.1f m short of a junction a street is %.2f m from it", s, far.DistTo(end))
					}
				}
			}
		}
	}
}

// Finding the point of the plan that lies somewhere on the ground undoes
// laying it.
func TestUnlayUndoesLay(t *testing.T) {
	land := villageLand(t)
	for i := range 300 {
		p := mathx.V(float32(i%15)*83-600, float32(i/15)*71-700)
		on, _ := land.lay(p)
		// What is found may not be the very point, but it has to lie where
		// the first one did.
		back := land.unlay(on)
		if again, _ := land.lay(back); again.DistTo(on) > 2 {
			t.Fatalf("%v lies at %v; %v was taken for it, which lies at %v", p, on, back, again)
		}
	}
	if land.reach() <= 0 {
		t.Error("the land is said not to wander at all")
	}
	// Nothing is laid further from the plan than the land can wander.
	for i := range 300 {
		p := mathx.V(float32(i%15)*97-700, float32(i/15)*89-900)
		if on, _ := land.lay(p); on.DistTo(p) > land.reach() {
			t.Fatalf("%v lies %.1f m from where it was planned, further than the %.1f m the land wanders",
				p, on.DistTo(p), land.reach())
		}
	}
}

// A place that has not moved far enough to reach new ground costs nothing,
// laid or not: this is asked every frame.
func TestStandingStillCostsNothingOnLaidGround(t *testing.T) {
	c := NewGrowing(ParamsFor(MapVillage, 8))
	if c.land == nil {
		t.Skip("the village is laid as drawn")
	}
	at := mathx.V(40, -30)
	c.EnsureAround(at, 700)
	lanes := len(c.Lanes)
	if n := testing.AllocsPerRun(50, func() { c.EnsureAround(at, 700) }); n != 0 {
		t.Errorf("a frame that went nowhere allocated %.0f times, want 0", n)
	}
	if len(c.Lanes) != lanes {
		t.Errorf("the place grew by %d lanes while the car stood still", len(c.Lanes)-lanes)
	}
}

func TestTurnedTurnsTheWayAHeadingDoes(t *testing.T) {
	v := turned(mathx.V(1, 0), 0.5)
	if mathx.Abs(v.Angle()-0.5) > 1e-5 || mathx.Abs(v.Len()-1) > 1e-5 {
		t.Errorf("a unit vector turned by 0.5 points %.4f and is %.4f long", v.Angle(), v.Len())
	}
}
