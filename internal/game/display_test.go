package game

import "testing"

func TestClampWindowKeepsTheDefaultAndRaisesTheRest(t *testing.T) {
	cases := []struct{ w, h, ww, wh int }{
		{0, 0, 0, 0},
		{800, 450, minMenuWidth, minMenuHeight},
		{1600, 900, 1600, 900},
		{99999, 99999, 7680, 4320},
		{-5, 1000, minMenuWidth, 1000},
	}
	for _, c := range cases {
		if w, h := clampWindow(c.w, c.h); w != c.ww || h != c.wh {
			t.Errorf("clampWindow(%d, %d) = %d, %d; want %d, %d", c.w, c.h, w, h, c.ww, c.wh)
		}
	}
}

func TestSizesForOffersWhatFitsTheScreen(t *testing.T) {
	got := sizesFor(1920, 1080, [2]int{1280, 720})
	if len(got) != 3 || got[0] != [2]int{1280, 720} || got[2] != [2]int{1920, 1080} {
		t.Errorf("sizes for 1080p: %v", got)
	}
	// A screen too small for any still gets the smallest window.
	if got := sizesFor(1024, 600, [2]int{1280, 720}); len(got) != 1 || got[0] != [2]int{1280, 720} {
		t.Errorf("sizes for a small screen: %v", got)
	}
	// A size the player has that is not a preset is kept in the list.
	got = sizesFor(1920, 1080, [2]int{1500, 800})
	if got[len(got)-1] != [2]int{1500, 800} || sizeIndex(got, [2]int{1500, 800}) != len(got)-1 {
		t.Errorf("a custom size was dropped: %v", got)
	}
	if sizeIndex(got, [2]int{1, 1}) != 0 {
		t.Error("an unknown size is not the first")
	}
}

func TestWindowSizeFallsBackToTheDefault(t *testing.T) {
	var s Settings
	if got := s.windowSize(); got != [2]int{DefaultOptions().Width, DefaultOptions().Height} {
		t.Errorf("default size %v", got)
	}
	s.WindowW, s.WindowH = 1600, 900
	if got := s.windowSize(); got != [2]int{1600, 900} {
		t.Errorf("stored size %v", got)
	}
	if sizeWords([2]int{1600, 900}) != "1600 x 900" {
		t.Errorf("words %q", sizeWords([2]int{1600, 900}))
	}
}

func TestSettingsClampTheWindow(t *testing.T) {
	s := DefaultSettings()
	s.WindowW, s.WindowH = 640, 480
	s.clamp()
	if s.WindowW != minMenuWidth || s.WindowH != minMenuHeight {
		t.Errorf("window %d x %d after clamp", s.WindowW, s.WindowH)
	}
}

func TestDisplayRowsLiveInTheirOwnSection(t *testing.T) {
	sh, pg := settingsPageOf(t)
	for _, key := range []string{"fullscreen", "windowsize", "vsync"} {
		if settingsSectionOf(key) != secDisplay {
			t.Errorf("%s is in section %d, want Display", key, settingsSectionOf(key))
		}
		if !pg.List.Select(key) {
			t.Errorf("no row %s", key)
		}
	}
	if settingsSectionOf("menusounds") != secSound {
		t.Error("menu sounds are not in the Sound section")
	}
	// The size is for a window, so it is out of reach full screen.
	pg.List.Select("windowsize")
	sh.settings.Fullscreen = true
	if pg.List.Reason() == "" {
		t.Error("the window size can be changed full screen")
	}
	sh.settings.Fullscreen = false
	if pg.List.Reason() != "" {
		t.Errorf("window size refused: %q", pg.List.Reason())
	}
	// With no window nothing is applied, and nothing fails.
	sh.applyDisplay()
	sh.sizeTo(1600, 900)
	if sh.opts.Width != 1600 || sh.opts.Height != 900 {
		t.Errorf("sizeTo left the shell at %d x %d", sh.opts.Width, sh.opts.Height)
	}
}

func TestEveryRowHasAPlaceInTheSettingsOrder(t *testing.T) {
	for key := range settingsKeySection {
		if settingsRowOrder(key) >= len(settingsOrder) {
			t.Errorf("row %s is missing from settingsOrder", key)
		}
	}
	for i := range settingsSections {
		if settingsFirstRow(i) == "" {
			t.Errorf("section %s has no first row", settingsSections[i])
		}
	}
}

// TestTheShellFollowsTheWindowsRealSize holds the bug where full screen grew
// the window but the menus went on drawing at the old size in its corner.
func TestTheShellFollowsTheWindowsRealSize(t *testing.T) {
	inTempConfig(t)
	openTestWindow(t, 640, 360)
	o := DefaultOptions()
	o.Mute = true
	sh := NewShell(o)
	defer sh.Close()
	if sh.opts.Width != 1280 {
		t.Fatalf("shell starts at %d wide", sh.opts.Width)
	}
	sh.followWindow()
	if sh.opts.Width != 640 || sh.opts.Height != 360 {
		t.Errorf("shell is %d x %d in a 640 x 360 window", sh.opts.Width, sh.opts.Height)
	}
}
