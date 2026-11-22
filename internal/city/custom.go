package city

import (
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// A custom map is one of the four presets with some of its numbers moved. This
// file says how far they may be moved.
//
// Every supported range runs from the lowest to the highest value any preset
// uses, which is the ground the gate has already measured the autopilot on;
// the edge profile in tools/eval drives every range at its hardest bound at
// once. testdata/ranges.json holds the same table, so widening a range means
// editing that file in the same change, where a reviewer will see it.
//
// Nothing here is the generator's business: Clamp only decides which Params
// are fit to be handed to it.

// Range is the supported span of one generation parameter.
type Range struct {
	Name string  `json:"name"`
	Lo   float64 `json:"lo"`
	Hi   float64 `json:"hi"`
}

// maxHillSlope is the steepest hill the village has: its hills over its span.
// The road ahead curving up or down is what throws the autopilot, and it did
// worse on the motorway's long sight lines only when swells were short (see
// CLAUDE.md), so one rule holds every combination to the village's slope.
const maxHillSlope = 8.0 / 850.0

// ranges are the supported ranges, and hard are the outer bounds Clamp keeps
// even for an unsupported map, because past them the generator itself is not
// safe to run. Lengths are metres; the speed limit is in miles per hour.
var ranges = []Range{
	{Name: "minSpan", Lo: 70, Hi: 170},
	{Name: "maxSpan", Lo: 120, Hi: 290},
	{Name: "roundabouts", Lo: 0.10, Hi: 0.28},
	{Name: "parks", Lo: 0.10, Hi: 0.42},
	{Name: "rise", Lo: 0, Hi: 1},
	{Name: "floor", Lo: 5.5, Hi: 7},
	{Name: "storeys", Lo: 4.5, Hi: 10},
	{Name: "mainEvery", Lo: 2, Hi: 5},
	{Name: "limit", Lo: 0, Hi: 50},
	{Name: "wind", Lo: 0, Hi: 40},
	{Name: "hills", Lo: 0, Hi: 11},
	{Name: "hillSpan", Lo: 700, Hi: 2600},
	{Name: "bluff", Lo: 0, Hi: 20},
}

var hard = map[string][2]float64{
	"minSpan":     {40, 400},
	"maxSpan":     {60, 600},
	"roundabouts": {0, 1},
	"parks":       {0, 0.8},
	"rise":        {0, 1.5},
	"floor":       {3, 12},
	"storeys":     {0, 30},
	"mainEvery":   {1, 8},
	"limit":       {0, 70},
	"wind":        {0, 80},
	"hills":       {0, 40},
	"hillSpan":    {300, 8000},
	"bluff":       {0, 60},
}

// Ranges returns the supported ranges, in a stable order.
func Ranges() []Range {
	return append([]Range(nil), ranges...)
}

// RangeOf returns the supported range of a parameter by name.
func RangeOf(name string) Range {
	for _, r := range ranges {
		if r.Name == name {
			return r
		}
	}
	return Range{}
}

// minGap is how much longer MaxSpan has to be than MinSpan for a block to have
// a span to be drawn from at all.
const minGap = 50

// Clamp brings custom parameters to ground the generator can be trusted on.
//
// Without allowOutside every value is pulled into its supported range and the
// result is always supported. With it a value past a supported range is kept
// as long as the generator can still run on it; either way the second result
// reports whether the returned parameters lie outside the supported ranges,
// which is what a code is flagged and a HUD tagged "unsupported" for.
func Clamp(p Params, allowOutside bool) (Params, bool) {
	outside := false
	f := func(v *float32, name string, scale func(float64) float32) {
		r, h := RangeOf(name), hard[name]
		lo, hi := scale(r.Lo), scale(r.Hi)
		if *v >= lo && *v <= hi {
			return
		}
		if allowOutside {
			*v = min(max(*v, scale(h[0])), scale(h[1]))
			outside = outside || *v < lo || *v > hi
			return
		}
		*v = min(max(*v, lo), hi)
	}
	plain := func(x float64) float32 { return float32(x) }
	mph := func(x float64) float32 { return mathx.MPH(float32(x)) }

	f(&p.MinSpan, "minSpan", plain)
	f(&p.MaxSpan, "maxSpan", plain)
	// A block's span is drawn between the two, so they cannot cross.
	p.MaxSpan = max(p.MaxSpan, p.MinSpan+minGap)
	f(&p.Roundabouts, "roundabouts", plain)
	f(&p.Parks, "parks", plain)
	f(&p.Rise, "rise", plain)
	f(&p.Floor, "floor", plain)
	f(&p.Storeys, "storeys", plain)
	f(&p.Wind, "wind", plain)
	f(&p.Bluff, "bluff", plain)
	for i := range p.Limits {
		if p.Limits[i] > 0 || allowOutside {
			f(&p.Limits[i], "limit", mph)
		}
	}

	every := float32(p.MainEvery)
	f(&every, "mainEvery", plain)
	p.MainEvery = int(every)

	f(&p.Hills, "hills", plain)
	if p.Hills > 0 {
		if p.HillSpan <= 0 {
			p.HillSpan = float32(RangeOf("hillSpan").Lo)
		}
		f(&p.HillSpan, "hillSpan", plain)
		// One combined rule on top of the ranges: the slope.
		if float64(p.Hills) > maxHillSlope*float64(p.HillSpan)*(1+1e-6) {
			if allowOutside {
				outside = true
			} else {
				p.HillSpan = float32(float64(p.Hills) / maxHillSlope)
			}
		}
	} else {
		p.Hills = 0
	}
	return p, outside
}
