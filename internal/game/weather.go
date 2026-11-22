package game

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/weather"
)

// Weather is drawn on top of the season: the same scene under a greyer, closer
// light, a wetter road, rain or sleet in the air and, in a storm at night,
// lightning. None of it is annotated, and none of it is stored: the weather is
// asked of the clock each frame and each piece of rain is worked out from its
// number and the time, as the season's falling leaves are.

// weatherLook is every colour the weather puts in the scene.
type weatherLook struct {
	// sky is the overcast the sky is drawn toward, fog the grey the distance
	// fades to, and flash the white of lightning.
	sky, fog, flash rl.Color
	// rain and sleet are what falls.
	rain, sleet rl.Color
}

// wxLook is the colours the weather draws with. Every channel is short of the
// vision system's full-bright reserved colours, and what is drawn only ever
// goes toward one of these or darker.
var wxLook = weatherLook{
	sky:   rl.NewColor(128, 136, 148, 255),
	fog:   rl.NewColor(158, 166, 176, 255),
	flash: rl.NewColor(222, 228, 240, 255),
	rain:  rl.NewColor(170, 182, 200, 255),
	sleet: rl.NewColor(214, 220, 228, 255),
}

// colours returns every colour the weather puts in the scene, for whoever has
// to be sure none of them is one the vision system has reserved.
func (wl weatherLook) colours() []rl.Color {
	return []rl.Color{wl.sky, wl.fog, wl.flash, wl.rain, wl.sleet}
}

// rewet turns the light of a season and an hour into the light of the weather
// in it: duller, greyer and closer, with the sky overcast, and lit for a
// moment by lightning. It runs after relight. Clear weather comes through
// unchanged.
//
// It only ever takes light away, or mixes toward the colours in [wxLook], so
// nothing it does can run a channel past full.
func rewet(l lighting, st weather.State, flash float32) lighting {
	cloud := mathx.Clamp(max(st.Rain, st.Fog*0.8), 0, 1)
	murk := mathx.Clamp(max(st.Fog, 0.6*st.Rain), 0, 1)
	dim := func(v rl.Vector3, f float32) rl.Vector3 { return vec3(v.X*f, v.Y*f, v.Z*f) }

	if cloud > 0 {
		l.sun = dim(l.sun, 1-0.55*cloud)
		l.sky = dim(l.sky, 1-0.15*cloud)
		// The dark is still the dark, so the greys are darkened with it.
		by := 1 - 0.75*l.night
		l.fog = mix3(l.fog, dim(vec3(float32(wxLook.fog.R)/255, float32(wxLook.fog.G)/255, float32(wxLook.fog.B)/255), by), murk)
		l.skyTop = mixColour(l.skyTop, scaleColor(wxLook.sky, by), 0.8*cloud)
		l.skyLow = mixColour(l.skyLow, scaleColor(wxLook.fog, by), 0.85*mathx.Clamp(max(cloud, st.Fog), 0, 1))
	}
	if st.Sight < weather.Unlimited {
		l.fogRange = min(l.fogRange, 1.3*st.Sight)
	}
	l.ground = dim(l.ground, 1-0.2*st.Wet)
	l.wet = mathx.Clamp(st.Wet, 0, 1)

	// Lightning only shows in the dark: by day it would be lost in the sky.
	if f := mathx.Clamp(flash, 0, 1) * l.night; f > 0 {
		l.sky = mix3(l.sky, vec3(0.7, 0.74, 0.82), 0.7*f)
		l.ground = mix3(l.ground, vec3(0.4, 0.42, 0.46), 0.5*f)
		l.skyTop = mixColour(l.skyTop, wxLook.flash, 0.7*f)
		l.skyLow = mixColour(l.skyLow, wxLook.flash, 0.5*f)
	}
	return l
}

// wetAsphalt is a road surface as the weather leaves it: darker the wetter it
// is, and never brighter.
func wetAsphalt(c rl.Color, wet float32) rl.Color {
	return scaleColor(c, 1-0.3*mathx.Clamp(wet, 0, 1))
}

// flashLevel is how bright a lightning strike is a moment after it, from 0 to
// 1: a hard flash, and a weaker one just behind it.
func flashLevel(f weather.Flash, now float32) float32 {
	age := float64(now - f.At)
	if age < 0 || age > 0.8 {
		return 0
	}
	v := math.Exp(-age * 12)
	if age > 0.22 {
		v += 0.7 * math.Exp(-(age-0.22)*14)
	}
	return mathx.Clamp(float32(v), 0, 1) * f.Strength
}

// Pieces of rain, kept in the same box round the camera as the season's.
const (
	maxRain   = 320
	maxSleet  = 220
	rainSpeed = 15 // metres a second
	rainLen   = 0.55
	sleetSize = 0.05
)

// rainCount is how many pieces are in the air for a weather.
func rainCount(st weather.State) int {
	if st.Sleet {
		return int(maxSleet * st.Rain)
	}
	return int(maxRain * st.Rain)
}

// drawRain draws the rain, or the sleet in winter, round the eye. Every piece
// is where its number and the clock put it.
func (g *Game) drawRain(eye mathx.Vec, st weather.State) {
	n := rainCount(st)
	if n == 0 {
		return
	}
	rl.Normal3f(0, 1, 0)
	now := g.world.Time
	if st.Sleet {
		for i := range n {
			x, y, z := fallAt(i+1000, now, eye, 3.2)
			y += g.land.at(mathx.V(x, z))
			rl.DrawCube(vec3(x, y, z), sleetSize, sleetSize, sleetSize, wxLook.sleet)
		}
		return
	}
	lean := 0.35 * st.Wind // metres a streak leans at full wind
	for i := range n {
		x, y, z := fallAt(i+1000, now, eye, rainSpeed)
		y += g.land.at(mathx.V(x, z))
		a, b := vec3(x, y, z), vec3(x+lean, y+rainLen, z)
		rl.DrawLine3D(a, b, wxLook.rain)
	}
}
