package game

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/keymap"
	"github.com/danielriddell21/crucible/netplay"
)

// The lobby card (docs/menus.md, "Lobby"): a card over the dimmed menu
// backdrop that waits for the other player. hostChase and joinChase open it
// once the session is up; lobbyStep watches the session every frame.

// lobbyLink is the part of a netplay session the lobby reads and leaves. The
// shell passes its own session; tests pass a fake that can fail on demand.
type lobbyLink interface {
	Role() netplay.Role
	Addr() string
	Peered() bool
	Err() error
	Leave()
	Status() string
}

// lobbySlot is one of the two player slots: who, what they have, and whether
// they are ready, waiting or here. Swatch is the paint drawn as a dot.
type lobbySlot struct {
	Who, What, State string
	Tone             tone
	Swatch           rl.Color
}

// The words of the lobby. Each is a function of the state it shows, so the
// tests can read the wording with no window.

// lobbyHead is the heading: HOSTING for the host, CONNECTING for a joiner.
func lobbyHead(hosting bool) string {
	if hosting {
		return "HOSTING"
	}
	return "CONNECTING"
}

// lobbyAddrLabel says what the address box holds.
func lobbyAddrLabel(hosting bool) string {
	if hosting {
		return "TELL THEM THIS ADDRESS"
	}
	return "HOST"
}

// lobbyAddress is the address box's text: the host's own address for a host,
// the one being reached for a joiner.
func lobbyAddress(hosting bool, hostAddr, reaching string) string {
	if hosting {
		return hostAddr
	}
	return reaching
}

// lobbyNote says the mode and map for a host, who chooses them, and says that
// a joiner gets the host's choices.
func lobbyNote(hosting bool, mode NetMode, mapName string) string {
	if hosting {
		return mode.String() + " on " + mapName
	}
	return "you get the host's map, mode and weather"
}

// lobbyCarWords is the car and paint the player will drive, as named.
func lobbyCarWords(set Settings) (car, paint string) {
	return set.Car.Name(), paintOf(set.Paint).name
}

// lobbySlots is the two player slots: you, ready with your car and paint, and
// the other player, waiting until they are peered.
func lobbySlots(set Settings, hosting, peered bool) [2]lobbySlot {
	car, paint := lobbyCarWords(set)
	you := lobbySlot{
		Who:    "You",
		What:   car + " - " + paint,
		State:  "READY",
		Tone:   toneYours,
		Swatch: paintOf(set.Paint).col,
	}
	if hosting {
		you.Who = "You - host"
	}
	other := lobbySlot{
		Who:    "Host",
		What:   "their world arrives first",
		State:  "WAITING",
		Tone:   toneOffer,
		Swatch: signRule,
	}
	if hosting {
		other.Who = "Player 2"
		other.What = "joins with your address"
	}
	if peered {
		other.State, other.Tone = "HERE", toneYours
	}
	return [2]lobbySlot{you, other}
}

// lobbyActions is the buttons along the foot of the card, in order: a host
// can copy its address, and everyone can cancel.
func lobbyActions(hosting bool) []string {
	if hosting {
		return []string{"copy", "cancel"}
	}
	return []string{"cancel"}
}

// lobbyStep applies a frame's keys to the lobby: esc leaves the session and
// goes back; a network error that is not a peer leaves it for the title page,
// with the error as the notice; a peer starts the game, unless the two builds
// disagree. It reports whether it used the keys.
func (s *Shell) lobbyStep(link lobbyLink, in navInput) bool {
	if in.Back {
		s.lobbyCancel(link)
		return true
	}
	if err := link.Err(); err != nil && !link.Peered() {
		s.lobbyFail(link, err)
		return true
	}
	if link.Peered() {
		if err := s.protoErr(); err != nil {
			s.lobbyFail(link, err)
			return true
		}
		// The host's own car is the autopilot's only in a chase; in co-op both
		// players are at their keyboards.
		autopilot := link.Role() == netplay.Hosting && s.hostMode == ModeChase
		// The game takes the window, so the lobby's trail is dropped first:
		// nothing should come back to a lobby that has started.
		s.pager = pager{}
		s.startGame(autopilot)
		return true
	}
	return false
}

// lobbyCancel ends the session and goes back one screen, to the page the
// lobby was opened from.
func (s *Shell) lobbyCancel(link lobbyLink) {
	link.Leave()
	s.backPage()
}

// lobbyFail ends the session and goes to the title page, where the notice
// says why.
func (s *Shell) lobbyFail(link lobbyLink, err error) {
	s.notice = err.Error()
	link.Leave()
	s.goTitle()
}

// hostingLobby reports whether this end of the lobby is the host: it is the
// end with an address to give out.
func (s *Shell) hostingLobby() bool { return s.net.Addr() != "" }

// lobbyCopy puts the host's address on the clipboard.
func lobbyCopy(addr string) {
	if addr != "" {
		rl.SetClipboardText(addr)
	}
}

// hintsLobby is the bar for the lobby.
var hintsLobby = []keymap.Binding{
	{Key: "left/right", Action: "choose"},
	{Key: "enter", Action: "select"},
	{Key: "esc", Action: "cancel"},
}

// buildLobby makes the lobby page as it stands when it is opened: whether this
// is a host is taken from the session then.
func buildLobby(s *Shell) *menuPage {
	hosting := s.hostingLobby()
	copied := false

	var acts []menuRow
	for _, key := range lobbyActions(hosting) {
		switch key {
		case "copy":
			acts = append(acts, actionRow("copy", "Copy", func() {
				lobbyCopy(s.net.Addr())
				copied = true
			}))
		case "cancel":
			acts = append(acts, actionRow("cancel", "Cancel", func() { s.lobbyCancel(s.net) }))
		}
	}
	list := newRowList(acts...)
	list.Across = true

	return &menuPage{
		Title: "LOBBY",
		List:  list,
		Step:  func(in navInput) bool { return s.lobbyStep(s.net, in) },
		Back:  func() { s.lobbyCancel(s.net) },
		Draw: func(float32) {
			s.drawLobbyCard(list, hosting, &copied)
		},
		Hints: func() []keymap.Binding { return hintsLobby },
	}
}

var _ = registerPage(pageLobby, buildLobby, hintsLobby...)

// Lobby card layout, in pixels.
const (
	lobbyCardMaxW = 720
	lobbyPad      = 28
	lobbyTop      = 110
	lobbyGap      = 18
	lobbyBtnH     = 44
	lobbySlotH    = 74
	lobbyAddrH    = 72
)

var (
	lobbyHeadFace = typeFace{32, weightHeavy}
	lobbyAddrFace = typeFace{22, weightHeavy}
	lobbyPanel    = rl.NewColor(20, 24, 32, 255)
)

// drawLobbyCard draws the card: the spinner and heading, the address, the two
// slots, the mode and map, and the buttons along the foot.
func (s *Shell) drawLobbyCard(list *rowList, hosting bool, copied *bool) {
	w, h := int32(s.opts.Width), int32(s.opts.Height)
	rl.DrawRectangle(0, 0, w, h, menuVeil)

	cardW := min(int32(lobbyCardMaxW), w-32)
	inner := cardW - 2*lobbyPad
	x := (w - cardW) / 2
	y := int32(lobbyTop)
	cardH := int32(396)
	drawCard(rect(x, y, cardW, cardH), false)

	// Spinner: a full track with a turning arc, in the accent.
	cx, cy := float32(x+lobbyPad+17), float32(y+lobbyPad+17)
	rl.DrawRing(rl.NewVector2(cx, cy), 12, 16, 0, 360, 32, signRule)
	spin := float32(math.Mod(rl.GetTime()*360, 360))
	rl.DrawRing(rl.NewVector2(cx, cy), 12, 16, spin, spin+100, 24, signBlue)

	tx := x + lobbyPad + 50
	ty := y + lobbyPad
	lobbyHeadFace.draw(lobbyHead(hosting), tx, ty, signInk)
	faceBody.draw(s.net.Status(), tx, ty+lobbyHeadFace.height()+4, signAmber)

	// Address box.
	ay := y + lobbyPad + 70
	rounded(rect(x+lobbyPad, ay, inner, lobbyAddrH), 8, lobbyPanel)
	faceChip.draw(lobbyAddrLabel(hosting), x+lobbyPad+16, ay+12, signDim)
	lobbyAddrFace.draw(lobbyAddress(hosting, s.net.Addr(), s.joinAddr), x+lobbyPad+16, ay+32, signInk)

	// Slots.
	sy := ay + lobbyAddrH + lobbyGap
	slotW := (inner - 12) / 2
	slots := lobbySlots(s.settings, hosting, s.net.Peered())
	for i, sl := range slots {
		sx := x + lobbyPad + int32(i)*(slotW+12)
		rounded(rect(sx, sy, slotW, lobbySlotH), 10, lobbyPanel)
		rl.DrawCircle(sx+30, sy+lobbySlotH/2, 14, sl.Swatch)
		faceRow.draw(sl.Who, sx+58, sy+14, signInk)
		faceSmall.draw(sl.What, sx+58, sy+42, signDim)
		stateW := faceChip.width(sl.State)
		faceChip.draw(sl.State, sx+slotW-stateW-16, sy+lobbySlotH/2-faceChip.height()/2, toneInk(sl.Tone))
	}

	// Mode and map.
	ny := sy + lobbySlotH + lobbyGap
	faceBody.draw(lobbyNote(hosting, s.hostMode, s.settings.Map.Name()), x+lobbyPad, ny, signDim)

	// Buttons.
	by := ny + faceBody.height() + lobbyGap
	bw := int32(160)
	bx := x + cardW - lobbyPad - int32(len(list.rows))*(bw+12) + 12
	for _, v := range list.Views() {
		label := "Cancel"
		if v.Key == "copy" {
			label = "Copy"
			if *copied {
				label = "Copied"
			}
		}
		drawButton(rect(bx, by, bw, lobbyBtnH), label, v.Selected, false, tonePlain)
		bx += bw + 12
	}
}
