package game

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

func TestTimesOfDayHaveNames(t *testing.T) {
	names := map[string]bool{}
	for tod := TimeDay; tod < NumTimes; tod++ {
		if tod.String() == "" || names[tod.String()] {
			t.Errorf("time %d is called %q, which is empty or already taken", tod, tod.String())
		}
		names[tod.String()] = true
	}
}

// Everything that only shows after dark is driven by one number, so it has to
// run the right way: nothing in daylight, everything at night.
func TestItGetsDarkerThroughTheDay(t *testing.T) {
	day, dusk, night := lightingFor(TimeDay), lightingFor(TimeDusk), lightingFor(TimeNight)
	if day.night != 0 || night.night != 1 || dusk.night <= day.night || dusk.night >= night.night {
		t.Errorf("night runs %.2f, %.2f, %.2f through day, dusk and night", day.night, dusk.night, night.night)
	}
	lum := func(l lighting) float32 { return l.sun.X + l.sun.Y + l.sun.Z + l.sky.X + l.sky.Y + l.sky.Z }
	if !(lum(day) > lum(dusk) && lum(dusk) > lum(night)) {
		t.Errorf("light falls %.2f, %.2f, %.2f through day, dusk and night", lum(day), lum(dusk), lum(night))
	}
	dawn := lightingFor(TimeDawn)
	if dawn.night <= day.night || dawn.night >= night.night {
		t.Errorf("dawn's night is %.2f, want between day and night", dawn.night)
	}
	for _, l := range []lighting{day, dusk, night, dawn} {
		d := l.sunDir
		if n := d.X*d.X + d.Y*d.Y + d.Z*d.Z; mathx.Abs(n-1) > 0.001 {
			t.Errorf("a light direction of length %.3f is not a direction", n)
		}
		if d.Y <= 0 {
			t.Error("the light comes from below the horizon")
		}
	}
}

// The scanner matches the annotation colours exactly, and every one of them
// has a channel at full brightness. The materials are named by the top few
// alpha values, so none of the world's own colours may already be using one.
func TestMaterialsDoNotCollideWithTransparency(t *testing.T) {
	for _, m := range []uint8{matPlain, matFacade, matRoad, matGlow, matPaint} {
		if m < matFloor {
			t.Errorf("material %d is below the floor of %d and would be drawn see-through", m, matFloor)
		}
	}
	if colShadow.A >= matFloor || colSkid.A >= matFloor {
		t.Error("a shadow or a skid mark is opaque enough to be taken for a material")
	}
	if got := as(colGlass, matGlow); got.A != matGlow || got.R != colGlass.R {
		t.Errorf("marking a colour as a material gave %v", got)
	}
}

func TestScatterIsRepeatableAndInRange(t *testing.T) {
	seen := map[[3]float32]bool{}
	for i := range 200 {
		a, b := scatter(12.5, -40, i), scatter(12.5, -40, i)
		if a != b {
			t.Fatalf("scatter gave %v then %v for the same place", a, b)
		}
		for _, v := range a {
			if v < 0 || v >= 1 {
				t.Fatalf("scatter gave %v, outside [0, 1)", v)
			}
		}
		seen[a] = true
	}
	if len(seen) < 190 {
		t.Errorf("200 indices gave only %d different results", len(seen))
	}
}

func TestMoneyIsGroupedInThousands(t *testing.T) {
	for v, want := range map[int]string{0: "£0", 950: "£950", 1250: "£1,250", 1234567: "£1,234,567"} {
		if got := money(v); got != want {
			t.Errorf("money(%d) = %q, want %q", v, got, want)
		}
	}
}

func TestOnlyASlidingCarLaysRubber(t *testing.T) {
	v := sim.NewVehicle(sim.CarSpec(), mathx.V(0, 0), 0)
	v.Vel = mathx.V(20, 0)
	if slipping(v) {
		t.Error("a car driving in a straight line is laying rubber")
	}
	v.Slip = 0.4
	if !slipping(v) {
		t.Error("a car in a slide is not")
	}
	v.Slip, v.Handbrake = 0, true
	if !slipping(v) {
		t.Error("a car on the handbrake is not")
	}
	v.Vel = mathx.V(1, 0)
	if slipping(v) {
		t.Error("a car at walking pace is laying rubber")
	}
}

func TestSkidMarksAreBounded(t *testing.T) {
	g := &Game{}
	for i := range maxSkids * 3 {
		g.addSkid(skid{a: mathx.V(float32(i), 0), b: mathx.V(float32(i)+1, 0)})
	}
	if len(g.fx.skids) != maxSkids {
		t.Errorf("%d skid marks kept, want the limit of %d", len(g.fx.skids), maxSkids)
	}
	// The oldest are the ones that went.
	oldest := float32(1e9)
	for _, s := range g.fx.skids {
		oldest = min(oldest, s.a.X)
	}
	if oldest != float32(maxSkids*2) {
		t.Errorf("the oldest mark kept is number %.0f, want %d", oldest, maxSkids*2)
	}
}

func TestEveryBodyHasRoomForItsCabin(t *testing.T) {
	for _, k := range sim.Bodies() {
		b := sim.BodyOf(k)
		if boot := bootShare(k); boot < 0 || boot+b.RoofFrac > 1 {
			t.Errorf("a %s has %.2f of boot and %.2f of cabin, which is more car than there is",
				k.Name(), boot, b.RoofFrac)
		}
		if b.Spec.Height-b.RoofRise <= b.Ride {
			t.Errorf("a %s has a waist below its own sill", k.Name())
		}
		if b.Wheel <= 0 || b.Ride <= 0 || b.Ride > b.Wheel*2 {
			t.Errorf("a %s rides %.2f m up on %.2f m wheels", k.Name(), b.Ride, b.Wheel)
		}
	}
}
