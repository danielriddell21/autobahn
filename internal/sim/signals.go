package sim

import (
	"github.com/danielriddell21/autobahn/internal/city"
)

// SignalState is the aspect a traffic light is showing.
type SignalState int

// The signal aspects. British signals show red and amber together before
// green, which warns drivers to prepare to move without permitting them to
// cross the line yet.
const (
	SignalRed SignalState = iota
	SignalRedAmber
	SignalAmber
	SignalGreen
)

// String returns the aspect name.
func (s SignalState) String() string {
	switch s {
	case SignalGreen:
		return "GREEN"
	case SignalAmber:
		return "AMBER"
	case SignalRedAmber:
		return "RED+AMBER"
	default:
		return "RED"
	}
}

// Signal phase durations in seconds.
const (
	redAmberTime float32 = 1.8
	greenTime    float32 = 11.5
	amberTime    float32 = 2.8
	allRedTime   float32 = 1.4
	halfCycle            = redAmberTime + greenTime + amberTime + allRedTime
	fullCycle            = 2 * halfCycle
)

// Signals drives the traffic lights across the whole city. Each signalised
// junction runs the same fixed cycle with its own offset, so the city does not
// pulse in unison.
type Signals struct {
	city  *city.City
	seed  uint64
	clock float32
}

// NewSignals builds the controller for a city's signalised junctions. It holds
// no per-junction state, so junctions that appear later are driven the moment
// they exist.
func NewSignals(c *city.City, seed uint64) *Signals {
	return &Signals{city: c, seed: seed}
}

// signalSalt keeps a junction's place in the cycle from correlating with the
// other decisions taken about it.
const signalSalt = 0x7f4a

// offsetOf returns how far through its cycle a junction's signals are.
//
// It is worked out from where the junction is rather than drawn once at
// startup, because in a city that grows there is no startup: a junction
// reached an hour into a drive has to be showing what it would have been
// showing all along.
func (s *Signals) offsetOf(node int) float32 {
	if node < 0 || node >= len(s.city.Nodes) {
		return 0
	}
	return s.city.Nodes[node].Chance(s.seed, signalSalt) * fullCycle
}

// Update advances the signal clock by dt seconds.
func (s *Signals) Update(dt float32) { s.clock += dt }

// State returns the aspect shown to the given phase group at a node. Group 0
// is the X-axis approaches and group 1 the Z-axis approaches.
func (s *Signals) State(node, group int) SignalState {
	if node < 0 || node >= len(s.city.Nodes) || !s.city.Nodes[node].Signalised {
		return SignalGreen // unsignalised junctions never show an aspect
	}
	t := mod(s.clock+s.offsetOf(node), fullCycle)
	// The first half of the cycle serves group 0, the second half group 1.
	serving := 0
	if t >= halfCycle {
		serving, t = 1, t-halfCycle
	}
	if serving != group {
		return SignalRed
	}
	switch {
	case t < redAmberTime:
		return SignalRedAmber
	case t < redAmberTime+greenTime:
		return SignalGreen
	case t < redAmberTime+greenTime+amberTime:
		return SignalAmber
	default:
		return SignalRed
	}
}

// LaneState returns the aspect governing a lane's approach to its junction.
func (s *Signals) LaneState(l *city.Lane) SignalState {
	if l.Control != city.ControlSignal {
		return SignalGreen
	}
	return s.State(l.ToNode, l.Group)
}

func mod(a, m float32) float32 {
	r := a - m*float32(int(a/m))
	if r < 0 {
		r += m
	}
	return r
}

// MustStop reports whether an aspect forbids crossing the stop line. Red and
// amber both do; red-and-amber does too, since it only means "prepare to go".
func (s SignalState) MustStop() bool { return s != SignalGreen }

// SetClock forces the signal clock, so a joining machine shows the same aspect
// at the same moment as the host without either simulating the other's timers.
func (s *Signals) SetClock(t float32) { s.clock = t }
