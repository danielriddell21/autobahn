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

// raylib's frame and its keys, reached through these so a test can drive the
// real loop with a scripted keyboard and count the frames drawn.
//
// raylib reads the keyboard only in EndDrawing (PollInputEvents runs there):
// a key pressed stays pressed, frame after frame, until a frame is drawn. A
// step of the shell that drew nothing would see the same esc again on the
// next, which is why Shell.Step draws exactly one frame whatever it did.
var (
	keyPressed   = rl.IsKeyPressed
	beginDrawing = rl.BeginDrawing
	endDrawing   = rl.EndDrawing
)

// drawScreen draws one frame of whatever is showing once the step is done:
// the drive, or the page with its backdrop and hint bar (signpost.go). It is
// the only place the shell begins and ends a frame.
func (s *Shell) drawScreen(dt float32) {
	beginDrawing()
	switch {
	case s.screen == screenPlaying && s.game != nil:
		s.game.drawFrame()
	case s.onPage():
		s.drawPage(dt)
	default:
		rl.ClearBackground(menuBackdrop)
	}
	endDrawing()
	if s.screen == screenPlaying && s.game != nil {
		s.game.capture() // the recorder, when one is running, as Game.Step does
	}
}
