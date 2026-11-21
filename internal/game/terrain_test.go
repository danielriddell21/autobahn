package game

import (
	"runtime"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

func TestLevelLandIsNoHeightAtAll(t *testing.T) {
	var l land
	if h := l.at(mathx.V(30, -400)); h != 0 {
		t.Errorf("land that is nil stands %.2fm up", h)
	}
	l = func(p mathx.Vec) float32 { return p.X / 10 }
	if h := l.at(mathx.V(30, -400)); h != 3 {
		t.Errorf("land = %.2f, want what it was given", h)
	}
}

func TestAPatchIsMadeOfNumberedCorners(t *testing.T) {
	var w patchwork
	if w.upload() != nil {
		t.Fatal("a patch with nothing in it was handed over")
	}
	up := vec3(0, 1, 0)
	a := w.corner(vec3(0, 1, 0), up, colKerb)
	b := w.corner(vec3(4, 2, 0), up, colKerb)
	c := w.corner(vec3(0, 3, 4), up, colKerb)
	d := w.corner(vec3(4, 4, 4), up, colKerb)
	if a != 0 || b != 1 || c != 2 || d != 3 {
		t.Fatalf("corners numbered %d %d %d %d", a, b, c, d)
	}
	if len(w.at) != 12 || len(w.up) != 12 || len(w.col) != 16 {
		t.Fatalf("four corners hold %d, %d and %d numbers", len(w.at), len(w.up), len(w.col))
	}
	// Whichever way round a triangle is given it goes in the same way up.
	w.tri(a, b, c)
	w.tri(a, c, b)
	if got := w.tris; got[0] != got[3] || got[1] != got[4] || got[2] != got[5] {
		t.Errorf("the same triangle given both ways round went in as %v", got)
	}
	w.quad(a, b, d, c)
	if len(w.tris) != 12 {
		t.Errorf("two triangles and a quad are %d corners, want 12", len(w.tris))
	}
	// A patch nobody made draws nothing and frees nothing.
	var none *patch
	none.draw(rl.Material{})
	none.free()
}

func TestNetLinesAreCloseAndFallOnTheirMarks(t *testing.T) {
	got := lines(10, 110, 31, 82)
	if got[0] != 10 || got[len(got)-1] != 110 {
		t.Fatalf("lines run from %v to %v", got[0], got[len(got)-1])
	}
	marks := map[float32]bool{}
	for i := 1; i < len(got); i++ {
		if step := got[i] - got[i-1]; step <= 0 || step > swellCell+1e-3 {
			t.Errorf("lines %d and %d are %.2fm apart", i-1, i, step)
		}
		marks[got[i]] = true
	}
	if !marks[31] || !marks[82] {
		t.Errorf("no line on a mark: %v", got)
	}
	// A mark outside, or on top of an end, is not a line of its own.
	if plain, marked := lines(10, 110), lines(10, 110, 5, 10.2, 109.9, 300); len(plain) != len(marked) {
		t.Errorf("marks off the net made %d lines of %d", len(marked), len(plain))
	}
}

// The net must stay inside the kerb: a corner of it out in a roundabout
// would be pavement on the carriageway.
func TestTheNetKeepsClearOfEveryCorner(t *testing.T) {
	bitten := 0
	for _, m := range []city.Map{city.MapSeaside, city.MapVillage, city.MapMotorway} {
		for _, b := range planOf(t, m).Blocks {
			in := netInset(b)
			if in < 4 {
				t.Fatalf("%s: a net starts %.1fm from the kerb", m.Name(), in)
			}
			lo, hi := b.Min.Add(mathx.V(in, in)), b.Max.Sub(mathx.V(in, in))
			if hi.X <= lo.X || hi.Z <= lo.Z {
				continue // too small for one; drawn as a slab
			}
			for _, c := range b.Corners {
				if c.Bite <= 0 {
					continue
				}
				bitten++
				for _, p := range []mathx.Vec{lo, {X: hi.X, Z: lo.Z}, {X: lo.X, Z: hi.Z}, hi} {
					if p.DistTo(c.Centre) < c.Bite {
						t.Errorf("%s: the net's corner at %v is inside the roundabout at %v", m.Name(), p, c.Centre)
					}
				}
			}
		}
	}
	if bitten == 0 {
		t.Error("setup failed: no block cut back for a roundabout")
	}
}

func TestEveryBlockOnHillsHasGroundWithAShape(t *testing.T) {
	g := &Game{world: sim.NewWorld(sim.Config{Seed: 7, Traffic: 0, Map: city.MapSeaside})}
	g.opts.Map = city.MapSeaside
	g.land = g.world.City.Height
	shaped, beaches := 0, 0
	for _, b := range g.world.City.Plan().Blocks {
		r := g.raise(b)
		switch {
		case b.Beach:
			beaches++
			if r != nil {
				t.Errorf("the beach at %v has a net", b.Min)
			}
		case r != nil:
			shaped++
			if !r.rises && r.at(b.Min.Lerp(b.Max, 0.5)) != 0 {
				t.Errorf("the block at %v does not rise and stands %.2fm up", b.Min, r.at(b.Min.Lerp(b.Max, 0.5)))
			}
		}
	}
	if shaped == 0 || beaches == 0 {
		t.Fatalf("%d blocks with a net and %d beaches", shaped, beaches)
	}
	// On the level only a block that rises has one, and in the city none.
	flat := &Game{world: sim.NewWorld(sim.Config{Seed: 7, Traffic: 0})}
	for _, b := range flat.world.City.Blocks {
		if flat.raise(b) != nil {
			t.Fatalf("a block of the city at %v has a net", b.Min)
		}
	}
}

func TestACarStandsOnTheHillItIsOn(t *testing.T) {
	g := &Game{world: sim.NewWorld(sim.Config{Seed: 7, Traffic: 0, Map: city.MapVillage})}
	if f := g.footingOf(mathx.V(50, 60), 1); f.land != nil || f.y != 0 || f.pitch != 0 || f.roll != 0 {
		t.Errorf("with no land a car's footing is %+v", f)
	}
	c := g.world.City
	g.land = c.Height
	var p mathx.Vec
	for x := float32(0); x < 3000; x += 15 {
		if c.Slope(mathx.V(x, 0)).Len() > 0.03 {
			p = mathx.V(x, 0)
			break
		}
	}
	uphill := c.Slope(p).Angle()
	f := g.footingOf(p, uphill)
	if f.y != c.Height(p) {
		t.Errorf("the car is at %.2fm on ground at %.2fm", f.y, c.Height(p))
	}
	if f.pitch <= 0.02 || mathx.Abs(f.roll) > 0.01 {
		t.Errorf("facing uphill the car pitches %.3f and rolls %.3f", f.pitch, f.roll)
	}
	if f.land == nil {
		t.Error("the car's shadow has no land to lie on")
	}
}

func TestABuildingIsStoodAtItsDoor(t *testing.T) {
	as := city.Building{Center: mathx.V(100, 50), W: 20, D: 12, Front: mathx.V(0, 1)}
	on := as
	if got, want := doorstep(as, on), mathx.V(100, 56); got.DistTo(want) > 1e-3 {
		t.Errorf("a building facing +Z has its door at %v, want %v", got, want)
	}
	as.Front = mathx.V(-1, 0)
	on = as
	if got, want := doorstep(as, on), mathx.V(90, 50); got.DistTo(want) > 1e-3 {
		t.Errorf("a building facing -X has its door at %v, want %v", got, want)
	}
	// Turned a quarter turn on the ground, its door turns with it.
	on.Heading = 1.5707963
	if got, want := doorstep(as, on), mathx.V(100, 40); got.DistTo(want) > 1e-2 {
		t.Errorf("turned a quarter turn the door is at %v, want %v", got, want)
	}
}

// TestHillyPlacesDraw steps each place that stands on hills through every
// camera, which is what finds a patch built wrong or drawn with something
// still on the matrix stack: neither shows in the arithmetic.
func TestHillyPlacesDraw(t *testing.T) {
	requireDisplay(t)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	rl.SetTraceLogLevel(rl.LogError)
	rl.InitWindow(320, 180, "autobahn test")
	defer rl.CloseWindow()

	for _, m := range []city.Map{city.MapSeaside, city.MapVillage, city.MapMotorway} {
		o := DefaultOptions()
		o.Mute, o.Seed, o.Map = true, 7, m
		o.Width, o.Height, o.Traffic = 320, 180, 12
		g := New(o)
		for _, mode := range []CameraMode{CameraChase, CameraBonnet, CameraHigh} {
			g.camMode = mode
			for range 4 {
				g.Step(1.0 / 60)
			}
		}
		if g.land == nil {
			t.Errorf("%s: no land", m.Name())
		}
		tiles, far := 0, 0
		for k, p := range g.tiles {
			if p == nil {
				t.Errorf("%s: tile %+v was never made", m.Name(), k)
			}
			tiles++
			if k.far {
				far++
			}
		}
		if far == 0 || tiles == far {
			t.Errorf("%s: %d tiles of ground, %d of them far", m.Name(), tiles, far)
		}
		built := 0
		for _, r := range g.risen {
			if r != nil && r.ground != nil {
				built++
			}
		}
		if built == 0 {
			t.Errorf("%s: no block's ground was made", m.Name())
		}
		// Driven a long way off, what was near is given back.
		before := len(g.tiles)
		for range 12 {
			p := g.world.Player
			g.world.Player.Place(p.Pos.Add(mathx.V(-900, -700)), p.Yaw)
			g.world.PlaceOnNearestLane()
			g.Step(1.0 / 60)
		}
		if len(g.tiles) > tilesKept+before {
			t.Errorf("%s: %d tiles held after a long drive", m.Name(), len(g.tiles))
		}
		g.Close()
		for _, r := range g.risen {
			if r != nil && r.ground != nil {
				t.Fatalf("%s: a block's ground outlived the game", m.Name())
			}
		}
		if len(g.tiles) != 0 {
			t.Errorf("%s: %d tiles outlived the game", m.Name(), len(g.tiles))
		}
	}
}
