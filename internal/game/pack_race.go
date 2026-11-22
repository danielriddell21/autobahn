package game

import (
	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/race"
)

// raceContent is the racing pack's hand-over to the game. This file belongs to
// that pack.
//
// Its jobs are the four events of package race: a time trial, a circuit race,
// a street race and a drift event. Its dressing is the checkpoint gates, drawn
// by [Game.drawGates] from the render. They are drawn from the job under way
// alone, so there is nothing to build with a game.
func raceContent() packContent {
	return packContent{
		ID: pack.Racing,
		Jobs: func() {
			for _, t := range race.All() {
				job.Register(t)
			}
		},
	}
}
