package game

// The quit and leave dialogs (docs/menus.md, "Quit and leave"): a card over
// the dimmed screen with two buttons side by side, Stay selected first.

var (
	_ = registerPage(pageQuit, newQuitPage, hintsDialog...)
	_ = registerPage(pageLeave, newLeavePage, hintsDialog...)
)

const (
	quitText  = "Your career is saved: bounty and earnings are kept."
	leaveText = "There is no pausing with someone else in it. Leaving ends the game for both of you."
)

// newQuitPage asks before the game closes. Esc stays.
func newQuitPage(s *Shell) *menuPage {
	return dialogPage(s, "QUIT?", quitText, "Quit", func() { s.quit = true }, false)
}

// newLeavePage asks before a networked game is left. It is drawn over the
// game, which keeps running: a chase cannot be paused.
func newLeavePage(s *Shell) *menuPage {
	return dialogPage(s, "LEAVE THE GAME?", leaveText, "Leave", func() {
		s.pager = pager{}
		s.endGame()
	}, true)
}

// dialogPage is a two-button dialog: Stay (back one screen) and a danger
// button that does act. Esc is Stay.
func dialogPage(s *Shell, title, body, label string, act func(), overGame bool) *menuPage {
	l := newRowList(
		actionRow("stay", "Stay", s.backPage),
		actionRow("go", label, act).toned(func() tone { return toneDanger }),
	)
	l.Across = true
	pg := &menuPage{Title: title, Lede: body, List: l, OverGame: overGame}
	pg.Draw = func(float32) { drawDialog(s, pg) }
	return pg
}

// drawDialog draws the card, its words and the buttons, centred.
func drawDialog(s *Shell, pg *menuPage) {
	w, h := int32(s.opts.Width), int32(s.opts.Height)
	cw, ch := min(w-2*pageMargin, int32(560)), int32(220)
	cx, cy := (w-cw)/2, (h-ch)/2
	drawCard(rect(cx, cy, cw, ch), true)
	faceRow.draw(pg.Title, cx+24, cy+24, signInk)
	drawWrapped(pg.Lede, cx+24, cy+64, cw-48)
	bw := (cw - 48 - 12) / 2
	for i, v := range pg.List.Views() {
		drawButton(rect(cx+24+int32(i)*(bw+12), cy+ch-24-44, bw, 44), v.Label, v.Selected, false, v.Tone)
	}
}

// drawWrapped draws words wrapped to width.
func drawWrapped(s string, x, y, width int32) {
	line := ""
	flush := func() {
		faceBody.draw(line, x, y, signDim)
		y += faceBody.height() + 6
		line = ""
	}
	word := ""
	add := func() {
		if line != "" && faceBody.width(line+" "+word) > width {
			flush()
		}
		if line != "" {
			line += " "
		}
		line += word
		word = ""
	}
	for _, r := range s {
		if r != ' ' {
			word += string(r)
			continue
		}
		add()
	}
	if word != "" {
		add()
	}
	if line != "" {
		flush()
	}
}
