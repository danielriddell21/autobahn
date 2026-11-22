package game

import "github.com/danielriddell21/autobahn/internal/sim"

// Seat is where a player sits in a car two people share.
type Seat uint8

const (
	// SeatWheel steers and works the handbrake. The host sits here.
	SeatWheel Seat = iota
	// SeatPedals works the throttle, brake, reverse and nitrous, and the
	// indicators. The joiner sits here.
	SeatPedals
)

// String names the seat for the HUD.
func (s Seat) String() string {
	if s == SeatPedals {
		return "PEDALS"
	}
	return "WHEEL"
}

// split makes one set of controls out of two people's. seat is where the local
// player sits: they supply their own half and the remote player the other.
//
// A remote player who is not there is passed as the zero Input, which zeroes
// their half and only their half: a missing pedal player leaves the car
// coasting, a missing driver leaves it going straight.
func split(seat Seat, local, remote Input) sim.Controls {
	wheel, pedals := local, remote
	if seat == SeatPedals {
		wheel, pedals = remote, local
	}
	return sim.Controls{
		Steer:     wheel.Steer,
		Handbrake: wheel.Handbrake,
		Throttle:  pedals.Throttle,
		Brake:     pedals.Brake,
		Reverse:   pedals.Reverse,
		Boost:     pedals.Boost,
	}
}

// controlsOf is every control in an Input, for a car one player drives alone.
func controlsOf(in Input) sim.Controls {
	return sim.Controls{
		Throttle: in.Throttle, Brake: in.Brake, Steer: in.Steer,
		Handbrake: in.Handbrake, Reverse: in.Reverse, Boost: in.Boost,
	}
}

// inputOf is a set of controls as they travel, with a turn signal.
func inputOf(c sim.Controls, ind sim.Indicator, auto bool) Input {
	return Input{
		Throttle: c.Throttle, Brake: c.Brake, Steer: c.Steer,
		Handbrake: c.Handbrake, Reverse: c.Reverse, Boost: c.Boost,
		Indicator: int8(ind), Auto: auto,
	}
}

// settle turns a remote player's brake into reverse once the car they are
// working has stopped, as the S key does for a player at their own keyboard.
// The host does it, because the host knows how fast the car is really going;
// the joiner only sees a pose that has been eased toward it.
func settle(in Input, forward float32) Input {
	if in.Auto || in.Reverse || in.Throttle > 0 || in.Brake <= 0 || forward >= 0.6 {
		return in
	}
	in.Reverse, in.Throttle, in.Brake = true, 1, 0
	return in
}

// indicatorOf reads a turn signal off the wire, ignoring a value it does not
// know.
func indicatorOf(i int8) sim.Indicator {
	switch sim.Indicator(i) {
	case sim.IndicatorLeft:
		return sim.IndicatorLeft
	case sim.IndicatorRight:
		return sim.IndicatorRight
	}
	return sim.IndicatorOff
}

// indicatorFlags is a turn signal as pose flags.
func indicatorFlags(i sim.Indicator) uint8 {
	switch i {
	case sim.IndicatorLeft:
		return FlagIndicateLeft
	case sim.IndicatorRight:
		return FlagIndicateRight
	}
	return 0
}

// indicatorIn is the turn signal a pose's flags carry.
func indicatorIn(flags uint8) sim.Indicator {
	switch {
	case flags&FlagIndicateLeft != 0:
		return sim.IndicatorLeft
	case flags&FlagIndicateRight != 0:
		return sim.IndicatorRight
	}
	return sim.IndicatorOff
}
