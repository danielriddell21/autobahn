// Package annotate draws the vision system's colour-coded detection boxes over
// the world, in the style of a security camera running object detection.
//
// It is the write half of perception. It knows about the simulation and about
// raylib; package vision, which the autopilot reads, knows about neither.
package annotate

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/vision"
)

// colorOf converts a class's exact annotation colour into a raylib colour.
func colorOf(c vision.Class) rl.Color {
	v := vision.RGBA(c)
	return rl.NewColor(v.R, v.G, v.B, v.A)
}

// annotation ranges, in metres.
const (
	vehicleRange   float32 = 85
	propRange      float32 = 95
	laneRange      float32 = 56
	laneSpacing    float32 = 7.0
	laneMarkerSize float32 = 0.5
	// Lane markers are given real height so their projected box stays taller
	// than the scanner's minimum size well into the distance.
	laneMarkerRise float32 = 0.32
)

// Annotate draws the detection boxes for everything the camera can see. It must
// be called with a 3D camera already ended, while the render target is still
// bound, because it draws in 2D screen space.
//
// showLabels adds the class name beside each box; it is purely cosmetic and has
// no effect on what the scanner recovers.
func Annotate(w *sim.World, cam rl.Camera3D, screenW, screenH int32, showLabels bool) {
	eye := mathx.V(cam.Position.X, cam.Position.Z)
	fwd := mathx.V(cam.Target.X-cam.Position.X, cam.Target.Z-cam.Position.Z).Norm()

	visible := func(p mathx.Vec, limit float32) bool {
		rel := p.Sub(eye)
		return rel.Dot(fwd) > 1.5 && rel.Len() < limit
	}

	for _, a := range w.Agents {
		if !visible(a.V.Pos, vehicleRange) {
			continue
		}
		s := a.V.Spec
		if r, ok := boxOf(cam, a.V.Pos, s.Height/2, s.HalfWidth, s.Height/2, s.HalfLength, a.V.Yaw, screenW, screenH); ok {
			draw(r, vision.ClassVehicle, showLabels)
		}
	}

	for _, p := range w.City.Props {
		if !visible(p.Pos, propRange) {
			continue
		}
		// Signs and signals only face one way, so skip those turned away.
		facing := mathx.FromAngle(p.Heading)
		if facing.Dot(fwd) > -0.25 {
			continue
		}
		switch p.Kind {
		case city.PropTrafficLight:
			cl := vision.ClassLightRed
			switch w.Signals.State(p.Node, p.Group) {
			case sim.SignalGreen:
				cl = vision.ClassLightGreen
			case sim.SignalAmber:
				cl = vision.ClassLightAmber
			case sim.SignalRedAmber:
				cl = vision.ClassLightRedAmber
			}
			if r, ok := boxOf(cam, p.Pos, p.Height-0.55, 0.55, 0.85, 0.3, p.Heading, screenW, screenH); ok {
				draw(r, cl, showLabels)
			}
		case city.PropStopSign:
			if r, ok := boxOf(cam, p.Pos, p.Height-0.42, 0.5, 0.5, 0.12, p.Heading, screenW, screenH); ok {
				draw(r, vision.ClassStopSign, showLabels)
			}
		case city.PropGiveWaySign:
			if r, ok := boxOf(cam, p.Pos, p.Height-0.42, 0.5, 0.5, 0.12, p.Heading, screenW, screenH); ok {
				draw(r, vision.ClassGiveWay, showLabels)
			}
		case city.PropSpeedSign:
			cl := speedClass(p.Limit)
			if r, ok := boxOf(cam, p.Pos, p.Height-0.4, 0.45, 0.55, 0.12, p.Heading, screenW, screenH); ok {
				draw(r, cl, showLabels)
			}
		}
	}

	// Lane guidance: the centreline of the route ahead, drawn as a run of small
	// ground markers for the autopilot to track.
	for i, p := range w.Route.Centreline(4, laneSpacing, int(laneRange/laneSpacing)) {
		if !visible(p, laneRange) {
			continue
		}
		cl := vision.ClassLaneMarker
		if i%2 == 1 {
			cl = vision.ClassLaneMarkerAlt
		}
		if r, ok := boxOf(cam, p, laneMarkerRise, laneMarkerSize, laneMarkerRise, laneMarkerSize, 0, screenW, screenH); ok {
			draw(r, cl, false)
		}
	}

	if p, ctrl, ok := w.Route.StopLine(95); ok && ctrl != city.ControlNone && visible(p, 95) {
		if l := w.Route.Lane(); l != nil {
			hw := w.City.RoadHalfWidth(l)
			if r, ok := boxOf(cam, p, 0.9, hw*0.5, 0.9, 0.3, l.Heading, screenW, screenH); ok {
				draw(r, vision.ClassStopLine, showLabels)
			}
		}
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

func draw(r rl.Rectangle, cl vision.Class, label bool) {
	col := colorOf(cl)
	rl.DrawRectangleLinesEx(r, 2, col)
	if label && r.Width > 26 {
		y := r.Y - 11
		if y < 1 {
			y = r.Y + r.Height + 2
		}
		rl.DrawText(cl.String(), int32(r.X), int32(y), 10, col)
	}
}

// boxOf projects an oriented 3D box to its screen-space bounding rectangle.
// It reports false when the box lands entirely off-screen or behind the eye.
func boxOf(cam rl.Camera3D, ground mathx.Vec, centreY, halfW, halfH, halfL, yaw float32, sw, sh int32) (rl.Rectangle, bool) {
	f := mathx.FromAngle(yaw)
	r := f.Right()

	minX, minY := float32(1e9), float32(1e9)
	maxX, maxY := float32(-1e9), float32(-1e9)
	behind := 0
	for i := range 8 {
		sx := float32(1)
		if i&1 == 0 {
			sx = -1
		}
		sz := float32(1)
		if i&2 == 0 {
			sz = -1
		}
		sy := float32(1)
		if i&4 == 0 {
			sy = -1
		}
		off := f.Mul(halfL * sz).Add(r.Mul(halfW * sx))
		p := rl.NewVector3(ground.X+off.X, centreY+halfH*sy, ground.Z+off.Z)

		// Reject points behind the camera; their projection is meaningless.
		toward := mathx.V(p.X-cam.Position.X, p.Z-cam.Position.Z)
		dir := mathx.V(cam.Target.X-cam.Position.X, cam.Target.Z-cam.Position.Z).Norm()
		if toward.Dot(dir) <= 0.35 {
			behind++
			continue
		}
		v := rl.GetWorldToScreenEx(p, cam, sw, sh)
		minX, maxX = min(minX, v.X), max(maxX, v.X)
		minY, maxY = min(minY, v.Y), max(maxY, v.Y)
	}
	if behind == 8 {
		return rl.Rectangle{}, false
	}
	// Clip to the image, then reject anything that ended up degenerate.
	minX, maxX = max(minX, 0), min(maxX, float32(sw)-1)
	minY, maxY = max(minY, 0), min(maxY, float32(sh)-1)
	if maxX-minX < 2 || maxY-minY < 2 {
		return rl.Rectangle{}, false
	}
	return rl.NewRectangle(minX, minY, maxX-minX, maxY-minY), true
}
