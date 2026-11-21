package game

import (
	"fmt"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/geom"
	"github.com/danielriddell21/crucible/menu"
	"github.com/danielriddell21/crucible/netplay"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/pack"
)

// screen is what the window is currently showing.
type screen int

const (
	// screenTitle is the front page: play, host, join, settings, quit.
	screenTitle screen = iota
	// screenPlaying hands the window to the game.
	screenPlaying
	// screenPaused is the in-game menu, over a frozen scene.
	screenPaused
	// screenSettings adjusts the stored preferences, reachable from either
	// the title screen or the pause menu.
	screenSettings
	// screenLobby waits for the other player, hosting or joining.
	screenLobby
	// screenJoin takes the address to connect to.
	screenJoin
	// screenGarage chooses the player's car, from the title screen or the
	// pause menu.
	screenGarage
	// screenPacks lists the packs and how far the player is toward each.
	screenPacks
)

// Shell is the menu layer around a session: the title screen a run opens on,
// the pause menu, the settings screen, and the multiplayer lobby.
//
// It owns the things that outlive a single game. The network session is the
// important one: a lobby exists precisely so a player can start hosting before
// there is a world to host, so the session is created here and handed to each
// game rather than the other way round.
type Shell struct {
	opts     Options
	settings Settings
	// career is what the player has banked across every session, which is
	// what the packs are unlocked with.
	career Career

	net    *netplay.Session[Snapshot, Input]
	screen screen
	prev   screen // where the settings screen returns to
	menu   *menu.Menu
	game   *Game
	// showroom is the garage's turntable.
	showroom showroom

	joinAddr string // the address being typed on screenJoin
	notice   string // the last thing that went wrong, shown under the title
	quit     bool
}

// NewShell builds the menu layer for a session, loading the player's stored
// settings.
func NewShell(opts Options) *Shell {
	s := &Shell{
		opts:     opts,
		settings: LoadSettings(),
		career:   LoadCareer(),
		net:      &netplay.Session[Snapshot, Input]{},
		joinAddr: "127.0.0.1",
	}
	s.settings.keepTo(s.access())
	s.showTitle()
	return s
}

// Close releases whatever the shell is holding.
func (s *Shell) Close() {
	if s.game != nil {
		s.game.Close()
	}
	s.showroom.unload()
	s.net.Leave()
}

// Step advances whichever screen is showing by one frame, drawing it. It
// reports false once the player has chosen to leave.
func (s *Shell) Step(dt float32) bool {
	if s.quit || rl.WindowShouldClose() {
		return false
	}
	if s.screen == screenPlaying {
		s.stepPlaying(dt)
		return !s.quit
	}
	s.stepMenu(dt)
	return !s.quit
}

// access is what the player holds: what they have banked, and whether the
// command line or the settings file waved the prices away.
func (s *Shell) access() pack.Access {
	return pack.Access{Banked: s.career.Banked, Bypass: s.opts.UnlockAll || s.settings.UnlockAll}
}

// collect moves the bounty the player has just banked into their career,
// and announces any pack it unlocked.
func (s *Shell) collect() {
	earned := s.game.TakeEarned()
	if earned == 0 {
		return
	}
	before := s.access()
	unlocked := s.career.Bank(earned)
	if err := s.career.Save(); err != nil {
		s.notice = err.Error()
	}
	for _, p := range unlocked {
		// A pack the player already had by bypass is not news.
		if !before.Has(p.ID) {
			s.game.Announce(strings.ToUpper(p.Name)+" PACK UNLOCKED", p.Blurb)
		}
	}
}

func (s *Shell) stepPlaying(dt float32) {
	if rl.IsKeyPressed(rl.KeyEscape) {
		// There is no pausing a game somebody else is in. A host that stops
		// simulating strands the other player and a joiner that stops sending
		// hands their car back to the simulation, so escape leaves the chase
		// rather than pretending to freeze it.
		if s.game.Networked() {
			s.endGame()
			return
		}
		s.showPause()
		return
	}
	s.game.Step(dt)
	s.collect()
}

func (s *Shell) stepMenu(dt float32) {
	switch s.screen {
	case screenJoin:
		s.editAddress()
	case screenLobby:
		s.watchLobby()
	default:
		s.menu.Update(pollMenu())
	}
	if s.quit || s.screen == screenPlaying {
		return
	}
	s.drawScreen(dt)
}

// pollMenu reads the family's conventional menu bindings from raylib. crucible
// ships the Ebiten equivalent; this is the same key set, read through the
// other backend, which is why the menu model itself stays display-free.
func pollMenu() menu.Input {
	down := func(keys ...int32) bool {
		for _, k := range keys {
			if rl.IsKeyPressed(k) {
				return true
			}
		}
		return false
	}
	return menu.Input{
		Up:     down(rl.KeyUp, rl.KeyW),
		Down:   down(rl.KeyDown, rl.KeyS),
		Left:   down(rl.KeyLeft, rl.KeyA),
		Right:  down(rl.KeyRight, rl.KeyD),
		Select: down(rl.KeyEnter, rl.KeySpace),
	}
}

func (s *Shell) showTitle() {
	s.screen = screenTitle
	s.menu = &menu.Menu{
		Title:    "AUTOBAHN",
		Subtitle: s.titleSubtitle(),
		Items: []menu.Item{
			{Label: text("Drive"), Action: func() { s.startGame(false) }},
			{Label: text("Autopilot"), Action: func() { s.startGame(true) }},
			{
				Label:  func() string { return "Map: " + s.settings.Map.Name() },
				Adjust: s.chooseMap,
				Action: func() { s.chooseMap(1) },
			},
			{Label: text("Host a chase"), Action: s.hostChase},
			{Label: text("Join a chase"), Action: s.showJoin},
			{Label: text("Garage"), Action: func() { s.showGarage(screenTitle) }},
			{Label: text("Packs"), Action: s.showPacks},
			{Label: text("Settings"), Action: func() { s.showSettings(screenTitle) }},
			{Label: text("Quit"), Action: func() { s.quit = true }},
		},
	}
}

// chooseMap steps the title screen's map through the ones the player holds.
func (s *Shell) chooseMap(dir int) {
	s.settings.Map = cycleMap(s.access().Maps(), s.settings.Map, dir)
}

// showPacks lists the packs: what each adds, and either that it is held or
// how much more has to be banked. The screen is drawn as cards; the menu
// behind it holds only the way back.
func (s *Shell) showPacks() {
	s.screen = screenPacks
	s.menu = &menu.Menu{
		Title: "PACKS",
		Items: []menu.Item{{Label: text("Back"), Action: s.showTitle}},
	}
}

func (s *Shell) titleSubtitle() []string {
	sub := []string{"a british driving game with a camera-driven autopilot"}
	if s.notice != "" {
		sub = append(sub, s.notice)
	}
	return sub
}

func (s *Shell) showPause() {
	s.screen = screenPaused
	s.menu = &menu.Menu{
		Title: "PAUSED",
		Items: []menu.Item{
			{Label: text("Resume"), Action: s.resume},
			{Label: text("Garage"), Action: func() { s.showGarage(screenPaused) }},
			{Label: text("Settings"), Action: func() { s.showSettings(screenPaused) }},
			{Label: text("Restart"), Action: func() { s.startGame(s.game.Autopilot()) }},
			{Label: text("Abandon the drive"), Action: s.endGame},
		},
	}
}

func (s *Shell) showSettings(from screen) {
	s.prev, s.screen = from, screenSettings
	set := &s.settings
	s.menu = &menu.Menu{
		Title:    "SETTINGS",
		Subtitle: []string{"left and right adjust, enter goes back"},
		Items: []menu.Item{
			{
				Label:  func() string { return "Volume        " + menu.Bar(set.Volume) },
				Adjust: func(d int) { set.Volume = geom.Clamp(set.Volume+float64(d)*volumeStep, 0, 1) },
			},
			{
				Label:  func() string { return "Sound         " + menu.OnOff(!set.Muted) },
				Adjust: func(int) { set.Muted = !set.Muted },
				Action: func() { set.Muted = !set.Muted },
			},
			{
				Label:  func() string { return "AI camera     " + menu.OnOff(set.ShowPanel) },
				Adjust: func(int) { set.ShowPanel = !set.ShowPanel },
				Action: func() { set.ShowPanel = !set.ShowPanel },
			},
			{
				Label:  func() string { return "Box labels    " + menu.OnOff(set.ShowLabels) },
				Adjust: func(int) { set.ShowLabels = !set.ShowLabels },
				Action: func() { set.ShowLabels = !set.ShowLabels },
			},
			{
				Label: func() string { return fmt.Sprintf("Traffic       %d cars", set.Traffic) },
				Adjust: func(d int) {
					set.Traffic = geom.Clamp(set.Traffic+d*trafficStep, trafficMin, trafficMax)
				},
			},
			{
				Label: func() string { return fmt.Sprintf("Police        %d units", set.Police) },
				Adjust: func(d int) {
					set.Police = geom.Clamp(set.Police+d*policeStep, policeMin, policeMax)
				},
			},
			{
				Label:  func() string { return "Viewpoint     " + set.Camera.String() },
				Adjust: func(d int) { set.Camera = cycleCamera(set.Camera, d) },
			},
			{
				Label: func() string { return "Autopilot     " + set.Mode.String() },
				Adjust: func(d int) {
					set.Mode = autopilot.Mode((int(set.Mode) + d + int(autopilot.NumModes)) % int(autopilot.NumModes))
				},
			},
			{
				Label:  func() string { return "Start time    " + set.Time.String() },
				Adjust: func(d int) { set.Time = cycleTime(set.Time, d) },
			},
			{
				Label:  func() string { return "Day length    " + set.DayLength.String() },
				Adjust: func(d int) { set.DayLength = cycleDayLength(set.DayLength, d) },
			},
			{Label: text("Back"), Action: s.leaveSettings},
		},
	}
}

// showGarage opens the garage, where the player's car is chosen. Changes take
// effect at once on a game that is running.
func (s *Shell) showGarage(from screen) {
	s.prev, s.screen = from, screenGarage
	set := &s.settings
	sub := "left and right choose, enter goes back"
	if from == screenPaused && s.game != nil && s.game.Networked() {
		sub = "the shape of the car is fixed for this chase"
	}
	s.menu = &menu.Menu{
		Title:    "GARAGE",
		Subtitle: []string{sub},
		Items: []menu.Item{
			{
				Label:  func() string { return "Car     " + set.Car.Name() },
				Adjust: func(d int) { set.Car = cycleBody(s.access().Bodies(), set.Car, d); s.applyCar() },
			},
			{
				Label:  func() string { return "Paint   " + paintOf(set.Paint).name },
				Adjust: func(d int) { set.Paint = cyclePaint(set.Paint, d); s.applyCar() },
			},
			{Label: text("Back"), Action: s.leaveSettings},
		},
	}
}

// applyCar puts the garage's choice on the car of a game that is running.
func (s *Shell) applyCar() {
	if s.game != nil {
		s.game.SetCar(s.settings.Car, s.settings.Paint)
	}
}

// cycleTime steps through the times of day, wrapping at both ends.
func cycleTime(t TimeOfDay, dir int) TimeOfDay {
	return TimeOfDay((int(t) + dir + int(NumTimes)) % int(NumTimes))
}

// cycleCamera steps through the viewpoints, wrapping at both ends.
func cycleCamera(m CameraMode, dir int) CameraMode {
	return CameraMode((int(m) + dir + int(numCameraModes)) % int(numCameraModes))
}

func (s *Shell) leaveSettings() {
	// Settings are written when the screen closes rather than on every
	// keypress, so holding a key down does not hammer the disk.
	if err := s.settings.Save(); err != nil {
		s.notice = err.Error()
	}
	s.applyLive()
	if s.prev == screenPaused && s.game != nil {
		s.showPause()
		return
	}
	s.showTitle()
}

// applyLive pushes the settings a running game can honour without restarting.
// Traffic and police counts cannot change under a live world, so they wait
// for the next session.
func (s *Shell) applyLive() {
	rl.SetMasterVolume(masterVolume(s.settings))
	if s.game == nil {
		return
	}
	s.game.ShowPanel(s.settings.ShowPanel)
	s.game.showLabels = s.settings.ShowLabels
	// The clock is only reset when the start time was changed, so opening
	// the settings mid-drive does not throw the evening away.
	if s.game.opts.Time != s.settings.Time {
		s.game.SetTime(s.settings.Time)
	}
	s.game.SetDayLength(s.settings.DayLength.Minutes())
	s.game.driver.Mode = s.settings.Mode
}

func masterVolume(set Settings) float32 {
	if set.Muted {
		return 0
	}
	return float32(set.Volume)
}

func (s *Shell) showJoin() {
	s.screen = screenJoin
	s.notice = ""
}

// editAddress runs the address entry on screenJoin. It is a plain text field
// rather than a menu, because there is no list of hosts to choose from.
func (s *Shell) editAddress() {
	const maxAddr = 64
	for {
		c := rl.GetCharPressed()
		if c == 0 {
			break
		}
		if len(s.joinAddr) < maxAddr && addressRune(c) {
			s.joinAddr += string(c)
		}
	}
	switch {
	case rl.IsKeyPressed(rl.KeyBackspace) && s.joinAddr != "":
		s.joinAddr = s.joinAddr[:len(s.joinAddr)-1]
	case rl.IsKeyPressed(rl.KeyEscape):
		s.showTitle()
	case rl.IsKeyPressed(rl.KeyEnter) && strings.TrimSpace(s.joinAddr) != "":
		s.joinChase()
	}
}

// addressRune reports whether a typed character can appear in a host and
// port. Anything else is dropped rather than shown and rejected later.
func addressRune(c rune) bool {
	switch {
	case c >= '0' && c <= '9',
		c >= 'a' && c <= 'z',
		c >= 'A' && c <= 'Z':
		return true
	default:
		return c == ':' || c == '.' || c == '-'
	}
}

func (s *Shell) hostChase() {
	if err := s.net.StartHosting(""); err != nil {
		s.notice = err.Error()
		s.showTitle()
		return
	}
	s.enterLobby()
}

func (s *Shell) joinChase() {
	if err := s.net.StartJoining(strings.TrimSpace(s.joinAddr)); err != nil {
		s.notice = err.Error()
		s.showTitle()
		return
	}
	s.enterLobby()
}

func (s *Shell) enterLobby() {
	s.screen, s.notice = screenLobby, ""
}

// watchLobby waits for the other player. The moment both ends are connected
// the chase starts on each of them; until then the only way out is back.
func (s *Shell) watchLobby() {
	if rl.IsKeyPressed(rl.KeyEscape) {
		s.net.Leave()
		s.showTitle()
		return
	}
	if err := s.net.Err(); err != nil && !s.net.Peered() {
		s.notice = err.Error()
		s.net.Leave()
		s.showTitle()
		return
	}
	if s.net.Peered() {
		s.startGame(s.net.Role() == netplay.Hosting)
	}
}

// startGame builds a session from the current settings and hands the window
// to it. A game already running is closed first.
func (s *Shell) startGame(autopilot bool) {
	if s.game != nil {
		s.game.Close()
	}
	o := s.opts
	if s.net.Online() {
		// The lobby starts the session, so the options never mention it; the
		// world still needs a police car for the other player to drive. The
		// stored world size is deliberately left out — see applyViewTo.
		o = s.settings.applyViewTo(o).chaseReady()
	} else {
		o = s.settings.applyTo(o)
	}
	o.Autopilot = autopilot
	if err := o.locked(s.access()); err != nil {
		// Only a command line can ask for what the player does not hold; the
		// menus never offer it.
		s.net.Leave()
		s.game = nil
		s.notice = err.Error()
		s.showTitle()
		return
	}
	// The map is chosen on the title screen, and starting a drive is the only
	// way off it that should remember the choice.
	if err := s.settings.Save(); err != nil {
		s.notice = err.Error()
	}

	s.game = New(o)
	s.game.net = s.net
	s.game.showLabels = s.settings.ShowLabels
	if s.game.Networked() {
		s.game.chaser = s.game.world.AssignChaser()
	}
	rl.SetMasterVolume(masterVolume(s.settings))
	s.screen = screenPlaying
}

func (s *Shell) resume() { s.screen = screenPlaying }

// endGame abandons the drive and returns to the title screen, ending any
// network session with it.
func (s *Shell) endGame() {
	if s.game != nil {
		s.game.Close()
		s.game = nil
	}
	s.net.Leave()
	s.showTitle()
}

// text wraps a fixed string as a menu label, for the rows that do not change.
func text(s string) func() string { return func() string { return s } }
