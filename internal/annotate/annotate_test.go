package annotate_test

import (
	"image/color"
	"testing"

	"github.com/danielriddell21/autobahn/internal/annotate"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/vision"
)

func world(t *testing.T) *sim.World {
	t.Helper()
	return sim.NewWorld(sim.Config{Seed: 7, Traffic: 40, Police: 3})
}

func bonnet() (vision.Camera, func(*sim.World) annotate.View) {
	cam := vision.DefaultCamera(420, 236)
	return cam, func(w *sim.World) annotate.View {
		return annotate.BonnetView(cam, w.Player.Pos, w.Player.Yaw)
	}
}

func TestLayoutProducesBoxesOnTheImage(t *testing.T) {
	w := world(t)
	cam, view := bonnet()

	boxes := annotate.Layout(w, view(w))
	if len(boxes) == 0 {
		t.Fatal("no boxes laid out for a car sitting on a road")
	}
	for _, b := range boxes {
		if b.MinX < 0 || b.MinY < 0 ||
			b.MaxX > float32(cam.Width) || b.MaxY > float32(cam.Height) {
			t.Errorf("%v box (%v,%v)-(%v,%v) falls outside the image",
				b.Class, b.MinX, b.MinY, b.MaxX, b.MaxY)
		}
		if b.Width() < 2 || b.Height() < 2 {
			t.Errorf("%v box is degenerate: %vx%v", b.Class, b.Width(), b.Height())
		}
	}
}

// The whole headless evaluation rests on this: rasterising the boxes and
// scanning them back must recover what was laid out.
func TestRasteriseRoundTripsThroughTheScanner(t *testing.T) {
	w := world(t)
	cam, view := bonnet()

	boxes := annotate.Layout(w, view(w))
	img := annotate.Rasterise(boxes, cam.Width, cam.Height)

	px := make([]color.RGBA, cam.Width*cam.Height)
	for i := range px {
		o := i * 4
		px[i] = color.RGBA{img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3]}
	}
	dets := vision.NewScanner().Scan(px, cam.Width, cam.Height)
	if len(dets) == 0 {
		t.Fatal("scanning the rasterised boxes recovered nothing")
	}

	// Every class that was laid out large enough to survive should come back.
	laid := map[vision.Class]int{}
	for _, b := range boxes {
		if b.Width() >= 4 && b.Height() >= 4 {
			laid[b.Class]++
		}
	}
	found := map[vision.Class]int{}
	for _, d := range dets {
		found[d.Class]++
	}
	for cl := range laid {
		if found[cl] == 0 {
			t.Errorf("class %v was laid out but never recovered by the scanner", cl)
		}
	}
}

func TestRasteriseIntoMatchesRasterise(t *testing.T) {
	w := world(t)
	cam, view := bonnet()
	boxes := annotate.Layout(w, view(w))

	fresh := annotate.Rasterise(boxes, cam.Width, cam.Height)
	reused := annotate.Rasterise(nil, cam.Width, cam.Height)
	// Dirty the buffer first, so a failure to clear would show up.
	for i := range reused.Pix {
		reused.Pix[i] = 0x7f
	}
	annotate.RasteriseInto(reused, boxes)

	for i := range fresh.Pix {
		if fresh.Pix[i] != reused.Pix[i] {
			t.Fatalf("reused buffer differs from a fresh one at byte %d", i)
		}
	}
}

// Boxes are laid out back to front, so a car standing on a stop line paints
// over it. That occlusion is what the autopilot has to cope with, and it must
// be reproduced identically in both drawing paths.
func TestLayoutOrdersLaneMarkersBeforeVehicles(t *testing.T) {
	w := world(t)
	_, view := bonnet()
	for range 600 {
		w.Update(sim.Controls{Throttle: 0.5}, 1.0/60)
	}

	boxes := annotate.Layout(w, view(w))
	lastLane, firstVehicle := -1, len(boxes)
	for i, b := range boxes {
		if vision.IsLaneMarker(b.Class) {
			lastLane = i
		}
		if vision.IsVehicle(b.Class) && i < firstVehicle {
			firstVehicle = i
		}
	}
	if lastLane >= 0 && firstVehicle < len(boxes) && lastLane > firstVehicle {
		t.Errorf("a lane marker at %d is drawn after a vehicle at %d", lastLane, firstVehicle)
	}
}

func TestBonnetViewSitsAheadOfTheCarAndLooksDown(t *testing.T) {
	cam := vision.DefaultCamera(420, 236)
	pos := mathx.V(10, 20)
	v := annotate.BonnetView(cam, pos, 0) // facing +X

	if v.Position.X <= pos.X {
		t.Errorf("camera at X=%v is not ahead of a car at X=%v", v.Position.X, pos.X)
	}
	if v.Position.Y != cam.Mount {
		t.Errorf("camera height = %v, want the calibrated %v", v.Position.Y, cam.Mount)
	}
	if v.Target.Y >= v.Position.Y {
		t.Error("the camera should be pitched down, so the target sits below the lens")
	}
	if v.FovY != cam.FovY {
		t.Errorf("field of view = %v, want %v", v.FovY, cam.FovY)
	}
}

func TestLayoutIsEmptyLookingAtNothing(t *testing.T) {
	w := world(t)
	cam := vision.DefaultCamera(420, 236)
	// Point the camera at the sky from far outside the city.
	v := annotate.View{
		Position: mathx.V3(9000, 400, 9000),
		Target:   mathx.V3(9000, 900, 9100),
		FovY:     cam.FovY, Width: cam.Width, Height: cam.Height,
	}
	if boxes := annotate.Layout(w, v); len(boxes) != 0 {
		t.Errorf("got %d boxes looking at empty sky, want 0", len(boxes))
	}
}
