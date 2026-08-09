package vision_test

import (
	"image/color"
	"testing"

	"github.com/danielriddell21/autobahn/internal/vision"
)

// paintRect draws a hollow rectangle of a class's colour into a pixel buffer,
// the same way the annotator draws one into the camera image.
func paintRect(px []color.RGBA, w int, x0, y0, x1, y1 int, cl vision.Class) {
	c := vision.RGBA(cl)
	for x := x0; x <= x1; x++ {
		px[y0*w+x] = c
		px[y1*w+x] = c
	}
	for y := y0; y <= y1; y++ {
		px[y*w+x0] = c
		px[y*w+x1] = c
	}
}

func blank(w, h int) []color.RGBA {
	px := make([]color.RGBA, w*h)
	// A muted background, of the kind the world palette actually uses.
	for i := range px {
		px[i] = color.RGBA{R: 58, G: 58, B: 63, A: 255}
	}
	return px
}

func TestScanRecoversBoxGeometry(t *testing.T) {
	const w, h = 160, 120
	px := blank(w, h)
	paintRect(px, w, 20, 30, 60, 70, vision.ClassVehicle)

	got := vision.NewScanner().Scan(px, w, h)
	if len(got) != 1 {
		t.Fatalf("got %d detections, want 1", len(got))
	}
	d := got[0]
	if d.Class != vision.ClassVehicle {
		t.Errorf("class = %v, want ClassVehicle", d.Class)
	}
	if d.MinX != 20 || d.MinY != 30 || d.MaxX != 60 || d.MaxY != 70 {
		t.Errorf("box = (%v,%v)-(%v,%v), want (20,30)-(60,70)",
			d.MinX, d.MinY, d.MaxX, d.MaxY)
	}
}

func TestScanSeparatesClasses(t *testing.T) {
	const w, h = 200, 150
	px := blank(w, h)
	paintRect(px, w, 10, 10, 40, 40, vision.ClassVehicle)
	paintRect(px, w, 60, 10, 90, 40, vision.ClassLightRed)
	paintRect(px, w, 110, 10, 140, 40, vision.ClassGiveWay)

	counts := map[vision.Class]int{}
	for _, d := range vision.NewScanner().Scan(px, w, h) {
		counts[d.Class]++
	}
	for _, cl := range []vision.Class{
		vision.ClassVehicle, vision.ClassLightRed, vision.ClassGiveWay,
	} {
		if counts[cl] != 1 {
			t.Errorf("class %v: got %d detections, want 1", cl, counts[cl])
		}
	}
}

// Touching boxes of the same class necessarily merge under a flood fill, which
// is exactly why consecutive lane markers alternate between two colours.
func TestAlternatingLaneClassesStaySeparable(t *testing.T) {
	const w, h = 120, 80
	px := blank(w, h)
	paintRect(px, w, 10, 10, 30, 30, vision.ClassLaneMarker)
	paintRect(px, w, 30, 10, 50, 30, vision.ClassLaneMarkerAlt)

	var lanes int
	for _, d := range vision.NewScanner().Scan(px, w, h) {
		if vision.IsLaneMarker(d.Class) {
			lanes++
		}
	}
	if lanes != 2 {
		t.Errorf("got %d lane detections from two touching markers, want 2", lanes)
	}
}

func TestScanIgnoresWorldColours(t *testing.T) {
	const w, h = 80, 60
	px := blank(w, h)
	// Every colour the world is painted in is muted, never a saturated
	// combination of 0, 128 and 255.
	for i := range px {
		px[i] = color.RGBA{R: 216, G: 214, B: 206, A: 255}
	}
	if got := vision.NewScanner().Scan(px, w, h); len(got) != 0 {
		t.Errorf("got %d detections from a scene with no annotations, want 0", len(got))
	}
}

func TestScanRejectsSpecks(t *testing.T) {
	const w, h = 60, 40
	px := blank(w, h)
	px[10*w+10] = vision.RGBA(vision.ClassVehicle)

	if got := vision.NewScanner().Scan(px, w, h); len(got) != 0 {
		t.Errorf("got %d detections from a single stray pixel, want 0", len(got))
	}
}

func TestScannerReuseIsStable(t *testing.T) {
	const w, h = 100, 80
	px := blank(w, h)
	paintRect(px, w, 10, 10, 40, 40, vision.ClassStopSign)

	s := vision.NewScanner()
	first := len(s.Scan(px, w, h))
	for range 5 {
		if got := len(s.Scan(px, w, h)); got != first {
			t.Fatalf("repeat scan returned %d detections, want %d", got, first)
		}
	}
}

func TestGroundDistanceFallsWithImageRow(t *testing.T) {
	cam := vision.DefaultCamera(420, 236)
	// Rows further down the image are closer to the car.
	near := cam.GroundDistance(220)
	far := cam.GroundDistance(140)
	if !(near > 0 && near < far) {
		t.Errorf("near row = %.1fm, far row = %.1fm; want near closer and positive", near, far)
	}
	// At the horizon the ground cannot be intersected at all.
	if cam.GroundDistance(0) < 1000 {
		t.Errorf("horizon row = %.1fm, want an effectively infinite range",
			cam.GroundDistance(0))
	}
}

func TestBearingSignFollowsColumn(t *testing.T) {
	cam := vision.DefaultCamera(420, 236)
	if cam.Bearing(410) <= 0 {
		t.Error("a column right of centre should give a positive bearing")
	}
	if cam.Bearing(10) >= 0 {
		t.Error("a column left of centre should give a negative bearing")
	}
	if b := cam.Bearing(210); b != 0 {
		t.Errorf("centre column bearing = %v, want 0", b)
	}
}

func TestSpeedClassesCarryTheirLimit(t *testing.T) {
	for _, tc := range []struct {
		class vision.Class
		mph   float32
	}{
		{vision.ClassSpeed20, 20},
		{vision.ClassSpeed30, 30},
		{vision.ClassSpeed40, 40},
	} {
		v, ok := tc.class.SpeedValue()
		if !ok {
			t.Errorf("%v is not reported as a speed sign", tc.class)
			continue
		}
		if want := tc.mph * 0.44704; v < want-0.01 || v > want+0.01 {
			t.Errorf("%v = %v m/s, want %v", tc.class, v, want)
		}
	}
	if _, ok := vision.ClassVehicle.SpeedValue(); ok {
		t.Error("a vehicle should not be reported as a speed sign")
	}
}

func TestSignalClassesThatStopTraffic(t *testing.T) {
	// Red and amber together still forbids crossing the line.
	for _, cl := range []vision.Class{
		vision.ClassLightRed, vision.ClassLightRedAmber, vision.ClassLightAmber,
	} {
		if !cl.StopsTraffic() {
			t.Errorf("%v should stop traffic", cl)
		}
	}
	if vision.ClassLightGreen.StopsTraffic() {
		t.Error("green should not stop traffic")
	}
}

// Distinct colours per class are what makes the scanner work at all.
func TestClassColoursAreDistinct(t *testing.T) {
	seen := map[color.RGBA]vision.Class{}
	for cl := vision.ClassVehicle; cl <= vision.ClassStopLine; cl++ {
		c := vision.RGBA(cl)
		if prev, dup := seen[c]; dup {
			t.Errorf("%v and %v share the colour %v", prev, cl, c)
		}
		seen[c] = cl
	}
}

func TestRangeAtHeightMatchesGroundForRoadLevelPoints(t *testing.T) {
	cam := vision.DefaultCamera(420, 236)
	for _, row := range []float32{140, 170, 200, 225} {
		ground := cam.GroundDistance(row)
		atZero := cam.RangeAtHeight(row, 0)
		if ground > 1e4 {
			continue
		}
		if d := ground - atZero; d > 0.01 || d < -0.01 {
			t.Errorf("row %v: GroundDistance = %v but RangeAtHeight(0) = %v", row, ground, atZero)
		}
	}
}

// A traffic light is mounted above the camera, so it appears above the optical
// axis and must still yield a sensible forward range.
func TestRangeAtHeightRangesAnOverheadSignal(t *testing.T) {
	cam := vision.DefaultCamera(420, 236)
	// Rows well above centre, which is where a mounted signal lands. Because the
	// camera is pitched down, only rows above the horizon can reach that height.
	near := cam.RangeAtHeight(70, vision.SignalHeadHeight)
	far := cam.RangeAtHeight(90, vision.SignalHeadHeight)
	if near <= 0 || near > 1e4 {
		t.Fatalf("signal range at row 70 = %v, want a finite positive distance", near)
	}
	if far <= 0 || far > 1e4 {
		t.Fatalf("signal range at row 90 = %v, want a finite positive distance", far)
	}
	// Higher in the image means nearer for an overhead object, the inverse of
	// how a point on the road behaves.
	if !(near < far) {
		t.Errorf("row 70 gave %v and row 90 gave %v; the higher box should be nearer", near, far)
	}
}

// A ray pitched downward never climbs to signal height, so rows below the
// horizon must report no range rather than a bogus one.
func TestRangeAtHeightRejectsRaysThatNeverReachIt(t *testing.T) {
	cam := vision.DefaultCamera(420, 236)
	if got := cam.RangeAtHeight(200, vision.SignalHeadHeight); got < 1000 {
		t.Errorf("a downward ray reported a signal at %v m; want no range", got)
	}
}
