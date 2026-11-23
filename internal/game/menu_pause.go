package game

import (
	"fmt"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/geom"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

// The Pause page (docs/menus.md, "Pause"): the menu over a paused drive. It is
// drawn over the frozen, dimmed scene (OverGame), and what it says is worked
// out from the Game by plain functions below, so tests need no window.
//
// The shell opens it when esc is pressed on a game that is not
// [Game.Networked] (Shell.escapeDrive). In a networked game esc leaves through
// the leave dialog (pageLeave) and this page is never offered: [pauseOffered]
// says so, and a builder run anyway makes a page with nothing to resume. The
// shell calls [Game.notePeakSpeed] after each step of the game, which is what
// the top-speed tile shows.

var _ = registerPage(pagePause, newPausePage, hintsAdjust...)

// pauseOffered reports whether a drive can be paused: there is one, and it is
// not a game somebody else is in.
func pauseOffered(g *Game) bool { return g != nil && !g.Networked() }

// peakSpeed is the fastest the player has gone in the game it is for. The
// Game keeps no such figure, and this file may not add one to it, so the shell
// feeds this through notePeakSpeed. Only one drive is ever running, and the
// shell lets go of it with forgetPeakSpeed when it closes the drive, so a
// closed game is not kept alive here.
var peakSpeed struct {
	g *Game
	v float32
}

// forgetPeakSpeed drops what was noted for g, if it is the game noted.
func forgetPeakSpeed(g *Game) {
	if peakSpeed.g == g {
		peakSpeed.g, peakSpeed.v = nil, 0
	}
}

// notePeakSpeed looks at the player's speed for one frame.
func (g *Game) notePeakSpeed() {
	if g == nil || g.world == nil || g.world.Player == nil {
		return
	}
	if peakSpeed.g != g {
		peakSpeed.g, peakSpeed.v = g, 0
	}
	peakSpeed.v = max(peakSpeed.v, g.world.Player.Speed())
}

// topSpeed is the fastest the player has gone, in metres per second, and
// whether anything has been noted at all.
func (g *Game) topSpeed() (float32, bool) {
	if peakSpeed.g != g {
		return 0, false
	}
	return peakSpeed.v, true
}

// bountySoFar is what the drive has banked and what the pursuit going on is
// worth, in bounty points.
func (g *Game) bountySoFar() (banked, live int) {
	wa := &g.world.Wanted
	return wa.Banked, wa.Bounty
}

// pauseState says whether the drive is banking now, and if not why not. The
// first reason that applies is the one given.
type pauseState struct {
	Banking bool
	// Why is the reason nothing banks, or "".
	Why string
}

// bankingState works out whether the drive counts toward the career.
func bankingState(g *Game) pauseState {
	switch {
	case g.chal != nil:
		return pauseState{Why: "a challenge is a score, not a drive: it banks nothing"}
	case g.opts.Custom != nil:
		return pauseState{Why: "a custom map banks no bounty, earnings or achievements"}
	case g.auto:
		return pauseState{Why: "the autopilot is driving, so bounty is not banked"}
	case g.career.assisted:
		return pauseState{Why: "the autopilot had a hand in this pursuit, so it is not banked"}
	case g.helped():
		return pauseState{Why: "this drive is not driven by you alone, so it banks nothing"}
	}
	return pauseState{Banking: true}
}

// pauseTile is one of the six figures across the top.
type pauseTile struct {
	Key, Label, Value, Note string
}

// clockText words a time in seconds as m:ss, or h:mm:ss past the hour.
func clockText(sec float32) string {
	n := max(int(sec), 0)
	if n >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", n/3600, n%3600/60, n%60)
	}
	return fmt.Sprintf("%d:%02d", n/60, n%60)
}

// milesText words a distance in metres as miles, or metres while it is short.
func milesText(m float32) string {
	if m < 160 {
		return fmt.Sprintf("%d m", int(m))
	}
	return fmt.Sprintf("%.1f mi", m/1609.344)
}

// pauseTiles works out the six tiles: time, distance, bounty (the score, in a
// challenge), wanted, faults and top speed.
func pauseTiles(g *Game) []pauseTile {
	sum := g.Summary()
	banked, live := g.bountySoFar()
	bounty := pauseTile{Key: "bounty", Label: "Bounty", Value: money(banked)}
	if live > 0 {
		bounty.Note = money(live) + " in this pursuit"
	}
	if g.chal != nil {
		bounty = pauseTile{
			Key: "bounty", Label: "Score", Value: fmt.Sprint(g.chal.run.Score()),
			Note: fmt.Sprintf("ghost %d", g.chal.ghost),
		}
	}
	wanted := pauseTile{Key: "wanted", Label: "Wanted", Value: fmt.Sprint(g.world.Wanted.Level)}
	if sum.Wanted > 0 {
		wanted.Note = fmt.Sprintf("worst %d", sum.Wanted)
	}
	faults := pauseTile{Key: "faults", Label: "Faults", Value: fmt.Sprint(sum.TotalFaults)}
	if sum.Points > 0 {
		faults.Note = fmt.Sprintf("%d penalty points", sum.Points)
	}
	top := pauseTile{Key: "top", Label: "Top speed", Value: "-"}
	if v, ok := g.topSpeed(); ok {
		top.Value = fmt.Sprintf("%d mph", int(mathx.ToMPH(v)+0.5))
	}
	return []pauseTile{
		{Key: "time", Label: "Time", Value: clockText(sum.Seconds)},
		{Key: "distance", Label: "Distance", Value: milesText(sum.Distance)},
		bounty, wanted, faults, top,
	}
}

// pauseHead is the top line besides PAUSED: the map and its season, the mode,
// and the banking tag.
type pauseHead struct {
	Map, Season, Mode string
	Banking           bool
	// Tag is BANKING or NOT BANKING, for the chip.
	Tag string
}

// pauseHeadOf works the top line out.
func pauseHeadOf(g *Game) pauseHead {
	h := pauseHead{Map: g.opts.Map.Name(), Season: g.season().Name()}
	if g.opts.Custom != nil {
		h.Map = "custom map"
	}
	switch {
	case g.chal != nil:
		h.Mode = "challenge: " + g.chal.run.Scenario().Name
	case g.auto:
		h.Mode = "autopilot, " + g.driver.Mode.String()
	default:
		h.Mode = "free drive"
	}
	h.Banking = bankingState(g).Banking
	h.Tag = "not banking"
	if h.Banking {
		h.Tag = "banking"
	}
	return h
}

// pauseCard is the card on the right about this drive.
type pauseCard struct {
	Title string
	Lines []string
	Tone  tone
}

// pauseCardOf says what the drive stands at and what leaving it does.
func pauseCardOf(g *Game) pauseCard {
	st := bankingState(g)
	if g.chal != nil {
		c := pauseCard{Title: "You against the ghost", Tone: toneOffer}
		c.Lines = []string{
			fmt.Sprintf("you %d   ghost %d", g.chal.run.Score(), g.chal.ghost),
			st.Why + ".",
			"Restarting begins the attempt again.",
		}
		return c
	}
	banked, live := g.bountySoFar()
	if !st.Banking {
		c := pauseCard{Title: "Nothing is banking", Tone: toneOffer, Lines: []string{st.Why + "."}}
		if g.opts.Custom == nil && !g.auto {
			c.Lines = append(c.Lines, "It counts again once the pursuit is over.")
		}
		return c
	}
	c := pauseCard{Title: "This drive", Tone: toneYours}
	c.Lines = []string{
		fmt.Sprintf("Banked so far: %s.", money(banked)),
		"Abandoning keeps what is banked.",
		"Restarting begins a new drive; what is banked stays banked.",
	}
	if live > 0 {
		c.Lines = append(c.Lines, fmt.Sprintf("The %s on this pursuit is lost if you leave now.", money(live)))
	}
	return c
}

// pauseFoot is the car and paint line along the bottom.
func (s *Shell) pauseFoot() string {
	return "Car: " + s.settings.Car.Name() + ", " + paintOf(s.settings.Paint).name
}

// leavePause is what every way off the page does first: the quick settings
// are written once, as the settings page writes them when it closes.
func (s *Shell) leavePause() {
	if err := s.settings.Save(); err != nil {
		s.notice = err.Error()
	}
	s.pager = pager{}
}

// resumePause goes back to the drive.
func (s *Shell) resumePause() {
	s.leavePause()
	s.resume()
}

// applyQuick puts the quick settings on the running game and the sound.
func (s *Shell) applyQuick() {
	rl.SetMasterVolume(masterVolume(s.settings))
	if g := s.game; g != nil {
		g.ShowPanel(s.settings.ShowPanel)
		g.showLabels = s.settings.ShowLabels
		g.SetCameraMode(s.settings.Camera)
	}
}

// pauseRows builds the page's rows: the left column in order, abandon last of
// them, then the quick settings. pauseLeftRows says how many are the left.
func (s *Shell) pauseRows() []menuRow {
	g := s.game
	set := &s.settings
	return []menuRow{
		actionRow("resume", "Resume", s.resumePause),
		actionRow("restart", "Restart", func() {
			s.leavePause()
			s.startGame(s.game.Autopilot())
		}).describe("A new drive on the same map."),
		switchRow("autopilot", "Autopilot",
			func() bool { return g.Autopilot() },
			func(on bool) {
				if on != g.Autopilot() {
					g.toggleAutopilot()
				}
			}).describe("Same as Tab. Turned on, the drive stops banking.").
			disabledBy(func() string {
				if g.chal != nil {
					return "a challenge is driven by hand"
				}
				return ""
			}),
		actionRow("garage", "Garage", func() { s.openPage(pageGarage) }).
			noted(func() string { return set.Car.Name() }),
		actionRow("settings", "All settings", func() { s.openPage(pageSettings) }),
		actionRow("abandon", "Abandon the drive", func() {
			s.leavePause()
			s.endGame()
		}).toned(func() tone { return toneDanger }),

		optionsRow("viewpoint", "Viewpoint", cameraNames(),
			func() int { return int(set.Camera) },
			func(i int) { set.Camera = CameraMode(i); s.applyQuick() }),
		switchRow("aicamera", "AI camera",
			func() bool { return set.ShowPanel },
			func(on bool) { set.ShowPanel = on; s.applyQuick() }),
		switchRow("labels", "Box labels",
			func() bool { return set.ShowLabels },
			func(on bool) { set.ShowLabels = on; s.applyQuick() }),
		barRow("volume", "Volume",
			func() float64 {
				if set.Muted {
					return 0
				}
				return set.Volume
			},
			func() string {
				if set.Muted {
					return "muted"
				}
				return fmt.Sprintf("%d%%", int(set.Volume*100+0.5))
			},
			func(dir int) {
				set.Volume = geom.Clamp(set.Volume+float64(dir)*volumeStep, 0, 1)
				set.Muted = false
				s.applyQuick()
			}),
	}
}

// pauseLeftRows is how many of pauseRows are in the left column.
const pauseLeftRows = 6

// cameraNames are the viewpoints, in CameraMode order.
func cameraNames() []string {
	out := make([]string, numCameraModes)
	for i := range out {
		out[i] = CameraMode(i).String()
	}
	return out
}

// newPausePage builds the page. A game that is not there, or is somebody
// else's, is not paused: the page then says so and offers the way back.
func newPausePage(s *Shell) *menuPage {
	if !pauseOffered(s.game) {
		return &menuPage{
			Title: "PAUSED",
			Lede:  "a game somebody else is in cannot be paused",
			List:  newRowList(actionRow("back", "Back", s.backPage)),
		}
	}
	pg := &menuPage{
		Title:    "PAUSED",
		List:     newRowList(s.pauseRows()...),
		OverGame: true,
		Back:     s.resumePause,
	}
	pg.Draw = func(float32) { s.drawPause(pg) }
	return pg
}

// Layout of the page, in pixels.
const (
	pauseTileTop  = 128
	pauseTileH    = 76
	pauseColTop   = 236
	pauseGap      = 20 // set before Abandon
	pauseColW     = 460
	pauseColSpace = 48
)

var faceTile = typeFace{28, weightHeavy}

// drawPause draws the page.
func (s *Shell) drawPause(pg *menuPage) {
	g := s.game
	w, h := int32(s.opts.Width), int32(s.opts.Height)
	faceTitle.draw("PAUSED", pageMargin, pageTop, signInk)

	// The top line: map and season, the mode, and the banking tag.
	head := pauseHeadOf(g)
	x := int32(pageMargin) + faceTitle.width("PAUSED") + 28
	y := int32(pageTop) + 10
	x += drawChip(x, y, head.Map+" "+head.Season, toneOffer) + 12
	faceBody.draw(head.Mode, x, y+2, signInk)
	x += faceBody.width(head.Mode) + 16
	tag := toneYours
	if !head.Banking {
		tag = toneOffer
	}
	drawChip(x, y, head.Tag, tag)
	if why := bankingState(g).Why; why != "" {
		faceSmall.draw(why, pageMargin, pageTop+faceTitle.height()+10, signDim)
	}

	// Six tiles across.
	tiles := pauseTiles(g)
	span := w - 2*pageMargin
	const tileGap = 12
	tw := (span - tileGap*int32(len(tiles)-1)) / int32(len(tiles))
	for i, t := range tiles {
		r := rect(pageMargin+int32(i)*(tw+tileGap), pauseTileTop, tw, pauseTileH)
		drawCard(r, false)
		faceChip.draw(strings.ToUpper(t.Label), int32(r.X)+14, int32(r.Y)+10, signDim)
		faceTile.draw(t.Value, int32(r.X)+14, int32(r.Y)+26, signInk)
		if t.Note != "" {
			faceChip.draw(t.Note, int32(r.X)+14, int32(r.Y)+pauseTileH-16, signDim)
		}
	}

	// The left column, Abandon set apart; then the right.
	vs := pg.List.Views()
	rowH := listRowHeight(vs)
	ly := int32(pauseColTop)
	for i := 0; i < pauseLeftRows && i < len(vs); i++ {
		if vs[i].Key == "abandon" {
			ly += pauseGap
			rl.DrawRectangle(pageMargin, ly-pauseGap/2, pauseColW, 1, signRule)
		}
		drawRowView(rect(pageMargin, ly, pauseColW, rowH-4), vs[i])
		ly += rowH
	}
	rx := int32(pageMargin + pauseColW + pauseColSpace)
	rw := max(w-pageMargin-rx, 300)
	ry := int32(pauseColTop)
	faceChip.draw("QUICK SETTINGS", rx+16, ry, signDim)
	ry += 22
	for i := pauseLeftRows; i < len(vs); i++ {
		drawRowView(rect(rx, ry, rw, rowH-4), vs[i])
		ry += rowH
	}
	ry += 12
	card := pauseCardOf(g)
	lines := wrapText(card.Lines, rw-32, faceBody)
	ch := 24 + faceRow.height() + int32(len(lines))*(faceBody.height()+4) + 12
	r := rect(rx, ry, rw, ch)
	drawCard(r, false)
	faceRow.draw(card.Title, rx+16, ry+12, toneInk(card.Tone))
	ty := ry + 20 + faceRow.height()
	for _, l := range lines {
		faceBody.draw(l, rx+16, ty, signDim)
		ty += faceBody.height() + 4
	}

	// The footer, above the hint bar.
	faceSmall.draw(s.pauseFoot(), pageMargin, h-pageFoot-24, signDim)
}

// wrapText breaks each paragraph into lines no wider than w in the face.
func wrapText(paras []string, w int32, f typeFace) []string {
	var out []string
	for _, p := range paras {
		line := ""
		for _, word := range strings.Fields(p) {
			try := strings.TrimSpace(line + " " + word)
			if line != "" && f.width(try) > w {
				out = append(out, line)
				line = word
				continue
			}
			line = try
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
