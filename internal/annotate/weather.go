package annotate

import (
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/weather"
)

// Weather takes away what a camera could not see. Past the weather's sight
// nothing is marked; in the last of it things come and go, so the autopilot
// cannot count on the far end of a row of markers being there from one frame
// to the next; and in rain the far boxes shake a little. Both the live window
// and the evaluation harness lay boxes out here, so they lose the same ones.

const (
	// hazeFrom is the share of the sight at which things start to drop out.
	hazeFrom float32 = 0.6
	// flickerRate is how many times a second a hazy object's fate is redrawn.
	flickerRate float32 = 20
	// jitterReach is the most a far box edge moves in rain, in pixels, and
	// jitterFrom the distance beyond which it moves at all.
	jitterReach float32 = 2
	jitterFrom  float32 = 30
	// keepNear is how near a marker on the car's own lane has to be to be
	// kept whatever the weather.
	keepNear float32 = 30
)

// Salts for what is drawn from a hash of an object and the moment.
const (
	saltDrop   uint64 = 0x2f6e9b1d47c80a53
	saltJitter uint64 = 0x94a7c3e05b1d2f68
)

// Keys name an object for the hash, in blocks that cannot meet.
const (
	keyLane    = 0
	keySide    = 1 << 12
	keyProp    = 1 << 16
	keyWorks   = 1 << 20
	keyVehicle = 1 << 24
)

// clearState is the weather the original layouts are laid out in: nothing to
// take away.
var clearState = weather.State{Sight: weather.Unlimited, Grip: 1}

// clearSky reports whether a weather takes nothing away, in which case a layout
// is exactly the one without weather.
func clearSky(st weather.State) bool { return st.Sight >= 1000 && st.Rain == 0 }

// seen reports whether the camera makes out something at a point, which is
// further than its usual limit allows only if the weather lets it. A thing
// kept is one that is never lost to haze.
func (lo *layout) seen(at mathx.Vec, limit float32, key int, kept bool) bool {
	if !lo.wet {
		return lo.visible(at, limit)
	}
	if !lo.visible(at, min(limit, lo.st.Sight)) {
		return false
	}
	d := at.Sub(lo.eye).Len()
	if kept && d < keepNear {
		return true
	}
	from := hazeFrom * lo.st.Sight
	if d <= from {
		return true
	}
	drop := (d - from) / (lo.st.Sight - from)
	return weather.Hash(uint64(lo.frame), saltDrop, key) >= drop
}

// put adds a box, shaken a little if it is far off and it is raining.
func (lo *layout) put(b Box, ok bool, at mathx.Vec, key int) {
	if !ok {
		return
	}
	if lo.wet && lo.st.Rain > 0 {
		if d := at.Sub(lo.eye).Len(); d > jitterFrom {
			amp := jitterReach * min(lo.st.Rain, 1) * min((d-jitterFrom)/jitterFrom, 1)
			j := func(i int) float32 {
				return (weather.Hash(uint64(lo.frame), saltJitter, key*4+i)*2 - 1) * amp
			}
			n := b
			n.MinX, n.MinY = max(b.MinX+j(0), 0), max(b.MinY+j(1), 0)
			n.MaxX, n.MaxY = min(b.MaxX+j(2), lo.p.w-1), min(b.MaxY+j(3), lo.p.h-1)
			if n.MaxX-n.MinX >= 2 && n.MaxY-n.MinY >= 2 {
				b = n
			}
		}
	}
	lo.out = append(lo.out, b)
}
