package game

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/pack"
)

// The title page (docs/menus.md, "Title"): a grouped list on the left and a
// card on the right that follows the selection. It is reached with
// s.openPage(pageTitle); the integrator links the old showTitle to it.

var _ = registerPage(pageTitle, newTitlePage, hintsChoose...)

// Row keys of the title page.
const (
	titlePlay     = "play"
	titleGarage   = "garage"
	titleCareer   = "career"
	titleSettings = "settings"
	titleQuit     = "quit"
)

// titleGroup is a heading and the keys of the rows under it.
type titleGroup struct {
	Name string
	Keys []string
}

var titleGroups = []titleGroup{
	{"PLAY", []string{titlePlay}},
	{"YOU", []string{titleGarage, titleCareer, titleSettings, titleQuit}},
}

// newPacks is the packs unlocked since the player last opened the career
// page. The shell calls markNewPacks with what Career.Bank returned, and the
// career page calls clearNewPacks when it is shown.
var newPacks struct {
	sync.Mutex
	list []pack.Pack
}

// markNewPacks remembers packs that have just unlocked, for the NEW chip.
func markNewPacks(ps []pack.Pack) {
	newPacks.Lock()
	defer newPacks.Unlock()
	for _, p := range ps {
		if !slices.ContainsFunc(newPacks.list, func(q pack.Pack) bool { return q.ID == p.ID }) {
			newPacks.list = append(newPacks.list, p)
		}
	}
}

// clearNewPacks forgets them: the player has seen the career page.
func clearNewPacks() {
	newPacks.Lock()
	defer newPacks.Unlock()
	newPacks.list = nil
}

// hasNewPacks reports whether a pack has unlocked and not been looked at.
func hasNewPacks() bool {
	newPacks.Lock()
	defer newPacks.Unlock()
	return len(newPacks.list) > 0
}

// nextPack is the cheapest pack the player does not hold yet.
func nextPack(a pack.Access) (pack.Pack, bool) {
	var best pack.Pack
	found := false
	for _, p := range pack.All() {
		if p.Price <= 0 || a.Has(p.ID) {
			continue
		}
		if !found || p.Price < best.Price {
			best, found = p, true
		}
	}
	return best, found
}

// titlePlayNote says what Play will start: the mode and map last chosen.
func (s *Shell) titlePlayNote() string {
	if s.customPlay {
		return "free drive, custom map"
	}
	return "free drive, " + s.settings.Map.Name()
}

func (s *Shell) titleGaragePaint() string {
	if p := s.settings.Paint; p >= 0 && p < len(playerPaints) {
		return playerPaints[p].name
	}
	return ""
}

// titleGarageNote is the car and paint.
func (s *Shell) titleGarageNote() string {
	return strings.TrimSpace(s.settings.Car.Name() + " - " + s.titleGaragePaint())
}

// titleRows builds the rows of the title page.
func (s *Shell) titleRows() []menuRow {
	open := func(id screen) func() { return func() { s.openPage(id) } }
	return []menuRow{
		actionRow(titlePlay, "Play", open(pagePlay)).noted(s.titlePlayNote),
		actionRow(titleGarage, "Garage", open(pageGarage)).noted(s.titleGarageNote),
		actionRow(titleCareer, "Career", open(pageCareer)).noted(func() string {
			if hasNewPacks() {
				return "NEW"
			}
			return ""
		}).toned(func() tone {
			if hasNewPacks() {
				return toneOffer
			}
			return tonePlain
		}),
		actionRow(titleSettings, "Settings", open(pageSettings)),
		actionRow(titleQuit, "Quit", open(pageQuit)).toned(func() tone { return toneDanger }),
	}
}

// titleCard is what the card on the right says for the selected row.
type titleCard struct {
	Head  string   // the card's title
	Tag   string   // a chip: the season, or ""
	Lines []string // plain lines under it
}

// titleCardFor works out the card for a row key.
func (s *Shell) titleCardFor(key string) titleCard {
	switch key {
	case titlePlay:
		m := s.settings.Map
		return titleCard{
			Head:  strings.ToUpper(m.Name()),
			Tag:   m.Season().Name(),
			Lines: []string{"Free drive, challenge or multiplayer."},
		}
	case titleGarage:
		return titleCard{
			Head:  strings.ToUpper(s.settings.Car.Name()),
			Lines: []string{"Paint: " + s.titleGaragePaint(), "Choose, paint and buy."},
		}
	case titleCareer:
		c := titleCard{
			Head:  "CAREER",
			Lines: []string{fmt.Sprintf("Bounty banked: %d", s.career.Banked)},
		}
		if p, ok := nextPack(s.access()); ok {
			c.Lines = append(c.Lines, fmt.Sprintf("Next: %s at %d", p.Name, p.Price))
		} else {
			c.Lines = append(c.Lines, "Every pack is held.")
		}
		if hasNewPacks() {
			c.Tag = "new"
		}
		return c
	case titleSettings:
		return titleCard{Head: "SETTINGS", Lines: []string{"Controls, display and sound."}}
	case titleQuit:
		return titleCard{Head: "QUIT", Lines: []string{"Your career is saved."}}
	}
	return titleCard{}
}

// newTitlePage builds the title page. Esc opens the quit dialog.
func newTitlePage(s *Shell) *menuPage {
	s.customPlay, s.challenge = false, nil
	pg := &menuPage{
		Title: "AUTOBAHN",
		Lede:  "a british driving game with a camera-driven autopilot",
		List:  newRowList(s.titleRows()...),
		Back:  func() { s.openPage(pageQuit) },
	}
	pg.Draw = func(float32) { s.drawTitlePage(pg) }
	return pg
}

// drawTitlePage draws the grouped list, the notice and the card.
func (s *Shell) drawTitlePage(pg *menuPage) {
	w, h := int32(s.opts.Width), int32(s.opts.Height)
	drawPageTitle(pg.Title, pg.Lede)
	if s.notice != "" {
		faceBody.draw(s.notice, pageMargin, pageTop+faceTitle.height()+40, toneInk(toneOffer))
	}
	y := int32(pageListTop)
	listW := min(pageListW, w-2*pageMargin)
	byKey := map[string]rowView{}
	for _, v := range pg.List.Views() {
		byKey[v.Key] = v
	}
	for _, g := range titleGroups {
		faceChip.draw(g.Name, pageMargin+16, y, signDim)
		y += 24
		for _, k := range g.Keys {
			drawTitleRow(rect(pageMargin, y, listW, pageRowH-4), byKey[k])
			y += pageRowH
		}
		y += 12
	}
	cx := int32(pageMargin*2 + pageListW)
	if w-cx-pageMargin > 200 {
		s.drawTitleCard(rect(cx, pageListTop, w-cx-pageMargin, min(300, h-pageListTop-pageFoot)), s.titleCardFor(pg.List.SelectedKey()))
	}
}

// drawTitleRow draws a row, the Career row's NEW as a chip.
func drawTitleRow(r rl.Rectangle, v rowView) {
	chip := v.Key == titleCareer && v.Value == "NEW"
	if chip {
		v.Value = ""
	}
	drawRowView(r, v)
	if chip {
		drawChip(int32(r.X+r.Width)-16-60, int32(r.Y)+(int32(r.Height)-20)/2, "new", toneOffer)
	}
}

// drawTitleCard draws the card on the right.
func (s *Shell) drawTitleCard(r rl.Rectangle, c titleCard) {
	drawCard(r, false)
	x, y := int32(r.X)+20, int32(r.Y)+20
	faceRow.draw(c.Head, x, y, signInk)
	y += faceRow.height() + 10
	if c.Tag != "" {
		drawChip(x, y, c.Tag, toneOffer)
		y += 30
	}
	for _, l := range c.Lines {
		faceBody.draw(l, x, y, signDim)
		y += faceBody.height() + 6
	}
}
