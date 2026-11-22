package game

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Fireworks are worked out from the clock alone. A burst has a number, and from
// its number and the time come where it is, how old it is and where each spark
// has got to, so nothing about one is kept and every player sees the same sky.
const (
	// burstEvery is how often each of a venue's launch sites lets one off, in
	// seconds, and burstLife how long a burst lasts.
	burstEvery float32 = 6
	burstLife  float32 = 3.4
	burstCount         = 4
	sparkCount         = 20
	// sparkSpeed is how fast a spark leaves the middle of its burst, in metres a
	// second, and sparkFall how fast gravity takes it down.
	sparkSpeed float32 = 11
	sparkFall  float32 = 4.5
)

// drawBursts draws a venue's fireworks over the block, in its own coordinates.
// salt makes one venue's bursts differ from the next.
func drawBursts(half mathx.Vec, now float32, salt int) {
	for k := range burstCount {
		at := now + float32(k)*burstEvery/burstCount
		age := float32(math.Mod(float64(at), float64(burstEvery)))
		if age > burstLife {
			continue
		}
		serial := int(at/burstEvery) + salt + k*7
		pos := vec3(
			half.X*0.6*float32(math.Sin(float64(serial)*2.3)),
			34+float32(k)*4,
			half.Z*0.6*float32(math.Cos(float64(serial)*1.7)),
		)
		col := sparkColour(serial, age/burstLife)
		for i := range sparkCount {
			a := float64(i) * 2.399963 // the golden angle spreads sparks over the shell
			up := 1 - 2*(float64(i)+0.5)/sparkCount
			ring := math.Sqrt(1 - up*up)
			x := pos.X + float32(ring*math.Cos(a))*sparkSpeed*age
			y := pos.Y + float32(up)*sparkSpeed*age - sparkFall*age*age/2
			z := pos.Z + float32(ring*math.Sin(a))*sparkSpeed*age
			rl.DrawCube(vec3(x, y, z), 0.35, 0.35, 0.35, as(col, matGlow))
		}
	}
}

// sparkColour is the colour of a burst a fraction of the way through its life.
// A spark only ever gets dimmer, so it is never brighter than it was defined.
func sparkColour(serial int, life float32) rl.Color {
	base := eventSparks[((serial%len(eventSparks))+len(eventSparks))%len(eventSparks)]
	fade := mathx.Clamp(1-life, 0, 1)
	return rl.NewColor(uint8(float32(base.R)*fade), uint8(float32(base.G)*fade), uint8(float32(base.B)*fade), 255)
}
