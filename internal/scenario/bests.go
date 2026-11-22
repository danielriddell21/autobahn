package scenario

// Best is the player's record on one scenario. It is what the career file
// holds, kept locally: there is no server and no leaderboard.
type Best struct {
	Score int   `json:"score"`
	Medal Medal `json:"medal"`
}

// Bests is the player's record on every scenario, by id.
type Bests map[string]Best

// Record takes a finished outcome into the bests and reports whether it set a
// new one. Only a passed outcome counts, which a helped run never is.
func (b Bests) Record(id string, o Outcome) bool {
	if !o.Passed {
		return false
	}
	old, had := b[id]
	if had && o.Score <= old.Score && o.Medal <= old.Medal {
		return false
	}
	b[id] = Best{Score: max(o.Score, old.Score), Medal: max(o.Medal, old.Medal)}
	return true
}

// Sanitise drops what a hand-edited or damaged file could put in the bests:
// scenarios that do not exist, negative scores, and medals the score does not
// earn against the checked-in ghost. It returns a map that is never nil.
func (b Bests) Sanitise() Bests {
	out := Bests{}
	ghosts := Ghosts()
	for _, sc := range All() {
		v, ok := b[sc.ID]
		g, haveGhost := ghosts[sc.ID]
		if !ok || v.Score < 0 || !haveGhost {
			continue
		}
		v.Medal = min(v.Medal, MedalFor(v.Score, g.Score))
		if v.Medal < Bronze {
			continue
		}
		out[sc.ID] = v
	}
	return out
}
