package game

import (
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/keymap"
)

// The Signpost look (docs/menus.md, "Principles"): a dark ground, white type,
// one blue accent for what is selected, and three colours that each mean one
// thing. Models say what a thing means with a [tone]; only this file turns a
// tone into a colour.

var (
	// signInk is the type.
	signInk = rl.NewColor(236, 239, 244, 255)
	// signDim is type that says less: descriptions, notes.
	signDim = hudDim
	// signBlue is the accent, and only ever means selected (#68ACE8).
	signBlue = menuSelected
	// signGreen means yours, held or done (#7BD389).
	signGreen = rl.NewColor(123, 211, 137, 255)
	// signAmber means for sale, next, or a warning (#F0B070).
	signAmber = rl.NewColor(240, 176, 112, 255)
	// signGrey means locked.
	signGrey = rl.NewColor(104, 110, 122, 255)
	// signRed is for quitting and abandoning.
	signRed = hudBad

	// signCard is the face of a card or a control; signTrack the groove a bar
	// or a switch runs in; signRule an outline; signLift the fill behind the
	// selected row.
	signCard  = rl.NewColor(20, 23, 28, 255)
	signTrack = rl.NewColor(34, 38, 46, 255)
	signRule  = menuRule
	signLift  = rl.NewColor(26, 40, 56, 255)
	// signGroove is the groove of a bar, which has to show against the drive
	// even where the bar is empty.
	signGroove = rl.NewColor(62, 68, 80, 255)
)

// toneInk is the colour a tone is drawn in.
func toneInk(t tone) rl.Color {
	switch t {
	case toneYours:
		return signGreen
	case toneOffer:
		return signAmber
	case toneLocked:
		return signGrey
	case toneDanger:
		return signRed
	default:
		return signInk
	}
}

// fontWeight is how heavy a face is. The built-in raylib face has one weight;
// the bundled Overpass the menus use (menufont.go) has three: regular, bold
// and extra bold.
type fontWeight int

const (
	weightRegular fontWeight = iota
	weightBold
	weightHeavy
)

// typeFace is a size and weight of type. Every menu draws and measures its
// words through one, so changing the font is a change to draw and width
// alone.
type typeFace struct {
	size   int32
	weight fontWeight
}

// The faces the Signpost screens use. A screen that needs another size makes
// its own typeFace rather than calling raylib's text functions.
var (
	faceTitle = typeFace{40, weightHeavy}
	faceRow   = typeFace{20, weightBold}
	faceBody  = typeFace{16, weightRegular}
	faceSmall = typeFace{14, weightRegular}
	faceChip  = typeFace{12, weightBold}
)

// draw puts s with its top-left corner at x, y.
func (f typeFace) draw(s string, x, y int32, col rl.Color) {
	f.drawText(s, x, y, col)
}

// width is how wide s is in this face.
func (f typeFace) width(s string) int32 { return f.measureText(s) }

// height is the height of a line in this face.
func (f typeFace) height() int32 { return f.size }

// keymap is the face as crucible's hint bar measures it.
func (f typeFace) keymap() keymap.Face {
	return keymap.Face{
		LineHeight: int(f.size) + 4,
		Measure:    func(s string) int { return int(f.width(s)) },
	}
}

// rect is a rectangle in whole pixels.
func rect(x, y, w, h int32) rl.Rectangle {
	return rl.NewRectangle(float32(x), float32(y), float32(w), float32(h))
}

// rounded fills r with corners of the given radius in pixels.
func rounded(r rl.Rectangle, radius float32, col rl.Color) {
	rl.DrawRectangleRounded(r, roundness(r, radius), 8, col)
}

// ring outlines r, thick pixels wide, with corners of the given radius.
func ring(r rl.Rectangle, radius, thick float32, col rl.Color) {
	rl.DrawRectangleRoundedLinesEx(r, roundness(r, radius), 8, thick, col)
}

// roundness is raylib's way of saying a radius: a share of the shorter side.
func roundness(r rl.Rectangle, radius float32) float32 {
	short := min(r.Width, r.Height)
	if short <= 0 {
		return 0
	}
	return min(2*radius/short, 1)
}

// padlock draws a small closed lock with its top-left corner at x, y.
func padlock(x, y int32, col rl.Color) {
	rl.DrawRectangleLines(x+4, y, 10, 12, col)
	rl.DrawRectangleLines(x+5, y+1, 8, 10, col)
	rl.DrawRectangle(x, y+9, 18, 14, col)
}

// drawCard draws a card: the face, outlined, and ringed with the accent when
// it has the selection.
func drawCard(r rl.Rectangle, lit bool) {
	rounded(r, 8, signCard)
	if lit {
		ring(r, 8, 2, signBlue)
		return
	}
	ring(r, 8, 1, signRule)
}

// drawChip draws a tag ("AUTUMN", "YOURS", "LOCKED") with its top-left corner
// at x, y, in the tone's colour, and returns how wide it is.
func drawChip(x, y int32, label string, t tone) int32 {
	label = strings.ToUpper(label)
	const padX, h = 8, 20
	w := faceChip.width(label) + 2*padX
	col := toneInk(t)
	r := rect(x, y, w, h)
	rounded(r, h/2, rl.ColorAlpha(col, 0.16))
	ring(r, h/2, 1, col)
	faceChip.draw(label, x+padX, y+(h-faceChip.height())/2, col)
	return w
}

// Control sizes, in pixels.
const (
	switchW, switchH = 44, 24
	barW, barH       = 160, 6
	stepW            = 24
)

// drawSwitch draws an on/off switch with its top-left corner at x, y, ringed
// with the accent when lit.
func drawSwitch(x, y int32, on, lit, disabled bool) {
	r := rect(x, y, switchW, switchH)
	if lit {
		ring(rect(x-3, y-3, switchW+6, switchH+6), switchH/2+3, 2, signBlue)
	}
	track, knob, kx := signTrack, signGrey, x+3
	if on {
		track, knob, kx = signGreen, signInk, x+switchW-switchH+3
	}
	if disabled {
		track, knob = signTrack, signGrey
	}
	rounded(r, switchH/2, track)
	rl.DrawCircle(kx+(switchH-6)/2, y+switchH/2, float32(switchH-6)/2, knob)
}

// drawBar draws a level from 0 to 1 as a filled groove w wide, its top-left
// corner at x, y.
func drawBar(x, y, w int32, level float64, col rl.Color) {
	rounded(rect(x, y, w, barH), barH/2, signGroove)
	if fill := int32(float64(w) * min(max(level, 0), 1)); fill > 0 {
		rounded(rect(x, y, fill, barH), barH/2, col)
	}
}

// optionsWidth is how wide drawOptions draws opts.
func optionsWidth(opts []string) int32 {
	const pad, gap = 10, 4
	var w int32
	for i, o := range opts {
		if i > 0 {
			w += gap
		}
		w += faceSmall.width(o) + 2*pad
	}
	return w
}

// drawOptions draws a row of choices with the chosen one filled, blue when
// the row is lit, its top-left corner at x, y.
func drawOptions(x, y int32, opts []string, choice int, lit, disabled bool) {
	const pad, gap, h = 10, 4, 24
	for i, o := range opts {
		w := faceSmall.width(o) + 2*pad
		ink := signDim
		if i == choice {
			fill := signRule
			ink = signInk
			if lit && !disabled {
				fill, ink = signBlue, menuBackdrop
			}
			rounded(rect(x, y, w, h), 6, fill)
		}
		if disabled {
			ink = signGrey
		}
		faceSmall.draw(o, x+pad, y+(h-faceSmall.height())/2, ink)
		x += w + gap
	}
}

// stepperWidth is how wide drawStepper draws value.
func stepperWidth(value string) int32 { return 2*stepW + faceBody.width(value) + 24 }

// drawStepper draws a value between − and +, its top-left corner at x, y.
func drawStepper(x, y int32, value string, lit bool, col rl.Color) {
	edge := signRule
	if lit {
		edge = signBlue
	}
	for _, at := range []int32{x, x + stepperWidth(value) - stepW} {
		r := rect(at, y, stepW, stepW)
		rounded(r, 4, signCard)
		ring(r, 4, 1, edge)
	}
	faceBody.draw("-", x+(stepW-faceBody.width("-"))/2, y+(stepW-faceBody.height())/2, col)
	plus := x + stepperWidth(value) - stepW
	faceBody.draw("+", plus+(stepW-faceBody.width("+"))/2, y+(stepW-faceBody.height())/2, col)
	faceBody.draw(value, x+stepW+12, y+(stepW-faceBody.height())/2, col)
}

// drawButton draws a button: filled with the accent when selected, its words
// in the tone's colour, and grey when disabled.
func drawButton(r rl.Rectangle, label string, selected, disabled bool, t tone) {
	ink := toneInk(t)
	switch {
	case disabled:
		rounded(r, 8, signCard)
		ring(r, 8, 1, signRule)
		ink = signGrey
		if selected {
			ring(r, 8, 2, signBlue)
		}
	case selected && t == toneDanger:
		rounded(r, 8, signRed)
		ink = signInk
	case selected:
		rounded(r, 8, signBlue)
		ink = menuBackdrop
	default:
		rounded(r, 8, signCard)
		ring(r, 8, 1, signRule)
	}
	x := int32(r.X) + (int32(r.Width)-faceRow.width(label))/2
	y := int32(r.Y) + (int32(r.Height)-faceRow.height())/2
	faceRow.draw(label, x, y, ink)
}

// drawHintBar puts the keys along the bottom of a w by h window, laid out by
// crucible's keymap as every screen in the family is.
func drawHintBar(hints []keymap.Binding, w, h int) {
	for _, line := range keymap.BottomBar(hints, w, h, 16, faceSmall.keymap()) {
		faceSmall.draw(line.Text, int32(line.X), int32(line.Y), signDim)
	}
}

// Standard page layout, in pixels.
const (
	pageMargin  = 64
	pageTop     = 48
	pageListTop = 150
	pageListW   = 560
	pageRowH    = 48
	pageDescH   = 18 // added to every row when any row has a description
	pageFoot    = 64 // kept clear for the hint bar
)

// drawPageTitle draws a page's title and lede at the top left.
func drawPageTitle(title, lede string) {
	faceTitle.draw(title, pageMargin, pageTop, signInk)
	if lede != "" {
		faceBody.draw(lede, pageMargin, pageTop+faceTitle.height()+12, signDim)
	}
}

// listRowHeight is the height of every row in a list: taller, all of them,
// when any row has a description, so the rows keep in step.
func listRowHeight(vs []rowView) int32 {
	for _, v := range vs {
		if v.Desc != "" {
			return pageRowH + pageDescH
		}
	}
	return pageRowH
}

// drawRowList draws as many of a list's rows as fit in h, from x, y and w
// wide, keeping the selection in view.
func drawRowList(l *rowList, x, y, w, h int32) {
	vs := l.Views()
	rowH := listRowHeight(vs)
	first, last := rowWindow(len(vs), l.Sel(), int(h/rowH))
	for i := first; i < last; i++ {
		drawRowView(rect(x, y, w, rowH-4), vs[i])
		y += rowH
	}
}

// drawRowView draws one row: the label on the left with its description
// under it, and the control on the right. The selected row is filled, with
// the accent down its left edge. A disabled row is grey, with a padlock, and
// says why when it is selected.
func drawRowView(r rl.Rectangle, v rowView) {
	x, y, w, h := int32(r.X), int32(r.Y), int32(r.Width), int32(r.Height)
	if v.Selected {
		rounded(r, 6, signLift)
		rl.DrawRectangle(x, y+6, 3, h-12, signBlue)
	}
	ink := toneInk(v.Tone)
	under := v.Desc
	if v.Disabled != "" && v.Selected {
		under = v.Disabled
	}
	labelY := y + (h-faceRow.height())/2
	if under != "" {
		labelY = y + 8
		faceSmall.draw(under, x+16, labelY+faceRow.height()+6, signDim)
	}
	faceRow.draw(v.Label, x+16, labelY, ink)

	right, mid := x+w-16, y+h/2
	if v.Disabled != "" {
		padlock(right-18, mid-12, signGrey)
		right -= 30
	}
	drawRowControl(v, right, mid)
}

// drawRowControl draws the control a row's kind calls for, its right edge at
// right and centred on mid.
func drawRowControl(v rowView, right, mid int32) {
	off := v.Disabled != ""
	value := signInk
	switch {
	case off:
		value = signGrey
	case v.Warn:
		value = signAmber
	}
	switch v.Kind {
	case rowSwitch:
		drawSwitch(right-switchW, mid-switchH/2, v.On, v.Selected && !off, off)
	case rowOptions:
		drawOptions(right-optionsWidth(v.Options), mid-12, v.Options, v.Choice, v.Selected, off)
	case rowBar:
		drawBarControl(v, right, mid, value)
	case rowStepper:
		drawStepper(right-stepperWidth(v.Value), mid-stepW/2, v.Value, v.Selected && !off, value)
	default:
		if v.Value != "" {
			if !off && !v.Warn {
				value = signDim
			}
			faceBody.draw(v.Value, right-faceBody.width(v.Value), mid-faceBody.height()/2, value)
		}
	}
}

// drawBarControl draws a bar row's control: the bar, then its words in ink.
// The fill is blue while the row is selected, amber past its range and grey
// while disabled.
func drawBarControl(v rowView, right, mid int32, ink rl.Color) {
	words := faceSmall.width(v.Value)
	faceSmall.draw(v.Value, right-words, mid-faceSmall.height()/2, ink)
	fill := signInk
	switch {
	case v.Disabled != "":
		fill = signGrey
	case v.Warn:
		fill = signAmber
	case v.Selected:
		fill = signBlue
	}
	drawBar(right-words-12-barW, mid-barH/2, barW, v.Level, fill)
}

// drawPage draws the page showing: its backdrop, the page, and the hint bar.
func (s *Shell) drawPage(dt float32) {
	pg := s.pager.cur
	w, h := int32(s.opts.Width), int32(s.opts.Height)
	if pg.OverGame && s.game != nil {
		// The drive's own HUD would show through the veil and lie under the
		// page's words, so it is left off the picture behind a page.
		hud := s.game.showHUD
		s.game.showHUD = false
		s.game.drawFrame()
		s.game.showHUD = hud
		rl.DrawRectangle(0, 0, w, h, menuVeil)
	} else if !s.drawMenuDrive() { // the autopilot behind the menus: menu_drive.go
		rl.ClearBackground(menuBackdrop)
	}
	if pg.Draw != nil {
		pg.Draw(dt)
	} else {
		drawStandardPage(pg, w, h)
	}
	drawHintBar(hintsFor(s.screen, pg), s.opts.Width, s.opts.Height)
}

// drawStandardPage is how a page with no Draw of its own looks: the title at
// the top left and the list under it. A disabled row says why in its own
// second line while it is selected, so nothing more is needed here.
func drawStandardPage(pg *menuPage, w, h int32) {
	drawPageTitle(pg.Title, pg.Lede)
	if pg.List == nil {
		return
	}
	listW := min(pageListW, w-2*pageMargin)
	drawRowList(pg.List, pageMargin, pageListTop, listW, h-pageListTop-pageFoot)
}
