// Package annotate decides where the vision system's detection boxes go and,
// when asked, draws them itself.
//
// It is the write half of perception, and it is deliberately renderer-free.
// [Layout] projects the world into screen-space [Box] values using nothing but
// arithmetic, and [Rasterise] paints those boxes into an image. The game draws
// the same boxes with raylib over the live scene; the evaluation harness
// rasterises them in software with no GPU at all.
//
// Both paths produce byte-identical detections, because the scanner matches
// only the exact class colours and never looks at the scene behind them. That
// is what makes the autopilot testable without a display.
package annotate

import (
	"image"
	"image/color"
	"math"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/vision"
)

// View is a camera pose and lens, independent of any renderer. It is the
// minimum needed to project a world point onto the image.
type View struct {
	Position, Target mathx.Vec3
	FovY             float32 // vertical field of view, degrees
	Width, Height    int
}

// Box is one detection box in screen space, in pixels.
type Box struct {
	Class                  vision.Class
	MinX, MinY, MaxX, MaxY float32
}

// Width returns the box width in pixels.
func (b Box) Width() float32 { return b.MaxX - b.MinX }

// Height returns the box height in pixels.
func (b Box) Height() float32 { return b.MaxY - b.MinY }

// Thickness is the width of a box outline, in pixels. It matches what the game
// draws, so a rasterised frame and a rendered one scan identically.
const Thickness = 2

// Annotation ranges, in metres.
const (
	vehicleRange   float32 = 85
	propRange      float32 = 95
	laneRange      float32 = 56
	laneSpacing    float32 = 7.0
	laneMarkerSize float32 = 0.5
	// Lane markers are given real height so their projected box stays taller
	// than the scanner's minimum size well into the distance.
	laneMarkerRise float32 = 0.32
	stopLineRange  float32 = 95
)

// projector turns world points into image coordinates for one view.
type projector struct {
	eye            mathx.Vec3
	right, up, fwd mathx.Vec3
	focal          float32
	w, h           float32
}

func newProjector(v View) projector {
	fwd := v.Target.Sub(v.Position).Norm()
	// A camera looking straight down has no usable roll reference, so nudge it.
	worldUp := mathx.V3(0, 1, 0)
	if mathx.Abs(fwd.Dot(worldUp)) > 0.999 {
		worldUp = mathx.V3(0, 0, 1)
	}
	right := fwd.Cross(worldUp).Norm()
	return projector{
		eye: v.Position, fwd: fwd, right: right, up: right.Cross(fwd).Norm(),
		// One focal length for both axes: the horizontal field of view follows
		// from the aspect ratio, which is how raylib's projection behaves too.
		focal: float32(v.Height) / 2 / mathx.Tan(v.FovY*0.5*math.Pi/180),
		w:     float32(v.Width), h: float32(v.Height),
	}
}

// project maps a world point to image coordinates. It reports false for points
// at or behind the eye plane, whose projection is meaningless.
func (p projector) project(w mathx.Vec3) (x, y float32, ok bool) {
	rel := w.Sub(p.eye)
	z := rel.Dot(p.fwd)
	if z <= 0.05 {
		return 0, 0, false
	}
	return p.w/2 + p.focal*rel.Dot(p.right)/z,
		p.h/2 - p.focal*rel.Dot(p.up)/z,
		true
}

// Layout returns the detection boxes visible from the given view, in the order
// they should be drawn. Later boxes paint over earlier ones, which is how a car
// standing on a stop line breaks that line into fragments — the same occlusion
// a real detector would have to cope with.
func Layout(w *sim.World, v View) []Box {
	p := newProjector(v)
	eye := v.Position.Ground()
	fwd := p.fwd.Ground().Norm()

	visible := func(at mathx.Vec, limit float32) bool {
		rel := at.Sub(eye)
		return rel.Dot(fwd) > 1.5 && rel.Len() < limit
	}

	var out []Box
	add := func(b Box, ok bool) {
		if ok {
			out = append(out, b)
		}
	}

	// Lane guidance first, so anything solid paints over it.
	for i, pt := range w.Route.Centreline(4, laneSpacing, int(laneRange/laneSpacing)) {
		if !visible(pt, laneRange) {
			continue
		}
		cl := vision.ClassLaneMarker
		if i%2 == 1 {
			cl = vision.ClassLaneMarkerAlt
		}
		add(boxOf(p, cl, pt, laneMarkerRise, laneMarkerSize, laneMarkerRise, laneMarkerSize, 0))
	}

	if at, ctrl, ok := w.Route.StopLine(stopLineRange); ok && ctrl != city.ControlNone && visible(at, stopLineRange) {
		if l := w.Route.Lane(); l != nil {
			hw := w.City.RoadHalfWidth(l)
			add(boxOf(p, vision.ClassStopLine, at, 0.9, hw*0.5, 0.9, 0.3, l.Heading))
		}
	}

	for _, prop := range w.City.Props {
		if !visible(prop.Pos, propRange) {
			continue
		}
		// Signs and signals only face one way, so skip those turned away.
		if mathx.FromAngle(prop.Heading).Dot(fwd) > -0.25 {
			continue
		}
		switch prop.Kind {
		case city.PropTrafficLight:
			add(boxOf(p, signalClass(w, prop), prop.Pos, prop.Height-0.55, 0.55, 0.85, 0.3, prop.Heading))
		case city.PropStopSign:
			add(boxOf(p, vision.ClassStopSign, prop.Pos, prop.Height-0.42, 0.5, 0.5, 0.12, prop.Heading))
		case city.PropGiveWaySign:
			add(boxOf(p, vision.ClassGiveWay, prop.Pos, prop.Height-0.42, 0.5, 0.5, 0.12, prop.Heading))
		case city.PropSpeedSign:
			add(boxOf(p, speedClass(prop.Limit), prop.Pos, prop.Height-0.4, 0.45, 0.55, 0.12, prop.Heading))
		}
	}

	for _, a := range w.Agents {
		if !visible(a.V.Pos, vehicleRange) {
			continue
		}
		cl := vision.ClassVehicle
		if a.Role == sim.RolePolice {
			cl = vision.ClassPolice
		}
		s := a.V.Spec
		add(boxOf(p, cl, a.V.Pos, s.Height/2, s.HalfWidth, s.Height/2, s.HalfLength, a.V.Yaw))
	}
	return out
}

func signalClass(w *sim.World, prop city.Prop) vision.Class {
	switch w.Signals.State(prop.Node, prop.Group) {
	case sim.SignalGreen:
		return vision.ClassLightGreen
	case sim.SignalAmber:
		return vision.ClassLightAmber
	case sim.SignalRedAmber:
		return vision.ClassLightRedAmber
	default:
		return vision.ClassLightRed
	}
}

func speedClass(limit float32) vision.Class {
	switch mph := mathx.ToMPH(limit); {
	case mph < 25:
		return vision.ClassSpeed20
	case mph < 35:
		return vision.ClassSpeed30
	default:
		return vision.ClassSpeed40
	}
}

// boxOf projects an oriented box standing on the ground to its screen-space
// bounding rectangle, and reports false when nothing usable lands on the image.
func boxOf(p projector, cl vision.Class, ground mathx.Vec, centreY, halfW, halfH, halfL, yaw float32) (Box, bool) {
	f := mathx.FromAngle(yaw)
	r := f.Right()

	minX, minY := float32(1e9), float32(1e9)
	maxX, maxY := float32(-1e9), float32(-1e9)
	seen := 0
	for i := range 8 {
		sx, sy, sz := corner(i)
		off := f.Mul(halfL * sz).Add(r.Mul(halfW * sx))
		x, y, ok := p.project(mathx.V3(ground.X+off.X, centreY+halfH*sy, ground.Z+off.Z))
		if !ok {
			continue
		}
		seen++
		minX, maxX = min(minX, x), max(maxX, x)
		minY, maxY = min(minY, y), max(maxY, y)
	}
	if seen == 0 {
		return Box{}, false
	}
	// Clip to the image, then reject anything that ended up degenerate.
	minX, maxX = max(minX, 0), min(maxX, p.w-1)
	minY, maxY = max(minY, 0), min(maxY, p.h-1)
	if maxX-minX < 2 || maxY-minY < 2 {
		return Box{}, false
	}
	return Box{Class: cl, MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY}, true
}

// corner expands a box corner index into its three sign components.
func corner(i int) (x, y, z float32) {
	sign := func(bit int) float32 {
		if i&bit == 0 {
			return -1
		}
		return 1
	}
	return sign(1), sign(4), sign(2)
}

// Rasterise paints boxes into a new image, in the same order and with the same
// outline thickness the game draws them. The background is transparent, which
// the scanner ignores, so only the boxes matter.
func Rasterise(boxes []Box, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for _, b := range boxes {
		outline(img, b, vision.RGBA(b.Class))
	}
	return img
}

// RasteriseInto paints boxes over an existing image, reusing its buffer so a
// long evaluation run does not allocate a frame at a time.
func RasteriseInto(img *image.RGBA, boxes []Box) {
	clear(img.Pix)
	for _, b := range boxes {
		outline(img, b, vision.RGBA(b.Class))
	}
}

func outline(img *image.RGBA, b Box, c color.RGBA) {
	x0, y0 := int(b.MinX), int(b.MinY)
	x1, y1 := int(b.MaxX), int(b.MaxY)
	// Two horizontal runs and two vertical ones, drawn inward from the edges
	// exactly as a rectangle outline of this thickness would be.
	for t := range Thickness {
		hline(img, x0, x1, y0+t, c)
		hline(img, x0, x1, y1-t, c)
		vline(img, y0, y1, x0+t, c)
		vline(img, y0, y1, x1-t, c)
	}
}

func hline(img *image.RGBA, x0, x1, y int, c color.RGBA) {
	if y < img.Rect.Min.Y || y >= img.Rect.Max.Y {
		return
	}
	for x := max(x0, img.Rect.Min.X); x <= min(x1, img.Rect.Max.X-1); x++ {
		set(img, x, y, c)
	}
}

func vline(img *image.RGBA, y0, y1, x int, c color.RGBA) {
	if x < img.Rect.Min.X || x >= img.Rect.Max.X {
		return
	}
	for y := max(y0, img.Rect.Min.Y); y <= min(y1, img.Rect.Max.Y-1); y++ {
		set(img, x, y, c)
	}
}

func set(img *image.RGBA, x, y int, c color.RGBA) {
	o := img.PixOffset(x, y)
	img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = c.R, c.G, c.B, c.A
}

// BonnetView returns the view for a bonnet-mounted camera on a car at the
// given pose. The game renders the scene through it and the evaluation harness
// projects boxes through it, so neither can drift from the calibration the
// autopilot was built against.
func BonnetView(c vision.Camera, pos mathx.Vec, yaw float32) View {
	const mountAhead = 1.7 // how far forward of the car's centre the lens sits
	const look = 24        // an arbitrary distance along the optical axis

	fwd := mathx.FromAngle(yaw)
	eye := pos.Add(fwd.Mul(mountAhead))
	at := eye.Add(fwd.Mul(look))
	return View{
		Position: mathx.V3(eye.X, c.Mount, eye.Z),
		Target:   mathx.V3(at.X, c.Mount-look*mathx.Tan(c.Pitch), at.Z),
		FovY:     c.FovY,
		Width:    c.Width, Height: c.Height,
	}
}
