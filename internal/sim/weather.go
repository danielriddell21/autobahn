package sim

import (
	"math"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/weather"
)

// gripStep is how coarsely the weather's grip is taken. The weather changes
// smoothly, and a car's profile is refitted only when the figure moves a step,
// so a frame in which nothing changed costs nothing.
const gripStep float32 = 0.05

// QuantiseGrip rounds a grip to the step the world refits cars at.
func QuantiseGrip(g float32) float32 {
	return float32(math.Round(float64(g/gripStep))) * gripStep
}

// Climate is the weather a world is driven in, and what it has done to the
// cars in it. It stores no weather: the weather is [weather.At] of the clock.
type Climate struct {
	Seed    uint64
	Season  city.Season
	Setting weather.Setting

	// applied is the grip every vehicle was last fitted to, or zero before the
	// first fitting.
	applied float32
	// scale multiplies the season's grip, for a custom map; zero is one.
	scale float32
}

// roadGrip is the grip the season gives, before any weather.
func (c *Climate) roadGrip() float32 {
	g := SeasonGrip(c.Season)
	if c.scale > 0 {
		g *= c.scale
	}
	return g
}

// NewClimate returns the climate of a map.
func NewClimate(seed uint64, m city.Map, set weather.Setting) Climate {
	return Climate{Seed: seed, Season: m.Season(), Setting: set}
}

// Weather returns the weather at a moment on the world's clock.
func (c *Climate) Weather(t float32) weather.State {
	return weather.At(c.Seed, c.Season, c.Setting, t)
}

// GripAt returns the grip the road gives at a moment: the worse of the
// season's and the weather's, in steps.
func (c *Climate) GripAt(t float32) float32 {
	return min(c.roadGrip(), QuantiseGrip(c.Weather(t).Grip))
}

// Grip returns the grip every vehicle is currently fitted to, which is what a
// car spawned or changed now should be fitted to as well.
func (c *Climate) Grip() float32 {
	if c.applied == 0 {
		return c.roadGrip()
	}
	return c.applied
}

// Apply fits the player's car and every agent to the road the weather has
// made, when it has changed, and reports whether it did. Called every frame it
// costs one weather lookup and a comparison.
func (c *Climate) Apply(w *World) bool {
	g := c.GripAt(w.Time)
	if g == c.applied {
		return false
	}
	c.applied = g
	w.Player.Spec = w.PlayerBody.Spec.OnSurface(g)
	for _, a := range w.Agents {
		a.surface = g
		a.V.Spec = a.Body.Spec.OnSurface(g)
	}
	return true
}
