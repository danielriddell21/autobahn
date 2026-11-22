package audio

import (
	"github.com/danielriddell21/autobahn/internal/weather"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/synth"
)

// Sky is the sound of the weather: rain on the roof, and thunder after the
// lightning. It is a second kit beside [Kit], opened after it, because the
// audio device belongs to that one.
type Sky struct {
	live bool
	rain rl.Sound
	rumb rl.Sound
	// last is when the strike whose thunder has been sounded struck.
	last float32
	seen bool
}

// OpenSky synthesises the sounds of the weather. It is silent, and safe to
// use, when there is no audio device, or when the kit was muted.
func OpenSky(k *Kit) *Sky {
	s := &Sky{}
	if k == nil || !k.live {
		return s
	}
	s.live = true
	s.rain = load(rainLoop())
	s.rumb = load(synth.Thud(46, 1.6, 3, 0x7411, 0xbeef))
	rl.SetSoundVolume(s.rain, 0.1)
	rl.SetSoundVolume(s.rumb, 0.8)
	return s
}

// Close releases the sounds. The device is the kit's to shut.
func (s *Sky) Close() {
	if !s.live {
		return
	}
	rl.UnloadSound(s.rain)
	rl.UnloadSound(s.rumb)
	s.live = false
}

// RainVolume is how loud the rain is for an amount of it, from 0 to 1.
func RainVolume(rain float32) float32 { return clamp(rain, 0, 1) * 0.35 }

// Update keeps the rain going at the volume the weather gives it, and sounds
// thunder when the moment comes for a strike's. strike is what
// [weather.Strike] says for this moment, and now is the world's clock: the
// thunder arrives Delay seconds after the flash, from the same strike the
// picture is lit by.
func (s *Sky) Update(st weather.State, strike weather.Flash, struck bool, now float32) {
	if !s.live {
		return
	}
	if vol := RainVolume(st.Rain); vol > 0 {
		rl.SetSoundVolume(s.rain, vol)
		if !rl.IsSoundPlaying(s.rain) {
			rl.PlaySound(s.rain)
		}
	} else if rl.IsSoundPlaying(s.rain) {
		rl.StopSound(s.rain)
	}
	if !s.ready(strike, struck, now) {
		return
	}
	rl.SetSoundVolume(s.rumb, clamp(strike.Strength*(1-strike.Delay/8), 0.15, 1))
	rl.PlaySound(s.rumb)
}

// ready reports whether a strike's thunder is due and has not been sounded,
// and marks it sounded.
func (s *Sky) ready(strike weather.Flash, struck bool, now float32) bool {
	if !struck || now < strike.At+strike.Delay {
		return false
	}
	if s.seen && s.last == strike.At {
		return false
	}
	s.seen, s.last = true, strike.At
	return true
}

func rainLoop() []byte {
	// Rain is bright, steady noise with the lowest of it taken out.
	noise := synth.Noise(0x4a17, 0x9e31)
	var low float64
	return synth.Render(1.5, func(t float64) float64 {
		n := noise()
		low += (n - low) * 0.08
		return (n - low) * 0.5
	})
}
