package game

import (
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/menu"

	"github.com/danielriddell21/autobahn/internal/audio"
)

// pointerShell is a shell showing a page of known rows, with the boxes a draw
// would have left.
func pointerShell(t *testing.T) (*Shell, *rowList, *[]string) {
	t.Helper()
	sh := testShell(t)
	var log []string
	choice := 0
	l := newRowList(
		actionRow("a", "A", func() { log = append(log, "a") }),
		actionRow("b", "B", func() { log = append(log, "b") }),
		barRow("bar", "Bar", func() float64 { return 0.5 }, func() string { return "" },
			func(d int) {}),
		optionsRow("opt", "Opt", []string{"x", "y", "z"},
			func() int { return choice }, func(i int) { choice = i }),
	)
	sh.pager.cur = &menuPage{Title: "T", List: l}
	resetHits()
	hitRow(rect(0, 0, 100, 40), "a")
	hitRow(rect(0, 50, 100, 40), "b")
	hitRow(rect(0, 100, 100, 40), "bar")
	hitRow(rect(0, 150, 100, 40), "opt")
	hitChoice(rect(0, 200, 30, 20), "opt", 2)
	hitDo(rect(0, 250, 30, 20), "b", func() { log = append(log, "fn") })
	pointer.seen = false
	t.Cleanup(func() { resetHits(); pointer.seen = false })
	return sh, l, &log
}

func TestHitAtTakesTheLastBoxDrawn(t *testing.T) {
	hits := []hitbox{
		{r: rect(0, 0, 100, 100), key: "under"},
		{r: rect(10, 10, 20, 20), key: "over"},
	}
	if h, ok := hitAt(hits, rl.NewVector2(15, 15)); !ok || h.key != "over" {
		t.Errorf("hit %q, %v; want the box drawn over", h.key, ok)
	}
	if h, ok := hitAt(hits, rl.NewVector2(60, 60)); !ok || h.key != "under" {
		t.Errorf("hit %q, %v; want the box under", h.key, ok)
	}
	if _, ok := hitAt(hits, rl.NewVector2(500, 500)); ok {
		t.Error("a point outside every box hit one")
	}
}

func TestHoveringSelectsTheRowUnderTheMouse(t *testing.T) {
	sh, l, _ := pointerShell(t)
	sh.pointerNav(navInput{}, pointerInput{Pos: rl.NewVector2(10, 60)})
	if l.SelectedKey() != "b" {
		t.Errorf("selected %q after hovering over b", l.SelectedKey())
	}
	// Standing still leaves a keyboard choice alone.
	l.Select("a")
	sh.pointerNav(navInput{}, pointerInput{Pos: rl.NewVector2(10, 60)})
	if l.SelectedKey() != "a" {
		t.Errorf("a still mouse moved the selection to %q", l.SelectedKey())
	}
}

func TestClickingARowIsEnter(t *testing.T) {
	sh, l, _ := pointerShell(t)
	in := sh.pointerNav(navInput{}, pointerInput{Pos: rl.NewVector2(10, 10), Click: true})
	if !in.Select || in.Left || in.Right || l.SelectedKey() != "a" {
		t.Errorf("click on a gave %+v with %q selected", in.Input, l.SelectedKey())
	}
}

func TestClickingABarMovesItByTheHalfClicked(t *testing.T) {
	sh, _, _ := pointerShell(t)
	left := sh.pointerNav(navInput{}, pointerInput{Pos: rl.NewVector2(10, 110), Click: true})
	if !left.Left || left.Right || left.Select {
		t.Errorf("left half gave %+v", left.Input)
	}
	right := sh.pointerNav(navInput{}, pointerInput{Pos: rl.NewVector2(90, 110), Click: true})
	if !right.Right || right.Left || right.Select {
		t.Errorf("right half gave %+v", right.Input)
	}
}

func TestClickingAChoicePicksIt(t *testing.T) {
	sh, l, _ := pointerShell(t)
	sh.pointerNav(navInput{}, pointerInput{Pos: rl.NewVector2(5, 205), Click: true})
	if l.SelectedKey() != "opt" {
		t.Fatalf("selected %q", l.SelectedKey())
	}
	r, _ := l.Selected()
	if got := r.Choice(); got != 2 {
		t.Errorf("choice %d after clicking the third, want 2", got)
	}
	// Wraps forward to reach an earlier one.
	hitChoice(rect(0, 300, 30, 20), "opt", 1)
	sh.pointerNav(navInput{}, pointerInput{Pos: rl.NewVector2(5, 305), Click: true})
	if got := r.Choice(); got != 1 {
		t.Errorf("choice %d after clicking the second, want 1", got)
	}
}

func TestAClickCanRunItsOwnAction(t *testing.T) {
	sh, l, log := pointerShell(t)
	in := sh.pointerNav(navInput{}, pointerInput{Pos: rl.NewVector2(5, 255), Click: true})
	if in.Select || len(*log) != 1 || (*log)[0] != "fn" || l.SelectedKey() != "b" {
		t.Errorf("got %+v, log %v, selected %q", in.Input, *log, l.SelectedKey())
	}
}

func TestTheWheelMovesAndARightClickGoesBack(t *testing.T) {
	sh, _, _ := pointerShell(t)
	if in := sh.pointerNav(navInput{}, pointerInput{Pos: rl.NewVector2(900, 900), Wheel: 1}); !in.Up || in.Down {
		t.Errorf("wheel up gave %+v", in.Input)
	}
	if in := sh.pointerNav(navInput{}, pointerInput{Pos: rl.NewVector2(900, 900), Wheel: -1}); !in.Down || in.Up {
		t.Errorf("wheel down gave %+v", in.Input)
	}
	if in := sh.pointerNav(navInput{}, pointerInput{Pos: rl.NewVector2(900, 900), Right: true}); !in.Back {
		t.Error("a right click did not go back")
	}
}

func TestAClickOnNothingDoesNothing(t *testing.T) {
	sh, l, _ := pointerShell(t)
	l.Select("b")
	in := sh.pointerNav(navInput{}, pointerInput{Pos: rl.NewVector2(900, 900), Click: true})
	if in.Select || l.SelectedKey() != "b" {
		t.Errorf("click on nothing gave %+v with %q selected", in.Input, l.SelectedKey())
	}
}

func TestMenuCuesFollowTheKeys(t *testing.T) {
	var got []audio.Cue
	menuCue = func(c audio.Cue) { got = append(got, c) }
	t.Cleanup(func() { menuCue = nil })

	locked := actionRow("locked", "L", func() {}).disabledBy(func() string { return "locked" })
	l := newRowList(actionRow("a", "A", func() {}), locked)
	l.Update(menu.Input{Down: true})
	l.Update(menu.Input{Select: true}) // refused
	l.Select("a")
	l.Update(menu.Input{Select: true})
	want := []audio.Cue{audio.CueMove, audio.CueDeny, audio.CueSelect}
	if len(got) != len(want) {
		t.Fatalf("cues %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cue %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestShellCueIsSilentWhenMenuSoundsAreOff(t *testing.T) {
	sh := testShell(t)
	sh.settings.SetMenuSounds(false)
	if sh.settings.MenuSounds() {
		t.Fatal("menu sounds still on after turning them off")
	}
	sh.cue(audio.CueMove) // a muted kit and a switch off: must not touch a device
	sh.settings.SetMenuSounds(true)
	sh.settings.Muted = true
	sh.cue(audio.CueMove)
}
