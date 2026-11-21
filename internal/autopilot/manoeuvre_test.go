package autopilot

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/vision"
)

func driver() *Driver { return New(vision.DefaultCamera(420, 236)) }

func straight(lateral float32) []lanePoint {
	var out []lanePoint
	for d := float32(4); d <= 40; d += 5 {
		out = append(out, lanePoint{dist: d, lateral: lateral})
	}
	return out
}

func TestOnlyTheBolderModesBendTheRules(t *testing.T) {
	for m := range NumModes {
		pf := m.profile()
		bold := m == ModeAggressive || m == ModeGetaway
		if pf.rolling != bold || pf.press != bold || pf.undertake != bold {
			t.Errorf("%s: rolling %v press %v undertake %v, want all %v", m, pf.rolling, pf.press, pf.undertake, bold)
		}
		if pf.oncoming != (bold || m == ModeBrisk) {
			t.Errorf("%s: passes on the other side %v", m, pf.oncoming)
		}
	}
	if !(ModeCautious.profile().patience > ModeNormal.profile().patience &&
		ModeNormal.profile().patience > ModeAggressive.profile().patience) {
		t.Error("patience should fall from cautious through normal to aggressive")
	}
}

func TestMoveIsALaneWidthAcross(t *testing.T) {
	d := driver()
	d.startMove(4.65, false)
	if d.moveSide != laneWidth || d.move != moveSignal {
		t.Errorf("a lane measured 4.65m right should be taken as %.1fm, got %.2f", laneWidth, d.moveSide)
	}
	d.startMove(-2.9, true)
	if d.moveSide != -laneWidth || !d.passing {
		t.Errorf("a lane measured 2.9m left should be taken as %.1fm, got %.2f", -laneWidth, d.moveSide)
	}
}

// When the route follows the car into the next lane the markers jump a lane's
// width; the shift has to absorb that so the car does not swerve back.
func TestShiftAbsorbsTheMarkersJumping(t *testing.T) {
	d := driver()
	d.move, d.moveSide = moveShift, laneWidth
	d.shift, d.shiftWant = 2.4, laneWidth
	d.shiftLane(straight(-2.4), 1.0/60) // the old lane, now off to the left
	path := d.shiftLane(straight(1.2), 1.0/60)
	if d.move != moveNone || d.shiftWant != 0 {
		t.Errorf("the change should be over once the markers have moved: move %d want %.2f", d.move, d.shiftWant)
	}
	// Where the car is steering must not have moved with the markers.
	if got := path[0].lateral; abs(got-0) > 0.1 {
		t.Errorf("the path is %.2fm across after the jump, want where it was (0)", got)
	}
}

func TestShiftIgnoresAJumpTheWrongWay(t *testing.T) {
	d := driver()
	d.move, d.shift, d.shiftWant = moveShift, 1, laneWidth
	d.shiftLane(straight(-1), 1.0/60)
	d.shiftLane(straight(-4.6), 1.0/60) // a jump to the left while moving right
	if d.move != moveShift {
		t.Error("a jump away from the lane being moved into is not arriving in it")
	}
}

func TestReachIsRememberedWhileHidden(t *testing.T) {
	d := driver()
	s := d.rememberSides(sideLanes{right: 3.6, rghReach: 40}, 10, 0.1)
	if s.rghReach != 40 {
		t.Fatalf("reach %.1f, want 40", s.rghReach)
	}
	// A frame later the far markers are hidden: what was seen still stands,
	// less how far the car has come.
	s = d.rememberSides(sideLanes{right: 3.6, rghReach: 14}, 10, 0.1)
	if abs(s.rghReach-39) > 0.01 {
		t.Errorf("reach %.1f, want 39", s.rghReach)
	}
}

func TestIndicatorFollowsTheMove(t *testing.T) {
	d := driver()
	if d.indicator() != IndicateOff {
		t.Error("nothing going on should show no indicator")
	}
	d.startMove(laneWidth, false)
	if d.indicator() != IndicateRight {
		t.Error("moving right should indicate right")
	}
	d.endMove()
	d.shift = laneWidth // out there, and coming back in
	if d.indicator() != IndicateLeft {
		t.Error("pulling back in from the right should indicate left")
	}
	d.move = moveAbort
	if d.indicator() != IndicateOff {
		t.Error("dropping back behind is not signalled")
	}
}

func TestRollingStopOnlyWhenTheModeAllows(t *testing.T) {
	for _, m := range []Mode{ModeNormal, ModeAggressive} {
		d := driver()
		d.haltAtStopSign(stopBuffer+1, 1.8, 1.0/60, m.profile())
		if got := d.stopDone; got != m.profile().rolling {
			t.Errorf("%s crawling at the line: stop done %v, want %v", m, got, m.profile().rolling)
		}
	}
}

func TestLeadSpeedFromClosingRange(t *testing.T) {
	d := driver()
	// Closing at 4 m/s while doing 10: the car in front is doing 6.
	dist := float32(30)
	for range 120 {
		d.trackLead(dist, true, 10, 1.0/60)
		dist -= 4.0 / 60
	}
	if abs(d.leadSpeed-6) > 0.3 {
		t.Errorf("lead speed %.2f, want 6", d.leadSpeed)
	}
}

func TestMirrorsMarkALaneOccupied(t *testing.T) {
	d := driver()
	d.around[vision.MirrorRear] = []Neighbour{{Ahead: -12, Right: 3.5}}
	if !d.mirrorOccupied(laneWidth, 20, 6) {
		t.Error("a car 12m back in the lane to the right should be seen")
	}
	if d.mirrorOccupied(-laneWidth, 20, 6) {
		t.Error("the lane to the left is empty")
	}
	if d.mirrorOccupied(laneWidth, 8, 6) {
		t.Error("a car further back than asked about does not count")
	}
}
