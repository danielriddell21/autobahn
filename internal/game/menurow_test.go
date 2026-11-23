package game

import (
	"fmt"
	"testing"

	"github.com/danielriddell21/crucible/menu"
)

var (
	keyUp     = menu.Input{Up: true}
	keyDown   = menu.Input{Down: true}
	keyLeft   = menu.Input{Left: true}
	keyRight  = menu.Input{Right: true}
	keyEnter  = menu.Input{Select: true}
	keyNobody = menu.Input{}
)

// rowOf finds a row's view by key.
func rowOf(t *testing.T, l *rowList, key string) rowView {
	t.Helper()
	for _, v := range l.Views() {
		if v.Key == key {
			return v
		}
	}
	t.Fatalf("no row %q", key)
	return rowView{}
}

func TestRowListMovesAndWraps(t *testing.T) {
	l := newRowList(
		actionRow("a", "A", nil),
		actionRow("b", "B", nil),
		actionRow("c", "C", nil),
	)
	if l.SelectedKey() != "a" {
		t.Fatalf("a new list starts on %q", l.SelectedKey())
	}
	for _, want := range []string{"b", "c", "a"} {
		if s := l.Update(keyDown); s != menu.SoundMove || l.SelectedKey() != want {
			t.Fatalf("down: on %q (sound %v), want %q", l.SelectedKey(), s, want)
		}
	}
	l.Update(keyUp)
	if l.SelectedKey() != "c" {
		t.Errorf("up from the first row went to %q, want the last", l.SelectedKey())
	}
	if l.Update(keyNobody) != menu.SoundNone {
		t.Error("no keys made a sound")
	}
	if vs := l.Views(); !vs[2].Selected || vs[0].Selected || vs[1].Selected {
		t.Error("the views do not mark the selected row alone")
	}
}

func TestActionRowRunsOnEnterAlone(t *testing.T) {
	ran := 0
	l := newRowList(actionRow("go", "Go", func() { ran++ }).noted(func() string { return "saloon" }))
	for _, in := range []menu.Input{keyLeft, keyRight} {
		if s := l.Update(in); s != menu.SoundNone || ran != 0 {
			t.Fatalf("%+v adjusted an action row", in)
		}
	}
	if s := l.Update(keyEnter); s != menu.SoundSelect || ran != 1 {
		t.Errorf("enter: ran %d, sound %v", ran, s)
	}
	if v := rowOf(t, l, "go"); v.Value != "saloon" || v.Kind != rowAction {
		t.Errorf("the view reads %+v", v)
	}
}

func TestSwitchRow(t *testing.T) {
	// Sound is stored inverted, as Muted: the row reads through a getter.
	muted := true
	l := newRowList(switchRow("sound", "Sound", func() bool { return !muted }, func(on bool) { muted = !on }))
	if v := rowOf(t, l, "sound"); v.On || v.Value != "off" {
		t.Fatalf("starts reading %+v", v)
	}
	l.Update(keyRight)
	if muted {
		t.Error("right did not turn it on")
	}
	l.Update(keyRight)
	if muted {
		t.Error("right again turned it off")
	}
	l.Update(keyLeft)
	if !muted {
		t.Error("left did not turn it off")
	}
	l.Update(keyEnter)
	if v := rowOf(t, l, "sound"); muted || !v.On || v.Value != "on" {
		t.Errorf("enter did not flip it: %+v", v)
	}
	l.Update(keyEnter)
	if !muted {
		t.Error("enter did not flip it back")
	}
}

func TestOptionsRowWrapsBothWays(t *testing.T) {
	at := 0
	opts := []string{"day", "dusk", "night", "dawn"}
	l := newRowList(optionsRow("time", "Start time", opts, func() int { return at }, func(i int) { at = i }))
	l.Update(keyLeft)
	if at != 3 {
		t.Fatalf("left from the first went to %d, want the last", at)
	}
	l.Update(keyRight)
	if at != 0 {
		t.Fatalf("right from the last went to %d, want the first", at)
	}
	l.Update(keyEnter)
	if v := rowOf(t, l, "time"); at != 1 || v.Choice != 1 || v.Value != "dusk" {
		t.Errorf("enter stepped to %d, reading %+v", at, v)
	}
	// A choice out of range is drawn as the nearest end, never as a panic.
	at = 9
	if v := rowOf(t, l, "time"); v.Choice != 3 || v.Value != "dawn" {
		t.Errorf("an out-of-range choice reads %+v", v)
	}
}

func TestBarRow(t *testing.T) {
	vol := 0.5
	l := newRowList(barRow("volume", "Volume",
		func() float64 { return vol },
		func() string { return fmt.Sprintf("%.0f%%", vol*100) },
		func(d int) { vol += 0.75 * float64(d) }))
	l.Update(keyRight)
	if v := rowOf(t, l, "volume"); vol != 1.25 || v.Level != 1 || v.Value != "125%" {
		t.Errorf("a bar past full reads %+v, want its fill held to 1 and its words as they are", v)
	}
	l.Update(keyLeft)
	l.Update(keyLeft)
	if v := rowOf(t, l, "volume"); v.Level != 0 {
		t.Errorf("a bar below empty fills %v", v.Level)
	}
	if s := l.Update(keyEnter); s != menu.SoundNone {
		t.Error("enter did something to a bar")
	}
}

func TestStepperRowWarnsPastItsRange(t *testing.T) {
	seed := 3
	l := newRowList(stepperRow("seed", "Seed", func() string { return fmt.Sprint(seed) }, func(d int) { seed += d }).
		warned(func() bool { return seed > 4 }))
	l.Update(keyRight)
	if v := rowOf(t, l, "seed"); seed != 4 || v.Value != "4" || v.Warn {
		t.Errorf("stepped to %d, reading %+v", seed, v)
	}
	l.Update(keyRight)
	if v := rowOf(t, l, "seed"); !v.Warn || v.Kind != rowStepper {
		t.Errorf("past the range the row reads %+v", v)
	}
	l.Update(keyLeft)
	if seed != 4 {
		t.Errorf("left stepped to %d", seed)
	}
}

func TestDisabledRowsAreShownAndSayWhy(t *testing.T) {
	locked, ran, at := true, false, 0
	why := func() string {
		if locked {
			return "bank 4,000 more to open the seatown"
		}
		return ""
	}
	l := newRowList(
		actionRow("drive", "Drive", func() { ran = true }).disabledBy(why).toned(func() tone { return toneDanger }),
		optionsRow("map", "Map", []string{"city", "seatown"}, func() int { return at }, func(i int) { at = i }).disabledBy(why),
		actionRow("back", "Back", nil),
	)

	// The locked row is where the list starts, and is not skipped.
	if l.SelectedKey() != "drive" {
		t.Fatalf("starts on %q", l.SelectedKey())
	}
	v := rowOf(t, l, "drive")
	if v.Disabled == "" || v.Tone != toneLocked {
		t.Errorf("a locked row reads %+v, want a reason and grey", v)
	}
	if l.Reason() != why() {
		t.Errorf("the reason is %q before asking", l.Reason())
	}
	if s := l.Update(keyEnter); s != menu.SoundNone || ran {
		t.Error("enter ran a disabled row")
	}
	if l.Refused != why() {
		t.Errorf("enter on a disabled row left %q", l.Refused)
	}

	l.Update(keyDown)
	if l.SelectedKey() != "map" || l.Refused != "" {
		t.Errorf("down went to %q and left %q", l.SelectedKey(), l.Refused)
	}
	l.Update(keyRight)
	if at != 0 || l.Refused == "" {
		t.Error("right adjusted a disabled row, or did not say why it would not")
	}

	l.Update(keyDown)
	if l.Reason() != "" {
		t.Errorf("an open row has a reason: %q", l.Reason())
	}

	// Unlocked, it works and takes its own tone again.
	locked = false
	l.Select("drive")
	if v := rowOf(t, l, "drive"); v.Tone != toneDanger || v.Disabled != "" {
		t.Errorf("unlocked, the row reads %+v", v)
	}
	l.Update(keyEnter)
	if !ran {
		t.Error("enter did not run the row once it was open")
	}
}

func TestSetRowsKeepsTheSelectionByKey(t *testing.T) {
	l := newRowList(actionRow("a", "A", nil), actionRow("b", "B", nil), actionRow("c", "C", nil))
	l.Select("c")
	l.SetRows([]menuRow{actionRow("c", "C", nil), actionRow("a", "A", nil)})
	if l.SelectedKey() != "c" || l.Sel() != 0 {
		t.Errorf("after the rows moved, on %q at %d", l.SelectedKey(), l.Sel())
	}
	// With its row gone, the selection stays as near its place as there is.
	l.Select("a")
	l.SetRows([]menuRow{actionRow("x", "X", nil)})
	if l.SelectedKey() != "x" {
		t.Errorf("with the row gone, on %q", l.SelectedKey())
	}
	if l.Select("gone") {
		t.Error("selected a row that is not there")
	}
	if _, ok := l.Row("x"); !ok {
		t.Error("Row did not find x")
	}
}

func TestAnEmptyListIgnoresKeys(t *testing.T) {
	l := newRowList()
	for _, in := range []menu.Input{keyUp, keyDown, keyLeft, keyRight, keyEnter} {
		if l.Update(in) != menu.SoundNone {
			t.Errorf("%+v made a sound on an empty list", in)
		}
	}
	l.Move(1)
	if _, ok := l.Selected(); ok || l.SelectedKey() != "" || l.Reason() != "" {
		t.Error("an empty list has a selection")
	}
}

func TestAListAcrossMovesWithLeftAndRight(t *testing.T) {
	quit := false
	l := newRowList(actionRow("stay", "Stay", nil), actionRow("quit", "Quit", func() { quit = true }))
	l.Across = true
	l.Update(keyRight)
	if l.SelectedKey() != "quit" {
		t.Fatalf("right went to %q", l.SelectedKey())
	}
	l.Update(keyRight)
	if l.SelectedKey() != "stay" {
		t.Fatalf("right from the last went to %q", l.SelectedKey())
	}
	l.Update(keyLeft)
	l.Update(keyEnter)
	if !quit {
		t.Error("enter on Quit did not quit")
	}
}

func TestRowWindowKeepsTheSelectionInView(t *testing.T) {
	cases := []struct {
		n, sel, fit, first, last int
	}{
		{n: 5, sel: 4, fit: 10, first: 0, last: 5},
		{n: 20, sel: 0, fit: 6, first: 0, last: 6},
		{n: 20, sel: 10, fit: 6, first: 7, last: 13},
		{n: 20, sel: 19, fit: 6, first: 14, last: 20},
		{n: 20, sel: 3, fit: 0, first: 0, last: 0},
		{n: 0, sel: 0, fit: 6, first: 0, last: 0},
	}
	for _, c := range cases {
		first, last := rowWindow(c.n, c.sel, c.fit)
		if first != c.first || last != c.last {
			t.Errorf("rowWindow(%d, %d, %d) = %d, %d, want %d, %d", c.n, c.sel, c.fit, first, last, c.first, c.last)
		}
		if c.fit > 0 && c.n > 0 && (c.sel < first || c.sel >= last) {
			t.Errorf("rowWindow(%d, %d, %d) left the selection out", c.n, c.sel, c.fit)
		}
	}
}
