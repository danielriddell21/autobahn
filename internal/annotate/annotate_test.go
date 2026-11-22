package annotate_test

import (
	"image/color"
	"testing"

	"github.com/danielriddell21/autobahn/internal/annotate"
	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/city"
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
		return annotate.BonnetView(cam, w.City.StandAt(w.Player.Pos, w.Player.Yaw))
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
	level := city.NewGrowing(city.ParamsFor(city.MapCity, 7))
	v := annotate.BonnetView(cam, level.StandAt(pos, 0)) // facing +X

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

// The autopilot is told how high the edges of a signal's and a sign's box
// stand, and ranges them from that. If the boxes move and the calibration does
// not, every signal is ranged wrong and nothing else will say so.
func TestBoxHeightsMatchTheCameraCalibration(t *testing.T) {
	w := world(t)
	seen := map[city.PropKind]bool{}
	for _, p := range w.City.Props {
		if seen[p.Kind] {
			continue
		}
		seen[p.Kind] = true
		var got, want float32
		switch p.Kind {
		case city.PropTrafficLight:
			got, want = p.Height-annotate.SignalDrop-annotate.SignalHalf, vision.SignalHeadHeight
		case city.PropStopSign, city.PropGiveWaySign:
			got, want = p.Height-annotate.SignDrop+annotate.SignHalf, vision.SignTopHeight
		case city.PropSpeedSign:
			got, want = p.Height-annotate.SpeedSignDrop+annotate.SpeedSignHalf, vision.SpeedSignTopHeight
		default:
			continue
		}
		if mathx.Abs(got-want) > 0.001 {
			t.Errorf("prop kind %d: box edge at %.3fm, calibrated as %.3fm", p.Kind, got, want)
		}
	}
	if !seen[city.PropTrafficLight] || !seen[city.PropSpeedSign] {
		t.Fatal("this city has no signals or speed signs to check")
	}
}

// A signal governing the cross street faces another way, and a driver who
// obeyed it would stop for somebody else's red.
func TestOnlyPropsFacingTheCarAreAnnotated(t *testing.T) {
	w := world(t)
	_, view := bonnet()
	// Stand on every signalled approach in turn, well back from the line.
	checked := 0
	for _, l := range w.City.Lanes {
		if l.Control != city.ControlSignal || l.Length < 60 || l.Index != 0 {
			continue
		}
		w.Player.Place(l.Point(l.Length-30), l.Heading)
		w.Route.Update(w.Player.Pos, w.Player.Forward())
		want := map[vision.Class]bool{}
		switch w.Signals.LaneState(l) {
		case sim.SignalGreen:
			want[vision.ClassLightGreen] = true
		case sim.SignalAmber:
			want[vision.ClassLightAmber] = true
		case sim.SignalRedAmber:
			want[vision.ClassLightRedAmber] = true
		default:
			want[vision.ClassLightRed] = true
		}
		lights := 0
		for _, b := range annotate.Layout(w, view(w)) {
			switch b.Class {
			case vision.ClassLightRed, vision.ClassLightRedAmber, vision.ClassLightAmber, vision.ClassLightGreen:
				// Heads beyond this junction may show anything; the two
				// nearest the car are its own.
				if lights++; lights <= 2 && !want[b.Class] {
					t.Fatalf("lane %d: a %v head is annotated, but this approach shows %v",
						l.ID, b.Class, w.Signals.LaneState(l))
				}
			}
		}
		if lights < 2 {
			t.Fatalf("lane %d: %d signal heads in view from 30m back, want the primary and the far-side one", l.ID, lights)
		}
		if checked++; checked >= 12 {
			break
		}
	}
	if checked == 0 {
		t.Fatal("no signalled approach long enough to stand on")
	}
}

// put parks an agent at a point beside the player, measured in the player's
// own frame, pointing the same way.
func put(w *sim.World, a *sim.Agent, ahead, right float32) {
	p := w.Player
	at := p.Pos.Add(p.Forward().Mul(ahead)).Add(p.Forward().Right().Mul(right))
	a.V.Place(at, p.Yaw)
}

// A car behind and to one side has to come out of the mirrors where it is, or
// the autopilot would change lane into it.
func TestMirrorsPlaceTheCarsAroundUs(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 3})
	w.Agents = w.Agents[:3]
	for _, a := range w.Agents {
		a.V.Place(mathx.V(1e4, 1e4), 0) // out of sight
	}
	type spot struct{ ahead, right float32 }
	cases := []spot{{-14, 0}, {-9, 3.6}, {-1, -3.6}, {-30, -3.6}}
	for _, at := range cases {
		put(w, w.Agents[0], at.ahead, at.right)
		d := autopilot.New(vision.DefaultCamera(420, 236))
		mirrors := d.Mirrors()
		for m := range mirrors {
			eye := annotate.NewMirrorEye(mirrors[m].Width, mirrors[m].Height)
			d.Watch(vision.Mirror(m), eye.Look(w, mirrors[m], w.City.StandAt(w.Player.Pos, w.Player.Yaw), w.Weather()))
		}
		found := false
		for _, n := range d.Around() {
			// The nearest point of the car, so up to a car's half-length nearer.
			if mathx.Abs(n.Right-at.right) < 1.5 && mathx.Abs(n.Ahead-at.ahead) < 3.5 {
				found = true
			}
		}
		if !found {
			t.Errorf("a car %.0fm ahead and %.1fm right was not placed there: saw %+v", at.ahead, at.right, d.Around())
		}
	}
}

func TestMirrorsShowOnlyWhatIsBehindAndBeside(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 1})
	a := w.Agents[0]
	put(w, a, 20, 0)
	cams := vision.MirrorCameras(256, 144)
	rear := annotate.LayoutMirror(w, annotate.MirrorView(cams[vision.MirrorRear], w.City.StandAt(w.Player.Pos, w.Player.Yaw)))
	if len(rear) != 0 {
		t.Errorf("a car 20m ahead showed in the rear mirror: %+v", rear)
	}
	put(w, a, -20, 0)
	rear = annotate.LayoutMirror(w, annotate.MirrorView(cams[vision.MirrorRear], w.City.StandAt(w.Player.Pos, w.Player.Yaw)))
	if len(rear) != 1 || rear[0].Class != vision.ClassVehicle {
		t.Errorf("a car 20m behind should show once in the rear mirror, got %+v", rear)
	}
}

func TestSideLanesAreMarkedOnADualCarriageway(t *testing.T) {
	w := world(t)
	// The player starts on an arterial, which has a lane alongside.
	if w.Route.Lane() == nil || w.Route.Lane().Class != city.Arterial {
		t.Skip("the player does not start on an arterial")
	}
	_, view := bonnet()
	side := 0
	for _, b := range annotate.Layout(w, view(w)) {
		if b.Class == vision.ClassLaneSide {
			side++
		}
	}
	if side == 0 {
		t.Error("no markers for the lane alongside on a dual carriageway")
	}
}

// hillStart puts the player on a lane where the road climbs or falls, with
// one other car a way ahead of it on the same lane and the rest sent away.
func hillStart(t *testing.T, ahead float32) (w *sim.World, grade float32) {
	t.Helper()
	w = sim.NewWorld(sim.Config{Seed: 7, Traffic: 12, Police: 0, Map: city.MapVillage})
	for _, l := range w.City.Lanes {
		if l.Length < ahead+30 || w.City.Roads[l.Road].Ring {
			continue
		}
		at := l.Point(10)
		g := w.City.Grade(at, l.DirAt(10))
		if mathx.Abs(g) < 0.035 {
			continue
		}
		w.Player.Place(at, l.HeadingAt(10))
		for i, a := range w.Agents {
			a.V.Place(mathx.V(5000+float32(i)*20, 5000), 0)
		}
		lead := w.Agents[0]
		lead.V.Place(l.Point(10+ahead), l.HeadingAt(10+ahead))
		return w, g
	}
	t.Fatal("setup failed: no lane on a hill")
	return nil, 0
}

// The camera is fixed to the car and tips with it, so a car ahead on the same
// slope is ranged as it would be on the level. Were the camera held level
// instead, a car up a one-in-twenty hill thirty metres off would sit a metre
// and a half higher in the picture than the autopilot allows for, and look
// most of a hundred metres away.
func TestACarAheadOnAHillIsRangedAsOnTheLevel(t *testing.T) {
	const ahead = 30
	w, grade := hillStart(t, ahead)
	cam, view := bonnet()
	lead := w.Agents[0]
	// The lens is ahead of the middle of the car, and the box's foot is the
	// lead car's tail.
	want := w.Player.Pos.DistTo(lead.V.Pos) - 1.7 - lead.V.Spec.HalfLength

	var found bool
	for _, b := range annotate.Layout(w, view(w)) {
		if b.Class != vision.ClassVehicle {
			continue
		}
		found = true
		got := cam.GroundDistance(b.MaxY)
		if got < want*0.85 || got > want*1.15 {
			t.Errorf("on a grade of %.3f a car %.1fm off is ranged at %.1fm", grade, want, got)
		}
	}
	if !found {
		t.Fatalf("on a grade of %.3f the car ahead is not in the picture", grade)
	}
}

func TestTheCameraStandsOnTheHillAndLeansWithIt(t *testing.T) {
	w, grade := hillStart(t, 30)
	cam, view := bonnet()
	v := view(w)
	ground := w.City.Height(mathx.V(v.Position.X, v.Position.Z))
	if up := v.Position.Y - ground; mathx.Abs(up-cam.Mount) > 0.05 {
		t.Errorf("the lens is %.2fm above the road under it, want the calibrated %.2f", up, cam.Mount)
	}
	if v.Up.Y <= 0.9 || v.Up.Y >= 1 {
		t.Errorf("on a grade of %.3f the camera's up is %v", grade, v.Up)
	}
	// It looks along the road: further up a climb, further down a fall, than
	// its own tilt would have it on the level.
	run := mathx.V(v.Target.X-v.Position.X, v.Target.Z-v.Position.Z).Len()
	level := -run * mathx.Tan(cam.Pitch)
	if lift := v.Target.Y - v.Position.Y - level; lift*grade <= 0 {
		t.Errorf("on a grade of %.3f the camera looks %.2fm off where a level one would", grade, lift)
	}
}

func TestMirrorsSeeACarBehindOnAHill(t *testing.T) {
	w, _ := hillStart(t, 30)
	// Turn the two round: the other car is now behind.
	p, lead := w.Player, w.Agents[0]
	pp, py := p.Pos, p.Yaw
	p.Place(lead.V.Pos, lead.V.Yaw)
	lead.V.Place(pp, py)
	cams := vision.MirrorCameras(160, 90)
	rear := annotate.LayoutMirror(w, annotate.MirrorView(cams[vision.MirrorRear], w.City.StandAt(p.Pos, p.Yaw)))
	if len(rear) != 1 {
		t.Fatalf("the rear mirror shows %d cars, want the one behind", len(rear))
	}
	want := p.Pos.DistTo(lead.V.Pos) - 2.1 - lead.V.Spec.HalfLength
	if got := cams[vision.MirrorRear].GroundDistance(rear[0].MaxY); got < want*0.8 || got > want*1.2 {
		t.Errorf("a car %.1fm behind on a hill is ranged at %.1fm", want, got)
	}
}
