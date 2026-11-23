package game

import (
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/keymap"
	"github.com/danielriddell21/crucible/menu"
	"github.com/danielriddell21/crucible/netplay"

	"github.com/danielriddell21/autobahn/internal/sim"
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

	if s.onPage() {
		// A screen from the registry draws its own backdrop and hints
		// (signpost.go); everything below is for the screens before it.
		s.drawPage(dt)
		return
	}
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
	case screenGarage:
		s.drawGarage(dt)
	case screenPacks:
		s.drawPacks()
	case screenShop:
		s.drawShop()
	case screenCustom:
		s.drawCompactMenu(s.menu)
	default:
		s.drawMenu(s.menu)
	}
	s.drawMenuHints()
}

// drawMenu renders a crucible menu with raylib. The model decides what the
// rows say and which is selected; this decides only how that looks, which is
// why the same menus can appear in an Ebiten game drawn a different way.
func (s *Shell) drawMenu(m *menu.Menu) {
	top := int32(s.opts.Height)/2 - int32(len(m.Items))*menuRowHeight/2
	s.drawMenuAt(m, top-menuTitleGap, top)
}

// drawMenuAt draws a menu with its title at one height and its first row at
// another.
func (s *Shell) drawMenuAt(m *menu.Menu, titleY, top int32) {
	w := int32(s.opts.Width)

	centred(m.Title, w, titleY, menuTitleSize, rl.RayWhite)
	for i, line := range m.Subtitle {
		centred(line, w, titleY+menuTitleSize+12+int32(i)*18, menuSubSize, hudDim)
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

// drawCompactMenu draws a menu with more rows than the usual spacing would fit,
// as the custom map's editor has: a smaller face and tighter rows.
func (s *Shell) drawCompactMenu(m *menu.Menu) {
	const titleSize, itemSize, rowHeight = 34, 16, 22
	w := int32(s.opts.Width)
	centred(m.Title, w, 12, titleSize, rl.RayWhite)
	for i, line := range m.Subtitle {
		centred(line, w, 12+titleSize+4+int32(i)*18, menuSubSize, hudDim)
	}
	top := int32(12 + titleSize + 4 + 20*len(m.Subtitle) + 6)
	for i, item := range m.Items {
		y := top + int32(i)*rowHeight
		label, col := item.Label(), hudText
		if i == m.Sel {
			col = menuSelected
			rl.DrawText(">", w/2-rl.MeasureText(label, itemSize)/2-18, y, itemSize, menuSelected)
		}
		centred(label, w, y, itemSize, col)
	}
}

// drawGarage shows the car on its turntable between the title and the rows
// that change it.
func (s *Shell) drawGarage(dt float32) {
	w, h := int32(s.opts.Width), int32(s.opts.Height)
	tex := s.showroom.render(sim.PlayerBodyOf(s.settings.Car), paintOf(s.settings.Paint), dt)
	// The title at the top, the car below it, and the rows under the car,
	// the whole stack centred in the window.
	stack := menuTitleSize + 40 + previewH + 44 + int32(len(s.menu.Items))*menuRowHeight
	titleY := max((h-stack)/2-20, 16)
	x, y := w/2-previewW/2, titleY+menuTitleSize+40
	src := rl.NewRectangle(0, 0, previewW, -previewH) // render targets are upside down
	rl.DrawTexturePro(tex, src, rl.NewRectangle(float32(x), float32(y), previewW, previewH), rl.NewVector2(0, 0), 0, rl.White)
	rl.DrawRectangleLines(x, y, previewW, previewH, menuRule)
	centred(carBlurb(s.settings.Car), w, y+previewH+10, menuSubSize, hudDim)
	s.drawMenuAt(s.menu, titleY, y+previewH+44)
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
	line := "the game begins the moment the host's world arrives"
	if s.hostingLobby() {
		line = strings.ToLower(s.hostMode.String()) + " begins the moment they arrive"
	}
	centred(line, w, h/2+64, menuSubSize, hudDim)
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
	case screenSettings, screenGarage, screenCustom:
		hints = []keymap.Binding{
			{Key: "up/down", Action: "choose"},
			{Key: "left/right", Action: "adjust"},
			{Key: "enter", Action: "back"},
		}
	case screenPacks:
		hints = []keymap.Binding{{Key: "enter", Action: "back"}}
	case screenShop:
		hints = []keymap.Binding{
			{Key: "up/down", Action: "choose"},
			{Key: "enter", Action: "buy"},
		}
	case screenTitle:
		hints = []keymap.Binding{
			{Key: "up/down", Action: "choose"},
			{Key: "left/right", Action: "change the map"},
			{Key: "enter", Action: "select"},
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
	g.sky(g.opts.Width, g.opts.Height)
	g.drawWorld(g.cam, g.camMode != CameraBonnet)
	if g.showHUD {
		g.drawHUD()
	}
}
