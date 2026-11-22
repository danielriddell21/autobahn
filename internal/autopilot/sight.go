package autopilot

import (
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/vision"
)

// Sight is how far the autopilot can tell the road runs, judged only from the
// farthest of its own lane's markers that it can currently see. It is told
// nothing of the weather: when the air thickens the far markers fade, and this
// is how it notices, so it drives within what it can see.
//
// It uses detections and the camera alone, like the rest of the autopilot.
type Sight struct {
	reach float32
}

// Sight tuning, in metres and metres a second.
const (
	// clearReach is the distance below which the farthest marker seen is
	// taken to mean the air is thick. A clear road shows markers out to about
	// fifty metres, and less over a crest, so this keeps clear weather as it
	// was.
	clearReach float32 = 36
	// sightMargin is the distance to be left over when stopped.
	sightMargin float32 = 5
	// sightFloor is the slowest the cap ever holds the car to.
	sightFloor float32 = 4
	// reachFall is how fast what has been seen is forgotten, so a marker
	// that flickers out for a frame does not slow the car.
	reachFall float32 = 20
)

// Update looks at a frame's detections, dt seconds after the last.
func (s *Sight) Update(cam vision.Camera, dets []vision.Detection, dt float32) {
	far := float32(0)
	for _, d := range dets {
		if !vision.IsLaneMarker(d.Class) {
			continue
		}
		if dist := cam.GroundDistance(d.MaxY); dist < 1e4 {
			far = max(far, dist)
		}
	}
	switch {
	case far >= s.reach:
		s.reach = far
	default:
		s.reach = max(far, s.reach-reachFall*dt)
	}
}

// Reach returns the distance of the farthest lane marker seen lately, or zero
// if none has been.
func (s *Sight) Reach() float32 { return s.reach }

// Cap returns the fastest the car may go and still stop, at the given braking,
// within the road it can see, or a very large number if there is no reason to
// hold it: nothing has been seen to judge by, or the road is seen as far as in
// clear weather.
func (s *Sight) Cap(decel float32) float32 {
	if s.reach <= 0 || s.reach >= clearReach || decel <= 0 {
		return 1e9
	}
	room := max(s.reach-sightMargin, 0)
	return max(mathx.Sqrt(2*decel*room), sightFloor)
}
