package sim

import (
	"math/rand/v2"

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
	offsets map[int]float32
	clock   float32
}

// NewSignals builds the controller for every signalised node in c.
func NewSignals(c *city.City, rng *rand.Rand) *Signals {
	s := &Signals{offsets: make(map[int]float32)}
	for _, n := range c.Nodes {
		if n.Signalised {
			s.offsets[n.ID] = rng.Float32() * fullCycle
		}
	}
	return s
}

// Update advances the signal clock by dt seconds.
func (s *Signals) Update(dt float32) { s.clock += dt }

// State returns the aspect shown to the given phase group at a node. Group 0
// is the X-axis approaches and group 1 the Z-axis approaches.
func (s *Signals) State(node, group int) SignalState {
	offset, ok := s.offsets[node]
	if !ok {
		return SignalGreen // unsignalised junctions never show an aspect
	}
	t := mod(s.clock+offset, fullCycle)
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
