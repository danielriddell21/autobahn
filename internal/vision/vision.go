// Package vision is the perception channel between the simulated world and the
// autopilot.
//
// This package is deliberately pure: it depends on nothing but the standard
// library and mathx, and knows nothing about the simulation. Package annotate
// draws the colour-coded boxes into an off-screen camera image, in the style of
// a security camera running object detection; [Scanner.Scan] then reads that
// image back a pixel at a time and recovers the boxes as [Detection] values.
//
// The autopilot depends on this package and nothing else from the project, so
// the compiler guarantees it cannot read simulation state. Run
// "go list -deps ./internal/autopilot" to check.
package vision

import (
	"image/color"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Class is the category of an annotated object. Each class is drawn in its own
// fully saturated colour, which is how the scanner recovers the category from
// pixels alone.
type Class int

// The detection classes.
const (
	ClassNone Class = iota
	ClassVehicle
	ClassPolice
	ClassLightRed
	ClassLightRedAmber
	ClassLightAmber
	ClassLightGreen
	ClassStopSign
	ClassGiveWay
	ClassSpeed20
	ClassSpeed30
	ClassSpeed40
	ClassLaneMarker
	ClassLaneMarkerAlt
	ClassStopLine
	numClasses
)

// String returns the class label drawn beside each box.
func (c Class) String() string {
	switch c {
	case ClassVehicle:
		return "VEHICLE"
	case ClassPolice:
		return "POLICE"
	case ClassLightRed:
		return "LIGHT:RED"
	case ClassLightRedAmber:
		return "LIGHT:RED+AMBER"
	case ClassLightAmber:
		return "LIGHT:AMBER"
	case ClassLightGreen:
		return "LIGHT:GREEN"
	case ClassStopSign:
		return "STOP"
	case ClassGiveWay:
		return "GIVE WAY"
	case ClassSpeed20:
		return "LIMIT 20"
	case ClassSpeed30:
		return "LIMIT 30"
	case ClassSpeed40:
		return "LIMIT 40"
	case ClassLaneMarker, ClassLaneMarkerAlt:
		return "LANE"
	case ClassStopLine:
		return "STOP LINE"
	default:
		return "?"
	}
}

// SpeedValue returns the limit a speed sign class encodes, in m/s, and whether
// the class is a speed sign at all. British limits are posted in miles per
// hour, so the classes name mph values.
func (c Class) SpeedValue() (float32, bool) {
	switch c {
	case ClassSpeed20:
		return mathx.MPH(20), true
	case ClassSpeed30:
		return mathx.MPH(30), true
	case ClassSpeed40:
		return mathx.MPH(40), true
	default:
		return 0, false
	}
}

// StopsTraffic reports whether a signal class forbids crossing the line. Red,
// red-and-amber and amber all do.
func (c Class) StopsTraffic() bool {
	return c == ClassLightRed || c == ClassLightRedAmber || c == ClassLightAmber
}

// classColors are the exact RGB values each class is drawn in. They are all
// fully saturated combinations of 0, 128 and 255, and no material in the world
// uses any of them, so an exact pixel match cannot produce a false positive.
var classColors = [numClasses]color.RGBA{
	ClassNone:          {0, 0, 0, 0},
	ClassVehicle:       {255, 0, 0, 255},
	ClassPolice:        {255, 128, 255, 255},
	ClassLightRed:      {255, 0, 255, 255},
	ClassLightRedAmber: {255, 0, 128, 255},
	ClassLightAmber:    {255, 128, 0, 255},
	ClassLightGreen:    {0, 255, 0, 255},
	ClassStopSign:      {255, 255, 0, 255},
	ClassGiveWay:       {128, 255, 255, 255},
	ClassSpeed20:       {0, 128, 255, 255},
	ClassSpeed30:       {0, 255, 255, 255},
	ClassSpeed40:       {128, 0, 255, 255},
	ClassLaneMarker:    {0, 0, 255, 255},
	ClassLaneMarkerAlt: {0, 255, 128, 255},
	ClassStopLine:      {128, 255, 0, 255},
}

// RGBA returns the exact colour a class is annotated in.
func RGBA(c Class) color.RGBA { return classColors[c] }

// IsVehicle reports whether a class is a car of any kind. A police car is
// still something to avoid rear-ending, so the autopilot treats both alike
// when it is deciding what to follow.
func IsVehicle(c Class) bool { return c == ClassVehicle || c == ClassPolice }

// IsLaneMarker reports whether a class is one of the lane centreline markers.
//
// Consecutive markers alternate between two colours. Neighbouring boxes often
// touch on the image once they are far enough away to be only a few pixels
// across, and two runs of the same colour would flood-fill into a single
// meaningless detection. Alternating guarantees they stay separable.
func IsLaneMarker(c Class) bool {
	return c == ClassLaneMarker || c == ClassLaneMarkerAlt
}

// Detection is one object recovered from the annotated image. Coordinates are
// in pixels within the camera image, with the origin at the top left.
type Detection struct {
	Class                  Class
	MinX, MinY, MaxX, MaxY float32
	Pixels                 int
}

// Width returns the box width in pixels.
func (d Detection) Width() float32 { return d.MaxX - d.MinX }

// Height returns the box height in pixels.
func (d Detection) Height() float32 { return d.MaxY - d.MinY }

// CenterX returns the horizontal centre of the box in pixels.
func (d Detection) CenterX() float32 { return (d.MinX + d.MaxX) / 2 }

// CenterY returns the vertical centre of the box in pixels.
func (d Detection) CenterY() float32 { return (d.MinY + d.MaxY) / 2 }

// Camera describes the geometry of the AI's camera. These are calibration
// constants, the same thing a real perception stack knows about its own lens,
// and they let the autopilot turn pixel positions into angles and distances.
type Camera struct {
	Width, Height int
	FovY          float32 // vertical field of view, degrees
	Mount         float32 // height above the road, metres
	Pitch         float32 // downward tilt, radians
}

// DefaultCamera returns the bonnet-mounted camera used by the autopilot.
func DefaultCamera(w, h int) Camera {
	return Camera{Width: w, Height: h, FovY: 62, Mount: 1.35, Pitch: 0.10}
}

func (c Camera) focal() float32 {
	// Returns the focal length in pixels for the vertical axis.
	return float32(c.Height) / 2 / mathx.Tan(c.FovY*0.5*3.14159265/180)
}

// GroundDistance estimates how far ahead the ground point under a given image
// row lies, assuming a flat road. Rows at or above the horizon return a large
// value, since they cannot be intersected with the ground.
func (c Camera) GroundDistance(pixelY float32) float32 {
	// Angle below the optical axis for this row, plus the camera's own tilt.
	down := c.Pitch + mathx.Atan2(pixelY-float32(c.Height)/2, c.focal())
	if down <= 0.012 {
		return 1e5
	}
	return c.Mount / mathx.Tan(down)
}

// SignalHeadHeight is the height above the road of the bottom of a traffic
// light's annotated box, used to range signals directly.
const SignalHeadHeight float32 = 2.0

// RangeAtHeight estimates how far ahead a point lies when its height above the
// road is known, which is what lets a traffic light be ranged directly instead
// of relying on a stop line painted at its feet.
//
// GroundDistance is the special case of this for objects sitting on the road.
// Objects mounted above the camera appear above the optical axis, so the sign
// of the drop and of the ray angle agree and the result stays positive.
func (c Camera) RangeAtHeight(pixelY, height float32) float32 {
	down := c.Pitch + mathx.Atan2(pixelY-float32(c.Height)/2, c.focal())
	drop := c.Mount - height
	t := mathx.Tan(down)
	if mathx.Abs(t) < 1e-4 || drop == 0 {
		return 1e5
	}
	d := drop / t
	if d <= 0 {
		return 1e5 // the ray never reaches that height ahead of the car
	}
	return d
}

// Bearing returns the horizontal angle to an image column in radians, positive
// to the right of straight ahead.
func (c Camera) Bearing(pixelX float32) float32 {
	return mathx.Atan2(pixelX-float32(c.Width)/2, c.focal())
}

// Scanner recovers detections from an annotated image. It reuses its working
// buffers between frames so scanning does not allocate per frame.
type Scanner struct {
	labels []int32
	stack  []int32
	byKey  map[uint32]Class
	boxes  []Detection
}

// NewScanner returns a scanner ready to read images of any size.
func NewScanner() *Scanner {
	s := &Scanner{byKey: make(map[uint32]Class, numClasses)}
	for cl := ClassVehicle; cl < numClasses; cl++ {
		v := classColors[cl]
		s.byKey[key(v.R, v.G, v.B)] = cl
	}
	return s
}

func key(r, g, b uint8) uint32 {
	return uint32(r)<<16 | uint32(g)<<8 | uint32(b)
}

// MinPixels is the smallest blob treated as a real detection, which rejects
// stray single pixels along box corners.
const MinPixels = 6

// Scan finds every annotation box in the image and returns one detection per
// connected run of a class colour. The pixels are read as raw RGBA rows, and
// nothing about the simulation is consulted: this is the only route by which
// world state reaches the autopilot.
func (s *Scanner) Scan(px []color.RGBA, w, h int) []Detection {
	n := w * h
	if len(px) < n || n == 0 {
		return nil
	}
	if cap(s.labels) < n {
		s.labels = make([]int32, n)
	}
	s.labels = s.labels[:n]
	clear(s.labels)
	s.boxes = s.boxes[:0]

	for i := range n {
		if s.labels[i] != 0 {
			continue
		}
		p := px[i]
		if p.A == 0 {
			continue
		}
		cl, ok := s.byKey[key(p.R, p.G, p.B)]
		if !ok {
			continue
		}
		s.boxes = append(s.boxes, s.flood(px, w, h, i, cl))
	}

	out := s.boxes[:0]
	for _, d := range s.boxes {
		if d.Pixels >= MinPixels {
			out = append(out, d)
		}
	}
	s.boxes = out
	return s.boxes
}

func (s *Scanner) flood(px []color.RGBA, w, h, start int, cl Class) Detection {
	// Eight-connected flood fill over pixels of exactly this class colour. The
	// annotations are hollow rectangles, so one fill recovers one box outline.
	want := classColors[cl]
	mark := int32(len(s.boxes) + 1)
	s.stack = append(s.stack[:0], int32(start))
	s.labels[start] = mark

	d := Detection{
		Class: cl,
		MinX:  float32(start % w), MaxX: float32(start % w),
		MinY: float32(start / w), MaxY: float32(start / w),
	}

	for len(s.stack) > 0 {
		idx := s.stack[len(s.stack)-1]
		s.stack = s.stack[:len(s.stack)-1]
		x, y := int(idx)%w, int(idx)/w
		d.Pixels++
		d.MinX = min(d.MinX, float32(x))
		d.MaxX = max(d.MaxX, float32(x))
		d.MinY = min(d.MinY, float32(y))
		d.MaxY = max(d.MaxY, float32(y))

		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				nx, ny := x+dx, y+dy
				if nx < 0 || ny < 0 || nx >= w || ny >= h {
					continue
				}
				ni := int32(ny*w + nx)
				if s.labels[ni] != 0 {
					continue
				}
				q := px[ni]
				if q.A == 0 || q.R != want.R || q.G != want.G || q.B != want.B {
					continue
				}
				s.labels[ni] = mark
				s.stack = append(s.stack, ni)
			}
		}
	}
	return d
}
