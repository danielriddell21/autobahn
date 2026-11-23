package game

import (
	"os"
	"reflect"
	"runtime"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/netplay"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// requireDisplay skips a test that needs a window. The scene is drawn on the
// GPU, so anything opening one needs a display; CI runs without and would
// otherwise abort inside raylib rather than failing cleanly.
func requireDisplay(t *testing.T) {
	t.Helper()
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("no display; run under xvfb-run to exercise this")
	}
}

// openTestWindow opens a w by h window for a test that draws, or skips the
// test with no display. Every display test comes through here. GL is current
// only on the thread that opened the window and a test runs on a goroutine the
// scheduler may move, so the goroutine is locked to its thread until the test
// is over; then the menu fonts are freed while their context is still there
// and the window closed (closeWindow), after anything the test itself cleans
// up, since a cleanup registered later runs first.
func openTestWindow(t *testing.T, w, h int32) {
	t.Helper()
	requireDisplay(t)
	runtime.LockOSThread()
	rl.SetTraceLogLevel(rl.LogError)
	openWindow(w, h, "autobahn test")
	t.Cleanup(func() {
		closeWindow()
		runtime.UnlockOSThread()
	})
}

func TestTheShellOpensOnTheTitlePage(t *testing.T) {
	sh := testShell(t)
	if sh.screen != pageTitle || !sh.onPage() || len(sh.pager.stack) != 0 {
		t.Fatalf("opened on screen %v (on a page: %v, trail %v)", sh.screen, sh.onPage(), sh.pager.stack)
	}
	// Going to the title keeps the notice, which is where it is read, and
	// drops the trail behind it.
	sh.openPage(pagePlay)
	sh.openPage(pageGarage)
	sh.notice = "something went wrong"
	sh.goTitle()
	if sh.screen != pageTitle || len(sh.pager.stack) != 0 || sh.notice == "" {
		t.Errorf("goTitle: screen %v, trail %v, notice %q", sh.screen, sh.pager.stack, sh.notice)
	}
}

// TestLobbyWaitsForTheOtherPlayer covers the reason the lobby exists: a player
// hosts before there is a world, and the lobby waits until the other arrives.
func TestLobbyWaitsForTheOtherPlayer(t *testing.T) {
	sh := testShell(t)
	if err := sh.net.StartHosting("127.0.0.1:0"); err != nil {
		t.Fatalf("hosting: %v", err)
	}
	sh.openPage(pageLobby)
	for range 3 {
		sh.stepPage(navInput{})
	}
	if sh.screen != pageLobby || !sh.net.Online() {
		t.Fatalf("screen = %v, online %v: want the lobby to keep waiting", sh.screen, sh.net.Online())
	}
	if got := sh.net.Status(); got == "" {
		t.Error("the lobby has nothing to tell the player")
	}
}

func TestAFailedJoinReturnsToTheTitleWithAReason(t *testing.T) {
	sh := testShell(t)
	sh.openPage(pagePlay)
	sh.joinAddr = "127.0.0.1:1" // nothing is listening
	sh.joinChase()

	if sh.screen != pageTitle || len(sh.pager.stack) != 0 {
		t.Errorf("screen = %v, trail %v: want the title with nothing behind it", sh.screen, sh.pager.stack)
	}
	if sh.notice == "" {
		t.Error("the player was not told why the join failed")
	}
	if sh.net.Online() {
		t.Error("a failed join left a session running")
	}
}

func TestEscPausesADriveAndResumeGoesBack(t *testing.T) {
	sh := testShell(t)
	sh.game = bareGame(Options{})
	sh.screen = screenPlaying
	if !sh.escapeDrive() || sh.screen != pagePause || !sh.pager.cur.OverGame {
		t.Fatalf("esc on a drive gave screen %v", sh.screen)
	}
	sh.stepPage(navInput{Back: true})
	if sh.screen != screenPlaying || sh.onPage() || len(sh.pager.stack) != 0 {
		t.Errorf("esc on the pause page gave screen %v, trail %v", sh.screen, sh.pager.stack)
	}

	// With the job menu open, esc is the job menu's.
	sh.game.jobMenu = &jobMenu{}
	if sh.escapeDrive() || sh.screen != screenPlaying {
		t.Error("esc paused over the job menu")
	}
}

func TestEscOnANetworkedDriveAsksBeforeLeaving(t *testing.T) {
	sh := testShell(t)
	if err := sh.net.StartHosting("127.0.0.1:0"); err != nil {
		t.Fatalf("hosting: %v", err)
	}
	g := bareGame(Options{})
	g.net = sh.net
	sh.game = g
	sh.screen = screenPlaying

	if !sh.escapeDrive() || sh.screen != pageLeave {
		t.Fatalf("esc on a networked drive gave screen %v, want the leave dialog", sh.screen)
	}
	if !sh.net.Online() || sh.game == nil {
		t.Fatal("asking ended the game")
	}
	// Stay is selected first, and goes back to the drive.
	sh.stepPage(navInput{Input: keyEnter})
	if sh.screen != screenPlaying || !sh.net.Online() {
		t.Fatalf("Stay gave screen %v, online %v", sh.screen, sh.net.Online())
	}
	// Leave ends it for both.
	sh.escapeDrive()
	sh.stepPage(navInput{Input: keyRight})
	sh.game = nil // a bare game has nothing on the GPU to close
	sh.stepPage(navInput{Input: keyEnter})
	if sh.screen != pageTitle || sh.net.Online() {
		t.Errorf("Leave gave screen %v, online %v", sh.screen, sh.net.Online())
	}
}

func TestANewPackIsMarkedForTheTitle(t *testing.T) {
	clearNewPacks()
	t.Cleanup(clearNewPacks)
	sh := testShell(t)
	next, ok := nextPack(sh.access())
	if !ok {
		t.Fatal("a new player has no pack to open")
	}
	sh.game = bareGame(Options{})
	sh.collect() // the first look is a baseline
	sh.game.career.earned = next.Price
	sh.collect()
	if !sh.access().Has(next.ID) || !hasNewPacks() {
		t.Fatalf("banking %d opened the %s pack: %v; NEW shown: %v", next.Price, next.Name, sh.access().Has(next.ID), hasNewPacks())
	}
	r, _ := sh.pager.cur.List.Row(titleCareer)
	if r.view(false).Value != "NEW" {
		t.Error("the title's Career row does not say NEW")
	}
	sh.openPage(pageCareer)
	if hasNewPacks() {
		t.Error("the career page did not clear NEW")
	}
}

func TestAPackHeldByBypassIsNotNews(t *testing.T) {
	clearNewPacks()
	t.Cleanup(clearNewPacks)
	sh := testShell(t)
	sh.opts.UnlockAll = true
	sh.game = bareGame(Options{})
	sh.collect()
	sh.game.career.earned = 1_000_000
	sh.collect()
	if hasNewPacks() {
		t.Error("a pack the player already had by bypass was marked new")
	}
}

// TestAHostCarriesTheCustomMap holds hosting a custom map to the rule the
// snapshot already keeps: the host builds it, the share code goes in the
// snapshot, and the joiner builds the same world from it.
func TestAHostCarriesTheCustomMap(t *testing.T) {
	sh := testShell(t)
	if err := sh.net.StartHosting("127.0.0.1:0"); err != nil {
		t.Fatalf("hosting: %v", err)
	}
	spec := custom.FromMap(city.MapCity, 5)
	spec.Step[custom.Wind] = 2
	spec.Step[custom.Hills] = 1
	spec = spec.Normalize()
	sh.settings.Custom = spec.Encode()
	sh.customPlay = true
	sh.hostMode = ModeCoopCars

	o, err := sh.sessionOptions(false)
	if err != nil {
		t.Fatalf("hosting a custom map was refused: %v", err)
	}
	if o.Custom == nil || *o.Custom != spec || !o.Networked || o.Play != ModeCoopCars {
		t.Fatalf("host options: custom %v, networked %v, mode %v", o.Custom, o.Networked, o.Play)
	}

	// The host as New leaves it, and the snapshot it sends.
	host := o
	host.Map, host.Seed = o.Custom.Base, o.Custom.Seed
	g := &Game{opts: host, mode: host.Play, world: sim.NewWorld(worldConfig(host)), net: sh.net}
	snap := g.snapshot()

	join := joinerWorld(DefaultSettings().applyViewTo(DefaultOptions()).chaseReady(), snap)
	join.Networked = true
	if join.Custom == nil || *join.Custom != spec {
		t.Fatalf("the joiner has custom map %v, want %v", join.Custom, spec)
	}
	hc, jc := worldConfig(host), worldConfig(join)
	hc.PlayerBody, jc.PlayerBody = sim.Body{}, sim.Body{} // each drives their own car
	if !reflect.DeepEqual(hc, jc) {
		t.Errorf("the joiner builds another world:\nhost %+v\njoin %+v", hc, jc)
	}

	// Without the custom map chosen, the host hosts the settings' map.
	sh.customPlay = false
	if o, _ := sh.sessionOptions(false); o.Custom != nil {
		t.Error("a host that did not choose the custom map hosts it")
	}
}

func TestAddressRuneAcceptsHostsAndPorts(t *testing.T) {
	for _, c := range "127.0.0.1:7777abcXYZ-" {
		if !addressRune(c) {
			t.Errorf("%q was rejected", c)
		}
	}
	for _, c := range " /\\@#\t\n" {
		if addressRune(c) {
			t.Errorf("%q was accepted", c)
		}
	}
}

// TestANetworkedGameRunsOnUnderTheLeaveDialog drives the real shell: a hosted
// game keeps going while the leave dialog is up, so the other player is never
// left waiting, and a drive alone stands still while it is paused.
func TestANetworkedGameRunsOnUnderTheLeaveDialog(t *testing.T) {
	inTempConfig(t)
	openTestWindow(t, 640, 360)

	o := DefaultOptions()
	o.Width, o.Height, o.Mute = 640, 360, true
	o.Traffic, o.Police = 5, 1
	o.Given = Given{"traffic": true, "police": true}
	sh := NewShell(o)
	defer sh.Close()
	if err := sh.net.StartHosting("127.0.0.1:0"); err != nil {
		t.Fatalf("hosting: %v", err)
	}
	sh.hostMode = ModeChase
	sh.startGame(true)
	if sh.game == nil || !sh.game.Networked() || sh.net.Role() != netplay.Hosting {
		t.Fatalf("no hosted game: notice %q", sh.notice)
	}
	sh.Step(1.0 / 60)
	sh.escapeDrive()
	if sh.screen != pageLeave {
		t.Fatalf("esc gave screen %v", sh.screen)
	}
	before := sh.game.world.Time
	for range 10 {
		sh.Step(1.0 / 60)
	}
	if sh.screen != pageLeave || sh.game.world.Time <= before {
		t.Errorf("under the dialog the hosted world went from %v to %v (screen %v)", before, sh.game.world.Time, sh.screen)
	}

	sh.endGame()
	sh.startGame(false)
	if sh.game == nil || sh.game.Networked() {
		t.Fatalf("no drive alone: notice %q", sh.notice)
	}
	sh.Step(1.0 / 60)
	sh.escapeDrive()
	before = sh.game.world.Time
	for range 10 {
		sh.Step(1.0 / 60)
	}
	if sh.screen != pagePause || sh.game.world.Time != before {
		t.Errorf("the paused world went from %v to %v (screen %v)", before, sh.game.world.Time, sh.screen)
	}
	if _, ok := sh.game.topSpeed(); !ok {
		t.Error("the drive's top speed was never noted")
	}
}

// TestAGameCanBeClosedTwice holds Close to being safe to repeat: the game the
// window is opened with is closed by the shell and again by WithWindow, and
// unloading a shader twice aborts the process.
func TestAGameCanBeClosedTwice(t *testing.T) {
	openTestWindow(t, 320, 180)
	o := DefaultOptions()
	o.Width, o.Height = 320, 180
	g := New(o)
	g.Close()
	g.Close()
}
