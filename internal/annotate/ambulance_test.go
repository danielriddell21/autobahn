package annotate_test

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/annotate"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/vision"
)

// ambulanceWorld is a world with one ambulance and nothing else on the road.
func ambulanceWorld() (*sim.World, *sim.Agent) {
	w := sim.NewWorld(sim.Config{Seed: 7, Ambulances: 1})
	a := w.Ambulances()[0]
	return w, a
}

func classesOf(boxes []annotate.Box, cl vision.Class) int {
	n := 0
	for _, b := range boxes {
		if b.Class == cl {
			n++
		}
	}
	return n
}

// An ambulance is something to get out of the way of only while its lights are
// on; without them it is any other vehicle.
func TestAmbulanceIsAnnotatedOnlyWithItsLightsOn(t *testing.T) {
	w, a := ambulanceWorld()
	_, view := bonnet()
	put(w, a, 20, 0)

	if a.BlueLights() {
		t.Fatal("an ambulance that has not been sent out has its lights on")
	}
	boxes := annotate.Layout(w, view(w))
	if classesOf(boxes, vision.ClassAmbulance) != 0 || classesOf(boxes, vision.ClassVehicle) != 1 {
		t.Errorf("lights off: %d ambulance boxes, %d vehicle boxes", classesOf(boxes, vision.ClassAmbulance), classesOf(boxes, vision.ClassVehicle))
	}

	a.StartRun(10)
	boxes = annotate.Layout(w, view(w))
	if classesOf(boxes, vision.ClassAmbulance) != 1 || classesOf(boxes, vision.ClassVehicle) != 0 {
		t.Errorf("lights on: %d ambulance boxes, %d vehicle boxes", classesOf(boxes, vision.ClassAmbulance), classesOf(boxes, vision.ClassVehicle))
	}
}

// The mirrors see it too, which is how a driver knows one is coming up behind.
func TestAmbulanceIsBoxedInTheMirrors(t *testing.T) {
	w, a := ambulanceWorld()
	cams := vision.MirrorCameras(256, 144)
	stand := func() annotate.View {
		return annotate.MirrorView(cams[vision.MirrorRear], w.City.StandAt(w.Player.Pos, w.Player.Yaw))
	}
	put(w, a, -20, 0)
	if got := classesOf(annotate.LayoutMirror(w, stand()), vision.ClassAmbulance); got != 0 {
		t.Errorf("lights off, the mirror boxed %d ambulances", got)
	}
	a.StartRun(10)
	if got := classesOf(annotate.LayoutMirror(w, stand()), vision.ClassAmbulance); got != 1 {
		t.Errorf("lights on, the mirror boxed %d ambulances, want 1", got)
	}
	// And a scan of what was drawn recovers the class.
	eye := annotate.NewMirrorEye(cams[vision.MirrorRear].Width, cams[vision.MirrorRear].Height)
	dets := eye.Look(w, cams[vision.MirrorRear], w.City.StandAt(w.Player.Pos, w.Player.Yaw), w.Weather())
	found := false
	for _, d := range dets {
		found = found || d.Class == vision.ClassAmbulance
	}
	if !found {
		t.Errorf("the scanner did not recover an ambulance: %+v", dets)
	}
}
