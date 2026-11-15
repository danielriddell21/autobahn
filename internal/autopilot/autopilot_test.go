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
	// From the top of the image down: everything above the horizon ranges as
	// infinitely far, so the search walks down past it on its own.
	lo, hi := float32(0), float32(c.Height)-1
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

// rowForRange finds the image row at which something a given height above
// the road appears when it is a given distance away. For anything mounted above
// the lens that row is above the middle of the image, and rises as it nears.
func rowForRange(c vision.Camera, metres, height float32) float32 {
	lo, hi := float32(0), float32(c.Height)/2
	for range 60 {
		mid := (lo + hi) / 2
		if c.RangeAtHeight(mid, height) < metres {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// signalAt builds a signal head whose lower edge sits where the camera would
// see one that far off, which is how the autopilot ranges a signal.
func signalAt(c vision.Camera, cl vision.Class, metres, bearingPx float32) vision.Detection {
	row := rowForRange(c, metres, vision.SignalHeadHeight)
	return vision.Detection{
		Class: cl, MinX: bearingPx - 5, MaxX: bearingPx + 5,
		MinY: row - 16, MaxY: row, Pixels: 80,
	}
}

// signAt builds a sign whose top edge sits where the camera would see one that
// far off. top is the height of that edge above the road.
func signAt(c vision.Camera, cl vision.Class, metres, bearingPx, top float32) vision.Detection {
	row := rowForRange(c, metres, top)
	return vision.Detection{
		Class: cl, MinX: bearingPx - 6, MaxX: bearingPx + 6,
		MinY: row, MaxY: row + 12, Pixels: 70,
	}
}

// drive runs the controller for a while on one unchanging view.
func drive(d *autopilot.Driver, dets []vision.Detection, speed, seconds float32) autopilot.Command {
	var cmd autopilot.Command
	for t := float32(0); t < seconds; t += 1.0 / 60 {
		cmd = d.Drive(dets, speed, 1.0/60)
	}
	return cmd
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
		signalAt(c, vision.ClassLightRed, 26, float32(c.Width)/2+60),
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
		signalAt(c, vision.ClassLightRedAmber, 23, float32(c.Width)/2+60),
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
		signalAt(c, vision.ClassLightGreen, 26, float32(c.Width)/2+60),
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
		signAt(c, vision.ClassSpeed20, 18, float32(c.Width)/2+70, vision.SpeedSignTopHeight))
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
		signAt(c, vision.ClassSpeed40, 18, float32(c.Width)/2+70, vision.SpeedSignTopHeight)), 6, 1.0/60)
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
		signalAt(c, vision.ClassLightAmber, 15, float32(c.Width)/2+60),
		atRange(t, c, vision.ClassStopLine, 14, float32(c.Width)/2))

	if cmd := d.Drive(dets, 9, 1.0/60); cmd.Brake <= 0 {
		t.Errorf("did not stop for an amber 14m away at 9 m/s; brake = %v", cmd.Brake)
	}
}

// Right on top of the line there is no safe stop, so the car commits. The test
// speed stays under the assumed 30 mph limit, or the speed governor would brake
// for that instead and mask the behaviour being checked.
func TestCrossesAnAmberItCannotStopFor(t *testing.T) {
	c := cam()
	d := autopilot.New(c)

	dets := append(straightLane(t, c),
		signalAt(c, vision.ClassLightAmber, 3, float32(c.Width)/2+60),
		atRange(t, c, vision.ClassStopLine, 2, float32(c.Width)/2))

	if cmd := d.Drive(dets, 13, 1.0/60); cmd.Brake > 0.2 {
		t.Errorf("braked hard for an amber 2m away at 13 m/s; brake = %v", cmd.Brake)
	}
}

// A head a long way beyond the stop line belongs to the junction after this
// one. Its green says nothing about the line the car is actually coming up to.
func TestIgnoresASignalAtAnotherJunction(t *testing.T) {
	c := cam()
	line := atRange(t, c, vision.ClassStopLine, 16.4, float32(c.Width)/2)

	// A green at this junction is permission to carry on at speed.
	here := autopilot.New(c)
	here.Drive(append(straightLane(t, c), line,
		signalAt(c, vision.ClassLightGreen, 17, float32(c.Width)/2+60)), 13.4, 1.0/60)

	// The same green two junctions off is not.
	there := autopilot.New(c)
	there.Drive(append(straightLane(t, c), line,
		signalAt(c, vision.ClassLightGreen, 85, float32(c.Width)/2+20)), 13.4, 1.0/60)

	if there.TargetSpeed >= here.TargetSpeed {
		t.Errorf("target %.1f m/s with the green two junctions off, no lower than %.1f with it here",
			there.TargetSpeed, here.TargetSpeed)
	}
}

// Pulled up at the line the head overhead is out of the lens. Having stopped
// for a red, the car must not move off just because it can no longer see it.
func TestHoldsAtARedItCanNoLongerSee(t *testing.T) {
	c := cam()
	d := autopilot.New(c)

	seen := append(straightLane(t, c),
		signalAt(c, vision.ClassLightRed, 3.2, float32(c.Width)/2+150),
		atRange(t, c, vision.ClassStopLine, 2.2, float32(c.Width)/2))
	drive(d, seen, 0, 0.5)

	// Nothing but the lane, for a good deal longer than a signal is normally
	// remembered.
	cmd := drive(d, straightLane(t, c), 0, 8)
	if cmd.Throttle > 0 || d.TargetSpeed > 0.1 {
		t.Errorf("moved off from a red it had stopped for: throttle %.2f, target %.1f",
			cmd.Throttle, d.TargetSpeed)
	}

	// The far-side head turning green is what releases it.
	green := append(straightLane(t, c), signalAt(c, vision.ClassLightGreen, 28, float32(c.Width)/2-60))
	if cmd := drive(d, green, 0, 0.2); cmd.Throttle <= 0 {
		t.Error("did not move off when the far-side head went green")
	}
}

// Turning through a junction swings the camera across heads that govern the
// traffic still waiting. A car already past its line does not stop for them.
func TestIgnoresSignalsOnceAcrossTheLine(t *testing.T) {
	c := cam()
	d := autopilot.New(c)

	// Approach a line under a green, and drive over it.
	for dist := float32(12); dist > 2; dist -= 8.0 / 60 {
		d.Drive(append(straightLane(t, c),
			signalAt(c, vision.ClassLightGreen, dist+1, float32(c.Width)/2+80),
			atRange(t, c, vision.ClassStopLine, dist, float32(c.Width)/2)), 8, 1.0/60)
	}
	drive(d, straightLane(t, c), 8, 0.6)

	// A red, close, with no line of its own: the cross street's.
	red := append(straightLane(t, c), signalAt(c, vision.ClassLightRed, 9, float32(c.Width)/2+90))
	if cmd := d.Drive(red, 8, 1.0/60); cmd.Brake > 0.05 {
		t.Errorf("braked in the middle of a junction for a signal it had already passed; brake = %v", cmd.Brake)
	}
}

func TestSlowsForABendAhead(t *testing.T) {
	c := cam()

	straight := autopilot.New(c)
	straight.Drive(straightLane(t, c), 13, 1.0/60)

	// The same markers, swinging off to the right from the third one on.
	bent := autopilot.New(c)
	dets := straightLane(t, c)
	for i := 2; i < len(dets); i++ {
		shift := float32(i-1) * 26
		dets[i].MinX += shift
		dets[i].MaxX += shift
	}
	bent.Drive(dets, 13, 1.0/60)

	if bent.TargetSpeed >= straight.TargetSpeed {
		t.Errorf("target %.1f m/s into a bend, no lower than %.1f on the straight",
			bent.TargetSpeed, straight.TargetSpeed)
	}
}

// Creeping blind is only worth it for a moment. If the lane does not come
// back the car stops, rather than carrying on into whatever is there.
func TestStopsIfTheLaneStaysLost(t *testing.T) {
	c := cam()
	d := autopilot.New(c)
	drive(d, nil, 3, 5)
	if d.TargetSpeed != 0 {
		t.Errorf("still aiming for %.1f m/s after five seconds with no lane", d.TargetSpeed)
	}
}

// A queue in the next lane runs together into one blob that reaches right
// across the image. Only its base says where the nearest car in it stands.
func TestAQueueBesideTheLaneIsNotInTheWay(t *testing.T) {
	c := cam()
	d := autopilot.New(c)

	row := rowForDistance(c, 6)
	queue := vision.Detection{
		Class: vision.ClassVehicle,
		// The blob runs from the vanishing point out to the edge of the frame…
		MinX: float32(c.Width)/2 + 8, MaxX: float32(c.Width) - 1,
		MinY: row - 60, MaxY: row,
		// …but the car at the front of it is well off to the side.
		BaseMinX: float32(c.Width) - 110, BaseMaxX: float32(c.Width) - 1,
		Pixels: 400,
	}
	if cmd := d.Drive(append(straightLane(t, c), queue), 8, 1.0/60); cmd.Brake > 0.01 {
		t.Errorf("braked for a queue in the next lane; brake = %v", cmd.Brake)
	}
}

func TestGivesWayToCrossingTraffic(t *testing.T) {
	c := cam()
	centre := float32(c.Width) / 2
	junction := append(straightLane(t, c),
		signAt(c, vision.ClassGiveWay, 3, centre-150, vision.SignTopHeight),
		atRange(t, c, vision.ClassStopLine, 3, centre))

	clear := autopilot.New(c)
	if cmd := clear.Drive(junction, 3, 1.0/60); cmd.Brake > 0.01 {
		t.Errorf("braked at an empty give way; brake = %v", cmd.Brake)
	}

	// The same junction with a car coming across it.
	busy := autopilot.New(c)
	crossing := atRange(t, c, vision.ClassVehicle, 18, centre+125)
	crossing.MinX, crossing.MaxX = centre+100, centre+150
	if cmd := busy.Drive(append(junction, crossing), 3, 1.0/60); cmd.Brake <= 0 {
		t.Errorf("did not give way to crossing traffic; brake = %v", cmd.Brake)
	}
}

// A stop sign wants a real halt at the line, then lets the car go.
func TestHaltsAtAStopSignThenGoes(t *testing.T) {
	c := cam()
	centre := float32(c.Width) / 2
	d := autopilot.New(c)

	junction := append(straightLane(t, c),
		signAt(c, vision.ClassStopSign, 3, centre-170, vision.SignTopHeight),
		atRange(t, c, vision.ClassStopLine, 2.6, centre))

	if cmd := drive(d, junction, 2, 0.1); cmd.Brake <= 0 {
		t.Fatalf("rolling toward a stop line without braking; brake = %v", cmd.Brake)
	}
	// Standing at the line: held for the pause, then released.
	drive(d, junction, 0, 0.5)
	if d.State != autopilot.StateWaiting {
		t.Errorf("state = %v half a second into the halt, want waiting", d.State)
	}
	if cmd := drive(d, junction, 0, 1.5); cmd.Throttle <= 0 {
		t.Errorf("still held two seconds after stopping; state %v", d.State)
	}
}

// A speed sign across a park belongs to somebody else's street.
func TestIgnoresASpeedSignOnAnotherStreet(t *testing.T) {
	c := cam()
	d := autopilot.New(c)
	before := d.SeenLimit

	far := signAt(c, vision.ClassSpeed20, 80, float32(c.Width)/2+60, vision.SpeedSignTopHeight)
	d.Drive(append(straightLane(t, c), far), 8, 1.0/60)
	if d.SeenLimit != before {
		t.Errorf("took a limit of %.1f m/s from a sign 80m away", d.SeenLimit)
	}
}
