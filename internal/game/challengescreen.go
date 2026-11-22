package game

import (
	"fmt"

	"github.com/danielriddell21/crucible/menu"

	"github.com/danielriddell21/autobahn/internal/scenario"
)

// challengeLabel is one row of the challenge list: the name, what is asked,
// the ghost to beat and the player's best.
func challengeLabel(sc scenario.Scenario, bests scenario.Bests) string {
	best := "-"
	if b, ok := bests[sc.ID]; ok {
		best = fmt.Sprintf("%d %s", b.Score, b.Medal)
	}
	return fmt.Sprintf("%s  ghost %d  best %s", sc.Name, scenario.Ghosts()[sc.ID].Score, best)
}

// challengeMenu lists every scenario, then a way back. start is called with
// the one chosen; the shell should begin it as a single-player drive with the
// autopilot off, on the world from challengeConfig.
func challengeMenu(bests scenario.Bests, start func(scenario.Scenario), back func()) *menu.Menu {
	m := &menu.Menu{
		Title:    "CHALLENGES",
		Subtitle: []string{"beat the autopilot on the same road", "bronze, silver and gold are shares of its score"},
	}
	for _, sc := range scenario.All() {
		m.Items = append(m.Items, menu.Item{
			Label:  text(challengeLabel(sc, bests)),
			Action: func() { start(sc) },
		})
	}
	m.Items = append(m.Items, menu.Item{Label: text("Back"), Action: back})
	return m
}

// showChallenges opens the list of challenges.
func (s *Shell) showChallenges() {
	s.screen = screenChallenges
	s.menu = challengeMenu(s.career.Challenges, s.startChallenge, s.showTitle)
}

// startChallenge begins a scenario as a single-player drive with the autopilot
// off, on the world challengeConfig builds.
func (s *Shell) startChallenge(sc scenario.Scenario) {
	s.challenge = &sc
	s.startGame(false)
}

// retryChallenge rebuilds the world from the same config and clears the run.
func (s *Shell) retryChallenge() {
	old := s.game.chal
	s.startGame(false)
	if s.game != nil && s.game.chal != nil && old != nil {
		old.retry()
		s.game.chal = old
	}
}
