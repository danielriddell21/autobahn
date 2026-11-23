package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/audio"
)

// The mouse in the menus. A page draws itself where it likes, so each thing it
// draws that can be chosen says where it is (hitRow and the others below) and
// what row it stands for. The next step reads those boxes and turns the mouse
// into the same navInput the keys make, so every page, and the code that
// handles keys on it, takes the mouse without knowing.
//
//   - Moving over a box selects its row.
//   - A left click on it does what enter does, or, on a bar or a stepper, what
//     ← or → does, by which half is clicked. A box that stands for one choice
//     of an options row (a card) picks that choice.
//   - The wheel is ↑ and ↓, and a right click is esc.
//
// The boxes are the last frame's, which is the picture the player is pointing
// at.

// hitbox is one thing on the page that can be pointed at.
type hitbox struct {
	r   rl.Rectangle
	key string // the row it stands for
	idx int    // the choice it is, of an options row, or -1 for the row as a whole
	fn  func() // what a click does instead, if not the row's own
}

// pointer is the boxes of the frame last drawn and where the mouse was.
var pointer struct {
	hits []hitbox
	last rl.Vector2
	seen bool
}

// resetHits forgets the boxes, as a page starts to draw.
func resetHits() { pointer.hits = pointer.hits[:0] }

// hitRow says that r is the row key.
func hitRow(r rl.Rectangle, key string) {
	pointer.hits = append(pointer.hits, hitbox{r: r, key: key, idx: -1})
}

// hitChoice says that r is choice i of the options row key.
func hitChoice(r rl.Rectangle, key string, i int) {
	pointer.hits = append(pointer.hits, hitbox{r: r, key: key, idx: i})
}

// hitDo says that r is the row key (or none, with "") and that a click on it
// runs fn.
func hitDo(r rl.Rectangle, key string, fn func()) {
	pointer.hits = append(pointer.hits, hitbox{r: r, key: key, idx: -1, fn: fn})
}

// hitAt is the box under p. Boxes drawn later lie over those drawn before, so
// the last one wins.
func hitAt(hits []hitbox, p rl.Vector2) (hitbox, bool) {
	for i := len(hits) - 1; i >= 0; i-- {
		if rl.CheckCollisionPointRec(p, hits[i].r) {
			return hits[i], true
		}
	}
	return hitbox{}, false
}

// pointerInput is what the mouse did in a frame.
type pointerInput struct {
	Pos          rl.Vector2
	Click, Right bool
	Wheel        float32
}

// readPointer reads the mouse from raylib. It sees nothing with no window.
func readPointer() pointerInput {
	if !rl.IsWindowReady() {
		return pointerInput{}
	}
	return pointerInput{
		Pos:   rl.GetMousePosition(),
		Click: rl.IsMouseButtonPressed(rl.MouseButtonLeft),
		Right: rl.IsMouseButtonPressed(rl.MouseButtonRight),
		Wheel: rl.GetMouseWheelMove(),
	}
}

// mouseNav adds what the mouse did to the keys of a frame, on the page showing.
func (s *Shell) mouseNav(in navInput) navInput {
	return s.pointerNav(in, readPointer())
}

// pointerNav is mouseNav for a given mouse, so it can be tried with none.
func (s *Shell) pointerNav(in navInput, m pointerInput) navInput {
	moved := !pointer.seen || m.Pos != pointer.last
	pointer.last, pointer.seen = m.Pos, true

	in.Back = in.Back || m.Right
	switch {
	case m.Wheel > 0:
		in.Up = true
	case m.Wheel < 0:
		in.Down = true
	}
	pg := s.pager.cur
	if pg == nil || pg.List == nil || (!moved && !m.Click) {
		return in
	}
	h, ok := hitAt(pointer.hits, m.Pos)
	if !ok {
		return in
	}
	if h.key != "" && pg.List.SelectedKey() != h.key && pg.List.Select(h.key) {
		playCue(audio.CueMove)
	}
	if !m.Click {
		return in
	}
	return clickOn(in, pg.List, h, m.Pos)
}

// clickOn is what a click on a box does: its own action, the choice it
// stands for, or what a click on its row means.
func clickOn(in navInput, l *rowList, h hitbox, p rl.Vector2) navInput {
	switch {
	case h.fn != nil:
		h.fn()
		playCue(audio.CueSelect)
	case h.idx >= 0:
		l.choose(h.idx)
	default:
		in = clickRow(in, l, h, p)
	}
	return in
}

// clickRow says what a click on a whole row means as keys: enter, except on a
// bar or a stepper, where the half clicked is the way to move it.
func clickRow(in navInput, l *rowList, h hitbox, p rl.Vector2) navInput {
	r, ok := l.Selected()
	if !ok || r.Key != h.key {
		return in
	}
	switch r.Kind {
	case rowBar, rowStepper:
		if p.X >= h.r.X+h.r.Width/2 {
			in.Right = true
		} else {
			in.Left = true
		}
	default:
		in.Select = true
	}
	return in
}

// choose sets the selected options row to choice i, stepping forward to it.
func (l *rowList) choose(i int) {
	r, ok := l.Selected()
	if !ok || r.Kind != rowOptions || r.Choice == nil || r.Adjust == nil || len(r.Options) == 0 {
		return
	}
	if why := r.why(); why != "" {
		l.Refused = why
		playCue(audio.CueDeny)
		return
	}
	n := len(r.Options)
	if i < 0 || i >= n {
		return
	}
	for range (i - r.Choice() + n) % n {
		r.Adjust(1)
	}
	playCue(audio.CueSelect)
}

// hitOptions says where each choice of an options row drawn by drawOptions is,
// so a click on one picks it.
func hitOptions(x, y int32, opts []string, key string) {
	const pad, gap, h = 10, 4, 24
	for i, o := range opts {
		w := faceSmall.width(o) + 2*pad
		hitChoice(rect(x, y, w, h), key, i)
		x += w + gap
	}
}
