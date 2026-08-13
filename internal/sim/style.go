package sim

import (
	"math/rand/v2"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Style is a driver personality. Every parameter the car-following model uses
// comes from here, so two cars in the same queue behave visibly differently:
// one leaves a gap and lifts early for an amber, the next sits close and takes
// it.
//
// The player is not styled. A human drives however they like, and the autopilot
// has its own controller.
type Style struct {
	// Name is what the HUD and the debug output call this style.
	Name string
	// SpeedBias multiplies the posted limit to give the driver's target speed.
	SpeedBias float32
	// Headway is the time gap they want to the car in front, in seconds.
	Headway float32
	// MinGap is the bumper-to-bumper gap they keep at a standstill, in metres.
	MinGap float32
	// Accel and Decel are the rates they find comfortable, in m/s^2.
	Accel, Decel float32
	// GapAcceptance scales how much room they need before pulling out of a
	// junction. Below one they will take a gap a careful driver would not.
	GapAcceptance float32
	// RunsAmber makes them carry on through an amber they could have stopped
	// for.
	RunsAmber bool
	// CornerBias scales how much they slow for a turn.
	CornerBias float32
}

// The stock driving styles, from the driver who holds everyone up to the one
// filling your mirrors.
func Cautious() Style {
	return Style{
		Name: "cautious", SpeedBias: 0.82, Headway: 2.1, MinGap: 3.4,
		Accel: 1.5, Decel: 2.4, GapAcceptance: 1.5, CornerBias: 0.78,
	}
}

// Normal returns the ordinary driver, and is the reference the others vary from.
func Normal() Style {
	return Style{
		Name: "normal", SpeedBias: 0.97, Headway: 1.4, MinGap: 2.6,
		Accel: 2.3, Decel: 3.1, GapAcceptance: 1.0, CornerBias: 1.0,
	}
}

// Brisk returns a driver who presses on but stays within the rules.
func Brisk() Style {
	return Style{
		Name: "brisk", SpeedBias: 1.08, Headway: 1.05, MinGap: 2.2,
		Accel: 3.0, Decel: 3.8, GapAcceptance: 0.82, CornerBias: 1.12,
	}
}

// Aggressive returns the tailgater: quick, close, and willing to chance an
// amber.
func Aggressive() Style {
	return Style{
		Name: "aggressive", SpeedBias: 1.22, Headway: 0.75, MinGap: 1.7,
		Accel: 3.6, Decel: 4.4, GapAcceptance: 0.6, RunsAmber: true,
		CornerBias: 1.28,
	}
}

// Pursuit returns the style a police car drives on a blue-light run. It is not
// handed out to ambient traffic.
func Pursuit() Style {
	return Style{
		Name: "pursuit", SpeedBias: 1.75, Headway: 0.7, MinGap: 2.0,
		Accel: 4.4, Decel: 5.2, GapAcceptance: 0.45, RunsAmber: true,
		CornerBias: 1.35,
	}
}

// styleWeights is the mix of styles in ambient traffic, out of 100. Most
// drivers are unremarkable; a few are not.
var styleWeights = []struct {
	style  func() Style
	weight int
}{
	{Cautious, 18},
	{Normal, 46},
	{Brisk, 26},
	{Aggressive, 10},
}

// Styles returns the ambient traffic styles, in order.
func Styles() []Style {
	out := make([]Style, 0, len(styleWeights))
	for _, w := range styleWeights {
		out = append(out, w.style())
	}
	return out
}

// RandomStyle draws a style from the ambient traffic mix, then varies it
// slightly so no two drivers of the same style are identical.
func RandomStyle(rng *rand.Rand) Style {
	total := 0
	for _, w := range styleWeights {
		total += w.weight
	}
	pick := rng.IntN(total)
	for _, w := range styleWeights {
		if pick -= w.weight; pick < 0 {
			return w.style().jitter(rng)
		}
	}
	return Normal().jitter(rng)
}

func (s Style) jitter(rng *rand.Rand) Style {
	vary := func(v, spread float32) float32 {
		return v * (1 + (rng.Float32()*2-1)*spread)
	}
	s.SpeedBias = vary(s.SpeedBias, 0.07)
	s.Headway = vary(s.Headway, 0.12)
	s.Accel = vary(s.Accel, 0.1)
	return s
}

// TargetSpeed returns the speed this driver wants on a road with the given
// limit, in metres per second.
func (s Style) TargetSpeed(limit float32) float32 { return limit * s.SpeedBias }

// CornerSpeed returns the speed this driver takes a junction turn at.
func (s Style) CornerSpeed() float32 {
	return mathx.Clamp(mathx.MPH(15)*s.CornerBias, mathx.MPH(8), mathx.MPH(28))
}

// StopsForAmber reports whether the driver would pull up for an amber seen at
// the given distance and speed, rather than carry on through it.
func (s Style) StopsForAmber(dist, speed float32) bool {
	if s.RunsAmber {
		return false
	}
	// Only stop if there is room to do it comfortably.
	return dist > speed*speed/(2*s.Decel)
}
