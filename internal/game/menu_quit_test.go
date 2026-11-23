package game

import "testing"

func TestQuitDialog(t *testing.T) {
	sh := testShell(t)
	sh.openPage(pageTitle)
	sh.openPage(pageQuit)
	l := sh.pager.cur.List
	if !l.Across || l.SelectedKey() != "stay" {
		t.Fatalf("quit dialog: across %v, selected %q; want Stay first", l.Across, l.SelectedKey())
	}
	if r, _ := l.Row("go"); r.Label != "Quit" || r.view(false).Tone != toneDanger {
		t.Errorf("second button %q tone %v", r.Label, r.view(false).Tone)
	}
	if sh.pager.cur.Lede != quitText {
		t.Errorf("quit text %q", sh.pager.cur.Lede)
	}
	sh.stepPage(navInput{Back: true})
	if sh.quit || sh.screen != pageTitle {
		t.Errorf("esc: quit %v screen %d, want to stay on the title", sh.quit, sh.screen)
	}
	sh.openPage(pageQuit)
	r, _ := sh.pager.cur.List.Row("stay")
	r.Activate()
	if sh.quit || sh.screen != pageTitle {
		t.Error("Stay quit the game")
	}
	sh.openPage(pageQuit)
	r, _ = sh.pager.cur.List.Row("go")
	r.Activate()
	if !sh.quit {
		t.Error("Quit did not set quit")
	}
}

func TestLeaveDialog(t *testing.T) {
	sh := testShell(t)
	sh.screen = screenPlaying
	sh.openPage(pageLeave)
	pg := sh.pager.cur
	if pg.Lede != leaveText || !pg.OverGame {
		t.Errorf("leave page %q over game %v", pg.Lede, pg.OverGame)
	}
	if pg.List.SelectedKey() != "stay" {
		t.Error("Stay is not first")
	}
	sh.stepPage(navInput{Back: true})
	if sh.screen != screenPlaying {
		t.Errorf("esc went to screen %d, want back to the game", sh.screen)
	}
	sh.openPage(pageLeave)
	r, _ := sh.pager.cur.List.Row("go")
	if r.Label != "Leave" {
		t.Errorf("label %q", r.Label)
	}
	r.Activate()
	if sh.screen != screenTitle || sh.game != nil || sh.onPage() {
		t.Errorf("Leave: screen %d, game %v", sh.screen, sh.game)
	}
	if sh.quit {
		t.Error("Leave quit the program")
	}
}
