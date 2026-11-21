package city_test

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

var hillyMaps = []city.Map{city.MapSeaside, city.MapVillage, city.MapMotorway}

func TestTheCityIsLevel(t *testing.T) {
	c := city.NewGrowing(city.ParamsFor(city.MapCity, 7))
	if c.Hilly() {
		t.Fatal("the city is said to stand on hills")
	}
	for _, p := range []mathx.Vec{{}, {X: 300, Z: -200}, {X: -900, Z: 1400}} {
		if h := c.Height(p); h != 0 {
			t.Errorf("the city stands %.2fm up at %v", h, p)
		}
		if s := c.Slope(p); s != (mathx.Vec{}) {
			t.Errorf("the city slopes %v at %v", s, p)
		}
		if g := c.Grade(p, mathx.V(1, 0)); g != 0 {
			t.Errorf("the city climbs %v at %v", g, p)
		}
	}
	if st := c.StandAt(mathx.V(40, 50), 0.7); st.Up != mathx.V3(0, 1, 0) || st.At.Y != 0 {
		t.Errorf("a car in the city stands %+v", st)
	}
}

func TestAPlanIsDrawnFlat(t *testing.T) {
	for _, m := range hillyMaps {
		c := city.NewGrowing(city.ParamsFor(m, 7))
		if !c.Hilly() {
			t.Errorf("%s: the ground is level", m.Name())
		}
		if c.Plan().Hilly() {
			t.Errorf("%s: the plan stands on hills", m.Name())
		}
	}
}

// The hills have to be there, and have to be gentle: the autopilot's camera
// ranges on the road being a plane as far as it can see.
func TestHillsAreThereAndAreGentle(t *testing.T) {
	const (
		steepest = 0.1  // one in ten
		tightest = 1000 // metres of radius, crest or dip
	)
	for _, m := range hillyMaps {
		for _, seed := range []uint64{1, 7, 23} {
			c := city.NewGrowing(city.ParamsFor(m, seed))
			lo, hi := float32(1e9), float32(-1e9)
			for x := float32(-2000); x <= 2000; x += 25 {
				for z := float32(-2000); z <= 2000; z += 25 {
					p := mathx.V(x, z)
					h := c.Height(p)
					lo, hi = min(lo, h), max(hi, h)
					if s := c.Slope(p).Len(); s > steepest {
						t.Fatalf("%s seed %d: the ground at %v climbs one in %.1f", m.Name(), seed, p, 1/s)
					}
					// How fast the slope changes is the curve of the ground.
					for _, d := range []mathx.Vec{{X: 1}, {Z: 1}, {X: 0.707, Z: 0.707}} {
						const step = 10
						bend := (c.Grade(p.Add(d.Mul(step)), d) - c.Grade(p.Sub(d.Mul(step)), d)) / (2 * step)
						if mathx.Abs(bend) > 1.0/tightest {
							t.Fatalf("%s seed %d: the ground at %v curves at a radius of %.0fm",
								m.Name(), seed, p, 1/mathx.Abs(bend))
						}
					}
				}
			}
			if hi-lo < 8 {
				t.Errorf("%s seed %d: the land only ranges over %.1fm", m.Name(), seed, hi-lo)
			}
		}
	}
}

func TestTheLandIsLevelByTheSeaAndClimbsFromIt(t *testing.T) {
	c := city.NewGrowing(city.ParamsFor(city.MapSeaside, 7))
	front, sea, ok := c.Shoreline()
	if !ok {
		t.Fatal("setup failed: no coast")
	}
	for x := float32(-1500); x <= 1500; x += 100 {
		for _, z := range []float32{sea + 200, sea, front, front - 40} {
			if h := c.Height(mathx.V(x, z)); h != 0 {
				t.Fatalf("the ground at (%v, %v), by the sea, stands %.2fm up", x, z, h)
			}
		}
		if h := c.Height(mathx.V(x, sea-1200)); h < 10 {
			t.Errorf("a long way inland at x=%v the ground has only climbed %.1fm", x, h)
		}
		for z := sea; z > sea-1500; z -= 50 {
			if h := c.Height(mathx.V(x, z)); h < 0 {
				t.Fatalf("the ground at (%v, %v) is %.2fm below the sea", x, z, h)
			}
		}
	}
}

func TestHeightIsAFunctionOfThePlace(t *testing.T) {
	a := city.NewGrowing(city.ParamsFor(city.MapVillage, 7))
	b := city.NewGrowing(city.ParamsFor(city.MapVillage, 7))
	b.EnsureAround(mathx.V(900, -400), 400)
	other := city.NewGrowing(city.ParamsFor(city.MapVillage, 8))
	differs := false
	for _, p := range []mathx.Vec{{}, {X: 310, Z: 95}, {X: -640, Z: 1210}} {
		if a.Height(p) != b.Height(p) {
			t.Errorf("the ground at %v is %.3fm in one city and %.3fm in the same city grown elsewhere",
				p, a.Height(p), b.Height(p))
		}
		differs = differs || a.Height(p) != other.Height(p)
	}
	if !differs {
		t.Error("two seeds have the same hills")
	}
}

func TestSlopeAndGradeAgreeWithHeight(t *testing.T) {
	c := city.NewGrowing(city.ParamsFor(city.MapVillage, 7))
	for _, p := range []mathx.Vec{{X: 120, Z: 40}, {X: -310, Z: 505}, {X: 77, Z: -960}} {
		for _, yaw := range []float32{0, 1.1, 2.6, -2} {
			d := mathx.FromAngle(yaw)
			want := (c.Height(p.Add(d.Mul(4))) - c.Height(p.Sub(d.Mul(4)))) / 8
			if got := c.Grade(p, d); mathx.Abs(got-want) > 0.002 {
				t.Errorf("at %v heading %.1f the grade is %.4f, and the ground climbs %.4f", p, yaw, got, want)
			}
		}
	}
}

func TestAStandIsSquareAndTiltsWithTheGround(t *testing.T) {
	c := city.NewGrowing(city.ParamsFor(city.MapVillage, 7))
	// Somewhere with a slope worth the name.
	var p mathx.Vec
	for x := float32(0); x < 2000; x += 20 {
		if c.Slope(mathx.V(x, 0)).Len() > 0.03 {
			p = mathx.V(x, 0)
			break
		}
	}
	up := c.Slope(p).Norm() // the way straight uphill
	s := c.StandAt(p, up.Angle())
	for name, v := range map[string]float32{
		"fwd.right": s.Fwd.Dot(s.Right), "fwd.up": s.Fwd.Dot(s.Up), "right.up": s.Right.Dot(s.Up),
	} {
		if mathx.Abs(v) > 1e-3 {
			t.Errorf("%s = %.4f, want square", name, v)
		}
	}
	if s.Up.Y <= 0.9 {
		t.Errorf("up is %v", s.Up)
	}
	if s.Pitch() <= 0.02 {
		t.Errorf("facing uphill the pitch is %.3f", s.Pitch())
	}
	if mathx.Abs(s.Roll()) > 0.005 {
		t.Errorf("facing straight uphill the roll is %.3f", s.Roll())
	}
	if down := c.StandAt(p, up.Angle()+3.14159265); down.Pitch() >= -0.02 {
		t.Errorf("facing downhill the pitch is %.3f", down.Pitch())
	}
	// Across the hill, with the high ground on the left, it leans right.
	if across := c.StandAt(p, up.Angle()+1.5707963); across.Roll() <= 0.02 {
		t.Errorf("across the hill, high side left, the roll is %.3f", across.Roll())
	}
	// A point ahead of a car on a slope is on the slope.
	ahead := s.Point(10, 0, 0)
	if got, want := ahead.Y, c.Height(mathx.V(ahead.X, ahead.Z)); mathx.Abs(got-want) > 0.08 {
		t.Errorf("ten metres up the hill is %.2fm up by the stand and %.2fm by the ground", got, want)
	}
	if s.At.Y != c.Height(p) {
		t.Errorf("the stand is at %.2fm on ground at %.2fm", s.At.Y, c.Height(p))
	}
}
