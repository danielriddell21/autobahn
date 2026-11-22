package autopilot

import "github.com/danielriddell21/autobahn/internal/vision"

// Giving way to an ambulance, from the mirrors and the camera alone.
//
// An ambulance with its blue lights on is boxed as its own class, and the
// mirrors see it coming up behind. The autopilot then does what a driver is
// taught: it signals left, moves into the kerbside lane if there is room, and
// eases to about sixty per cent of the limit and keeps to it until the
// ambulance has gone by. It never makes room by running a signal or stopping
// short of one: every control at a junction is judged as usual and this only
// ever takes speed away, never adds it. It never stops in a junction box, since
// it never stops; and it never overtakes an ambulance in front.

// Giving way, in seconds, metres and a share of the limit.
const (
	// yieldShare is the share of the limit the car eases to.
	yieldShare float32 = 0.6
	// yieldNear is how close behind an ambulance is counted as on its way
	// past even when it is not seen to be gaining, as it is not on a car it is
	// stuck behind.
	yieldNear float32 = 30
	// yieldClosing is the closing speed, in m/s, that counts as coming up on
	// the car.
	yieldClosing float32 = 0.3
	// yieldLetGo is how long the ambulance has to be out of the mirrors before
	// the car carries on: it has gone by.
	yieldLetGo float32 = 1.5
	// yieldSight is how far ahead an ambulance is seen and left alone.
	yieldSight float32 = 70
	// alongside is how far ahead of the middle of the car an ambulance may be
	// and still be counted as in the mirror: beside it, it has not yet passed.
	alongside float32 = 3
)

// yieldState is what the driver knows about the ambulances around it.
type yieldState struct {
	// clock is the driver's own time, in seconds, and seen the moment an
	// ambulance was last in a mirror.
	clock, seen float32
	// dist is how far behind the nearest ambulance was at its last look, at
	// distAt, and closing how fast it is gaining, smoothed.
	dist, distAt, closing float32
	// active is set from the first sight of one coming up until it has gone
	// by, and ahead while one is in view in front.
	active, ahead bool
}

// noPass reports whether the car must not start passing or changing lane for
// its own sake: while giving way, and with an ambulance in front. Cones are the
// exception, which the car has to get round whoever is watching (see manoeuvre).
func (d *Driver) noPass() bool { return d.yield.active || d.yield.ahead }

// watchEmergency reads the mirrors and the camera for ambulances, and keeps the
// state of giving way: begun at the first sight of one coming up, and ended
// once it has been out of every mirror for long enough to have gone by.
func (d *Driver) watchEmergency(dets []vision.Detection, dt float32) {
	y := &d.yield
	y.clock += dt
	y.ahead = false
	for _, det := range dets {
		if det.Class == vision.ClassAmbulance && d.cam.GroundDistance(det.MaxY) < yieldSight {
			y.ahead = true
		}
	}

	nearest, found := float32(1e9), false
	for _, seen := range d.around {
		for _, n := range seen {
			if n.Emergency && n.Ahead < alongside {
				nearest, found = min(nearest, -n.Ahead), true
			}
		}
	}
	if found {
		y.sighted(nearest)
	}
	if y.active && y.clock-y.seen > yieldLetGo {
		y.active, y.closing = false, 0
	}
}

// sighted takes a sighting of the nearest ambulance in the mirrors, nearest
// metres behind.
func (y *yieldState) sighted(nearest float32) {
	if nearest != y.dist {
		// A new look. How fast it is gaining is how its range changed since
		// the last, while it was still in sight.
		if span := y.clock - y.distAt; y.clock-y.seen < 1 && span > 0 {
			y.closing += ((y.dist-nearest)/span - y.closing) * clamp(span/0.5, 0, 1)
		} else {
			y.closing = 0
		}
		y.dist, y.distAt = nearest, y.clock
	}
	y.seen = y.clock
	if y.closing > yieldClosing || nearest < yieldNear {
		y.active = true
	}
}

// giveWayToEmergency looks for ambulances and gives way to one, and returns
// the speed the car should now aim for. It never raises it.
func (d *Driver) giveWayToEmergency(dets []vision.Detection, raw []lanePoint, cruise, speed, dt float32) float32 {
	d.watchEmergency(dets, dt)
	if !d.yield.active {
		return cruise
	}

	// Into the kerbside lane, if one is there with room, and the road is
	// plain enough to do it on. What is already being done is left to finish.
	if d.move == moveNone && !d.passing && speed >= moveSpeed && d.straightAhead(raw) {
		s := d.readSides(dets, raw)
		if room := max(sideReach, speed*moveTime); s.left != 0 && max(s.leftReach, d.reach[0]) > room {
			d.startMove(s.left, false)
		}
	}
	if d.move == movePass || d.move == moveAbort {
		return cruise // half way past something: finish it, it is quick
	}
	d.Reason = "giving way to an ambulance"
	return min(cruise, yieldShare*d.SeenLimit)
}
