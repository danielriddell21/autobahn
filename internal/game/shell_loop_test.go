package game

import (
	"runtime"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/netplay"

	"github.com/danielriddell21/autobahn/internal/scenario"
)

// fakeKeys is the keyboard as raylib hands it to the shell, scripted, and
// faithful to the one thing about it that matters here: raylib reads the keys
// only when a frame ends (PollInputEvents runs inside EndDrawing), so a key
// pressed stays pressed until a frame has been drawn. It also counts the
// frames begun and ended. The real BeginDrawing and EndDrawing still run.
type fakeKeys struct {
	down         map[int32]bool
	begins, ends int
	// script is the keys each poll after this one sees pressed, one entry a
	// frame, for a loop the test cannot reach into between frames.
	script [][]int32
}

// fakeKeyboard puts a fakeKeys in place of raylib's keyboard and frame for
// the rest of the test.
func fakeKeyboard(t *testing.T) *fakeKeys {
	t.Helper()
	k := &fakeKeys{down: map[int32]bool{}}
	kp, bd, ed := keyPressed, beginDrawing, endDrawing
	t.Cleanup(func() { keyPressed, beginDrawing, endDrawing = kp, bd, ed })
	keyPressed = func(key int32) bool { return k.down[key] }
	beginDrawing = func() {
		k.begins++
		rl.BeginDrawing()
	}
	endDrawing = func() {
		rl.EndDrawing()
		k.ends++
		k.down = map[int32]bool{} // the poll: what was pressed has been seen
		if len(k.script) > 0 {
			k.press(k.script[0]...)
			k.script = k.script[1:]
		}
	}
	return k
}

// press holds the keys down as of the last poll: pressed until a frame ends.
func (k *fakeKeys) press(keys ...int32) {
	for _, key := range keys {
		k.down[key] = true
	}
}

// loopShell opens a window and a shell on the title page, with a scripted
// keyboard, the career and settings in a scratch directory, every pack held
// and the menu drive off so the frames are cheap.
func loopShell(t *testing.T) (*Shell, *fakeKeys) {
	t.Helper()
	inTempConfig(t)
	openTestWindow(t, 480, 270)
	keys := fakeKeyboard(t)
	o := DefaultOptions()
	o.Width, o.Height, o.Mute = 480, 270, true
	o.Traffic, o.Police = 5, 1
	o.Given = Given{"traffic": true, "police": true}
	o.UnlockAll = true
	sh := NewShell(o)
	sh.settings.NoMenuDrive = true
	t.Cleanup(sh.Close)
	return sh, keys
}

// step runs one frame of the shell, holding it to exactly one frame drawn.
func step(t *testing.T, sh *Shell, k *fakeKeys) bool {
	t.Helper()
	b, e := k.begins, k.ends
	more := sh.Step(1.0 / 60)
	if k.begins-b != 1 || k.ends-e != 1 {
		t.Errorf("on screen %v a step began %d frames and ended %d, want one of each", sh.screen, k.begins-b, k.ends-e)
	}
	return more
}

// TestEscPausesAndResumesOnceThroughTheLoop drives the real loop: a single
// esc pauses, and the next resumes, each exactly once. raylib hands a key
// over until a frame is drawn, so a step that drew nothing would see its esc
// again and flip between the two for ever. Every step on every path (title,
// drive, pause, resume, restart, abandon, quit) draws one frame.
func TestEscPausesAndResumesOnceThroughTheLoop(t *testing.T) {
	sh, k := loopShell(t)
	step(t, sh, k) // the title
	sh.startGame(false)
	if sh.game == nil {
		t.Fatalf("no drive: notice %q", sh.notice)
	}
	step(t, sh, k)

	// screens runs n frames and lists each screen the shell was on after one.
	screens := func(n int) []screen {
		var out []screen
		for range n {
			step(t, sh, k)
			out = append(out, sh.screen)
		}
		return out
	}
	same := func(got []screen, want screen) bool {
		for _, s := range got {
			if s != want {
				return false
			}
		}
		return true
	}

	k.press(rl.KeyEscape)
	before := sh.game.world.Time
	if got := screens(4); !same(got, pagePause) {
		t.Fatalf("one esc on the drive gave screens %v, want the pause page throughout", got)
	}
	if sh.game.world.Time != before {
		t.Error("the paused drive moved")
	}
	k.press(rl.KeyEscape)
	if got := screens(4); !same(got, screenPlaying) {
		t.Fatalf("one esc on the pause page gave screens %v, want the drive throughout", got)
	}
	if sh.game.world.Time <= before {
		t.Error("the resumed drive did not move")
	}

	// Restart: a new drive, built once.
	k.press(rl.KeyEscape)
	step(t, sh, k)
	old := sh.game
	sh.pager.cur.List.Select("restart")
	k.press(rl.KeyEnter)
	step(t, sh, k)
	restarted := sh.game
	if sh.screen != screenPlaying || restarted == old || restarted == nil {
		t.Fatalf("restart left screen %v (new game: %v)", sh.screen, restarted != old)
	}
	screens(3)
	if sh.game != restarted {
		t.Error("the drive was built again after the restart")
	}

	// Abandon: the title.
	k.press(rl.KeyEscape)
	step(t, sh, k)
	sh.pager.cur.List.Select("abandon")
	k.press(rl.KeyEnter)
	step(t, sh, k)
	if sh.screen != pageTitle || sh.game != nil {
		t.Fatalf("abandoning left screen %v, game %v", sh.screen, sh.game != nil)
	}
	if peakSpeed.g != nil {
		t.Error("the abandoned drive is still held for its top speed")
	}
	screens(2)

	// Quit: esc on the title asks, and Quit ends the loop.
	k.press(rl.KeyEscape)
	step(t, sh, k)
	if sh.screen != pageQuit {
		t.Fatalf("esc on the title gave screen %v", sh.screen)
	}
	if got := screens(2); !same(got, pageQuit) {
		t.Fatalf("the quit dialog gave way to %v", got)
	}
	sh.pager.cur.List.Select("go")
	k.press(rl.KeyEnter)
	if step(t, sh, k) {
		t.Error("the shell went on after Quit")
	}
}

// TestEscOnANetworkedGameAsksOnceAndTheGameRunsOn: esc on a hosted game opens
// the leave dialog once, the world goes on under it, and esc again (Stay)
// hands the window back once.
func TestEscOnANetworkedGameAsksOnceAndTheGameRunsOn(t *testing.T) {
	sh, k := loopShell(t)
	if err := sh.net.StartHosting("127.0.0.1:0"); err != nil {
		t.Fatalf("hosting: %v", err)
	}
	sh.hostMode = ModeChase
	sh.startGame(true)
	if sh.game == nil || !sh.game.Networked() || sh.net.Role() != netplay.Hosting {
		t.Fatalf("no hosted game: notice %q", sh.notice)
	}
	step(t, sh, k)
	k.press(rl.KeyEscape)
	step(t, sh, k)
	if sh.screen != pageLeave {
		t.Fatalf("esc gave screen %v", sh.screen)
	}
	before := sh.game.world.Time
	for range 5 {
		step(t, sh, k)
		if sh.screen != pageLeave {
			t.Fatalf("the leave dialog gave way to screen %v", sh.screen)
		}
	}
	if sh.game.world.Time <= before {
		t.Errorf("under the dialog the hosted world stood at %v", sh.game.world.Time)
	}
	k.press(rl.KeyEscape)
	for range 4 {
		step(t, sh, k)
		if sh.screen != screenPlaying {
			t.Fatalf("Stay gave screen %v", sh.screen)
		}
	}
}

// TestF5RetriesAChallengeOnce: F5 on a challenge rebuilds the world once, not
// on every frame after.
func TestF5RetriesAChallengeOnce(t *testing.T) {
	sh, k := loopShell(t)
	sh.startChallenge(scenario.All()[0])
	if sh.game == nil || sh.game.chal == nil {
		t.Fatalf("no challenge: notice %q", sh.notice)
	}
	step(t, sh, k)
	old := sh.game
	k.press(rl.KeyF5)
	step(t, sh, k)
	retried := sh.game
	if retried == old || retried == nil || retried.chal == nil || sh.screen != screenPlaying {
		t.Fatalf("F5 left screen %v (new game %v)", sh.screen, retried != old)
	}
	for range 3 {
		step(t, sh, k)
	}
	if sh.game != retried {
		t.Error("the challenge was rebuilt again after one F5")
	}
}

// TestOnlyTheMenusTurnEscsExitOff: the menus read esc, so raylib's exit key is
// turned off for them; a run from the command line reads none, so there esc
// still closes the window. The menus are driven through shell itself, with a
// scripted esc, Quit and enter.
func TestOnlyTheMenusTurnEscsExitOff(t *testing.T) {
	requireDisplay(t)
	inTempConfig(t)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	rl.SetTraceLogLevel(rl.LogError)
	var set []int32
	saved := setExitKey
	t.Cleanup(func() { setExitKey = saved })
	setExitKey = func(key int32) {
		set = append(set, key)
		saved(key)
	}
	o := DefaultOptions()
	o.Width, o.Height, o.Mute = 320, 180, true
	o.Traffic, o.Police = 5, 1

	// What run is handed: the window as WithWindow opens it.
	err := WithWindow(o, func(*Game) error {
		if len(set) != 0 {
			t.Errorf("a command-line window had its exit key set to %v", set)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// The menus: shell, handed a game in a window as WithWindow would hand it.
	// (Not through WithWindow itself, which would close that game a second
	// time after shell has closed it.)
	t.Cleanup(func() { rl.SetTargetFPS(0) }) // shell paces itself to 60
	openTestWindow(t, 320, 180)
	keys := fakeKeyboard(t)
	keys.script = [][]int32{{rl.KeyEscape}, {rl.KeyRight}, {rl.KeyEnter}}
	o.UnlockAll = true
	g := New(o)
	g.net = &netplay.Session[Snapshot, Input]{}
	if err := shell(g); err != nil {
		t.Fatal(err)
	}
	if len(set) != 1 || set[0] != rl.KeyNull {
		t.Errorf("the menus set the exit key to %v, want only KeyNull", set)
	}
	// The title, esc's quit dialog, Quit chosen, and enter: a frame each.
	if len(keys.script) != 0 || keys.ends != 4 {
		t.Errorf("the menus quit after %d frames with %v of the script left, want 4 and none", keys.ends, keys.script)
	}
}
