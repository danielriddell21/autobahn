package game

import (
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/scenario"
)

// Drawing the Play page. Everything it shows is worked out in menu_play.go;
// this file only places it, in the Signpost colours and with signpost.go's
// controls. The layout follows the locked prototype: the title and the switch
// along the top, the mode's own panel in the middle, the bar along the bottom.

// The faces the page adds to signpost.go's.
var (
	facePlayCard  = typeFace{24, weightHeavy}
	facePlayPanel = typeFace{26, weightHeavy}
	facePlayAddr  = typeFace{22, weightRegular}
)

// Page geometry, in pixels.
const (
	playBarH   = 72 // the bottom bar
	playGap    = 12 // between cards and cells
	playCellH  = 66 // a control's cell
	playLabelH = 22 // a "WHERE" or "HOW" heading and the space under it
	playSwitch = 40 // the height of the mode switch
)

// playTints are the map cards' grounds, in city.Maps() order, then Custom's.
// They are only grounds: no meaning is carried by them, so they keep well
// away from the Signpost colours.
var playTints = []rl.Color{
	rl.NewColor(58, 46, 36, 255), // city, autumn
	rl.NewColor(30, 58, 74, 255), // seatown, summer
	rl.NewColor(36, 56, 42, 255), // village, spring
	rl.NewColor(42, 50, 64, 255), // roadtown, winter
	rl.NewColor(43, 36, 64, 255), // custom
}

// playBox is the part of the window the page lays itself out in.
type playBox struct {
	left, right, top, bar int32
}

func (b playBox) width() int32 { return b.right - b.left }

// draw draws the page.
func (p *playPage) draw(float32) {
	w, h := int32(p.s.opts.Width), int32(p.s.opts.Height)
	b := playBox{
		left: pageMargin, right: w - pageMargin,
		top: pageTop + faceTitle.height() + 28, bar: h - pageFoot - playBarH,
	}
	vs := map[string]rowView{}
	for _, v := range p.list.Views() {
		vs[v.Key] = v
	}
	p.drawHeader(b, vs["mode"])
	switch p.mode {
	case playChallenge:
		p.drawChallenges(b, vs)
	case playMulti:
		p.drawMulti(b, vs)
	default:
		p.drawFree(b, vs)
	}
	p.drawBar(b, vs)
}

// drawHeader draws the title, the three-way switch and what the mode is.
func (p *playPage) drawHeader(b playBox, mode rowView) {
	faceTitle.draw("PLAY", b.left, pageTop, signInk)
	x := b.left + faceTitle.width("PLAY") + 24
	y := pageTop + (faceTitle.height()-playSwitch)/2
	x += playSegments(x, y, playSwitch, faceRow, playModeNames, int(p.mode), nil, mode.Selected, true) + 20
	lede := playLedes[p.mode]
	if lw := faceSmall.width(lede); x+lw <= b.right {
		faceSmall.draw(lede, b.right-lw, pageTop+(faceTitle.height()-faceSmall.height())/2, signDim)
	}
}

// drawFree draws free drive: the map cards, and the controls under them.
func (p *playPage) drawFree(b playBox, vs map[string]rowView) {
	keys := []string{"who", "style", "weather", "time", "car", "edit"}
	cells := make([]rowView, 0, len(keys))
	for _, k := range keys {
		if v, ok := vs[k]; ok {
			cells = append(cells, v)
		}
	}
	const cols = 3
	rows := int32((len(cells) + cols - 1) / cols)
	how := playLabelH + rows*(playCellH+playGap)
	cardH := min(max(b.bar-16-b.top-playLabelH-how-16, 70), 200)

	y := b.top
	faceChip.draw("WHERE", b.left, y, signDim)
	y += playLabelH
	cards := p.cards()
	cw := (b.width() - playGap*int32(len(cards)-1)) / int32(len(cards))
	for i, c := range cards {
		drawPlayMapCard(rect(b.left+int32(i)*(cw+playGap), y, cw, cardH), c, i, vs["map"].Selected)
	}
	y += cardH + 16

	faceChip.draw("HOW", b.left, y, signDim)
	y += playLabelH
	cellW := (b.width() - playGap*(cols-1)) / cols
	for i, v := range cells {
		at := rect(b.left+int32(i%cols)*(cellW+playGap), y+int32(i/cols)*(playCellH+playGap), cellW, playCellH)
		note := ""
		if v.Key == "who" && p.auto {
			note = "banks nothing"
		}
		drawPlayCell(at, v, note)
	}
}

// drawPlayMapCard draws one map card: its ground, the season tag, a padlock
// and how far there is to go when it is locked, and its name. The chosen card
// is outlined, in the accent while the map row has the selection.
func drawPlayMapCard(r rl.Rectangle, c playCard, i int, lit bool) {
	x, y, w, h := int32(r.X), int32(r.Y), int32(r.Width), int32(r.Height)
	ground := playTints[min(i, len(playTints)-1)]
	if c.Locked {
		ground = playBlend(ground, signCard, 0.6)
	}
	rounded(r, 10, ground)
	switch {
	case c.Chosen && lit:
		ring(r, 10, 3, signBlue)
	case c.Chosen:
		ring(r, 10, 2, signInk)
	default:
		ring(r, 10, 1, signRule)
	}
	cx := x + 12 + drawChip(x+12, y+12, c.Season, tonePlain) + 6
	if c.Flagged {
		drawChip(cx, y+12, "unsupported", toneOffer)
	}
	if c.Locked {
		padlock(x+w-30, y+12, signInk)
	}

	inner := w - 28
	bottom := y + h - 14
	if c.Locked {
		bottom -= barH + 8
		drawBar(x+14, bottom+8, inner, c.Progress, signInk)
	}
	for _, line := range []string{c.Note2, c.Note} {
		if line == "" {
			continue
		}
		bottom -= faceSmall.height() + 4
		faceSmall.draw(playFit(faceSmall, line, inner), x+14, bottom, signDim)
	}
	name := signInk
	if c.Locked {
		name = signGrey
	}
	facePlayCard.draw(playFit(facePlayCard, c.Title, inner), x+14, bottom-facePlayCard.height()-6, name)
}

// drawPlayCell draws one control in a cell: its name small above it, the
// control under, and an optional note on the right of the name. The selected
// cell is ringed with the accent.
func drawPlayCell(r rl.Rectangle, v rowView, note string) {
	x, y, w := int32(r.X), int32(r.Y), int32(r.Width)
	rounded(r, 8, signCard)
	if v.Selected {
		ring(r, 8, 2, signBlue)
	} else {
		ring(r, 8, 1, signRule)
	}
	off := v.Disabled != ""
	faceChip.draw(strings.ToUpper(v.Label), x+14, y+10, signDim)
	if off {
		padlock(x+w-30, y+8, signGrey)
	} else if note != "" {
		faceSmall.draw(note, x+w-14-faceSmall.width(note), y+8, signDim)
	}
	cy, inner := y+32, w-28
	switch {
	case v.Kind == rowOptions && optionsWidth(v.Options) <= inner:
		drawOptions(x+14, cy, v.Options, v.Choice, v.Selected, off)
	case v.Kind == rowOptions:
		// Too many to show side by side: the one chosen, with the way to step.
		playStepWords(x+14, cy, "<  "+v.Value+"  >", v.Selected, off)
	default:
		playStepWords(x+14, cy, playFit(faceSmall, v.Value, inner-40)+"  >", v.Selected, off)
	}
}

// playStepWords draws words on a small button, as a cell's control.
func playStepWords(x, y int32, words string, lit, off bool) {
	const pad, h = 10, 24
	w := faceSmall.width(words) + 2*pad
	fill, ink := signTrack, signInk
	switch {
	case off:
		ink = signGrey
	case lit:
		fill, ink = signBlue, menuBackdrop
	}
	rounded(rect(x, y, w, h), 6, fill)
	faceSmall.draw(words, x+pad, y+(h-faceSmall.height())/2, ink)
}

// drawChallenges draws the challenges as cards, four across, and the car.
func (p *playPage) drawChallenges(b playBox, vs map[string]rowView) {
	cards := p.chalCards()
	const cols = 4
	rows := int32((len(cards) + cols - 1) / cols)
	cardH := min(max((b.bar-16-b.top-playCellH-playGap)/max(rows, 1)-playGap, 60), 150)
	cw := (b.width() - playGap*(cols-1)) / cols
	for i, c := range cards {
		at := rect(b.left+int32(i%cols)*(cw+playGap), b.top+int32(i/cols)*(cardH+playGap), cw, cardH)
		drawPlayChalCard(at, c, vs["chal"].Selected)
	}
	y := b.top + rows*(cardH+playGap)
	drawPlayCell(rect(b.left, y, min(380, b.width()), playCellH), vs["car"], "")
}

// drawPlayChalCard draws one challenge: its map and season, name, goal, the
// ghost to beat and the player's best.
func drawPlayChalCard(r rl.Rectangle, c playChalCard, lit bool) {
	x, y, w, h := int32(r.X), int32(r.Y), int32(r.Width), int32(r.Height)
	switch {
	case c.Chosen && lit:
		rounded(r, 10, signLift)
		ring(r, 10, 3, signBlue)
	case c.Chosen:
		rounded(r, 10, signLift)
		ring(r, 10, 2, signInk)
	default:
		drawCard(r, false)
	}
	inner := w - 28
	faceSmall.draw(c.Map, x+14, y+14, signDim)
	drawChip(x+14+faceSmall.width(c.Map)+8, y+11, c.Season, tonePlain)
	name := signInk
	if c.Locked {
		padlock(x+w-30, y+10, signGrey)
		name = signGrey
	}
	faceRow.draw(playFit(faceRow, c.Name, inner), x+14, y+40, name)
	gy := y + 40 + faceRow.height() + 6
	for _, line := range playWrap(faceSmall, c.Goal, inner, 2) {
		if gy+faceSmall.height() > y+h-faceSmall.height()-16 {
			break
		}
		faceSmall.draw(line, x+14, gy, signDim)
		gy += faceSmall.height() + 2
	}
	by := y + h - faceSmall.height() - 12
	faceSmall.draw(c.Ghost, x+14, by, signInk)
	best := signDim
	if c.Medal >= scenario.Bronze {
		best = signGreen
	}
	faceSmall.draw(c.Best, x+w-14-faceSmall.width(c.Best), by, best)
}

// drawMulti draws the two multiplayer panels side by side, the one chosen lit
// and the other dimmed.
func (p *playPage) drawMulti(b playBox, vs map[string]rowView) {
	pw := (b.width() - 16) / 2
	ph := b.bar - 16 - b.top
	role := vs["role"].Selected
	host := rect(b.left, b.top, pw, ph)
	join := rect(b.left+pw+16, b.top, pw, ph)
	p.drawPanel(host, "Host a game", "it runs on this machine, on your map", !p.joining, role)
	p.drawPanel(join, "Join a game", "you get the host's map, mode and weather", p.joining, role)
	if p.joining {
		p.drawJoin(join, vs)
		return
	}
	x, y, w := int32(host.X)+20, int32(host.Y)+88, pw-40
	for _, k := range []string{"game", "map", "car"} {
		v := vs[k]
		at := rect(x, y, w, playCellH)
		if k == "map" {
			p.drawHostMap(at, v)
		} else {
			drawPlayCell(at, v, "")
		}
		y += playCellH + 10
	}
}

// drawPanel draws a multiplayer panel's ground, name and line. The panel not
// chosen is dimmed; the chosen one is ringed with the accent while the host
// or join row has the selection.
func (p *playPage) drawPanel(r rl.Rectangle, title, line string, chosen, lit bool) {
	x, y := int32(r.X), int32(r.Y)
	rounded(r, 12, signCard)
	switch {
	case chosen && lit:
		ring(r, 12, 2, signBlue)
	case chosen:
		ring(r, 12, 2, signRule)
	default:
		ring(r, 12, 1, signRule)
	}
	facePlayPanel.draw(title, x+22, y+20, signInk)
	faceSmall.draw(playFit(faceSmall, line, int32(r.Width)-44), x+22, y+20+facePlayPanel.height()+8, signDim)
	if !chosen {
		rounded(r, 12, rl.ColorAlpha(menuBackdrop, 0.55))
	}
}

// drawHostMap draws the host's map row as a cell of five choices, the locked
// ones grey.
func (p *playPage) drawHostMap(r rl.Rectangle, v rowView) {
	x, y := int32(r.X), int32(r.Y)
	rounded(r, 8, signCard)
	if v.Selected {
		ring(r, 8, 2, signBlue)
	} else {
		ring(r, 8, 1, signRule)
	}
	faceChip.draw("MAP", x+14, y+10, signDim)
	cards := p.cards()
	locked := make([]bool, len(cards))
	for i, c := range cards {
		// Hosting a custom map is for the command line, so it is greyed too.
		locked[i] = c.Locked || c.Custom
	}
	// The cell is ringed already, so the choices are not ringed again.
	playSegments(x+14, y+32, 24, faceSmall, v.Options, v.Choice, locked, v.Selected, false)
}

// drawJoin draws the join panel's address field and car.
func (p *playPage) drawJoin(r rl.Rectangle, vs map[string]rowView) {
	x, y, w := int32(r.X)+22, int32(r.Y)+88, int32(r.Width)-44
	faceChip.draw("HOST ADDRESS", x, y, signDim)
	y += playLabelH
	field := rect(x, y, w, 52)
	rounded(field, 6, menuBackdrop)
	lit := vs["addr"].Selected
	if lit {
		ring(field, 6, 2, signBlue)
	} else {
		ring(field, 6, 1, signRule)
	}
	ty := y + (52-facePlayAddr.height())/2
	addr := p.s.joinAddr
	if addr == "" {
		facePlayAddr.draw("192.168.1.20", x+16, ty, signGrey)
	} else {
		facePlayAddr.draw(addr, x+16, ty, signInk)
	}
	if lit && int(rl.GetTime()*2)%2 == 0 {
		cx := x + 16 + facePlayAddr.width(addr) + 2
		if addr == "" {
			cx = x + 16
		}
		rl.DrawRectangle(cx, ty, 2, facePlayAddr.height(), signInk)
	}
	y += 52 + 8
	faceSmall.draw("port 7777 is assumed if you leave it off", x, y, signDim)
	y += faceSmall.height() + 16
	drawPlayCell(rect(x, y, w, playCellH), vs["car"], "")
}

// drawBar draws the bottom bar: Back, the summary, the reason the start
// cannot happen, and the start button.
func (p *playPage) drawBar(b playBox, vs map[string]rowView) {
	r := rect(b.left, b.bar, b.width(), playBarH)
	rounded(r, 12, signCard)
	ring(r, 12, 1, signRule)

	backW := faceRow.width("< Back") + 32
	drawButton(rect(b.left+10, b.bar+14, backW, 44), "< Back", vs["back"].Selected, false, tonePlain)

	goV := vs["go"]
	label := goV.Label + "  >"
	goW := max(240, faceRow.width(label)+48)
	goX := b.right - 8 - goW
	drawButton(rect(goX, b.bar+8, goW, playBarH-16), label, goV.Selected, goV.Disabled != "", tonePlain)

	x := b.left + 10 + backW + 18
	room := goX - 16 - x
	why := p.list.Reason()
	if why == "" {
		why = p.why()
	}
	if why == "" {
		faceBody.draw(playFit(faceBody, p.summary(), room), x, b.bar+(playBarH-faceBody.height())/2, signInk)
		return
	}
	faceBody.draw(playFit(faceBody, p.summary(), room), x, b.bar+14, signInk)
	faceSmall.draw(playFit(faceSmall, why, room), x, b.bar+playBarH-14-faceSmall.height(), signAmber)
}

// playSegments draws a row of choices as one control, the chosen one filled,
// and returns how wide it is: the accent while lit, grey for any locked. A
// control standing alone is ringed with the accent while lit; one inside a
// cell leaves that to the cell.
func playSegments(x, y, h int32, f typeFace, opts []string, choice int, locked []bool, lit, alone bool) int32 {
	const pad, inset = 14, 4
	w := int32(inset)
	for _, o := range opts {
		w += f.width(o) + 2*pad
	}
	w += inset
	r := rect(x, y-inset, w, h+2*inset)
	rounded(r, 10, signCard)
	if lit && alone {
		ring(r, 10, 2, signBlue)
	}
	at := x + inset
	for i, o := range opts {
		sw := f.width(o) + 2*pad
		ink := signDim
		if i < len(locked) && locked[i] {
			ink = signGrey
		}
		if i == choice {
			fill := signRule
			ink = signInk
			if lit {
				fill, ink = signBlue, menuBackdrop
			}
			rounded(rect(at, y, sw, h), 7, fill)
		}
		f.draw(o, at+pad, y+(h-f.height())/2, ink)
		at += sw
	}
	return w
}

// playFit shortens s with "..." until it fits w in face f.
func playFit(f typeFace, s string, w int32) string {
	if f.width(s) <= w {
		return s
	}
	for n := len(s) - 1; n > 0; n-- {
		if t := s[:n] + "..."; f.width(t) <= w {
			return t
		}
	}
	return ""
}

// playWrap breaks s into at most lines lines that fit w in face f, the last
// shortened if there is more.
func playWrap(f typeFace, s string, w int32, lines int) []string {
	var out []string
	cur := ""
	for _, word := range strings.Fields(s) {
		next := strings.TrimSpace(cur + " " + word)
		if cur == "" || f.width(next) <= w {
			cur = next
			continue
		}
		out = append(out, cur)
		cur = word
	}
	if cur != "" {
		out = append(out, cur)
	}
	if len(out) > lines {
		out = out[:lines]
		out[lines-1] = playFit(f, out[lines-1]+" ...", w)
	}
	return out
}

// playBlend mixes a toward b by t.
func playBlend(a, b rl.Color, t float32) rl.Color {
	mix := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t) }
	return rl.NewColor(mix(a.R, b.R), mix(a.G, b.G), mix(a.B, b.B), 255)
}
