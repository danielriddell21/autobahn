package audio

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/synth"
)

// Cue is one of the menus' sounds.
type Cue int

const (
	// CueMove is the selection moving, or a setting changing.
	CueMove Cue = iota
	// CueSelect is an action running.
	CueSelect
	// CueBack is going back a page.
	CueBack
	// CueDeny is a press that did nothing, on a row that is locked.
	CueDeny

	cueCount
)

// deviceGen counts the times a [Kit] has opened or closed the audio device,
// so a [UI] knows the sounds it loaded went with the device they were made for.
var deviceGen int

// UI plays the menus' cues. It is separate from the [Kit] because the menus
// are on screen when no drive is: it opens the audio device itself if none is
// open, and lets go of the sounds it made if a drive closes the device from
// under it. With nothing to play to, or muted, every call does nothing.
type UI struct {
	muted bool
	owns  bool // this UI opened the device
	gen   int  // deviceGen when the sounds were loaded
	ready bool // sounds is loaded for the device now open
	snd   [cueCount]rl.Sound
}

// OpenUI makes the menus' sound set. It opens nothing until the first cue is
// played, and a muted set never does.
func OpenUI(muted bool) *UI { return &UI{muted: muted} }

// Muted reports whether the set will stay silent.
func (u *UI) Muted() bool { return u == nil || u.muted }

// Play makes the sound for c, if there is anything to hear it with.
func (u *UI) Play(c Cue) {
	if u.Muted() || c < 0 || c >= cueCount || !u.load() {
		return
	}
	rl.PlaySound(u.snd[c])
}

// load makes sure the sounds exist for the device that is open, opening it
// first if it must. It reports whether they do.
func (u *UI) load() bool {
	if u.ready && u.gen == deviceGen && rl.IsAudioDeviceReady() {
		return true
	}
	u.ready = false
	if !rl.IsAudioDeviceReady() {
		rl.InitAudioDevice()
		if !rl.IsAudioDeviceReady() {
			return false
		}
		u.owns = true
	}
	for c, pcm := range cuePCM() {
		u.snd[c] = load(pcm)
		rl.SetSoundVolume(u.snd[c], cueVolume[c])
	}
	u.gen, u.ready = deviceGen, true
	return true
}

// cueVolume is how loud each cue is, kept under the drive's own sounds.
var cueVolume = [cueCount]float32{CueMove: 0.35, CueSelect: 0.5, CueBack: 0.4, CueDeny: 0.45}

// cuePCM synthesises the cues: a tick to move, a rising pair to select, a
// falling one to go back and a low dull note to refuse.
func cuePCM() [cueCount][]byte {
	return [cueCount][]byte{
		CueMove:   synth.Blip(1320, 0.05, 70),
		CueSelect: synth.Arpeggio([]float64{880, 1320}, 0.045, 0.16, 28),
		CueBack:   synth.Arpeggio([]float64{990, 660}, 0.045, 0.16, 28),
		CueDeny:   synth.Blip(160, 0.12, 22),
	}
}

// Close frees the sounds, and the device if this set opened it and nobody
// else has taken it over since.
func (u *UI) Close() {
	if u == nil || !u.ready {
		return
	}
	if rl.IsAudioDeviceReady() && u.gen == deviceGen {
		for _, s := range u.snd {
			rl.UnloadSound(s)
		}
		if u.owns {
			rl.CloseAudioDevice()
			deviceGen++
		}
	}
	u.ready, u.owns = false, false
}
