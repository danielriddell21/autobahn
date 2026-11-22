package game

import (
	"github.com/danielriddell21/autobahn/internal/audio"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/night"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/works"
)

// nightContent is the night pack's hand-over to the game. This file belongs to
// that pack.
//
// The pack has no job types. Its content is the evening events of package
// night, which come to every map the player owns once the pack is held (or
// everything is unlocked) and are on from dusk to one in the morning on the
// game's clock. Dressing hands the clock to the world, which puts the events on
// the road: a crowd slows the traffic, and the street an event shuts is shut
// through the roadworks. They are drawn by [Game.drawEvents] from the render,
// and sounded by [Game.updateNightAudio].
//
// A challenge is not given events, since its rules and its ghost are fixed, and
// nor is a game with another player in it, whose clock is the host's.
func nightContent() packContent {
	return packContent{
		ID: pack.Night,
		Dressing: func(g *Game) {
			if !g.nightEvents() {
				return
			}
			g.world.EnableNight(func() float32 { return g.clock.abs() })
		},
		Audio: func(g *Game) {
			if g.nightEvents() {
				g.nightSound = audio.OpenNight(g.audio)
			}
		},
	}
}

// nightEvents reports whether this game has the evening events: the player holds
// the pack, and the game is one they can be fair in.
func (g *Game) nightEvents() bool {
	o := g.opts
	return g.access().Has(pack.Night) && o.Challenge == nil && o.Host == "" && o.Join == ""
}

// launchEvery is how often a fair lets a firework off, in seconds.
const launchEvery = float32(burstEvery) / burstCount

// updateNightAudio sounds the crowd and the fireworks of the nearest event: the
// louder the nearer the player is to it.
func (g *Game) updateNightAudio() {
	ix := g.world.Night
	if g.nightSound == nil || ix == nil {
		return
	}
	p := g.world.Player
	var crowd, fire, pan float32
	if ix.Phase() == works.PhaseSolid {
		for _, e := range ix.Events() {
			rel := e.Pos.Sub(p.Pos)
			near := mathx.Clamp(1-(rel.Len()-e.Radius)/(night.Fade*2), 0, 1)
			if near <= crowd {
				continue
			}
			crowd = near
			fwd := p.Forward()
			pan = mathx.Sin(mathx.Atan2(rel.Dot(fwd.Right()), rel.Dot(fwd)))
			fire = 0
			if e.Kind == night.Fireworks {
				fire = near
			}
		}
	}
	g.nightSound.Update(crowd, fire, pan, int(g.world.Time/launchEvery))
}
