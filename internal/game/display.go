package game

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Display settings: the window's size, whether it fills the screen, and
// whether the frame rate follows the screen's. They are applied to the window
// that is open (applyDisplay) and again at the start of a run (Run).

// windowSizes are the sizes the settings offer, smallest first. None is below
// the smallest window the menus open in.
var windowSizes = [][2]int{{1280, 720}, {1600, 900}, {1920, 1080}, {2560, 1440}, {3840, 2160}}

// clampWindow makes a stored size one the menus can open in: zero, meaning the
// default, is kept, and anything else is raised to the smallest window and
// held to a sane largest.
func clampWindow(w, h int) (int, int) {
	if w == 0 && h == 0 {
		return 0, 0
	}
	return min(max(w, minMenuWidth), 7680), min(max(h, minMenuHeight), 4320)
}

// sizesFor lists the window sizes that fit a screen of w by h, always with the
// smallest, and with current among them if it is not one of the presets.
func sizesFor(screenW, screenH int, current [2]int) [][2]int {
	var out [][2]int
	for _, s := range windowSizes {
		if s[0] <= screenW && s[1] <= screenH || len(out) == 0 {
			out = append(out, s)
		}
	}
	for _, s := range out {
		if s == current {
			return out
		}
	}
	return append(out, current)
}

// sizeWords is how a size reads in the settings.
func sizeWords(s [2]int) string { return fmt.Sprintf("%d x %d", s[0], s[1]) }

// windowSize is the size the window is meant to have: the stored one, or the
// default.
func (s Settings) windowSize() [2]int {
	if s.WindowW == 0 || s.WindowH == 0 {
		return [2]int{DefaultOptions().Width, DefaultOptions().Height}
	}
	return [2]int{s.WindowW, s.WindowH}
}

// monitorSize is the size of the screen the window is on, or 0 by 0 with no
// window.
func monitorSize() (int, int) {
	if !rl.IsWindowReady() {
		return 0, 0
	}
	m := rl.GetCurrentMonitor()
	return rl.GetMonitorWidth(m), rl.GetMonitorHeight(m)
}

// sizeChoices are the window sizes offered on this screen.
func (s *Shell) sizeChoices() [][2]int {
	mw, mh := monitorSize()
	if mw == 0 {
		mw, mh = windowSizes[len(windowSizes)-1][0], windowSizes[len(windowSizes)-1][1]
	}
	return sizesFor(mw, mh, s.settings.windowSize())
}

// applyDisplay puts the display settings on the open window and tells whoever
// draws what size it is. It does nothing with no window, and nothing at all to
// a window already as asked.
func (s *Shell) applyDisplay() {
	if !rl.IsWindowReady() {
		return
	}
	set := s.settings
	if set.VSync != rl.IsWindowState(rl.FlagVsyncHint) {
		if set.VSync {
			rl.SetWindowState(rl.FlagVsyncHint)
		} else {
			rl.ClearWindowState(rl.FlagVsyncHint)
		}
	}
	if set.Fullscreen != rl.IsWindowState(rl.FlagBorderlessWindowedMode) {
		rl.ToggleBorderlessWindowed()
	}
	if !set.Fullscreen {
		if sz := set.windowSize(); rl.GetScreenWidth() != sz[0] || rl.GetScreenHeight() != sz[1] {
			rl.SetWindowSize(sz[0], sz[1])
		}
	}
	s.sizeTo(rl.GetScreenWidth(), rl.GetScreenHeight())
}

// followWindow keeps what draws in step with the window's real size. Raylib
// reports a new size a frame after full screen is switched, and the window can
// be resized by the system too, so the size is read every frame rather than
// taken once when it was asked for.
func (s *Shell) followWindow() {
	if rl.IsWindowReady() {
		s.sizeTo(rl.GetScreenWidth(), rl.GetScreenHeight())
	}
}

// sizeTo tells everything that draws that the window is now w by h.
func (s *Shell) sizeTo(w, h int) {
	if w <= 0 || h <= 0 || (s.opts.Width == w && s.opts.Height == h) {
		return
	}
	s.opts.Width, s.opts.Height = w, h
	if s.game != nil {
		s.game.opts.Width, s.game.opts.Height = w, h
	}
	// The drive behind the menus was built for the old size; it is built again
	// at the new one when it is next wanted.
	s.closeMenuDrive()
}

// sizeWordsOf words each size.
func sizeWordsOf(sizes [][2]int) []string {
	out := make([]string, len(sizes))
	for i, s := range sizes {
		out[i] = sizeWords(s)
	}
	return out
}

// sizeIndex is where want is in sizes, or 0.
func sizeIndex(sizes [][2]int, want [2]int) int {
	for i, s := range sizes {
		if s == want {
			return i
		}
	}
	return 0
}
