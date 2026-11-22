package game

import "github.com/danielriddell21/autobahn/internal/pack"

// emergencyContent is the emergency pack's hand-over to the game. This file belongs to that pack.
// It is a PLACEHOLDER: fill in Jobs (job.Register of its types), Dressing and
// Audio when the pack has content.
func emergencyContent() packContent {
	return packContent{ID: pack.Emergency}
}
