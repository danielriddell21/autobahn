package game

import "testing"

func TestMenuWindowIsNeverSmallerThanThePagesAreLaidOutFor(t *testing.T) {
	cases := []struct{ w, h, ww, wh int }{
		{800, 450, minMenuWidth, minMenuHeight},
		{1280, 720, 1280, 720},
		{1920, 1080, 1920, 1080},
		{1000, 1200, minMenuWidth, 1200},
	}
	for _, c := range cases {
		if w, h := menuWindowSize(c.w, c.h); w != c.ww || h != c.wh {
			t.Errorf("menuWindowSize(%d, %d) = %d x %d, want %d x %d", c.w, c.h, w, h, c.ww, c.wh)
		}
	}
}

// TestEveryPageDrawsAtTheSizesTheMenusOpenIn draws each page at the default
// window, at the smallest one a menu run opens in (menuWindowSize turns 800 by
// 450 into it) and at a large one. It needs a window, so it skips without a
// display.
func TestEveryPageDrawsAtTheSizesTheMenusOpenIn(t *testing.T) {
	inTempConfig(t)
	openTestWindow(t, 1280, 720)

	smallW, smallH := menuWindowSize(800, 450)
	if smallW < 1280 || smallH < 720 {
		t.Fatalf("a menu window can open at %d x %d", smallW, smallH)
	}
	o := DefaultOptions()
	o.Mute = true
	sh := NewShell(o)
	defer sh.Close()
	sh.career.Banked = 2500

	for id := pageTitle; id < pageEnd; id++ {
		if id == pagePause || id == pageLobby {
			continue // these open over a drive or a session, which they have their own tests for
		}
		sh.openPage(id)
		for _, size := range [][2]int{{1280, 720}, {smallW, smallH}, {1920, 1080}} {
			sh.opts.Width, sh.opts.Height = size[0], size[1]
			sh.drawScreen(1.0 / 60)
		}
	}
}
