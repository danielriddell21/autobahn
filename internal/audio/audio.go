// Package audio gives the game its voice.
//
// Every sound is synthesised at startup from crucible's synth package — there
// are no audio files, just as there are no models or textures. synth renders
// 16-bit stereo PCM and wraps it in a WAV header with [synth.WAV], because a
// file in memory is the one thing raylib will load that is not on disk.
//
// A [Kit] with no audio device behaves as a working kit that happens to be
// silent, so callers never have to ask whether sound is available.
package audio

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/synth"
)

// Engine note limits, in multiples of the recorded pitch.
const (
	enginePitchIdle float32 = 0.62
	enginePitchMax  float32 = 2.35
	// engineTop is the speed at which the note reaches its highest pitch.
	engineTop float32 = 42
	// sirenRange is how far a unit's siren carries, in metres.
	sirenRange float32 = 130
)

// Kit holds the game's sounds and the state needed to keep the continuous
// ones running.
type Kit struct {
	live   bool
	muted  bool
	engine rl.Sound
	siren  rl.Sound
	skid   rl.Sound
	thud   rl.Sound
}

// Open starts the audio device and synthesises the kit. It returns a silent
// kit when no device is available, which is the normal case on a headless
// machine, so the caller has nothing to handle.
func Open(muted bool) *Kit {
	k := &Kit{muted: muted}
	if muted {
		return k
	}
	rl.InitAudioDevice()
	if !rl.IsAudioDeviceReady() {
		return k
	}
	k.live = true

	k.engine = load(engineLoop())
	k.siren = load(sirenCall())
	k.skid = load(skidLoop())
	k.thud = load(synth.Thud(70, 0.45, 9, 0x5eed, 0xf00d))

	rl.SetSoundVolume(k.engine, 0.30)
	rl.SetSoundVolume(k.siren, 0.45)
	rl.SetSoundVolume(k.skid, 0.35)
	rl.SetSoundVolume(k.thud, 0.75)
	return k
}

// Close releases the sounds and shuts the device down.
func (k *Kit) Close() {
	if !k.live {
		return
	}
	for _, s := range []rl.Sound{k.engine, k.siren, k.skid, k.thud} {
		rl.UnloadSound(s)
	}
	rl.CloseAudioDevice()
	k.live = false
}

// Engine holds the engine note, raising its pitch with road speed and its
// volume with throttle. It is called every frame.
func (k *Kit) Engine(speed, throttle float32) {
	if !k.live {
		return
	}
	// The note is a short loop retriggered as it runs out, so the pitch can be
	// changed continuously without streaming.
	rev := clamp(speed/engineTop, 0, 1)
	rl.SetSoundPitch(k.engine, enginePitchIdle+(enginePitchMax-enginePitchIdle)*rev)
	rl.SetSoundVolume(k.engine, 0.16+0.24*clamp(throttle, 0, 1)+0.10*rev)
	if !rl.IsSoundPlaying(k.engine) {
		rl.PlaySound(k.engine)
	}
}

// Siren sounds a police two-tone. bearing is the direction to the nearest unit
// relative to straight ahead, in radians, so the siren arrives from the side
// the car is actually on. A distance beyond the siren's range silences it.
func (k *Kit) Siren(distance, bearing float32) {
	if !k.live {
		return
	}
	if distance > sirenRange {
		if rl.IsSoundPlaying(k.siren) {
			rl.StopSound(k.siren)
		}
		return
	}
	// raylib wants a position from -1 (left) through 0 (centre) to 1 (right),
	// which is the same left-right axis synth.Pan folds a bearing onto. Passing
	// one of Pan's channel gains instead would put a siren dead ahead hard over
	// to one side, and reverse the two that matter.
	rl.SetSoundPan(k.siren, clamp(sin(float64(bearing)), -1, 1))
	rl.SetSoundVolume(k.siren, 0.15+0.45*(1-clamp(distance/sirenRange, 0, 1)))
	if !rl.IsSoundPlaying(k.siren) {
		rl.PlaySound(k.siren)
	}
}

// Skid plays tyre noise while the rear axle is sliding. slip is the rear slip
// angle in radians.
func (k *Kit) Skid(slip, speed float32) {
	if !k.live {
		return
	}
	sliding := slip > 0.18 && speed > 3
	if !sliding {
		if rl.IsSoundPlaying(k.skid) {
			rl.StopSound(k.skid)
		}
		return
	}
	rl.SetSoundVolume(k.skid, clamp((slip-0.18)*1.6, 0.1, 0.55))
	if !rl.IsSoundPlaying(k.skid) {
		rl.PlaySound(k.skid)
	}
}

// Impact plays a collision, scaled by how hard it was in metres per second.
func (k *Kit) Impact(severity float32) {
	if !k.live {
		return
	}
	rl.SetSoundVolume(k.thud, clamp(severity/14, 0.2, 1))
	rl.SetSoundPitch(k.thud, clamp(1.3-severity/26, 0.7, 1.3))
	rl.PlaySound(k.thud)
}

func engineLoop() []byte {
	// A four-stroke note is mostly a low fundamental with a rough harmonic on
	// top. Drone gives the beating between voices that stops it sounding like
	// a test tone.
	return synth.Drone([]synth.Voice{
		{Freq: 62, Amp: 0.55},
		{Freq: 93, Amp: 0.28},
		{Freq: 124, Amp: 0.16},
		{Freq: 187, Amp: 0.07},
	}, 6.5, 0.03, 0.5)
}

func sirenCall() []byte {
	// The British two-tone: a pair of pitches alternating on about a half
	// second, rather than the continuous wail used elsewhere.
	return synth.TwoTone(760, 570, 0.42, 1.7, 0.0)
}

func skidLoop() []byte {
	// Filtered noise, which is close enough to a tyre letting go.
	noise := synth.Noise(0xabcd, 0x1234)
	var last float64
	return synth.Render(0.5, func(t float64) float64 {
		// A one-pole low pass takes the hiss off and leaves a scrub.
		last += (noise() - last) * 0.22
		return last * 0.9
	})
}

func load(pcm []byte) rl.Sound {
	// raylib will only take an encoded file from memory, so the raw PCM gets a
	// RIFF header in front of it.
	w := synth.WAV(pcm)
	wave := rl.LoadWaveFromMemory(".wav", w, int32(len(w)))
	s := rl.LoadSoundFromWave(wave)
	rl.UnloadWave(wave)
	return s
}

func clamp(v, lo, hi float32) float32 { return min(max(v, lo), hi) }

func sin(v float64) float32 { return float32(math.Sin(v)) }
