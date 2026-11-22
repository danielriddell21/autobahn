package game

import (
	"fmt"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/synth"

	crucihud "github.com/danielriddell21/crucible/hud"

	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/taxi"
)

// taxiContent is the taxi pack's hand-over to the game. This file belongs to
// that pack.
//
// Its jobs are the fare, the rush fare and the courier round of package taxi.
// Its dressing is the index of pick-up places and the ranks of rank.go, and its
// audio the chime that sounds as a fare boards. A game's taxi state is the
// Game.taxi field.
func taxiContent() packContent {
	return packContent{
		ID: pack.Taxi,
		Jobs: func() {
			for _, t := range taxi.All() {
				job.Register(t)
			}
		},
		Dressing: func(g *Game) {
			g.taxi.ix = taxi.NewIndex(g.world.City)
		},
		Audio: func(g *Game) {
			// With no device, which is a muted game or a headless one, there is
			// nothing to load and the chime is simply not played.
			if !rl.IsAudioDeviceReady() {
				return
			}
			ts := &g.taxi
			w := synth.WAV(boardingChime())
			wave := rl.LoadWaveFromMemory(".wav", w, int32(len(w)))
			ts.chime = rl.LoadSoundFromWave(wave)
			rl.UnloadWave(wave)
			rl.SetSoundVolume(ts.chime, 0.5)
			ts.live = true
		},
	}
}

// taxiState is what the taxi pack keeps for one game.
type taxiState struct {
	ix *taxi.Index
	// boarded is whether the last frame had a fare aboard.
	boarded bool
	chime   rl.Sound
	live    bool
}

// boardingChime is two bright notes, a fourth apart, each dying away.
func boardingChime() []byte {
	return synth.Render(0.6, func(t float64) float64 {
		v := synth.Sine(880, t) * math.Exp(-t*9)
		if t > 0.16 {
			v += synth.Sine(1174.7, t-0.16) * math.Exp(-(t-0.16)*8)
		}
		return v * 0.5 * synth.Attack(t, 400)
	})
}

// taxiStep is called every frame after the jobs have been stepped. As a fare
// boards, or a load is collected, it posts a notice and sounds the chime.
func (g *Game) taxiStep() {
	ts := &g.taxi
	s := g.ActiveJob()
	aboard := s != nil && taxi.Boarded(*s)
	if aboard && !ts.boarded {
		msg := "Fare aboard: to the drop-off"
		if s.Type == taxi.CourierID {
			msg = fmt.Sprintf("Load collected: %d drops", taxi.Drops(*s))
		}
		g.notices.Post(msg, 150, crucihud.Notice)
		if ts.live {
			rl.PlaySound(ts.chime)
		}
	}
	ts.boarded = aboard
}

// taxiLine is the line the job readout shows under a taxi job's name: the
// streak, and where the job stands. It is empty for any other job.
func (g *Game) taxiLine() string {
	s := g.ActiveJob()
	if s == nil {
		return ""
	}
	var line string
	switch s.Type {
	case taxi.FareID, taxi.RiskyID:
		line = "heading for the pick-up"
		if taxi.Boarded(*s) {
			line = "fare aboard"
		}
		if s.Type == taxi.RiskyID {
			line = "rush: " + line
		}
	case taxi.CourierID:
		line = "collect the load"
		if taxi.Boarded(*s) {
			line = fmt.Sprintf("drop %d of %d", s.Next, taxi.Drops(*s))
			if c, ok := job.Lookup(s.Type); ok {
				if cc, ok := c.(*taxi.Courier); ok {
					line += fmt.Sprintf(", load %d%% damaged", int(cc.Damage(*s)*100))
				}
			}
		}
	default:
		return ""
	}
	if s.Streak > 0 {
		line += fmt.Sprintf(" (streak x%d: +%d%%)", s.Streak, taxi.Multiplier(s.Streak)-100)
	}
	return line
}
