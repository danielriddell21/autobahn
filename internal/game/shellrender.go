package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// The menus' ground colours, which signpost.go builds the Signpost look on.
var (
	menuBackdrop = rl.NewColor(12, 14, 18, 255)
	menuVeil     = rl.NewColor(8, 10, 14, 215)
	menuSelected = rl.NewColor(104, 172, 232, 255)
	menuRule     = rl.NewColor(60, 66, 78, 255)
)

// drawScreen paints the page showing for a frame: its backdrop, the page and
// the hint bar (signpost.go).
func (s *Shell) drawScreen(dt float32) {
	rl.BeginDrawing()
	defer rl.EndDrawing()
	s.drawPage(dt)
}
