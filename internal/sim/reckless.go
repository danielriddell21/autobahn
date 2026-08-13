package sim

import (
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// RecklessControls drives the player's car along its navigation route at full
// throttle, taking no notice of limits, signals or anything else.
//
// It exists to exercise the parts of the game that only appear when somebody
// drives badly: the judge's infractions and the police response. It steers, and
// nothing more — every fault it collects is earned through the ordinary rules,
// with the judge scoring it exactly as it scores a human.
func RecklessControls(w *World) Controls {
	speed := max(w.Player.ForwardSpeed(), 0)

	// Look far enough ahead to stay on the road at speed, but not so far that
	// it cuts corners at a junction.
	look := mathx.Clamp(7+0.75*speed, 8, 20)
	pts := w.Route.Centreline(look, 1, 1)
	if len(pts) == 0 {
		return Controls{Throttle: 0.3}
	}

	rel := pts[0].Sub(w.Player.Pos)
	fwd := w.Player.Forward()
	alpha := mathx.Atan2(rel.Dot(fwd.Right()), rel.Dot(fwd))
	angle := mathx.Atan2(2*w.Player.Spec.Wheelbase()*mathx.Sin(alpha), max(rel.Len(), 1))

	c := Controls{
		Throttle: 1,
		Steer:    mathx.Clamp(angle/w.Player.Spec.MaxSteer, -1, 1),
	}

	// Slow for a turn. This is not obedience — signals, signs and limits are
	// still ignored — it is only enough car control to stay on the road, since
	// a car wrapped around a building stops exercising anything.
	corner := mathx.MPH(22)
	if kind, ok := w.Route.NextTurn(); ok && kind != city.TurnStraight {
		if d := w.Route.DistanceToJunction(); d < 34 && speed > corner {
			c.Throttle, c.Brake = 0, mathx.Clamp((speed-corner)/6, 0.25, 1)
		}
	}
	if mathx.Abs(alpha) > 0.45 && speed > corner {
		c.Throttle, c.Brake = 0, 0.6
	}
	return c
}
