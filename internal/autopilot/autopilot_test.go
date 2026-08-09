package autopilot_test

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/vision"
)

const (
	camW = 420
	camH = 236
)

func cam() vision.Camera { return vision.DefaultCamera(camW, camH) }

// atRange builds a detection whose bottom edge sits at the image row matching a
// given ground distance, which is how the camera model recovers range.
func atRange(t *testing.T, c vision.Camera, cl vision.Class, metres, bearingPx float32) vision.Detection {
	t.Helper()
	row := rowForDistance(c, metres)
	return vision.Detection{
		Class: cl,
		MinX:  bearingPx - 8, MaxX: bearingPx + 8,
		MinY: row - 10, MaxY: row,
		Pixels: 60,
	}
}

func rowForDistance(c vision.Camera, metres float32) float32 {
	// Binary search the row whose ground distance matches, so the test depends
	// only on the camera's public behaviour.
	lo, hi := float32(c.Height)/2+0.5, float32(c.Height)-1
	for range 60 {
		mid := (lo + hi) / 2
		if c.GroundDistance(mid) > metres {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// straightLane returns markers running away down the centre of the image.
func straightLane(t *testing.T, c vision.Camera) []vision.Detection {
	t.Helper()
	var out []vision.Detection
	centre := float32(c.Width) / 2
	for i, d := range []float32{6, 12, 19, 26, 33, 40} {
		cl := vision.ClassLaneMarker
		if i%2 == 1 {
			cl = vision.ClassLaneMarkerAlt
		}
		out = append(out, atRange(t, c, cl, d, centre))
	}
	return out
}

func TestCruisesOnAClearRoad(t *testing.T) {
	c := cam()
	d := autopilot.New(c)
	cmd := d.Drive(straightLane(t, c), 8, 1.0/60)

	if cmd.Brake > 0.01 {
		t.Errorf("braked on a clear road: %v", cmd.Brake)
	}
	if cmd.Throttle <= 0 {
		t.Error("no throttle applied on a clear road")
	}
	if d.State != autopilot.StateCruise {
		t.Errorf("state = %v, want cruising", d.State)
	}
}

func TestStopsForARedLight(t *testing.T) {
	c := cam()
	d := autopilot.New(c)

	dets := straightLane(t, c)
	dets = append(dets,
		atRange(t, c, vision.ClassLightRed, 30, float32(c.Width)/2+60),
		atRange(t, c, vision.ClassStopLine, 25, float32(c.Width)/2),
	)

	cmd := d.Drive(dets, 12, 1.0/60)
	if cmd.Brake <= 0 {
		t.Errorf("did not brake for a red light 25m away; brake = %v", cmd.Brake)
	}
	if cmd.Throttle > 0 {
		t.Errorf("applied throttle while stopping for a red light: %v", cmd.Throttle)
	}
}

// Red and amber together is not permission to go.
func TestStopsForRedAndAmber(t *testing.T) {
	c := cam()
	d := autopilot.New(c)

	dets := straightLane(t, c)
	dets = append(dets,
		atRange(t, c, vision.ClassLightRedAmber, 30, float32(c.Width)/2+60),
		atRange(t, c, vision.ClassStopLine, 22, float32(c.Width)/2),
	)
	if cmd := d.Drive(dets, 11, 1.0/60); cmd.Brake <= 0 {
		t.Errorf("did not brake for red and amber; brake = %v", cmd.Brake)
	}
}

func TestDoesNotStopForAGreenLight(t *testing.T) {
	c := cam()
	d := autopilot.New(c)

	dets := straightLane(t, c)
	dets = append(dets,
		atRange(t, c, vision.ClassLightGreen, 30, float32(c.Width)/2+60),
		atRange(t, c, vision.ClassStopLine, 25, float32(c.Width)/2),
	)
	if cmd := d.Drive(dets, 10, 1.0/60); cmd.Brake > 0.01 {
		t.Errorf("braked at a green light; brake = %v", cmd.Brake)
	}
}

func TestSlowsForTheCarInFront(t *testing.T) {
	c := cam()
	d := autopilot.New(c)

	dets := append(straightLane(t, c),
		atRange(t, c, vision.ClassVehicle, 9, float32(c.Width)/2))

	cmd := d.Drive(dets, 14, 1.0/60)
	if cmd.Brake <= 0 {
		t.Errorf("did not slow for a car 9m ahead; brake = %v", cmd.Brake)
	}
	if d.State != autopilot.StateFollow && d.State != autopilot.StateSlowing {
		t.Errorf("state = %v, want following or slowing", d.State)
	}
}

// A car in the next lane is not a reason to brake.
func TestIgnoresTrafficOutsideItsLane(t *testing.T) {
	c := cam()
	d := autopilot.New(c)

	// 9m ahead but displaced well to the side.
	side := rowForDistance(c, 9)
	dets := append(straightLane(t, c), vision.Detection{
		Class: vision.ClassVehicle,
		MinX:  float32(c.Width) - 20, MaxX: float32(c.Width) - 2,
		MinY: side - 12, MaxY: side, Pixels: 80,
	})

	if cmd := d.Drive(dets, 12, 1.0/60); cmd.Brake > 0.01 {
		t.Errorf("braked for a car in an adjacent lane; brake = %v", cmd.Brake)
	}
}

func TestSteersTowardTheLane(t *testing.T) {
	c := cam()
	centre := float32(c.Width) / 2

	// Markers offset to the right of the image mean the lane runs right.
	right := autopilot.New(c)
	var dets []vision.Detection
	for i, m := range []float32{6, 12, 19, 26, 33} {
		cl := vision.ClassLaneMarker
		if i%2 == 1 {
			cl = vision.ClassLaneMarkerAlt
		}
		dets = append(dets, atRange(t, c, cl, m, centre+70))
	}
	// Several steps, because the steering command is smoothed.
	var cmd autopilot.Command
	for range 30 {
		cmd = right.Drive(dets, 8, 1.0/60)
	}
	if cmd.Steer <= 0 {
		t.Errorf("steer = %v, want a positive (rightward) command", cmd.Steer)
	}

	left := autopilot.New(c)
	for i := range dets {
		dets[i].MinX -= 140
		dets[i].MaxX -= 140
	}
	for range 30 {
		cmd = left.Drive(dets, 8, 1.0/60)
	}
	if cmd.Steer >= 0 {
		t.Errorf("steer = %v, want a negative (leftward) command", cmd.Steer)
	}
}

func TestReadsTheSpeedLimitFromASign(t *testing.T) {
	c := cam()
	d := autopilot.New(c)

	dets := append(straightLane(t, c),
		atRange(t, c, vision.ClassSpeed20, 18, float32(c.Width)/2+70))
	d.Drive(dets, 6, 1.0/60)

	want, _ := vision.ClassSpeed20.SpeedValue()
	if d.SeenLimit != want {
		t.Errorf("SeenLimit = %v, want %v", d.SeenLimit, want)
	}

	// The limit persists once the sign has passed out of view.
	d.Drive(straightLane(t, c), 6, 1.0/60)
	if d.SeenLimit != want {
		t.Errorf("limit forgotten after the sign left view: %v", d.SeenLimit)
	}
}

func TestCrawlsWhenItCannotSeeTheLane(t *testing.T) {
	c := cam()
	d := autopilot.New(c)

	// No lane markers at all: the car must not press on at speed.
	var cmd autopilot.Command
	for range 40 {
		cmd = d.Drive(nil, 14, 1.0/60)
	}
	if d.State != autopilot.StateSearching {
		t.Errorf("state = %v, want searching", d.State)
	}
	if cmd.Brake <= 0 {
		t.Errorf("did not slow with no lane in view; brake = %v", cmd.Brake)
	}
}

func TestResetClearsLearnedState(t *testing.T) {
	c := cam()
	d := autopilot.New(c)

	d.Drive(append(straightLane(t, c),
		atRange(t, c, vision.ClassSpeed40, 18, float32(c.Width)/2+70)), 6, 1.0/60)
	d.Reset()

	def, _ := vision.ClassSpeed30.SpeedValue()
	if d.SeenLimit != def {
		t.Errorf("SeenLimit after reset = %v, want the %v default", d.SeenLimit, def)
	}
}

// The stop line is routinely hidden by the car waiting on it, so a red light
// must still be obeyed when only the signal head is visible.
func TestStopsForARedLightWithNoStopLineVisible(t *testing.T) {
	c := cam()
	d := autopilot.New(c)

	// A signal head well above the optical axis, and no stop line at all. For an
	// object mounted above the camera, a higher box means a nearer signal.
	dets := append(straightLane(t, c), vision.Detection{
		Class: vision.ClassLightRed,
		MinX:  float32(c.Width)/2 + 50, MaxX: float32(c.Width)/2 + 66,
		MinY: 66, MaxY: 88, Pixels: 90,
	})

	cmd := d.Drive(dets, 12, 1.0/60)
	if cmd.Brake <= 0 {
		t.Errorf("did not brake for a red light with the stop line occluded; brake = %v", cmd.Brake)
	}
}

// Amber means stop unless stopping would be unsafe, so a car with room to
// pull up must not carry on through.
func TestStopsForAnAmberItCanStopFor(t *testing.T) {
	c := cam()
	d := autopilot.New(c)

	dets := append(straightLane(t, c),
		atRange(t, c, vision.ClassLightAmber, 40, float32(c.Width)/2+60),
		atRange(t, c, vision.ClassStopLine, 38, float32(c.Width)/2))

	if cmd := d.Drive(dets, 9, 1.0/60); cmd.Brake <= 0 {
		t.Errorf("did not stop for an amber 38m away at 9 m/s; brake = %v", cmd.Brake)
	}
}

// Right on top of the line there is no safe stop, so the car commits. The test
// speed stays under the assumed 30 mph limit, or the speed governor would brake
// for that instead and mask the behaviour being checked.
func TestCrossesAnAmberItCannotStopFor(t *testing.T) {
	c := cam()
	d := autopilot.New(c)

	dets := append(straightLane(t, c),
		atRange(t, c, vision.ClassLightAmber, 3, float32(c.Width)/2+60),
		atRange(t, c, vision.ClassStopLine, 2, float32(c.Width)/2))

	if cmd := d.Drive(dets, 13, 1.0/60); cmd.Brake > 0.2 {
		t.Errorf("braked hard for an amber 2m away at 13 m/s; brake = %v", cmd.Brake)
	}
}
