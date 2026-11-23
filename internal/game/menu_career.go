package game

import (
	"fmt"
	"slices"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/keymap"

	"github.com/danielriddell21/autobahn/internal/feats"
	"github.com/danielriddell21/autobahn/internal/pack"
)

// The Career page (docs/menus.md, "Career"): the bounty and earnings, the next
// pack, the packs as a route and the achievements as a grid. It is one list of
// rows to the keys (the packs, then the achievements, then Back) and is drawn
// as two columns, so ↑↓ walk the route and then the grid.
//
// The model (what each pack and achievement says) is worked out here from a
// pack.Access and a feats.Feats and read by the tests; the drawing only lays
// it out.

var _ = registerPage(pageCareer, newCareerPage, hintsCareer...)

var hintsCareer = []keymap.Binding{
	{Key: "up/down", Action: "browse"},
	{Key: "esc", Action: "back"},
}

// packState is where a pack stands for a player.
type packState int

const (
	// packHeld is unlocked.
	packHeld packState = iota
	// packNext is the cheapest pack not held: the one being saved for.
	packNext
	// packLocked is further on than the next.
	packLocked
)

// word is the state as a chip says it.
func (p packState) word() string {
	switch p {
	case packHeld:
		return "held"
	case packNext:
		return "next"
	default:
		return "locked"
	}
}

// tone is the colour the state means.
func (p packState) tone() tone {
	switch p {
	case packHeld:
		return toneYours
	case packNext:
		return toneOffer
	default:
		return toneLocked
	}
}

// packEntry is one stop on the route.
type packEntry struct {
	Pack  pack.Pack
	State packState
	// Left is the bounty still to bank for it, 0 when held.
	Left int
	// Progress runs 0 to 1 toward its price.
	Progress float64
	// Maps and Cars are what it adds, by name.
	Maps, Cars []string
}

// packRoute is every pack in price order with where each stands for a.
func packRoute(a pack.Access) []packEntry {
	all := pack.All()
	slices.SortStableFunc(all, func(x, y pack.Pack) int { return x.Price - y.Price })
	out := make([]packEntry, 0, len(all))
	nextSeen := false
	for _, p := range all {
		e := packEntry{Pack: p, Progress: a.Progress(p.ID)}
		for _, m := range p.Maps {
			e.Maps = append(e.Maps, careerDrawable(m.Name()))
		}
		for _, b := range p.Bodies {
			e.Cars = append(e.Cars, careerDrawable(b.Name()))
		}
		switch {
		case a.Has(p.ID):
			e.State = packHeld
		case !nextSeen:
			e.State, nextSeen = packNext, true
			e.Left = max(p.Price-a.Banked, 0)
		default:
			e.State = packLocked
			e.Left = max(p.Price-a.Banked, 0)
		}
		out = append(out, e)
	}
	return out
}

// careerNextPack is the pack being saved for, if there is one.
func careerNextPack(route []packEntry) (packEntry, bool) {
	for _, e := range route {
		if e.State == packNext {
			return e, true
		}
	}
	return packEntry{}, false
}

// contents words what a pack adds: "seatown; convertible, city car".
func (e packEntry) contents() string {
	if len(e.Maps) == 0 && len(e.Cars) == 0 {
		return e.Pack.Blurb
	}
	var parts []string
	if len(e.Maps) > 0 {
		parts = append(parts, "map: "+strings.Join(e.Maps, ", "))
	}
	if len(e.Cars) > 0 {
		parts = append(parts, "cars: "+strings.Join(e.Cars, ", "))
	}
	return strings.Join(parts, "  /  ")
}

// status is the words on the right of the pack's line.
func (e packEntry) status() string {
	if e.State == packHeld {
		return "held"
	}
	return money(e.Left) + " to go"
}

// featEntry is one tile of the grid.
type featEntry struct {
	Feat feats.Feat
	// Earned is whether it has been; Shown whether the player may see what it
	// is. A hidden one unearned is neither.
	Earned, Shown bool
	// Have and Of are the progress, where the achievement reports any.
	Have, Of int
}

// name is what the tile calls it.
func (e featEntry) name() string {
	if !e.Shown {
		return "???"
	}
	return careerDrawable(e.Feat.Name)
}

// progress is "3/10", or "" when there is none to show.
func (e featEntry) progress() string {
	if e.Of == 0 || !e.Shown {
		return ""
	}
	return fmt.Sprintf("%d/%d", e.Have, e.Of)
}

// note is how it is earned, or a hint that it is hidden.
func (e featEntry) note() string {
	if !e.Shown {
		return "hidden: earn it to find out what it is"
	}
	return careerDrawable(e.Feat.Note)
}

// featGrid is every achievement, in the list's order.
func featGrid(f feats.Feats) []featEntry {
	out := make([]featEntry, 0, len(feats.List()))
	for _, a := range feats.List() {
		e := featEntry{Feat: a, Earned: f.Has(a.ID), Shown: f.Visible(a)}
		e.Have, e.Of = f.Progress(a.ID)
		if e.Earned && e.Of > 0 {
			e.Have = e.Of
		}
		out = append(out, e)
	}
	return out
}

// careerDrawable drops anything the menu face has no glyph for: every rune
// outside menuGlyphs (menufont.go), so the pound sign, dashes and curly quotes
// stay and control characters and anything exotic in a name go.
func careerDrawable(s string) string {
	return strings.Map(func(r rune) rune {
		if !strings.ContainsRune(menuGlyphs, r) {
			return -1
		}
		return r
	}, s)
}

// careerPage is the page's own state: its rows and what each stands for.
type careerPage struct {
	s     *Shell
	route []packEntry
	grid  []featEntry
	list  *rowList
}

// Row keys.
const (
	careerPackKey = "pack-"
	careerFeatKey = "feat-"
	careerBackKey = "back"
)

// rebuild works the rows out from the career as it stands now.
func (c *careerPage) rebuild() {
	c.route = packRoute(c.s.access())
	c.grid = featGrid(c.s.career.Feats)
	var rows []menuRow
	for _, e := range c.route {
		rows = append(rows, menuRow{
			Key: careerPackKey + fmt.Sprint(int(e.Pack.ID)), Label: careerDrawable(e.Pack.Name), Kind: rowAction,
			Note: text(e.status()), Tone: func() tone { return e.State.tone() },
		})
	}
	for _, e := range c.grid {
		r := menuRow{Key: careerFeatKey + e.Feat.ID, Label: e.name(), Kind: rowAction, Note: text(e.progress())}
		switch {
		case e.Earned:
			r.Tone = func() tone { return toneYours }
		case !e.Shown:
			r.Tone = func() tone { return toneLocked }
		}
		rows = append(rows, r)
	}
	rows = append(rows, actionRow(careerBackKey, "Back", c.s.backPage))
	if c.list == nil {
		c.list = newRowList(rows...)
		return
	}
	c.list.SetRows(rows)
}

// selection says what has the selection: a pack or an achievement by index,
// or neither (Back).
func (c *careerPage) selection() (pk, ft int) {
	pk, ft = -1, -1
	i := c.list.Sel()
	switch {
	case i < len(c.route):
		pk = i
	case i < len(c.route)+len(c.grid):
		ft = i - len(c.route)
	}
	return pk, ft
}

// detail is what the selected thing needs, as a heading and a line.
func (c *careerPage) detail() (head, line string) {
	pk, ft := c.selection()
	switch {
	case pk >= 0:
		e := c.route[pk]
		head = strings.ToUpper(careerDrawable(e.Pack.Name)) + "  " + strings.ToUpper(e.State.word())
		switch e.State {
		case packHeld:
			line = e.contents()
		default:
			line = fmt.Sprintf("bank %s bounty in all (%s to go). %s", money(e.Pack.Price), money(e.Left), e.contents())
		}
		return head, careerDrawable(line)
	case ft >= 0:
		e := c.grid[ft]
		head = strings.ToUpper(e.name())
		if e.Earned {
			head += "  EARNED"
		}
		line = e.note()
		if p := e.progress(); p != "" && !e.Earned {
			line += "  (" + p + ")"
		}
		return head, line
	}
	return "BACK", "return to the previous screen"
}

// newCareerPage builds the page.
func newCareerPage(s *Shell) *menuPage {
	c := &careerPage{s: s}
	c.rebuild()
	pg := &menuPage{Title: "CAREER", Lede: "what you bank getting away from the police opens the packs", List: c.list}
	pg.Shown = func() {
		c.rebuild()
		clearNewPacks()
	}
	pg.Draw = func(float32) { c.draw() }
	return pg
}

// Layout, in pixels.
const (
	careerTilesY = 140
	careerTileH  = 76
	careerGap    = 16
	careerDetail = 58
)

// draw lays the page out.
func (c *careerPage) draw() {
	w, h := int32(c.s.opts.Width), int32(c.s.opts.Height)
	drawPageTitle("CAREER", "what you bank getting away from the police opens the packs")
	inner := w - 2*pageMargin

	backR := rect(w-pageMargin-120, pageTop+2, 120, 40)
	drawButton(backR, "Back", c.list.Sel() == c.list.Len()-1, false, tonePlain)

	// Tiles.
	tw := (inner - 2*careerGap) / 3
	next, haveNext := careerNextPack(c.route)
	nextName, nextSub, nextTone := "all held", "nothing left to bank for", toneYours
	if haveNext {
		nextName, nextSub, nextTone = careerDrawable(next.Pack.Name), money(next.Left)+" bounty to go", toneOffer
	}
	c.tile(pageMargin, tw, "BOUNTY BANKED", money(c.s.career.Banked), "kept from every pursuit", tonePlain)
	c.tile(pageMargin+tw+careerGap, tw, "EARNINGS", money(c.s.career.Earnings), "what the shop spends", tonePlain)
	c.tile(pageMargin+2*(tw+careerGap), tw, "NEXT PACK", nextName, nextSub, nextTone)

	top := int32(careerTilesY + careerTileH + 22)
	foot := h - pageFoot - careerDetail - 10
	leftW := inner * 46 / 100
	c.drawRoute(pageMargin, top, leftW, foot-top)
	c.drawGrid(pageMargin+leftW+careerGap*2, top, inner-leftW-careerGap*2, foot-top)

	head, line := c.detail()
	dr := rect(pageMargin, foot+8, inner, careerDetail)
	drawCard(dr, false)
	faceRow.draw(head, pageMargin+16, foot+8+8, signInk)
	faceSmall.draw(careerClip(faceSmall, line, inner-32), pageMargin+16, foot+8+8+faceRow.height()+6, signDim)
}

// faceFigure is the big figure in a tile: smaller than a title, which Overpass
// sets taller than the built-in face and would run into the line under it.
var faceFigure = typeFace{28, weightHeavy}

// faceRoute is the pack name down the route.
var faceRoute = typeFace{18, weightBold}

// tile draws one of the three tiles across the top.
func (c *careerPage) tile(x, w int32, label, big, sub string, t tone) {
	drawCard(rect(x, careerTilesY, w, careerTileH), false)
	faceChip.draw(label, x+16, careerTilesY+10, signDim)
	faceFigure.draw(careerClip(faceFigure, big, w-32), x+16, careerTilesY+10+faceChip.height()+2, toneInk(t))
	faceSmall.draw(careerClip(faceSmall, sub, w-32), x+16, careerTilesY+careerTileH-faceSmall.height()-4, signDim)
}

// drawRoute draws the packs one under another with a line down the dots.
func (c *careerPage) drawRoute(x, y, w, h int32) {
	n := int32(len(c.route))
	if n == 0 {
		return
	}
	rowH := min(max(h/n, 40), 64)
	pk, _ := c.selection()
	for i, e := range c.route {
		ry := y + int32(i)*rowH
		sel := i == pk
		r := rect(x, ry, w, rowH-4)
		if sel {
			rounded(r, 6, signLift)
			rl.DrawRectangle(x, ry+6, 3, rowH-16, signBlue)
		}
		col := toneInk(e.State.tone())
		dx, dy := x+22, ry+(rowH-4)/2
		if i < len(c.route)-1 {
			rl.DrawRectangle(dx-1, dy, 2, rowH, signRule)
		}
		switch e.State {
		case packLocked:
			ring(rect(dx-6, dy-6, 12, 12), 6, 2, col)
		default:
			rl.DrawCircle(dx, dy, 7, col)
		}
		tx := x + 44
		ink := signInk
		if e.State == packLocked {
			ink = signGrey
		}
		faceRoute.draw(strings.ToUpper(careerDrawable(e.Pack.Name)), tx, ry+3, ink)
		st := e.status()
		faceSmall.draw(st, x+w-12-faceSmall.width(st), ry+8, col)
		sub := careerClip(faceSmall, e.contents(), w-44-12)
		faceSmall.draw(sub, tx, ry+3+faceRoute.height(), signDim)
		if e.State != packHeld {
			drawBar(tx, ry+rowH-8, w-44-12, e.Progress, col)
		}
	}
}

// drawGrid draws the achievements as tiles, flowing in rows.
func (c *careerPage) drawGrid(x, y, w, h int32) {
	if len(c.grid) == 0 {
		return
	}
	cols := max(w/170, 1)
	tw := (w - (cols-1)*8) / cols
	rows := (int32(len(c.grid)) + cols - 1) / cols
	th := min(max((h-(rows-1)*8)/rows, 44), 64)
	_, sel := c.selection()
	for i, e := range c.grid {
		tx := x + int32(i)%cols*(tw+8)
		ty := y + int32(i)/cols*(th+8)
		r := rect(tx, ty, tw, th)
		face, edge := signCard, signRule
		ink := signGrey
		switch {
		case e.Earned:
			face, edge, ink = signLift, signGreen, signInk
		case e.Shown:
			ink = signDim
		}
		rounded(r, 6, face)
		ring(r, 6, 1, edge)
		if i == sel {
			ring(r, 6, 2, signBlue)
		}
		name := e.name()
		if e.Earned {
			name = "[x] " + name
		}
		faceBody.draw(careerClip(faceBody, name, tw-16), tx+8, ty+6, ink)
		if p := e.progress(); p != "" && !e.Earned {
			faceSmall.draw(p, tx+8, ty+6+faceBody.height()+4, signAmber)
		}
	}
}

// careerClip shortens s with ".." until it fits w pixels in face f.
func careerClip(f typeFace, s string, w int32) string {
	if f.width(s) <= w {
		return s
	}
	for len(s) > 0 && f.width(s+"..") > w {
		s = s[:len(s)-1]
	}
	return s + ".."
}
