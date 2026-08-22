package game

import (
	"os"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"
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

// TestShellScreensDraw walks every menu screen and draws a frame of each,
// which catches a screen wired to a nil menu or a draw call the model cannot
// satisfy — neither of which the pure settings tests can see.
func TestShellScreensDraw(t *testing.T) {
	requireDisplay(t)

	rl.SetTraceLogLevel(rl.LogError)
	rl.InitWindow(640, 360, "autobahn test")
	defer rl.CloseWindow()

	o := DefaultOptions()
	o.Width, o.Height, o.Mute = 640, 360, true
	sh := NewShell(o)
	defer sh.Close()

	// The lobby only has anything to draw with a session behind it.
	if err := sh.net.StartHosting("127.0.0.1:0"); err != nil {
		t.Fatalf("hosting: %v", err)
	}
	defer sh.net.Leave()

	cases := []struct {
		name string
		show func()
	}{
		{"title", sh.showTitle},
		{"settings", func() { sh.showSettings(screenTitle) }},
		{"join", sh.showJoin},
		{"lobby", sh.enterLobby},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.show()
			sh.drawScreen(1.0 / 60)
			if sh.quit {
				t.Error("drawing the screen quit the shell")
			}
		})
	}
}

// TestLobbyStartsTheChaseWhenBothEndsArrive covers the reason the lobby
// exists: a player hosts before there is a world, and the game begins the
// moment the other one connects.
func TestLobbyStartsTheChaseWhenBothEndsArrive(t *testing.T) {
	o := DefaultOptions()
	o.Mute = true
	sh := NewShell(o)
	defer sh.net.Leave()

	if err := sh.net.StartHosting("127.0.0.1:0"); err != nil {
		t.Fatalf("hosting: %v", err)
	}
	sh.enterLobby()
	if sh.screen != screenLobby {
		t.Fatalf("screen = %v, want the lobby", sh.screen)
	}

	// Nobody has joined, so the lobby waits rather than starting.
	sh.watchLobby()
	if sh.screen != screenLobby {
		t.Fatalf("screen = %v, want the lobby to keep waiting", sh.screen)
	}
	if got := sh.net.Status(); got == "" {
		t.Error("the lobby has nothing to tell the player")
	}
}

func TestLeavingTheLobbyEndsTheSession(t *testing.T) {
	o := DefaultOptions()
	o.Mute = true
	sh := NewShell(o)
	defer sh.Close()

	if err := sh.net.StartHosting("127.0.0.1:0"); err != nil {
		t.Fatalf("hosting: %v", err)
	}
	sh.enterLobby()
	sh.net.Leave()
	sh.showTitle()

	if sh.net.Online() {
		t.Error("the session outlived the lobby")
	}
	if sh.screen != screenTitle {
		t.Errorf("screen = %v, want the title", sh.screen)
	}
}

func TestAFailedJoinReturnsToTheTitleWithAReason(t *testing.T) {
	o := DefaultOptions()
	o.Mute = true
	sh := NewShell(o)
	defer sh.Close()

	sh.joinAddr = "127.0.0.1:1" // nothing is listening
	sh.joinChase()

	if sh.screen != screenTitle {
		t.Errorf("screen = %v, want the title", sh.screen)
	}
	if sh.notice == "" {
		t.Error("the player was not told why the join failed")
	}
	if sh.net.Online() {
		t.Error("a failed join left a session running")
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
