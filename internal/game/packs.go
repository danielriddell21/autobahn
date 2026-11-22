package game

import "github.com/danielriddell21/autobahn/internal/pack"

// packContent is what a pack hands the game. Each pack builds its own in its
// own pack_ file, and packs.go is the only place they are listed, so adding a
// pack's content edits that pack's file and nothing shared.
type packContent struct {
	// ID is the pack the content belongs to.
	ID pack.ID
	// Jobs registers the pack's job types (job.Register).
	Jobs func()
	// Dressing is called as each game is built, to add the pack's scenery.
	Dressing func(g *Game)
	// Audio is called as each game is built, to add the pack's sounds.
	Audio func(g *Game)
}

// registered is every pack's content, in the order Register was called.
var registered []packContent

// Register adds a pack's content. Nothing is called until [applyPacks].
func Register(c packContent) { registered = append(registered, c) }

func init() {
	Register(taxiContent())
	Register(emergencyContent())
	Register(raceContent())
	Register(nightContent())
}

// jobsRegistered is set once the packs' job types have been handed over, which
// happens once however many games are built.
var jobsRegistered bool

// applyPacks hands the packs' content over: their job types, once, and their
// dressing and audio to this game.
func applyPacks(g *Game) {
	if !jobsRegistered {
		jobsRegistered = true
		for _, c := range registered {
			if c.Jobs != nil {
				c.Jobs()
			}
		}
	}
	for _, c := range registered {
		if c.Dressing != nil {
			c.Dressing(g)
		}
		if c.Audio != nil {
			c.Audio(g)
		}
	}
}
