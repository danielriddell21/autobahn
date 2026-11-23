package game

import (
	"errors"
	"testing"

	"github.com/danielriddell21/crucible/menu"
	"github.com/danielriddell21/crucible/netplay"
)

// fakeLink is a lobby link a test can make fail. Leave records that the
// session was left.
type fakeLink struct {
	err    error
	peered bool
	role   netplay.Role
	left   bool
}

func (f *fakeLink) Role() netplay.Role { return f.role }
func (f *fakeLink) Addr() string       { return "" }
func (f *fakeLink) Peered() bool       { return f.peered }
func (f *fakeLink) Err() error         { return f.err }
func (f *fakeLink) Leave()             { f.left = true }
func (f *fakeLink) Status() string     { return "" }

func TestLobbyIsRegistered(t *testing.T) {
	sh := testShell(t)
	e, stub := pageFor(pageLobby)
	if stub {
		t.Fatal("the lobby page is still a stub")
	}
	if pg := e.build(sh); pg == nil || pg.Title == "" || pg.Step == nil || pg.Draw == nil {
		t.Fatal("the lobby page builds without its step or drawing")
	}
}

func TestLobbyWordingForHostAndJoin(t *testing.T) {
	if got := lobbyHead(true); got != "HOSTING" {
		t.Errorf("host heading %q, want HOSTING", got)
	}
	if got := lobbyHead(false); got != "CONNECTING" {
		t.Errorf("join heading %q, want CONNECTING", got)
	}
	if got := lobbyAddrLabel(true); got != "TELL THEM THIS ADDRESS" {
		t.Errorf("host address label %q", got)
	}
	if got := lobbyAddrLabel(false); got != "HOST" {
		t.Errorf("join address label %q", got)
	}
	if got := lobbyAddress(true, "10.0.0.2:7777", "127.0.0.1"); got != "10.0.0.2:7777" {
		t.Errorf("host shows %q, want its own listening address", got)
	}
	if got := lobbyAddress(false, "10.0.0.2:7777", "127.0.0.1:7777"); got != "127.0.0.1:7777" {
		t.Errorf("joiner shows %q, want the one being reached", got)
	}
	if got := lobbyNote(true, ModeCoopCar, "village", false); got != "Co-op: one car on village" {
		t.Errorf("host note %q", got)
	}
	if got := lobbyNote(true, ModeChase, "village", true); got != "Chase on a custom map: banks nothing" {
		t.Errorf("custom host note %q", got)
	}
	if got := lobbyNote(false, ModeChase, "city", false); got != "you get the host's map, mode and weather" {
		t.Errorf("joiner note %q", got)
	}
}

func TestLobbySlots(t *testing.T) {
	set := Settings{Car: 0, Paint: 1}
	car, paint := lobbyCarWords(set)
	if paint != paintOf(1).name {
		t.Errorf("paint %q, want %q", paint, paintOf(1).name)
	}
	if car == "" {
		t.Error("car has no name")
	}

	slots := lobbySlots(set, true, false)
	you, other := slots[0], slots[1]
	if you.Who != "You - host" || you.State != "READY" || you.Tone != toneYours {
		t.Errorf("host's own slot %+v", you)
	}
	if you.What != car+" - "+paint {
		t.Errorf("own slot says %q, want the car and paint", you.What)
	}
	if other.Who != "Player 2" || other.State != "WAITING" || other.Tone != toneOffer {
		t.Errorf("host's other slot %+v", other)
	}

	slots = lobbySlots(set, true, true)
	if slots[1].State != "HERE" || slots[1].Tone != toneYours {
		t.Errorf("a peered player should be HERE, got %+v", slots[1])
	}

	join := lobbySlots(set, false, false)
	if join[0].Who != "You" || join[1].Who != "Host" || join[1].State != "WAITING" {
		t.Errorf("joiner's slots %+v", join)
	}
}

func TestLobbyActions(t *testing.T) {
	if got := lobbyActions(true); len(got) != 2 || got[0] != "copy" || got[1] != "cancel" {
		t.Errorf("host actions %v, want copy then cancel", got)
	}
	if got := lobbyActions(false); len(got) != 1 || got[0] != "cancel" {
		t.Errorf("joiner actions %v, want cancel only", got)
	}
}

func TestLobbyCancelLeavesTheSession(t *testing.T) {
	sh := testShell(t)
	if err := sh.net.StartHosting("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	sh.openPage(pageLobby)
	if !sh.onPage() || sh.screen != pageLobby {
		t.Fatal("the lobby did not open")
	}

	sh.stepPage(navInput{Back: true})

	if sh.net.Role() != netplay.Offline {
		t.Errorf("cancel left the session as %v, want offline", sh.net.Role())
	}
	if sh.screen != pageTitle {
		t.Errorf("cancel went to screen %v, want back to the title", sh.screen)
	}
}

func TestLobbyEnterOnCancelLeavesTheSession(t *testing.T) {
	sh := testShell(t)
	if err := sh.net.StartHosting("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	sh.openPage(pageLobby)
	// A host has Copy first; Cancel is the one to the right.
	sh.stepPage(navInput{Input: menu.Input{Right: true}})
	sh.stepPage(navInput{Input: menu.Input{Select: true}})
	if sh.net.Role() != netplay.Offline || sh.screen != pageTitle {
		t.Errorf("enter on Cancel: role %v, screen %v", sh.net.Role(), sh.screen)
	}
}

func TestLobbyErrorLeavesWithANotice(t *testing.T) {
	sh := testShell(t)
	sh.openPage(pageLobby)
	link := &fakeLink{err: errors.New("the host went away")}

	if !sh.lobbyStep(link, navInput{}) {
		t.Fatal("an error should be handled by the lobby")
	}
	if !link.left {
		t.Error("an error did not leave the session")
	}
	if sh.notice != "the host went away" {
		t.Errorf("notice %q", sh.notice)
	}
	if !sh.onPage() || sh.screen != pageTitle {
		t.Errorf("error went to screen %v, want the title", sh.screen)
	}
}

func TestLobbyWaitsWhenNothingHappens(t *testing.T) {
	sh := testShell(t)
	sh.openPage(pageLobby)
	link := &fakeLink{}
	if sh.lobbyStep(link, navInput{}) {
		t.Error("an idle lobby should not use the keys")
	}
	if link.left || sh.notice != "" || !sh.onPage() {
		t.Error("an idle lobby changed state")
	}
}
