package game

import (
	"fmt"
	"strings"

	"github.com/danielriddell21/crucible/menu"

	"github.com/danielriddell21/autobahn/internal/feats"
)

// The achievements list is an ordinary menu: one row per achievement, grouped
// under the group's name, with a Back row. Earned ones are ticked; a hidden
// one is left out until it is earned, and the subtitle says how many there are
// still to find.

// featRows words the list for a player's feats: a heading row for each group
// that has anything to show, then its achievements.
func featRows(f feats.Feats) []string {
	var rows []string
	for _, g := range feats.Groups() {
		var in []string
		for _, a := range feats.List() {
			if a.Group != g || !f.Visible(a) {
				continue
			}
			in = append(in, featRow(f, a))
		}
		if len(in) > 0 {
			rows = append(rows, strings.ToUpper(g.Name()))
			rows = append(rows, in...)
		}
	}
	return rows
}

func featRow(f feats.Feats, a feats.Feat) string {
	if f.Has(a.ID) {
		return fmt.Sprintf("[x] %s: %s", a.Name, a.Note)
	}
	if have, of := f.Progress(a.ID); of > 0 {
		return fmt.Sprintf("[ ] %s: %s (%d/%d)", a.Name, a.Note, have, of)
	}
	return fmt.Sprintf("[ ] %s: %s", a.Name, a.Note)
}

// featSubtitle says how many are earned and how many hidden ones remain.
func featSubtitle(f feats.Feats) string {
	all := feats.List()
	hidden := 0
	for _, a := range all {
		if !f.Visible(a) {
			hidden++
		}
	}
	s := fmt.Sprintf("%d of %d earned", len(f.Unlocked), len(all))
	if hidden > 0 {
		s += fmt.Sprintf(", %d hidden", hidden)
	}
	return s
}

// showFeats opens the achievements list, and back returns to the title.
func (s *Shell) showFeats() {
	f := s.career.Feats
	m := &menu.Menu{Title: "ACHIEVEMENTS", Subtitle: []string{featSubtitle(f)}}
	for _, r := range featRows(f) {
		m.Items = append(m.Items, menu.Item{Label: text(r), Action: func() {}})
	}
	m.Items = append(m.Items, menu.Item{Label: text("Back"), Action: s.showTitle})
	s.screen = screenTitle
	s.menu = m
}
