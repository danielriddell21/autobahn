package audio

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/synth"
)

// Night is the sound of the evening events: the murmur of a crowd, and the bang
// of a firework. There are no people in the game, so this is all there is of
// them. It is a third kit beside [Kit] and [Sky], opened after the first,
// because the audio device belongs to that one.
type Night struct {
	live  bool
	crowd rl.Sound
	bang  rl.Sound
	// slot is the launch whose bang has been sounded.
	slot int
	seen bool
}

// OpenNight synthesises the sounds of the evening. It is silent, and safe to
// use, when there is no audio device, or when the kit was muted.
func OpenNight(k *Kit) *Night {
	n := &Night{}
	if k == nil || !k.live {
		return n
	}
	n.live = true
	n.crowd = load(crowdLoop())
	n.bang = load(synth.Thud(190, 0.9, 5, 0x3a91, 0x70c5))
	rl.SetSoundVolume(n.crowd, 0.1)
	rl.SetSoundVolume(n.bang, 0.5)
	return n
}

// Close releases the sounds. The device is the kit's to shut.
func (n *Night) Close() {
	if n == nil || !n.live {
		return
	}
	rl.UnloadSound(n.crowd)
	rl.UnloadSound(n.bang)
	n.live = false
}

// CrowdVolume is how loud a crowd is for how near the player is to it, from 0 to
// 1: nothing at all beyond the reach, and the most it will be at the middle.
func CrowdVolume(near float32) float32 { return clamp(near, 0, 1) * 0.4 }

// Update keeps the crowd going at a level from 0 to 1 and sounds the bang of a
// firework when a new launch comes round. pan is the way the nearest event lies,
// as Kit.Siren's is, and slot numbers the launches, so each is sounded once
// however often this is called. fireworks is how near the player is to one, from
// 0 to 1.
func (n *Night) Update(crowd, fireworks, pan float32, slot int) {
	if n == nil || !n.live {
		return
	}
	if vol := CrowdVolume(crowd); vol > 0 {
		rl.SetSoundPan(n.crowd, clamp(pan, -1, 1))
		rl.SetSoundVolume(n.crowd, vol)
		if !rl.IsSoundPlaying(n.crowd) {
			rl.PlaySound(n.crowd)
		}
	} else if rl.IsSoundPlaying(n.crowd) {
		rl.StopSound(n.crowd)
	}
	if fireworks <= 0 {
		return
	}
	if n.seen && n.slot == slot {
		return
	}
	n.seen, n.slot = true, slot
	rl.SetSoundPan(n.bang, clamp(pan, -1, 1))
	rl.SetSoundVolume(n.bang, clamp(fireworks, 0.1, 1)*0.7)
	rl.PlaySound(n.bang)
}

func crowdLoop() []byte {
	// A crowd is low, soft noise swelling and falling in cheers.
	noise := synth.Noise(0x6d21, 0x4be9)
	var low float64
	return synth.Render(4, func(t float64) float64 {
		low += (noise() - low) * 0.06
		swell := 0.55 + 0.45*math.Sin(2*math.Pi*t/4)*math.Sin(2*math.Pi*t*0.75/4)
		return low * 2.2 * swell
	})
}
