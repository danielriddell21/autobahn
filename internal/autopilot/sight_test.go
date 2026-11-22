package autopilot_test

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/vision"
)

func markers(t *testing.T, metres ...float32) []vision.Detection {
	t.Helper()
	var out []vision.Detection
	for _, m := range metres {
		out = append(out, atRange(t, cam(), vision.ClassLaneMarker, m, 210))
	}
	return out
}

func TestTheCarStopsWithinTheRoadItCanSee(t *testing.T) {
	var s autopilot.Sight
	s.Update(cam(), markers(t, 10, 20, 30), 0.1)
	const decel = 4.0
	v := s.Cap(decel)
	if stop := v * v / (2 * decel); stop > s.Reach() {
		t.Errorf("at %.1f m/s it takes %.1f m to stop and it sees %.1f m", v, stop, s.Reach())
	}
	if v >= 1e8 {
		t.Error("seeing thirty metres of road did not hold the car back")
	}

	var short autopilot.Sight
	short.Update(cam(), markers(t, 8, 15), 0.1)
	if short.Cap(decel) >= v {
		t.Error("seeing less road did not hold the car back more")
	}
}

func TestClearRoadHoldsNothingBack(t *testing.T) {
	var s autopilot.Sight
	if s.Cap(4) < 1e8 {
		t.Error("a car that has seen nothing was held to a speed")
	}
	s.Update(cam(), markers(t, 15, 30, 45, 50), 0.1)
	if s.Cap(4) < 1e8 {
		t.Errorf("a clear road, seen to %.0f m, held the car to %.1f m/s", s.Reach(), s.Cap(4))
	}
	// Other things on the road say nothing about how far it can be seen.
	var other autopilot.Sight
	other.Update(cam(), []vision.Detection{atRange(t, cam(), vision.ClassVehicle, 20, 210)}, 0.1)
	if other.Reach() != 0 {
		t.Error("a vehicle was taken for a lane marker")
	}
}

func TestAMarkerThatFlickersOutIsNotForgottenAtOnce(t *testing.T) {
	var s autopilot.Sight
	s.Update(cam(), markers(t, 10, 25, 40), 0.05)
	far := s.Reach()
	s.Update(cam(), markers(t, 10, 25), 0.05)
	if s.Reach() >= far || s.Reach() < far-2 {
		t.Errorf("sight went from %.1f to %.1f in one frame", far, s.Reach())
	}
	for range 40 {
		s.Update(cam(), markers(t, 10, 25), 0.05)
	}
	if got := s.Reach(); got < 24 || got > 26 {
		t.Errorf("after a while sight settles at %.1f, want about 25", got)
	}
}

func TestTheDriverHoldsItsSpeedToWhatItCanSee(t *testing.T) {
	far, near := autopilot.New(cam()), autopilot.New(cam())
	for range 30 {
		far.Drive(markers(t, 8, 16, 24, 32, 40, 48, 56), 15, 0.05)
		near.Drive(markers(t, 6, 12, 18), 15, 0.05)
	}
	if near.TargetSpeed >= far.TargetSpeed {
		t.Errorf("seeing %.0f m of road gave a target of %.1f m/s, against %.1f for %.0f m",
			near.Sight.Reach(), near.TargetSpeed, far.TargetSpeed, far.Sight.Reach())
	}
	if stop := near.TargetSpeed * near.TargetSpeed / (2 * 5.35); stop > near.Sight.Reach() {
		t.Errorf("the target of %.1f m/s cannot be stopped from in %.1f m", near.TargetSpeed, near.Sight.Reach())
	}
}
