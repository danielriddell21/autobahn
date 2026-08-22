package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/keymap"
	"github.com/danielriddell21/crucible/menu"
	"github.com/danielriddell21/crucible/netplay"
)

// Menu layout, in pixels.
const (
	menuTitleSize = 52
	menuSubSize   = 14
	menuItemSize  = 22
	menuRowHeight = 38
	menuTitleGap  = 96
)

var (
	menuBackdrop = rl.NewColor(12, 14, 18, 255)
	menuVeil     = rl.NewColor(8, 10, 14, 215)
	menuSelected = rl.NewColor(104, 172, 232, 255)
	menuRule     = rl.NewColor(60, 66, 78, 255)
)

// drawScreen paints whichever menu screen is showing. A pause menu is drawn
// over the frozen scene behind it, so the player can see what they are going
// back to; every other screen stands on its own backdrop.
func (s *Shell) drawScreen(dt float32) {
	rl.BeginDrawing()
	defer rl.EndDrawing()

	if s.screen == screenPaused && s.game != nil {
		s.game.drawFrozen(dt)
		rl.DrawRectangle(0, 0, int32(s.opts.Width), int32(s.opts.Height), menuVeil)
	} else {
		rl.ClearBackground(menuBackdrop)
	}

	switch s.screen {
	case screenJoin:
		s.drawJoin()
	case screenLobby:
		s.drawLobby()
	default:
		s.drawMenu(s.menu)
	}
	s.drawMenuHints()
}

// drawMenu renders a crucible menu with raylib. The model decides what the
// rows say and which is selected; this decides only how that looks, which is
// why the same menus can appear in an Ebiten game drawn a different way.
func (s *Shell) drawMenu(m *menu.Menu) {
	w := int32(s.opts.Width)
	top := int32(s.opts.Height)/2 - int32(len(m.Items))*menuRowHeight/2

	centred(m.Title, w, top-menuTitleGap, menuTitleSize, rl.RayWhite)
	for i, line := range m.Subtitle {
		centred(line, w, top-menuTitleGap+menuTitleSize+12+int32(i)*18, menuSubSize, hudDim)
	}

	for i, item := range m.Items {
		y := top + int32(i)*menuRowHeight
		label, col := item.Label(), hudText
		if i == m.Sel {
			col = menuSelected
			// A caret rather than a filled bar: the rows are wide and
			// unevenly long, so a highlight block would look ragged.
			caret := rl.MeasureText(label, menuItemSize)/2 + 18
			rl.DrawText(">", w/2-caret, y, menuItemSize, menuSelected)
		}
		centred(label, w, y, menuItemSize, col)
	}
}

func (s *Shell) drawJoin() {
	w, h := int32(s.opts.Width), int32(s.opts.Height)
	centred("JOIN A CHASE", w, h/2-140, menuTitleSize, rl.RayWhite)
	centred("the host's address, and the port if it is not the usual one",
		w, h/2-76, menuSubSize, hudDim)

	// The field, with a blinking caret so it reads as somewhere to type.
	const fieldW, fieldH = 460, 52
	x, y := w/2-fieldW/2, h/2-fieldH/2
	rl.DrawRectangle(x, y, fieldW, fieldH, rl.NewColor(20, 23, 28, 255))
	rl.DrawRectangleLines(x, y, fieldW, fieldH, menuRule)

	shown := s.joinAddr
	if int(rl.GetTime()*2)%2 == 0 {
		shown += "_"
	}
	rl.DrawText(shown, x+16, y+16, menuItemSize, hudText)
	centred("port "+netplay.DefaultPort+" is assumed if you leave it off", w, y+fieldH+14, menuSubSize, hudDim)
}

func (s *Shell) drawLobby() {
	w, h := int32(s.opts.Width), int32(s.opts.Height)
	title := "HOSTING"
	if !s.hostingLobby() {
		title = "CONNECTING"
	}
	centred(title, w, h/2-120, menuTitleSize, rl.RayWhite)

	// crucible's session already words this for a player to read.
	centred(s.net.Status(), w, h/2-30, menuItemSize, hudWarn)
	if addr := s.net.Addr(); addr != "" {
		centred("they join with the address of this machine", w, h/2+10, menuSubSize, hudDim)
	}
	centred("the chase begins the moment they arrive", w, h/2+64, menuSubSize, hudDim)
}

func (s *Shell) hostingLobby() bool { return s.net.Addr() != "" }

// drawMenuHints puts the controls along the bottom, laid out by crucible's
// keymap so the bar reads the same as every other screen in the family.
func (s *Shell) drawMenuHints() {
	var hints []keymap.Binding
	switch s.screen {
	case screenJoin:
		hints = []keymap.Binding{
			{Key: "type", Action: "address"},
			{Key: "enter", Action: "connect"},
			{Key: "esc", Action: "back"},
		}
	case screenLobby:
		hints = []keymap.Binding{{Key: "esc", Action: "cancel"}}
	case screenSettings:
		hints = []keymap.Binding{
			{Key: "up/down", Action: "choose"},
			{Key: "left/right", Action: "adjust"},
			{Key: "enter", Action: "back"},
		}
	default:
		hints = []keymap.Binding{
			{Key: "up/down", Action: "choose"},
			{Key: "enter", Action: "select"},
		}
	}
	for _, line := range keymap.BottomBar(hints, s.opts.Width, s.opts.Height, 16, menuHintFace) {
		rl.DrawText(line.Text, int32(line.X), int32(line.Y), menuSubSize, hudDim)
	}
}

// menuHintFace measures in the same font the hints are drawn in, so keymap
// wraps them where they actually stop fitting.
var menuHintFace = keymap.Face{
	LineHeight: 18,
	Measure:    func(s string) int { return int(rl.MeasureText(s, menuSubSize)) },
}

func centred(text string, w, y int32, size int32, col rl.Color) {
	rl.DrawText(text, w/2-rl.MeasureText(text, size)/2, y, size, col)
}

// drawFrozen paints the scene as it stands without advancing it, so the pause
// menu has the world behind it rather than a black screen.
func (g *Game) drawFrozen(float32) {
	rl.ClearBackground(colSky)
	g.drawWorld(g.cam, g.camMode != CameraBonnet)
	if g.showHUD {
		g.drawHUD()
	}
}
