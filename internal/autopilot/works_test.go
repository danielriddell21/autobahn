package autopilot

import (
	"math"
	"testing"

	"github.com/danielriddell21/autobahn/internal/vision"
)

// Synthetic scenes, built straight from the camera's own calibration: where a
// thing standing the given distance ahead and to the side appears in the image.

// rowFor is the image row the foot of something dist metres ahead lands on.
func rowFor(cam vision.Camera, dist float32) float32 {
	down := math.Atan(float64(cam.Mount / dist))
	focal := float64(cam.Height) / 2 / math.Tan(float64(cam.FovY)*0.5*math.Pi/180)
	return float32(float64(cam.Height)/2 + focal*math.Tan(down-float64(cam.Pitch)))
}

// colFor is the image column of something lateral metres right of the road ahead.
func colFor(cam vision.Camera, dist, lateral float32) float32 {
	focal := float64(cam.Height) / 2 / math.Tan(float64(cam.FovY)*0.5*math.Pi/180)
	return float32(float64(cam.Width)/2 + focal*float64(lateral/dist))
}

// at is a box of a class standing on the road dist ahead and lateral across,
// halfW metres wide each way and tall pixels high.
func at(cam vision.Camera, cl vision.Class, dist, lateral, halfW, tall float32) vision.Detection {
	lo, hi := colFor(cam, dist, lateral-halfW), colFor(cam, dist, lateral+halfW)
	y := rowFor(cam, dist)
	return vision.Detection{
		Class: cl, MinX: lo, MaxX: hi, MinY: y - tall, MaxY: y,
		BaseMinX: lo, BaseMaxX: hi, Pixels: 100,
	}
}

// signAt is a sign whose top edge is at works height.
func signAt(cam vision.Camera, cl vision.Class, dist, lateral float32) vision.Detection {
	focal := float64(cam.Height) / 2 / math.Tan(float64(cam.FovY)*0.5*math.Pi/180)
	down := math.Atan(float64(cam.Mount-vision.WorksSignTopHeight) / float64(dist))
	top := float32(float64(cam.Height)/2 + focal*math.Tan(down-float64(cam.Pitch)))
	col := colFor(cam, dist, lateral)
	return vision.Detection{Class: cl, MinX: col - 10, MaxX: col + 10, MinY: top, MaxY: top + 30, Pixels: 100}
}

// twoLanes is the markers of the car's own lane going straight on, and the lane
// alongside it to the right, marked as far as the camera would show it.
func twoLanes(cam vision.Camera) []vision.Detection {
	var out []vision.Detection
	for i, d := 0, float32(5); d <= 50; i, d = i+1, d+5 {
		cl := vision.ClassLaneMarker
		if i%2 == 1 {
			cl = vision.ClassLaneMarkerAlt
		}
		out = append(out, at(cam, cl, d, 0, 0.3, 4))
	}
	for d := float32(8); d <= 40; d += 8 {
		out = append(out, at(cam, vision.ClassLaneSide, d, laneWidth, 0.5, 5))
	}
	return out
}

func TestSignAndBoxGeometryRoundTrips(t *testing.T) {
	d := driver()
	for _, dist := range []float32{12, 25, 40} {
		det := at(d.cam, vision.ClassCone, dist, 0, 0.4, 8)
		if got := d.cam.GroundDistance(det.MaxY); abs(got-dist) > 0.5 {
			t.Errorf("cone at %.0f m ranged as %.1f", dist, got)
		}
		s := signAt(d.cam, vision.ClassDiversion, dist, 3)
		if got := d.cam.RangeAtHeight(s.MinY, vision.WorksSignTopHeight); abs(got-dist) > 0.5 {
			t.Errorf("sign at %.0f m ranged as %.1f", dist, got)
		}
	}
}

// run feeds one scene to a driver for the given time and returns the driver.
func run(d *Driver, scene []vision.Detection, speed, seconds float32) Command {
	var cmd Command
	for range int(seconds * 30) {
		cmd = d.Drive(scene, speed, 1.0/30)
	}
	return cmd
}

func TestAConeInTheLaneIsHeldUpBehindAndPassed(t *testing.T) {
	d := driver()
	scene := append(twoLanes(d.cam), at(d.cam, vision.ClassCone, 14, 0, 0.4, 10))
	// Stood still behind it: below the speed anyone changes lane at in a queue.
	run(d, scene, 0, 4)
	if d.move == moveNone {
		t.Fatalf("stopped at a cone with a free lane alongside, and not moving over (state %v, %s)", d.State, d.Reason)
	}
	if d.shiftWant <= 0 && d.move != moveSignal {
		t.Errorf("moving the wrong way: want %.1f", d.shiftWant)
	}
}

func TestABarrierIsAnObstacleToo(t *testing.T) {
	d := driver()
	scene := append(twoLanes(d.cam), at(d.cam, vision.ClassBarrier, 14, 0, 1.8, 12))
	cmd := run(d, scene, 6, 3)
	if cmd.Brake <= 0 {
		t.Errorf("a barrier 14 m ahead at 6 m/s and the car is not braking: %+v", cmd)
	}
}

func TestACarInAQueueIsNotMovedOverForBelowMoveSpeed(t *testing.T) {
	// The ordinary rule stands for vehicles: in a crawl the gaps alongside open
	// and shut faster than the mirrors can be trusted.
	d := driver()
	scene := append(twoLanes(d.cam), at(d.cam, vision.ClassVehicle, 14, 0, 1, 12))
	run(d, scene, 0, 12)
	if d.move != moveNone {
		t.Errorf("moved over from a standstill behind a car (move %d)", d.move)
	}
}

func TestMovingOverWaitsForTheMirrors(t *testing.T) {
	d := driver()
	d.around[vision.MirrorRear] = []Neighbour{{Ahead: -6, Right: laneWidth}}
	scene := append(twoLanes(d.cam), at(d.cam, vision.ClassCone, 14, 0, 0.4, 10))
	run(d, scene, 0, 3)
	if d.move == moveShift {
		t.Error("moved across into a lane the mirror shows a car in")
	}
	// Cleared: now it goes.
	d.around[vision.MirrorRear] = nil
	run(d, scene, 0, 3)
	if d.move != moveShift && d.move != moveSignal {
		t.Errorf("the lane is clear and the car is still sat at the cone (move %d)", d.move)
	}
}

func TestADiversionSignSlowsTheCarForAWhile(t *testing.T) {
	lane := twoLanes(driver().cam)
	ref := driver()
	ref.SeenLimit = 13.4
	run(ref, lane, 8, 1)

	d := driver()
	d.SeenLimit = 13.4
	scene := append(append([]vision.Detection{}, lane...), signAt(d.cam, vision.ClassDiversion, 30, 3))
	run(d, scene, 8, 1)
	if d.TargetSpeed > ref.TargetSpeed*0.85 {
		t.Errorf("target %.1f after a diversion sign, %.1f without: want 20%% off", d.TargetSpeed, ref.TargetSpeed)
	}
	// 150 m on, the warning has run out.
	d.odo += diversionRun + 1
	run(d, lane, 8, 1)
	if abs(d.TargetSpeed-ref.TargetSpeed) > 0.2 {
		t.Errorf("target %.1f long after the sign, want %.1f again", d.TargetSpeed, ref.TargetSpeed)
	}
}

func TestARoadClosedSignAheadOfTheMarkersStopsTheCar(t *testing.T) {
	d := driver()
	d.SeenLimit = 13.4
	scene := append(twoLanes(d.cam), signAt(d.cam, vision.ClassRoadClosed, 15, 3))
	run(d, scene, 8, 2)
	// Stopping 8 m short of a sign 15 m off leaves 7 m to stop in.
	if d.TargetSpeed > 7 {
		t.Errorf("target speed %.1f towards a closed road with the markers running on through it", d.TargetSpeed)
	}
	// With the markers stopping short of the sign, the car is not asked to.
	short := []vision.Detection{}
	for _, det := range twoLanes(d.cam) {
		if det.MaxY > rowFor(d.cam, 18) {
			short = append(short, det)
		}
	}
	short = append(short, signAt(d.cam, vision.ClassRoadClosed, 40, 3))
	e := driver()
	e.SeenLimit = 13.4
	run(e, short, 8, 2)
	if e.TargetSpeed < 3 {
		t.Errorf("stopped for a closed sign the route does not lead to (target %.1f)", e.TargetSpeed)
	}
}
