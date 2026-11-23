package game

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/scenario"
)

// The title page (docs/menus.md, "Title"): a grouped list on the left and a
// card on the right that follows the selection. The shell opens on it, and
// every drive, failed start and lobby ends on it (Shell.goTitle).

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

// titleChallenge is the challenge the Play page would open on: the one last
// chosen, or the first.
func (s *Shell) titleChallenge() scenario.Scenario {
	if sc, ok := scenarioByID(s.settings.Challenge); ok {
		return sc
	}
	return scenario.All()[0]
}

// titlePlayNote says what Play will start: the mode last chosen on the Play
// page, and the map, the challenge, or whether it hosts or joins.
func (s *Shell) titlePlayNote() string {
	switch s.settings.PlayMode {
	case playChallenge:
		return "challenge, " + s.titleChallenge().Name
	case playMulti:
		if s.joining {
			return strings.TrimSpace("multiplayer, join " + s.joinAddr)
		}
		return "multiplayer, host " + strings.ToLower(s.hostMode.String())
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
	// Pictured says the card shows a plan of Map, when there is room for one.
	Pictured bool
	Map      city.Map
}

// titleCardFor works out the card for a row key.
func (s *Shell) titleCardFor(key string) titleCard {
	switch key {
	case titlePlay:
		return s.titlePlayCard()
	case titleGarage:
		return titleCard{
			Head:  strings.ToUpper(s.settings.Car.Name()),
			Lines: []string{"Paint: " + s.titleGaragePaint(), "Choose, paint and buy."},
		}
	case titleCareer:
		c := titleCard{
			Head:  "CAREER",
			Lines: []string{"Bounty banked: " + money(s.career.Banked)},
		}
		if p, ok := nextPack(s.access()); ok {
			c.Lines = append(c.Lines, fmt.Sprintf("Next: %s at %s", p.Name, money(p.Price)))
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

// titlePlayCard is the Play row's card: the map, or the challenge, that Play
// would start on.
func (s *Shell) titlePlayCard() titleCard {
	m := s.settings.Map
	c := titleCard{
		Head:     strings.ToUpper(m.Name()),
		Tag:      m.Season().Name(),
		Lines:    []string{"Free drive, challenge or multiplayer."},
		Pictured: true, Map: m,
	}
	switch s.settings.PlayMode {
	case playChallenge:
		sc := s.titleChallenge()
		c.Head, c.Map = strings.ToUpper(sc.Name), sc.CityMap()
		c.Tag = c.Map.Season().Name()
		c.Lines = []string{playMapTitle(c.Map) + ". " + sc.Brief(), "Beat the autopilot's ghost. Banks nothing."}
	case playMulti:
		c.Lines = []string{"Host " + strings.ToLower(s.hostMode.String()) + " on " + playMapTitle(m) + "."}
		if s.joining {
			// A joiner drives the host's map, which is not known until it arrives.
			c = titleCard{Head: "MULTIPLAYER", Lines: []string{"Join a game: you get the host's map, mode and weather."}}
		}
	}
	return c
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
	card := s.titleCardFor(pg.List.SelectedKey())
	if s.menuDriveOn() {
		// The drive is the picture: the card drops its own and sits small at
		// the bottom right, out of the drive's way.
		cw := min(int32(titleSmallW), w-2*pageMargin)
		lines := wrapText(card.Lines, cw-40, faceBody)
		ch := titleCardHeight(card, lines)
		s.drawTitleCard(rect(w-pageMargin-cw, h-pageFoot-16-ch, cw, ch), card, lines, nil)
		return
	}
	cx := int32(pageMargin*2 + pageListW)
	if w-cx-pageMargin <= 200 {
		return
	}
	cw := w - cx - pageMargin
	lines := wrapText(card.Lines, cw-40, faceBody)
	pic := min(titlePictureH, max(h-pageListTop-pageFoot-titleCardHeight(card, lines)-20, 0))
	var plan *customPlan
	if card.Pictured && pic >= 80 {
		plan = s.titlePlan.Ask(custom.FromMap(card.Map, 1))
	} else {
		pic = 0
	}
	ch := titleCardHeight(card, lines) + pic
	s.drawTitleCard(rect(cx, pageListTop, cw, ch), card, lines, &titlePicture{plan: plan, h: pic})
}

// Title card sizes, in pixels.
const (
	titleSmallW   = 340 // the card while the menu drive is behind the page
	titlePictureH = 220 // the map's picture on the full card, at most
)

// titlePicture is the plan drawn at the top of the full card, and its height.
type titlePicture struct {
	plan *customPlan
	h    int32
}

// titleCardHeight is how tall the card's words stand.
func titleCardHeight(c titleCard, lines []string) int32 {
	h := 20 + faceRow.height() + 10 + int32(len(lines))*(faceBody.height()+6) + 14
	if c.Tag != "" {
		h += 30
	}
	return h
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

// drawTitleCard draws the card on the right, with the map's picture across
// its top when it has one.
func (s *Shell) drawTitleCard(r rl.Rectangle, c titleCard, lines []string, pic *titlePicture) {
	drawCard(r, false)
	x, y := int32(r.X)+20, int32(r.Y)+20
	if pic != nil && pic.h > 0 {
		inset := rl.NewRectangle(r.X+6, r.Y+6, r.Width-12, float32(pic.h)-6)
		drawPlanPicture(inset, pic.plan, c.Map.Season(), false)
		y += pic.h
	}
	faceRow.draw(c.Head, x, y, signInk)
	y += faceRow.height() + 10
	if c.Tag != "" {
		drawChip(x, y, c.Tag, toneOffer)
		y += 30
	}
	for _, l := range lines {
		faceBody.draw(l, x, y, signDim)
		y += faceBody.height() + 6
	}
}
