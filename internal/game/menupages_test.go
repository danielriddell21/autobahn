package game

import (
	"slices"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/keymap"
)

// testShell is a shell on a fresh config directory, on its title screen.
func testShell(t *testing.T) *Shell {
	t.Helper()
	inTempConfig(t)
	o := DefaultOptions()
	o.Mute = true
	sh := NewShell(o)
	t.Cleanup(sh.net.Leave)
	return sh
}

// allPages is every page id, in order.
func allPages() []screen {
	out := make([]screen, 0, pageEnd-pageTitle)
	for id := pageTitle; id < pageEnd; id++ {
		out = append(out, id)
	}
	return out
}

// stubsOnly empties the registry until the test ends, so every page is its
// stub and the registry's own rules can be tested whatever pages are built.
func stubsOnly(t *testing.T) {
	t.Helper()
	kept := pages
	pages = map[screen]pageEntry{}
	t.Cleanup(func() { pages = kept })
}

// registerForTest registers build under id until the test ends.
func registerForTest(t *testing.T, id screen, build pageBuilder, hints ...keymap.Binding) {
	t.Helper()
	if !registerPage(id, build, hints...) {
		t.Fatal("registerPage reported false, which a one-line registration cannot use")
	}
	t.Cleanup(func() { delete(pages, id) })
}

func TestEveryPageIDHasAScreen(t *testing.T) {
	if isPage(screenPlaying) || isPage(pageEnd) || !isPage(pageTitle) {
		t.Fatal("isPage takes in ids that are not pages")
	}
	sh := testShell(t)
	ids := allPages()
	if len(ids) != 10 {
		t.Errorf("%d page ids, want title, play, garage, pause, career, settings, custom, lobby, quit and leave", len(ids))
	}
	for _, id := range ids {
		if pageTitles[id] == "" {
			t.Errorf("page %d has no title", id)
		}
		e, stub := pageFor(id)
		pg := e.build(sh)
		if pg == nil || pg.Title == "" {
			t.Errorf("%s builds no page, or one with no title", pageTitles[id])
			continue
		}
		if !stub {
			continue
		}
		if pg.List == nil {
			t.Fatalf("the %s stub has no rows", pageTitles[id])
		}
		if _, ok := pg.List.Row("back"); !ok {
			t.Errorf("the %s stub has no way back", pageTitles[id])
		}
	}
}

func TestARegisteredPageReplacesItsStub(t *testing.T) {
	stubsOnly(t)
	id := pageCareer
	built := 0
	registerForTest(t, id, func(*Shell) *menuPage {
		built++
		return &menuPage{Title: "MINE"}
	}, hintsAdjust...)

	e, stub := pageFor(id)
	if stub {
		t.Fatal("the registry still hands out the stub")
	}
	if pg := e.build(testShell(t)); pg.Title != "MINE" || built != 1 {
		t.Errorf("built %q, %d times", pg.Title, built)
	}
	if !slices.Equal(hintsFor(id, nil), hintsAdjust) {
		t.Errorf("hints %v, want the set it registered with", hintsFor(id, nil))
	}
}

func TestRegisteringBadlyPanics(t *testing.T) {
	stubsOnly(t)
	id := pagePlay
	build := func(*Shell) *menuPage { return &menuPage{Title: "X"} }
	registerForTest(t, id, build)

	panics := func(name string, f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s did not panic", name)
			}
		}()
		f()
	}
	panics("registering twice", func() { registerPage(id, build) })
	panics("registering the drive", func() { registerPage(screenPlaying, build) })
	panics("registering past the end", func() { registerPage(pageEnd, build) })
	panics("registering no builder", func() { registerPage(pageGarage, nil) })
	if _, ok := pages[pageGarage]; ok {
		t.Error("a page with no builder was registered")
	}
}

func TestPageHints(t *testing.T) {
	stubsOnly(t)
	id := pageSettings
	if !slices.Equal(hintsFor(id, nil), hintsChoose) {
		t.Errorf("a stub's hints are %v, want hintsChoose", hintsFor(id, nil))
	}
	own := []keymap.Binding{{Key: "type", Action: "address"}}
	pg := &menuPage{Hints: func() []keymap.Binding { return own }}
	if !slices.Equal(hintsFor(id, pg), own) {
		t.Error("a page's own hints did not win")
	}
	for _, set := range [][]keymap.Binding{hintsChoose, hintsAdjust, hintsDialog} {
		if !slices.ContainsFunc(set, func(b keymap.Binding) bool { return b.Key == "esc" }) {
			t.Errorf("%v does not say esc goes back", set)
		}
	}
}

func TestPagesGoBackTheWayTheyCame(t *testing.T) {
	stubsOnly(t)
	sh := testShell(t)
	sh.openPage(pagePlay)
	if sh.screen != pagePlay || !sh.onPage() {
		t.Fatalf("screen = %v, want play", sh.screen)
	}
	play := sh.pager.cur

	sh.openPage(pageGarage)
	if sh.screen != pageGarage {
		t.Fatalf("screen = %v, want the garage", sh.screen)
	}

	// esc comes back to the same page, not a new one.
	sh.stepPage(navInput{Back: true})
	if sh.screen != pagePlay || sh.pager.cur != play {
		t.Errorf("esc from the garage gave %v", sh.screen)
	}

	// The stub's Back row is the same as esc: back to the title page it was
	// opened from.
	sh.stepPage(navInput{Input: keyEnter})
	if sh.screen != pageTitle || !sh.onPage() || len(sh.pager.stack) != 0 {
		t.Errorf("back from play gave %v (on a page: %v)", sh.screen, sh.onPage())
	}

	// With nothing behind it, back is the title.
	sh.openPage(pageCareer)
	sh.pager.stack = nil
	sh.backPage()
	if sh.screen != pageTitle || !sh.onPage() {
		t.Errorf("back from a page with nothing behind it gave %v", sh.screen)
	}
}

func TestOpeningAPageFromTheDriveStartsTheTrailAgain(t *testing.T) {
	stubsOnly(t)
	sh := testShell(t)
	sh.openPage(pagePlay)
	sh.openPage(pageGarage)
	// A drive starts, and a page is opened over it.
	sh.game = bareGame(Options{})
	sh.pager, sh.screen = pager{}, screenPlaying
	sh.openPage(pagePause)
	sh.openPage(pageSettings)
	if len(sh.pager.stack) != 2 || sh.pager.stack[0].id != screenPlaying {
		t.Fatalf("the trail is %v, want the drive and the pause page", sh.pager.stack)
	}
	sh.stepPage(navInput{Back: true})
	sh.stepPage(navInput{Back: true})
	if sh.screen != screenPlaying || sh.onPage() || len(sh.pager.stack) != 0 {
		t.Errorf("back went to %v, trail %v", sh.screen, sh.pager.stack)
	}
	// A trail back to a drive that has gone ends on the title.
	sh.openPage(pagePause)
	sh.game = nil
	sh.stepPage(navInput{Back: true})
	if sh.screen != pageTitle || !sh.onPage() {
		t.Errorf("back to no drive went to %v", sh.screen)
	}
}

func TestPageHooks(t *testing.T) {
	stubsOnly(t)
	id := pageLobby
	shown, backs, stepped := 0, 0, 0
	moved := false
	registerForTest(t, id, func(*Shell) *menuPage {
		return &menuPage{
			Title: "HOOKS",
			List:  newRowList(actionRow("a", "A", nil), actionRow("b", "B", func() { moved = true })),
			Shown: func() { shown++ },
			Back:  func() { backs++ },
			Step: func(in navInput) bool {
				stepped++
				return in.Left // the page keeps left for itself
			},
		}
	})
	sh := testShell(t)
	sh.openPage(id)
	if shown != 1 {
		t.Errorf("shown %d times on opening", shown)
	}
	sh.stepPage(navInput{Input: keyDown})
	sh.stepPage(navInput{Input: keyEnter})
	if !moved || stepped != 2 {
		t.Errorf("the list did not get the keys the page passed on (stepped %d)", stepped)
	}
	sh.stepPage(navInput{Input: keyLeft, Back: true})
	if backs != 0 {
		t.Error("a key the page used went on to esc")
	}
	sh.stepPage(navInput{Back: true})
	if backs != 1 || sh.screen != id {
		t.Errorf("esc ran %d backs, and left the page for %v", backs, sh.screen)
	}

	// Coming back to it from another page shows it again.
	sh.openPage(pageQuit)
	sh.backPage()
	if shown != 2 || sh.screen != id {
		t.Errorf("shown %d times, on %v", shown, sh.screen)
	}
}

// TestPagesDraw draws every page, and a page with a row of every kind in
// every state, which catches a draw the model cannot satisfy.
func TestPagesDraw(t *testing.T) {
	requireDisplay(t)
	inTempConfig(t)

	rl.SetTraceLogLevel(rl.LogError)
	rl.InitWindow(800, 450, "autobahn test")
	defer rl.CloseWindow()

	o := DefaultOptions()
	o.Width, o.Height, o.Mute = 800, 450, true
	sh := NewShell(o)
	defer sh.Close()

	for _, id := range allPages() {
		sh.openPage(id)
		sh.drawScreen(1.0 / 60)
	}

	on, at, level := true, 1, 0.4
	locked := func() string { return "locked" }
	rows := []menuRow{
		actionRow("go", "Go", nil).noted(func() string { return "saloon" }).describe("starts the drive"),
		switchRow("sw", "Switch", func() bool { return on }, func(b bool) { on = b }),
		switchRow("sw2", "Switch off", func() bool { return false }, func(bool) {}).disabledBy(locked),
		optionsRow("op", "Options", []string{"a", "b", "c"}, func() int { return at }, func(i int) { at = i }),
		barRow("bar", "Bar", func() float64 { return level }, func() string { return "40%" }, func(int) {}),
		stepperRow("st", "Stepper", func() string { return "12" }, func(int) {}).warned(func() bool { return true }),
		actionRow("quit", "Quit", nil).toned(func() tone { return toneDanger }),
	}
	sh.pager.cur = &menuPage{Title: "EVERY ROW", Lede: "a test", List: newRowList(rows...)}
	sh.screen = pageTitle
	for range rows {
		sh.drawScreen(1.0 / 60)
		sh.pager.cur.List.Update(keyDown)
	}
	sh.pager.cur.Draw = func(float32) {
		drawCard(rect(10, 10, 200, 120), true)
		drawCard(rect(220, 10, 200, 120), false)
		x := int32(10)
		for _, tn := range []tone{tonePlain, toneYours, toneOffer, toneLocked, toneDanger} {
			x += drawChip(x, 140, "tag", tn) + 6
			drawButton(rect(x, 180, 120, 40), "Button", tn == toneDanger, tn == toneLocked, tn)
		}
		padlock(10, 240, signGrey)
	}
	sh.drawScreen(1.0 / 60)
}
