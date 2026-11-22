package game

import (
	"fmt"
	"slices"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/geom"
	"github.com/danielriddell21/crucible/menu"
	"github.com/danielriddell21/crucible/netplay"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/scenario"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/weather"
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
	// screenShop lists what earnings can buy.
	screenShop
	// screenChallenges lists the scenarios to drive against the autopilot's ghost.
	screenChallenges
	// screenCustom edits the custom map.
	screenCustom
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

	net *netplay.Session[Snapshot, Input]
	// hostMode is what the host row on the title screen will start.
	hostMode NetMode
	screen   screen
	prev     screen // where the settings screen returns to
	menu     *menu.Menu
	game     *Game
	// showroom is the garage's turntable.
	showroom showroom

	// feedFeats turns the running drive's totals into what the achievements are
	// fed, and is renewed with each session.
	feedFeats featFeed
	// customPlay says the next game is on the custom map in the settings, and
	// challenge that it is that scenario; both are cleared on the title screen.
	customPlay bool
	challenge  *scenario.Scenario
	// customEd is the custom map screen's editor, and customNote its last message.
	customEd   *customScreen
	customNote string

	joinAddr string // the address being typed on screenJoin
	notice   string // the last thing that went wrong, shown under the title
	quit     bool
}

// NewShell builds the menu layer for a session, loading the player's stored
// settings.
func NewShell(opts Options) *Shell {
	set := LoadSettings()
	s := &Shell{
		opts:     opts,
		settings: set,
		career:   loadCareerFor(set),
		net:      &netplay.Session[Snapshot, Input]{},
		joinAddr: "127.0.0.1",
		hostMode: set.HostMode,
	}
	// A migrated file is written back once. Under bypass nothing about
	// ownership is ever written, so the file is left as it was found.
	if !s.access().Bypass {
		if err := s.career.Save(); err != nil {
			s.notice = err.Error()
		}
	}
	s.settings.keepTo(s.holdings())
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

// payJobs adds what the finished jobs paid to the career's earnings and
// reports whether anything was added.
func (s *Shell) payJobs(rs []job.Result) bool {
	paid := false
	for _, r := range rs {
		if r.Pay > 0 {
			s.career.Earn(r.Pay)
			paid = true
		}
	}
	return paid
}

// collect moves the bounty the player has just banked into their career,
// and announces any pack it unlocked.
func (s *Shell) collect() {
	g := s.game
	if out, ok := g.TakeChallenge(); ok && g.chal != nil {
		if s.career.Challenges == nil {
			s.career.Challenges = scenario.Bests{}
		}
		if s.career.Challenges.Record(g.chal.run.Scenario().ID, out) {
			s.saveCareer()
		}
	}
	jobs, earned := g.TakeJobResults(), g.TakeEarned()
	if !g.banks() {
		// A custom map or a challenge pays nothing and counts for nothing: what
		// was collected is thrown away, and the achievements never see the drive.
		return
	}
	// Job pay is earnings, which the shop spends. Bounty is not earnings: it
	// only ever opens packs.
	if s.payJobs(jobs) {
		s.saveCareer()
	}
	s.collectFeats(earned)
	if earned == 0 {
		return
	}
	before := s.access()
	unlocked := s.career.Bank(earned)
	s.saveCareer()
	for _, p := range unlocked {
		// A pack the player already had by bypass is not news.
		if !before.Has(p.ID) {
			g.Announce(strings.ToUpper(p.Name)+" PACK UNLOCKED", p.Blurb)
		}
	}
}

// saveCareer writes the career, telling the player when it will not stick.
func (s *Shell) saveCareer() {
	if err := s.career.Save(); err != nil {
		s.notice = err.Error()
	}
}

// collectFeats gives the achievements the stretch of drive since they last
// looked, and announces what it unlocked.
func (s *Shell) collectFeats(earned int) {
	x, ok := s.feedFeats.fact(s.game.featCounters(), earned)
	if !ok {
		return
	}
	got := s.career.RecordFeats(x)
	if len(got) == 0 {
		return
	}
	s.saveCareer()
	names := make([]string, len(got))
	for i, a := range got {
		names[i] = a.Name
	}
	s.game.Announce("ACHIEVEMENT", strings.Join(names, ", "))
}

// banks reports whether this game counts toward the career: bounty, earnings
// and achievements. A custom map never does, and nor does a challenge, which
// is a score and not a drive.
func (g *Game) banks() bool { return custom.Banks(g.opts.Custom) && g.chal == nil }

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
		// Escape shuts the job menu, if it is open, before it pauses: Step does it.
		if s.game.jobMenu == nil {
			s.showPause()
			return
		}
	}
	if s.game.chal != nil && rl.IsKeyPressed(rl.KeyF5) {
		s.retryChallenge()
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
	s.customPlay, s.challenge = false, nil
	s.menu = &menu.Menu{
		Title:    "AUTOBAHN",
		Subtitle: s.titleSubtitle(),
		Items: s.withShop([]menu.Item{
			{Label: text("Drive"), Action: func() { s.startGame(false) }},
			{Label: text("Autopilot"), Action: func() { s.startGame(true) }},
			{
				Label: func() string {
					return fmt.Sprintf("Map: %s in %s", s.settings.Map.Name(), s.settings.Map.Season().Name())
				},
				Adjust: s.chooseMap,
				Action: func() { s.chooseMap(1) },
			},
			{
				Label:  func() string { return "Host: " + s.hostMode.String() },
				Adjust: func(d int) { s.hostMode = cycleNetMode(s.hostMode, d) },
				Action: s.hostChase,
			},
			{Label: text("Join a chase"), Action: s.showJoin},
			{Label: text("Challenges"), Action: s.showChallenges},
			{Label: text("Custom map"), Action: s.showCustom},
			{Label: text("Garage"), Action: func() { s.showGarage(screenTitle) }},
			{Label: text("Packs"), Action: s.showPacks},
			{Label: text("Achievements"), Action: s.showFeats},
			{Label: text("Settings"), Action: func() { s.showSettings(screenTitle) }},
			{Label: text("Quit"), Action: func() { s.quit = true }},
		}),
	}
}

// withShop puts the Shop row after Packs. A player who has bypassed the
// prices has nothing to buy, so the row is left out.
func (s *Shell) withShop(items []menu.Item) []menu.Item {
	if s.access().Bypass {
		return items
	}
	row := menu.Item{Label: text("Shop"), Action: func() { s.showShop(0) }}
	for i, it := range items {
		if it.Label() == "Packs" {
			return slices.Insert(items, i+1, row)
		}
	}
	return items
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
				Label:  func() string { return "Jobs on auto  " + menu.OnOff(set.AutopilotJobs) },
				Adjust: func(int) { set.AutopilotJobs = !set.AutopilotJobs },
				Action: func() { set.AutopilotJobs = !set.AutopilotJobs },
			},
			{
				Label:  func() string { return "Weather       " + set.Weather.String() },
				Adjust: func(d int) { set.Weather = cycleWeather(set.Weather, d) },
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
				Adjust: func(d int) { set.Car = cycleBody(s.holdings().Drivables(), set.Car, d); s.applyCar() },
			},
			{
				Label:  func() string { return "Paint   " + paintOf(set.Paint).name },
				Adjust: func(d int) { set.Paint = cyclePaint(s.holdings().Wearables(), set.Paint, d); s.applyCar() },
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

// cycleWeather steps through the weather settings, drift first, wrapping at
// both ends.
func cycleWeather(w weather.Setting, dir int) weather.Setting {
	all := append([]weather.Setting{weather.Drift}, weather.Held(weather.Clear), weather.Held(weather.Rain),
		weather.Held(weather.Fog), weather.Held(weather.Storm))
	i := slices.Index(all, w)
	return all[(max(i, 0)+dir+len(all))%len(all)]
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
		if err := s.protoErr(); err != nil {
			s.notice = err.Error()
			s.net.Leave()
			s.showTitle()
			return
		}
		// The host's own car is the autopilot's only in a chase; in co-op both
		// players are at their keyboards.
		s.startGame(s.net.Role() == netplay.Hosting && s.hostMode == ModeChase)
	}
}

// startGame builds a session from the current settings and hands the window
// to it. A game already running is closed first.
func (s *Shell) startGame(autopilot bool) {
	if s.game != nil {
		s.game.Close()
	}
	o := s.opts
	switch {
	case s.net.Role() == netplay.Joining:
		// The joiner builds what the host built, whatever its own settings say:
		// the snapshot names the world, and the lobby has already seen one.
		o = s.joinerOptions(o)
	case s.net.Online():
		// The host chooses the world and the mode, and the joiner is told them.
		o = s.settings.applyTo(o).chaseReady()
		if !o.Given["play"] {
			o.Play = s.hostMode
		}
	default:
		o = s.settings.applyTo(o)
		if s.customPlay {
			o.Custom = customOf(s.settings.Custom)
		}
		o.Challenge = s.challenge
	}
	o.Autopilot = autopilot && !o.Play.SharedCar()
	if err := o.coopValid(); err != nil {
		s.net.Leave()
		s.game = nil
		s.notice = err.Error()
		s.showTitle()
		return
	}
	o.Access = s.access()
	if err := o.lockedFor(s.holdings()); err != nil {
		// Only a command line can ask for what the player does not hold or
		// own; the menus never offer it.
		s.net.Leave()
		s.game = nil
		s.notice = err.Error()
		s.showTitle()
		return
	}
	// The map is chosen on the title screen, and starting a drive is the only
	// way off it that should remember the choice.
	s.settings.HostMode = s.hostMode
	if err := s.settings.Save(); err != nil {
		s.notice = err.Error()
	}

	s.feedFeats = featFeed{}
	s.game = New(o)
	s.game.net = s.net
	s.game.showLabels = s.settings.ShowLabels
	if s.game.Networked() {
		s.game.seat()
	}
	rl.SetMasterVolume(masterVolume(s.settings))
	s.screen = screenPlaying
}

// protoErr is why a joiner cannot play against the host it has reached, or nil.
// The two builds have to agree on what a snapshot means.
func (s *Shell) protoErr() error {
	if s.net.Role() != netplay.Joining {
		return nil
	}
	snap, ok := s.net.Snapshot()
	if ok && snap.Proto != Proto {
		return fmt.Errorf("the host is running a different version of the game (protocol %d, this is %d)",
			snap.Proto, Proto)
	}
	return nil
}

// joinerOptions is what a joining player builds a game from: the host's world
// and mode, in the joiner's own view of it.
func (s *Shell) joinerOptions(o Options) Options {
	o = s.settings.applyViewTo(o).chaseReady()
	snap, ok := s.net.Snapshot()
	if !ok {
		return o
	}
	o.Seed, o.Map = snap.Seed, city.Map(snap.Map)
	o.Traffic, o.Police = snap.Traffic, max(snap.Police, 1)
	o.Weather = weather.Setting(snap.Weather)
	o.Play = snap.Mode
	o.Custom = customOf(snap.Custom)
	if snap.Mode.SharedCar() {
		o.Car, o.Paint = sim.BodyKind(snap.Car), snap.Paint
	}
	return o
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
