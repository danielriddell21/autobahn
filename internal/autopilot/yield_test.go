package autopilot

import (
	"math"
	"testing"

	"github.com/danielriddell21/autobahn/internal/vision"
)

// leftLane is the car's own lane with the lane alongside it to the left, which
// is the kerbside one, marked as far as the camera would show it.
func leftLane(cam vision.Camera) []vision.Detection {
	var out []vision.Detection
	for i, d := 0, float32(5); d <= 50; i, d = i+1, d+5 {
		cl := vision.ClassLaneMarker
		if i%2 == 1 {
			cl = vision.ClassLaneMarkerAlt
		}
		out = append(out, at(cam, cl, d, 0, 0.3, 4))
	}
	for d := float32(8); d <= 40; d += 8 {
		out = append(out, at(cam, vision.ClassLaneSide, d, -laneWidth, 0.5, 5))
	}
	return out
}

// ownLane is the car's own lane and nothing alongside it.
func ownLane(cam vision.Camera) []vision.Detection { return leftLane(cam)[:10] }

// rearBox is a vehicle of a class as the rear mirror shows it, ahead metres
// from the middle of the car (negative behind) and right across.
func rearBox(d *Driver, cl vision.Class, ahead, right float32) vision.Detection {
	cam := d.mirrors[vision.MirrorRear]
	// Along the lens axis, which points backwards.
	dist := -(ahead - cam.Ahead)
	down := math.Atan(float64(cam.Mount / dist))
	focal := float64(cam.Height) / 2 / math.Tan(float64(cam.FovY)*0.5*math.Pi/180)
	row := float32(float64(cam.Height)/2 + focal*math.Tan(down-float64(cam.Pitch)))
	// Looking backwards, the car's right is the lens's left.
	col := float32(float64(cam.Width)/2 - focal*float64(right/dist))
	return vision.Detection{
		Class: cl, MinX: col - 8, MaxX: col + 8, MinY: row - 12, MaxY: row,
		BaseMinX: col - 8, BaseMaxX: col + 8, Pixels: 100,
	}
}

// approach has an ambulance close in behind the car from far back, one look
// at a time, as the world's mirrors show it, while the car holds its speed.
func approach(d *Driver, scene []vision.Detection, speed float32, from, to float32) {
	for ahead := -from; ahead <= -to; ahead += 1.5 {
		d.Watch(vision.MirrorRear, []vision.Detection{rearBox(d, vision.ClassAmbulance, ahead, 0)})
		run(d, scene, speed, 0.2)
	}
}

// signalAt is a traffic light of a class, ranged from the bottom of its box.
func signalAt(cam vision.Camera, cl vision.Class, dist, lateral float32) vision.Detection {
	focal := float64(cam.Height) / 2 / math.Tan(float64(cam.FovY)*0.5*math.Pi/180)
	down := math.Atan(float64(cam.Mount-vision.SignalHeadHeight) / float64(dist))
	bottom := float32(float64(cam.Height)/2 + focal*math.Tan(down-float64(cam.Pitch)))
	col := colFor(cam, dist, lateral)
	return vision.Detection{Class: cl, MinX: col - 6, MaxX: col + 6, MinY: bottom - 20, MaxY: bottom, Pixels: 100}
}

func TestMirrorGroundRoundTripsForTheTestHelper(t *testing.T) {
	d := driver()
	cam := d.mirrors[vision.MirrorRear]
	for _, ahead := range []float32{-8, -15, -30} {
		det := rearBox(d, vision.ClassAmbulance, ahead, 1)
		lo, hi := det.Base()
		a, r, ok := cam.Ground((lo+hi)/2, det.MaxY)
		if !ok || abs(a-ahead) > 0.5 || abs(r-1) > 0.3 {
			t.Errorf("%.0f behind and 1 right came back as %.1f, %.1f", ahead, a, r)
		}
	}
}

func TestWatchMarksAnAmbulanceInTheMirror(t *testing.T) {
	d := driver()
	d.Watch(vision.MirrorRear, []vision.Detection{
		rearBox(d, vision.ClassVehicle, -20, 0), rearBox(d, vision.ClassAmbulance, -12, 0),
	})
	var emergency int
	for _, n := range d.Around() {
		if n.Emergency {
			emergency++
		}
	}
	if len(d.Around()) != 2 || emergency != 1 {
		t.Errorf("around %+v", d.Around())
	}
}

func TestAnAmbulanceClosingInTheMirrorIsGivenWayTo(t *testing.T) {
	d := driver()
	d.SeenLimit = 13.4
	scene := leftLane(d.cam)
	ref := driver()
	ref.SeenLimit = 13.4
	run(ref, scene, 12, 1)

	approach(d, scene, 12, 45, 20)
	if !d.yield.active {
		t.Fatal("an ambulance closing from 45 m to 20 m was not given way to")
	}
	if d.TargetSpeed > d.SeenLimit*yieldShare+0.01 || d.TargetSpeed >= ref.TargetSpeed {
		t.Errorf("target %.1f for a limit of %.1f, want %.0f%% of it (and under %.1f)", d.TargetSpeed, d.SeenLimit, yieldShare*100, ref.TargetSpeed)
	}
	cmd := run(d, scene, 12, 0.1)
	if cmd.Indicator != IndicateLeft {
		t.Errorf("indicator %v, want left", cmd.Indicator)
	}
	if d.move == moveNone || d.moveSide >= 0 {
		t.Errorf("not moving to the kerbside lane: move %d side %.1f", d.move, d.moveSide)
	}
}

func TestItStaysInLaneWhenTheKerbsideLaneIsTaken(t *testing.T) {
	d := driver()
	d.SeenLimit = 13.4
	scene := append(leftLane(d.cam), at(d.cam, vision.ClassVehicle, 9, -laneWidth, 1, 12))
	approach(d, scene, 12, 45, 20)
	if !d.yield.active {
		t.Fatal("not giving way")
	}
	if d.move == moveShift {
		t.Error("moved across into a lane with a car in it")
	}
	if d.TargetSpeed > d.SeenLimit*yieldShare+0.01 {
		t.Errorf("target %.1f, want at most %.1f", d.TargetSpeed, d.SeenLimit*yieldShare)
	}
}

func TestWithNoKerbsideLaneItEasesAndIndicates(t *testing.T) {
	d := driver()
	d.SeenLimit = 13.4
	scene := ownLane(d.cam)
	approach(d, scene, 12, 45, 20)
	if !d.yield.active || d.move != moveNone {
		t.Fatalf("active %v move %d", d.yield.active, d.move)
	}
	if cmd := run(d, scene, 12, 0.1); cmd.Indicator != IndicateLeft || d.TargetSpeed > d.SeenLimit*yieldShare+0.01 {
		t.Errorf("indicator %v target %.1f", cmd.Indicator, d.TargetSpeed)
	}
}

// It holds its speed down until the ambulance has gone by, and no longer.
func TestItHoldsUntilTheAmbulanceHasPassed(t *testing.T) {
	d := driver()
	d.SeenLimit = 13.4
	scene := ownLane(d.cam)
	approach(d, scene, 12, 45, 10)
	// Alongside, in the side mirror: not yet by.
	d.Watch(vision.MirrorRear, nil)
	d.around[vision.MirrorRight] = []Neighbour{{Ahead: 0.5, Right: 3.6, Emergency: true}}
	run(d, scene, 12, 3)
	if !d.yield.active {
		t.Fatal("let go while the ambulance was alongside")
	}
	// Gone from every mirror: it has passed.
	d.around[vision.MirrorRight] = nil
	run(d, scene, 12, yieldLetGo+0.5)
	if d.yield.active {
		t.Error("still giving way after the ambulance has gone")
	}
	if cmd := run(d, scene, 12, 0.2); cmd.Indicator != IndicateOff {
		t.Errorf("still indicating: %v", cmd.Indicator)
	}
}

// An ambulance lying off a long way back and not gaining is not given way to.
func TestADistantAmbulanceNotGainingIsLeftAlone(t *testing.T) {
	d := driver()
	d.SeenLimit = 13.4
	scene := ownLane(d.cam)
	for range 20 {
		d.Watch(vision.MirrorRear, []vision.Detection{rearBox(d, vision.ClassAmbulance, -50, 0)})
		run(d, scene, 12, 0.2)
	}
	if d.yield.active {
		t.Error("gave way to an ambulance 50 m back that was not gaining")
	}
	e := driver()
	e.SeenLimit = 13.4
	run(e, scene, 12, 1)
	if d.TargetSpeed != e.TargetSpeed {
		t.Errorf("target %.2f, want the ordinary %.2f", d.TargetSpeed, e.TargetSpeed)
	}
}

// Giving way never takes the car through anything it would have stopped for:
// it only ever takes speed away.
func TestGivingWayNeverRunsARedLight(t *testing.T) {
	for _, dist := range []float32{12, 20, 35} {
		cam := driver().cam
		scene := append(leftLane(cam), at(cam, vision.ClassStopLine, dist, 0, 1.8, 4), signalAt(cam, vision.ClassLightRed, dist+2, 1.8))

		ref := driver()
		ref.SeenLimit = 13.4
		run(ref, scene, 8, 1.5)

		d := driver()
		d.SeenLimit = 13.4
		approach(d, scene, 8, 45, 20)
		run(d, scene, 8, 1)
		if d.TargetSpeed > ref.TargetSpeed+0.01 {
			t.Errorf("red %.0f m ahead: target %.2f giving way against %.2f without", dist, d.TargetSpeed, ref.TargetSpeed)
		}
	}
}

// It never starts a lane change for the ambulance with a junction near, where
// the lane might not go on and it would be stuck in the box.
func TestItDoesNotMoveOverAtAJunction(t *testing.T) {
	d := driver()
	d.SeenLimit = 13.4
	scene := append(leftLane(d.cam), at(d.cam, vision.ClassStopLine, 16, 0, 1.8, 4))
	approach(d, scene, 12, 45, 20)
	if d.move != moveNone {
		t.Errorf("started a lane change 16 m from a stop line (move %d)", d.move)
	}
}

// An ambulance in front is a vehicle to follow and never to overtake.
func TestItNeverOvertakesAnAmbulanceAhead(t *testing.T) {
	passes := func(cl vision.Class) bool {
		d := driver()
		d.Mode = ModeAggressive
		d.SeenLimit = 13.4
		dist := float32(40)
		for range 900 {
			scene := append(twoLanes(d.cam), at(d.cam, cl, dist, 0, 1, 12))
			d.Drive(scene, 10, 1.0/30)
			if d.move != moveNone {
				return true
			}
			if dist > 16 {
				dist -= 7.0 / 30
			}
		}
		return false
	}
	if !passes(vision.ClassVehicle) {
		t.Skip("the ordinary car in this scene is not passed, so the test would prove nothing")
	}
	if passes(vision.ClassAmbulance) {
		t.Error("began to overtake an ambulance")
	}
}
