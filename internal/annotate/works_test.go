package annotate_test

import (
	"image/color"
	"testing"

	"github.com/danielriddell21/autobahn/internal/annotate"
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/vision"
	"github.com/danielriddell21/autobahn/internal/works"
)

// The autopilot ranges a roadworks sign from the top of its box, so the box and
// the calibration have to agree, as they do for every other sign.
func TestWorksSignBoxMatchesTheCameraCalibration(t *testing.T) {
	got := works.SignHeight - annotate.WorksSignDrop + annotate.WorksSignHalf
	if mathx.Abs(got-vision.WorksSignTopHeight) > 0.001 {
		t.Errorf("works sign box edge at %.3fm, calibrated as %.3fm", got, vision.WorksSignTopHeight)
	}
}

// standingWorld returns a dense world at a moment when something is solid.
func standingWorld(t *testing.T, m city.Map, seed uint64, kind works.Kind) (*sim.World, works.Closure) {
	t.Helper()
	w := sim.NewWorld(sim.Config{Seed: seed, Map: m, Traffic: 0, Police: 0, Works: works.Dense})
	for at := float32(100); at < 20000; at += 130 {
		w.Time = at
		w.Update(sim.Controls{}, 0.001)
		for _, cl := range w.Works.Active() {
			if cl.Kind == kind && cl.Phase == works.PhaseSolid {
				return w, cl
			}
		}
	}
	t.Fatalf("%s: no closure of kind %d standing at any time", m.Name(), kind)
	return nil, works.Closure{}
}

func classesIn(boxes []annotate.Box) map[vision.Class]int {
	out := map[vision.Class]int{}
	for _, b := range boxes {
		out[b.Class]++
	}
	return out
}

func TestLayoutBoxesTheConesAndTheirSign(t *testing.T) {
	w, cl := standingWorld(t, city.MapMotorway, 5, works.KindLane)
	l := w.City.Lanes[cl.Lanes[0]]
	first := l.Along(cl.Cones[0].Center)
	from := max(first-45, 1)
	at := l.Point(from)
	w.Player.Place(at, l.HeadingAt(from))
	w.Route.Update(w.Player.Pos, w.Player.Forward())
	_, view := bonnet()
	got := classesIn(annotate.Layout(w, view(w)))
	if got[vision.ClassCone] == 0 {
		t.Errorf("no cone annotated from %.0f m back: %v", first-from, got)
	}
	if first-from > 30 && got[vision.ClassDiversion] == 0 {
		t.Errorf("no diversion sign annotated from %.0f m back: %v", first-from, got)
	}
}

func TestLayoutBoxesABarrierAndTheClosedSign(t *testing.T) {
	w, cl := standingWorld(t, city.MapCity, 3, works.KindStreet)
	l := w.City.Lanes[cl.Lanes[0]]
	at := l.A.Sub(l.DirAt(0).Mul(12))
	w.Player.Place(at, l.StartDir().Angle())
	w.Route.Update(w.Player.Pos, w.Player.Forward())
	_, view := bonnet()
	got := classesIn(annotate.Layout(w, view(w)))
	if got[vision.ClassBarrier] == 0 || got[vision.ClassRoadClosed] == 0 {
		t.Errorf("barrier and closed sign should both be annotated at the mouth of a shut street: %v", got)
	}
}

func TestWorksRoundTripThroughTheScanner(t *testing.T) {
	w, cl := standingWorld(t, city.MapCity, 3, works.KindStreet)
	l := w.City.Lanes[cl.Lanes[0]]
	w.Player.Place(l.A.Sub(l.DirAt(0).Mul(10)), l.StartDir().Angle())
	w.Route.Update(w.Player.Pos, w.Player.Forward())
	cam, view := bonnet()
	boxes := annotate.Layout(w, view(w))
	img := annotate.Rasterise(boxes, cam.Width, cam.Height)
	dets := vision.NewScanner().Scan(annotate.Pixels(img, make([]color.RGBA, cam.Width*cam.Height)), cam.Width, cam.Height)
	seen := map[vision.Class]bool{}
	for _, d := range dets {
		seen[d.Class] = true
	}
	if !seen[vision.ClassBarrier] {
		t.Errorf("a barrier did not survive the trip through the scanner: %v", seen)
	}
}

func TestNoWorksMeansNoWorksBoxes(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 10, Police: 0, Works: works.Off})
	_, view := bonnet()
	for _, b := range annotate.Layout(w, view(w)) {
		switch b.Class {
		case vision.ClassCone, vision.ClassBarrier, vision.ClassRoadClosed, vision.ClassDiversion:
			t.Fatalf("%v annotated with the works switched off", b.Class)
		}
	}
}
