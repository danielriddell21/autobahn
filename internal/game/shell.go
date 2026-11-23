package game

import (
	"fmt"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/menu"
	"github.com/danielriddell21/crucible/netplay"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/scenario"
	"github.com/danielriddell21/autobahn/internal/shop"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/weather"
)

// screen is what the window is showing: the drive, or one of the pages of the
// screen registry (menupages.go). The zero value is the title page.
type screen int

// screenPlaying hands the window to the game. It is the one screen that is not
// a page.
const screenPlaying screen = -1

// Shell is the menu layer around a session: the pages of the screen registry
// (menupages.go, one menu_*.go file each), the drive they start, and the menu
// drive behind them.
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
	// hostMode is what hosting from the Play page will start.
	hostMode NetMode
	screen   screen
	game     *Game
	// drive is the autopilot driving behind the menus, never s.game, so nothing
	// it does reaches collect; see menu_drive.go.
	drive menuDrive
	// showroom is the garage's turntable.
	showroom showroom
	// titlePlan is the picture of the map on the title page's card.
	titlePlan planCache

	// feedFeats turns the running drive's totals into what the achievements are
	// fed, and is renewed with each session.
	feedFeats featFeed
	// customPlay says the next game is on the custom map in the settings, and
	// challenge that it is that scenario; both are cleared on the title page.
	customPlay bool
	challenge  *scenario.Scenario

	// pager is the page from the screen registry that is showing, if one is,
	// and what esc goes back through; see menupages.go.
	pager pager

	joinAddr string // the address typed on the Play page's join panel
	// joining is whether the Play page's multiplayer joins rather than hosts.
	// Like the address it lasts the session and is not stored.
	joining bool
	notice  string // the last thing that went wrong, shown under the title
	quit    bool
}

// NewShell builds the menu layer for a session, loading the player's stored
// settings. It opens on the title page.
func NewShell(opts Options) *Shell {
	set := LoadSettings()
	career, migrated := readCareer(set)
	s := &Shell{
		opts:     opts,
		settings: set,
		career:   career,
		net:      &netplay.Session[Snapshot, Input]{},
		joinAddr: "127.0.0.1",
		hostMode: set.HostMode,
	}
	// A migrated file is written back once, and nothing else is: a file that
	// was already current is left alone, and one that could not be read is never
	// replaced. Under bypass nothing about ownership is ever written, so the
	// file is left as it was found.
	switch {
	case career.keep:
		s.notice = "the career file could not be read: playing without saving it"
	case migrated && !s.access().Bypass:
		if err := s.career.Save(); err != nil {
			s.notice = err.Error()
		}
	}
	s.settings.keepTo(s.holdings())
	s.goTitle()
	return s
}

// goTitle shows the title page with nothing behind it to go back to. It is
// where every drive, failed start and lobby ends; the notice, if there is one,
// stays to be read there.
func (s *Shell) goTitle() {
	s.pager = pager{}
	s.showPage(pageTitle, s.buildPage(pageTitle))
}

// Close releases whatever the shell is holding.
func (s *Shell) Close() {
	if s.game != nil {
		s.game.Close()
	}
	s.closeMenuDrive() // menu_drive.go
	s.showroom.unload()
	unloadMenuFonts() // menufont.go
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
	var news []pack.Pack
	for _, p := range unlocked {
		// A pack the player already had by bypass is not news.
		if !before.Has(p.ID) {
			g.Announce(strings.ToUpper(p.Name)+" PACK UNLOCKED", p.Blurb)
			news = append(news, p)
		}
	}
	// The title's Career row says NEW until the career page has been seen.
	markNewPacks(news)
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

// stepPlaying runs a frame of the drive. Esc leaves the drive for a page: the
// pause page, or in a networked game the leave dialog, under which the game
// runs on (stepBehindPage).
func (s *Shell) stepPlaying(dt float32) {
	if rl.IsKeyPressed(rl.KeyEscape) && s.escapeDrive() {
		return
	}
	if s.game.chal != nil && rl.IsKeyPressed(rl.KeyF5) {
		s.retryChallenge()
		return
	}
	s.game.Step(dt)
	s.game.notePeakSpeed() // for the pause page's top speed: menu_pause.go
	s.collect()
}

// escapeDrive is esc on the drive, and reports whether it took the window from
// the game. A game somebody else is in has no pause: a host that stops
// simulating strands the other player and a joiner that stops sending hands
// their car back to the simulation, so esc asks whether to leave it instead.
// Esc shuts the job menu, if it is open, before it pauses; Step does that.
func (s *Shell) escapeDrive() bool {
	switch {
	case s.game.Networked():
		s.openPage(pageLeave)
	case s.game.jobMenu == nil:
		s.openPage(pagePause)
	default:
		return false
	}
	return true
}

// stepMenu runs a frame of the page showing: its keys, the game or menu drive
// behind it, and the drawing.
func (s *Shell) stepMenu(dt float32) {
	s.stepPage(pollNav())
	if s.quit || s.screen == screenPlaying {
		return
	}
	s.stepBehindPage(dt)
	s.stepMenuDrive(dt) // the autopilot behind the menus: menu_drive.go
	s.drawScreen(dt)
}

// stepBehindPage keeps a networked game going while a page is over it, which
// is only ever the leave dialog. The other player's game must neither freeze
// nor time out while this one decides, so the world, the session and the sound
// advance as on the drive, with the keys left to the dialog; the page draws the
// scene behind itself (drawPage). A single-player game stays paused.
func (s *Shell) stepBehindPage(dt float32) {
	if s.game == nil || !s.game.Networked() {
		return
	}
	s.game.advanceHandsOff(dt)
	s.game.notePeakSpeed()
	s.collect()
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

// applyCar puts the garage's choice on the car of a game that is running.
func (s *Shell) applyCar() {
	if s.game != nil {
		s.game.SetCar(s.settings.Car, s.settings.Paint)
	}
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

// hostChase starts hosting and waits in the lobby for the other player. The
// lobby opens only once the session is up; a session that cannot start goes
// back to the title with the reason.
func (s *Shell) hostChase() {
	if err := s.net.StartHosting(""); err != nil {
		s.notice = err.Error()
		s.goTitle()
		return
	}
	s.notice = ""
	s.openPage(pageLobby)
}

// joinChase connects to the address typed on the Play page and waits in the
// lobby for the host's world, as hostChase does.
func (s *Shell) joinChase() {
	if err := s.net.StartJoining(strings.TrimSpace(s.joinAddr)); err != nil {
		s.notice = err.Error()
		s.goTitle()
		return
	}
	s.notice = ""
	s.openPage(pageLobby)
}

// startChallenge begins a scenario as a single-player drive with the autopilot
// off, on the world challengeConfig builds.
func (s *Shell) startChallenge(sc scenario.Scenario) {
	s.challenge = &sc
	s.startGame(false)
}

// retryChallenge rebuilds the world from the same config and clears the run.
func (s *Shell) retryChallenge() {
	old := s.game.chal
	s.startGame(false)
	if s.game != nil && s.game.chal != nil && old != nil {
		old.retry()
		s.game.chal = old
	}
}

// startGame builds a session from the current settings and hands the window
// to it. A game already running is closed first. A session that cannot start
// goes back to the title with the reason.
func (s *Shell) startGame(autopilot bool) {
	if s.game != nil {
		s.game.Close()
		s.game = nil
	}
	o, err := s.sessionOptions(autopilot)
	if err != nil {
		s.net.Leave()
		s.notice = err.Error()
		s.goTitle()
		return
	}
	// What the Play page chose is remembered once a drive starts from it.
	s.settings.HostMode = s.hostMode
	if err := s.settings.Save(); err != nil {
		s.notice = err.Error()
	}

	s.feedFeats = featFeed{}
	s.closeMenuDrive() // a real drive replaces the one behind the menus: menu_drive.go
	s.pager = pager{}  // and nothing comes back to the page that started it
	s.game = New(o)
	s.game.net = s.net
	s.game.showLabels = s.settings.ShowLabels
	if s.game.Networked() {
		s.game.seat()
	}
	rl.SetMasterVolume(masterVolume(s.settings))
	s.screen = screenPlaying
}

// sessionOptions is what a drive started now is built from, or why it cannot
// start: a joiner builds the host's world, a host its own settings' (with the
// custom map, when Play chose it), and a drive alone its settings' with the
// custom map or the challenge Play chose.
func (s *Shell) sessionOptions(autopilot bool) (Options, error) {
	o := s.opts
	switch {
	case s.net.Role() == netplay.Joining:
		// The joiner builds what the host built, whatever its own settings say:
		// the snapshot names the world, and the lobby has already seen one.
		o = s.joinerOptions(o)
	case s.net.Online():
		// The host chooses the world and the mode, and the joiner is told them:
		// a custom map goes to it as its share code, in the snapshot.
		o = s.settings.applyTo(o).chaseReady()
		if !o.Given["play"] {
			o.Play = s.hostMode
		}
		if s.customPlay {
			o.Custom = customOf(s.settings.Custom)
		}
	default:
		o = s.settings.applyTo(o)
		if s.customPlay {
			o.Custom = customOf(s.settings.Custom)
		}
		o.Challenge = s.challenge
	}
	o.Autopilot = autopilot && !o.Play.SharedCar()
	// Known here and not in New, which builds the game before the session is
	// attached to it.
	o.Networked = s.net.Online()
	if err := o.coopValid(); err != nil {
		return o, err
	}
	o.Access = s.access()
	// Only a command line can ask for what the player does not hold or own;
	// the menus never offer it.
	return o, s.lockedFor(o)
}

// lockedFor is why the session cannot be started with what the player holds, or
// nil. A joiner in a shared car drives the host's car and wears the host's
// paint, which are the host's to own: it needs only the place's pack.
func (s *Shell) lockedFor(o Options) error {
	if s.net.Role() == netplay.Joining && o.Play.SharedCar() {
		return o.lockedMap(s.access())
	}
	return o.lockedFor(s.holdings())
}

// holdings is what the player has, in the shop's terms.
func (s *Shell) holdings() shop.Holdings { return s.career.holdings(s.access()) }

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
	if snap, ok := s.net.Snapshot(); ok {
		o = joinerWorld(o, snap)
	}
	return o
}

// joinerWorld puts the world a host's snapshot names into a joiner's options:
// the place, its size, the weather, the mode and any custom map, and the
// host's car when the two share one.
func joinerWorld(o Options, snap Snapshot) Options {
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

// endGame abandons the drive and returns to the title page, ending any
// network session with it.
func (s *Shell) endGame() {
	if s.game != nil {
		s.game.Close()
		s.game = nil
	}
	s.net.Leave()
	s.goTitle()
}

// text wraps a fixed string as a label, for the rows that do not change.
func text(s string) func() string { return func() string { return s } }
