package game

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/keymap"
	"github.com/danielriddell21/crucible/menu"
)

// The screen registry. Each menu screen is a page: one file that registers
// itself in one line,
//
//	var _ = registerPage(pagePlay, newPlayPage, hintsAdjust...)
//
// and is then reached with s.openPage(pagePlay) from anywhere. Nothing in
// shell.go needs editing for a new page: stepMenu and drawScreen hand whatever
// page is showing to stepPage and drawPage.
//
// Every id below has a screen from the start. One no file has registered is a
// stub, its title and a Back row, so a page can link to one before it is built.
// A file that registers an id replaces its stub.

// The ids of the pages. The only screen that is not one is screenPlaying, the
// drive itself (shell.go).
const (
	// pageTitle is the front page: Play, Garage, Career, Settings, Quit. It is
	// the zero screen.
	pageTitle screen = iota
	// pagePlay starts a drive: free drive, a challenge or multiplayer.
	pagePlay
	// pageGarage chooses, paints and buys the car.
	pageGarage
	// pagePause is the menu over a paused drive.
	pagePause
	// pageCareer is the bounty, earnings, packs and achievements.
	pageCareer
	// pageSettings is the stored preferences, in sections.
	pageSettings
	// pageCustom edits the custom map.
	pageCustom
	// pageLobby waits for the other player.
	pageLobby
	// pageQuit is the dialog that asks before the game closes.
	pageQuit
	// pageLeave is the dialog that asks before leaving a networked game.
	pageLeave

	pageEnd // one past the last page; not a screen
)

// pageTitles is what each stub is called until its screen is built.
var pageTitles = map[screen]string{
	pageTitle:    "AUTOBAHN",
	pagePlay:     "PLAY",
	pageGarage:   "GARAGE",
	pagePause:    "PAUSED",
	pageCareer:   "CAREER",
	pageSettings: "SETTINGS",
	pageCustom:   "CUSTOM MAP",
	pageLobby:    "LOBBY",
	pageQuit:     "QUIT",
	pageLeave:    "LEAVE THE GAME",
}

// isPage reports whether id is one of the registry's screens.
func isPage(id screen) bool { return id >= pageTitle && id < pageEnd }

// navInput is a frame's keys on a page: crucible's menu intent, read by
// pollMenu with the family's bindings, and esc, which goes back.
type navInput struct {
	menu.Input
	Back bool
}

// pollNav reads a page's keys from raylib.
func pollNav() navInput {
	return navInput{Input: pollMenu(), Back: rl.IsKeyPressed(rl.KeyEscape)}
}

// The hint sets a page registers with, for the bar along the bottom.
var (
	// hintsChoose is for a list of actions. It is what a page that names no
	// hints gets.
	hintsChoose = []keymap.Binding{
		{Key: "up/down", Action: "choose"},
		{Key: "enter", Action: "select"},
		{Key: "esc", Action: "back"},
	}
	// hintsAdjust is for a list with rows that ←→ change.
	hintsAdjust = []keymap.Binding{
		{Key: "up/down", Action: "choose"},
		{Key: "left/right", Action: "change"},
		{Key: "enter", Action: "select"},
		{Key: "esc", Action: "back"},
	}
	// hintsDialog is for buttons side by side.
	hintsDialog = []keymap.Binding{
		{Key: "left/right", Action: "choose"},
		{Key: "enter", Action: "select"},
		{Key: "esc", Action: "back"},
	}
)

// menuPage is a screen as its builder makes it. Only Title is needed: a page
// with a List and nothing else is drawn as the standard Signpost list, and
// every hook is there for a screen that wants more.
type menuPage struct {
	Title string
	// Lede is an optional line under the title.
	Lede string
	// List is the rows. A page may have none, if Step reads the keys itself.
	List *rowList
	// OverGame draws the page over the paused drive, dimmed, when there is
	// one, as the pause page is; otherwise it stands on its Backdrop.
	OverGame bool
	// Backdrop is what the page stands on when it is not over a game. The zero
	// value is the menu drive, while it is on; a page that draws a whole scene
	// of its own (the garage) asks for the plain one. See menu_drive.go.
	Backdrop pageBackdrop

	// Step sees each frame's keys first, and reports whether it used them; it
	// runs every frame, so it is also where a page watches something (the
	// lobby, a session). Unused keys go to esc, then to List.
	Step func(in navInput) bool
	// Draw replaces the standard drawing of the title and list; the backdrop
	// and the hint bar are drawn either way.
	Draw func(dt float32)
	// Back is what esc does. A builder that leaves it nil gets s.backPage.
	Back func()
	// Shown runs whenever the page comes into view, first or on coming back
	// to it, so it can refresh what it shows.
	Shown func()
	// Hints, when set, replaces the hint set the page was registered with,
	// for a page whose keys change with what is selected.
	Hints func() []keymap.Binding
}

// pageBuilder makes a page for the shell. It runs each time the page is
// opened, not on coming back to it.
type pageBuilder func(s *Shell) *menuPage

type pageEntry struct {
	build pageBuilder
	hints []keymap.Binding
}

// pages is the registry: every page a file has registered.
var pages = map[screen]pageEntry{}

// registerPage files a screen under its id, with the hints for its bar (none
// gives hintsChoose). It returns true so a file can register in one line at
// package level. Registering an id twice, or one that is not a page, is a
// mistake in the program and panics as the package loads.
func registerPage(id screen, build pageBuilder, hints ...keymap.Binding) bool {
	if !isPage(id) {
		panic(fmt.Sprintf("registerPage: %d is not a page id", id))
	}
	if build == nil {
		panic(fmt.Sprintf("registerPage: %s has no builder", pageTitles[id]))
	}
	if _, dup := pages[id]; dup {
		panic(fmt.Sprintf("registerPage: %s is registered twice", pageTitles[id]))
	}
	pages[id] = pageEntry{build: build, hints: hints}
	return true
}

// pageFor is the entry for id: the registered one, or the stub. stub reports
// which.
func pageFor(id screen) (e pageEntry, stub bool) {
	if e, ok := pages[id]; ok {
		return e, false
	}
	title := pageTitles[id]
	return pageEntry{build: func(s *Shell) *menuPage { return stubPage(s, title) }}, true
}

// stubPage is a screen not built yet: its title and the way back.
func stubPage(s *Shell, title string) *menuPage {
	return &menuPage{
		Title: title,
		Lede:  "not built yet",
		List:  newRowList(actionRow("back", "Back", s.backPage)),
	}
}

// hintsFor is the bar for a page: its own, else what it registered, else
// hintsChoose.
func hintsFor(id screen, pg *menuPage) []keymap.Binding {
	if pg != nil && pg.Hints != nil {
		return pg.Hints()
	}
	if e, _ := pageFor(id); len(e.hints) > 0 {
		return e.hints
	}
	return hintsChoose
}

// pager is the shell's place among the pages: the page showing, and the
// screens esc goes back through.
type pager struct {
	cur   *menuPage
	stack []pageFrame
}

// pageFrame is a screen to go back to: a page, kept as it was so it comes
// back with its selection, or the drive (pg nil).
type pageFrame struct {
	id screen
	pg *menuPage
}

// onPage reports whether the shell is showing a page from the registry.
func (s *Shell) onPage() bool { return isPage(s.screen) && s.pager.cur != nil }

// buildPage makes the page id afresh, with esc going back unless the page
// says otherwise.
func (s *Shell) buildPage(id screen) *menuPage {
	e, _ := pageFor(id)
	pg := e.build(s)
	if pg.Back == nil {
		pg.Back = s.backPage
	}
	return pg
}

// openPage shows the page id, building it afresh; esc on it comes back to
// what was showing. Opened from the drive, the trail starts again with the
// drive, which is where esc goes back to.
func (s *Shell) openPage(id screen) {
	if s.onPage() {
		s.pager.stack = append(s.pager.stack, pageFrame{s.screen, s.pager.cur})
	} else {
		s.pager.stack = []pageFrame{{id: s.screen}}
	}
	s.showPage(id, s.buildPage(id))
}

// backPage goes back one screen: to the page before, as it was, or to the
// drive the trail started from. With nothing to go back to, or a drive that
// has gone, it goes to the title page.
func (s *Shell) backPage() {
	n := len(s.pager.stack)
	if n == 0 {
		s.goTitle()
		return
	}
	f := s.pager.stack[n-1]
	s.pager.stack = s.pager.stack[:n-1]
	switch {
	case f.pg != nil:
		s.showPage(f.id, f.pg)
	case f.id == screenPlaying && s.game != nil:
		s.pager = pager{}
		s.resume()
	default:
		s.goTitle()
	}
}

// showPage puts a built page on the screen.
func (s *Shell) showPage(id screen, pg *menuPage) {
	s.screen, s.pager.cur = id, pg
	if pg.Shown != nil {
		pg.Shown()
	}
}

// stepPage applies a frame's keys to the page showing.
func (s *Shell) stepPage(in navInput) {
	pg := s.pager.cur
	if pg.Step != nil && pg.Step(in) {
		return
	}
	if in.Back {
		pg.Back()
		return
	}
	if pg.List != nil {
		pg.List.Update(in.Input)
	}
}
