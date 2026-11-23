package game

import (
	"fmt"
	"slices"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/keymap"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/scenario"
	"github.com/danielriddell21/autobahn/internal/weather"
)

// The Play page (docs/menus.md, "Play"): one screen for everything that starts
// a drive. A three-way switch along the top picks free drive, a challenge or
// multiplayer; a bar along the bottom has Back, a line saying what will start,
// the reason it cannot when it cannot, and the start button.
//
// This file is the model: which rows there are, what the cards say, why a
// start is refused, and which of the shell's own routes a start takes. Starting
// goes through startGame, startChallenge, hostChase and joinChase exactly as
// any drive does, so what banks and what is locked is decided where
// it always was. menu_play_draw.go draws it.

var _ = registerPage(pagePlay, func(s *Shell) *menuPage { return newPlayModel(s).page() }, hintsAdjust...)

// playMode is which of the three ways to start a drive the page shows. The
// settings keep the last one (Settings.PlayMode), so the page and the title's
// Play row open on it.
type playMode int

const (
	playFree playMode = iota
	playChallenge
	playMulti

	numPlayModes // not a mode
)

// playModeNames are the switch's words, in playMode order.
var playModeNames = []string{"Free drive", "Challenge", "Multiplayer"}

// playLedes say what each mode is, beside the switch.
var playLedes = []string{
	"where, who drives, and go",
	"beat the autopilot on the same road: banks nothing, F5 retries",
	"two players, two machines",
}

// playWeathers are the weather settings in the order the page offers them:
// drift first, then each one held.
func playWeathers() []weather.Setting {
	return []weather.Setting{
		weather.Drift, weather.Held(weather.Clear), weather.Held(weather.Rain),
		weather.Held(weather.Fog), weather.Held(weather.Storm),
	}
}

// playPage is the Play page's state. What a drive is started with lives in
// the shell's settings (the mode, the challenge, the map, weather, start time,
// autopilot style, car) and its hostMode, joining and joinAddr; the page keeps
// only what is chosen here and nowhere else.
type playPage struct {
	s    *Shell
	list *rowList
	mode playMode
	// card is the chosen map card: an index into city.Maps(), or playCustomCard.
	// A locked map can be chosen, to see what it would take; it cannot start.
	card int
	// auto is whether the autopilot drives a free drive.
	auto bool
	// chal is the chosen challenge, an index into scenarios.
	chal int

	scenarios []scenario.Scenario
	ghosts    map[string]scenario.Ghost

	// The routes a start takes: the shell's own, replaced only by tests, which
	// have no window to start a drive in.
	startGame      func(autopilot bool)
	startChallenge func(scenario.Scenario)
	hostChase      func()
	joinChase      func()
	// typed is what was typed this frame, read from raylib; tests replace it.
	typed func() (chars []rune, backspace bool)
}

// playCustomCard is the index of the Custom card, after every preset map.
var playCustomCard = len(city.Maps())

// newPlayModel makes the page's state, on the mode, challenge and map the
// settings hold and with the start button selected, so enter starts what the
// summary says.
func newPlayModel(s *Shell) *playPage {
	p := &playPage{
		s:              s,
		mode:           s.settings.PlayMode,
		card:           max(slices.Index(city.Maps(), s.settings.Map), 0),
		scenarios:      scenario.All(),
		ghosts:         scenario.Ghosts(),
		startGame:      s.startGame,
		startChallenge: s.startChallenge,
		hostChase:      s.hostChase,
		joinChase:      s.joinChase,
		typed:          playTyped,
	}
	p.chal = max(slices.IndexFunc(p.scenarios, func(sc scenario.Scenario) bool {
		return sc.ID == s.settings.Challenge
	}), 0)
	p.list = newRowList(p.rows()...)
	p.list.Select("go")
	return p
}

// scenarioByID is the challenge with the given ID.
func scenarioByID(id string) (scenario.Scenario, bool) {
	for _, sc := range scenario.All() {
		if sc.ID == id {
			return sc, true
		}
	}
	return scenario.Scenario{}, false
}

// page is the registry's view of the page.
func (p *playPage) page() *menuPage {
	return &menuPage{
		Title: "PLAY",
		List:  p.list,
		Step:  p.step,
		Draw:  p.draw,
		Back:  p.back,
		Shown: p.refresh,
		Hints: p.hints,
	}
}

// refresh rebuilds the rows from the state, keeping the selection. The rows
// depend on the mode, the map and who drives, so it runs after every key.
func (p *playPage) refresh() { p.list.SetRows(p.rows()) }

// rows is the page's rows in the order ↑↓ walks them, which is the order they
// are drawn in: the switch, the mode's own rows, then Back and the start button.
func (p *playPage) rows() []menuRow {
	rows := []menuRow{optionsRow("mode", "Mode", playModeNames,
		func() int { return int(p.mode) }, func(i int) {
			p.mode = playMode(i)
			p.s.settings.PlayMode = p.mode
		})}
	switch p.mode {
	case playChallenge:
		rows = append(rows, p.chalRow(), p.carRow())
	case playMulti:
		rows = append(rows, p.multiRows()...)
	default:
		rows = append(rows, p.freeRows()...)
	}
	return append(rows,
		actionRow("back", "Back", p.back),
		actionRow("go", p.goLabel(), p.start).disabledBy(p.why))
}

// freeRows are free drive's: the map, then how it is driven.
func (p *playPage) freeRows() []menuRow {
	rows := []menuRow{
		p.mapRow(),
		optionsRow("who", "Who drives", []string{"You", "Autopilot"},
			func() int { return playBool(p.auto) }, func(i int) { p.auto = i == 1 }),
	}
	set := &p.s.settings
	if p.auto {
		rows = append(rows, optionsRow("style", "Autopilot style", playStyles(),
			func() int { return int(set.Mode) }, func(i int) { set.Mode = autopilot.Mode(i) }))
	}
	ws := playWeathers()
	rows = append(rows,
		optionsRow("weather", "Weather", playWeatherNames(ws),
			func() int { return max(slices.Index(ws, set.Weather), 0) },
			func(i int) { set.Weather = ws[i] }).
			disabledBy(func() string {
				if p.card == playCustomCard {
					return "the custom map has its own weather: edit the map to change it"
				}
				return ""
			}),
		optionsRow("time", "Start time", playTimes(),
			func() int { return int(set.Time) }, func(i int) { set.Time = TimeOfDay(i) }),
		p.carRow())
	if p.card == playCustomCard {
		rows = append(rows, actionRow("edit", "Edit the custom map", p.editCustom).
			noted(func() string { return p.customCode() }))
	}
	return rows
}

// multiRows are multiplayer's: host or join, then that panel's rows.
func (p *playPage) multiRows() []menuRow {
	rows := []menuRow{optionsRow("role", "Host or join", []string{"Host a game", "Join a game"},
		func() int { return playBool(p.s.joining) }, func(i int) { p.s.joining = i == 1 })}
	if p.s.joining {
		addr := actionRow("addr", "Host address", p.start).
			noted(func() string { return p.s.joinAddr })
		return append(rows, addr, p.carRow())
	}
	modes := make([]string, numNetModes)
	for m := range numNetModes {
		modes[m] = m.String()
	}
	return append(rows,
		optionsRow("game", "Game", modes,
			func() int { return int(p.s.hostMode) }, func(i int) { p.s.hostMode = NetMode(i) }),
		p.mapRow(),
		p.carRow())
}

// mapRow chooses among the five cards. Enter takes the selection to the start
// button, the map being chosen.
func (p *playPage) mapRow() menuRow {
	names := make([]string, playCustomCard+1)
	for i := range names {
		names[i] = p.cardTitle(i)
	}
	r := optionsRow("map", "Map", names, func() int { return p.card }, p.pickCard)
	r.Activate = func() { p.list.Select("go") }
	return r
}

// chalRow chooses a challenge, and enter takes the selection to the start.
func (p *playPage) chalRow() menuRow {
	names := make([]string, len(p.scenarios))
	for i, sc := range p.scenarios {
		names[i] = sc.Name
	}
	r := optionsRow("chal", "Challenge", names, func() int { return p.chal }, func(i int) {
		p.chal = i
		p.s.settings.Challenge = p.scenarios[i].ID
	})
	r.Activate = func() { p.list.Select("go") }
	return r
}

// carRow opens the garage, and says what the car is.
func (p *playPage) carRow() menuRow {
	return actionRow("car", "Car", func() { p.s.openPage(pageGarage) }).
		noted(func() string {
			return p.s.settings.Car.Name() + ", " + paintOf(p.s.settings.Paint).name
		})
}

// pickCard chooses a map card. A preset the player holds becomes the map in
// the settings; a locked one is only looked at, so nothing that drives the
// settings' map is ever handed one.
func (p *playPage) pickCard(i int) {
	p.card = i
	if maps := city.Maps(); i < len(maps) && p.s.access().HasMap(maps[i]) {
		p.s.settings.Map = maps[i]
	}
}

// goLabel is what the start button says.
func (p *playPage) goLabel() string {
	switch {
	case p.mode == playChallenge:
		return "Start the challenge"
	case p.mode == playMulti && p.s.joining:
		return "Connect"
	case p.mode == playMulti:
		return "Start hosting"
	case p.auto:
		return "Watch the autopilot"
	default:
		return "Drive"
	}
}

// why is the reason the start button cannot be used, or "".
func (p *playPage) why() string {
	switch {
	case p.mode == playChallenge:
		if sc, ok := p.scenario(); ok {
			if why := p.mapLock(sc.CityMap()); why != "" {
				return why
			}
		}
	case p.mode == playMulti && p.s.joining:
		if strings.TrimSpace(p.s.joinAddr) == "" {
			return "type the host's address first"
		}
		return ""
	case p.card == playCustomCard:
		if err := p.customSpec().Locked(p.s.access()); err != nil {
			return "the custom map cannot start: " + err.Error()
		}
	default:
		if why := p.mapLock(city.Maps()[p.card]); why != "" {
			return why
		}
	}
	return p.carLock()
}

// mapLock is why a preset map cannot be driven, or "".
func (p *playPage) mapLock(m city.Map) string {
	a := p.s.access()
	if a.HasMap(m) {
		return ""
	}
	pk := pack.ForMap(m)
	return fmt.Sprintf("%s is in the %s pack: bank %s more bounty to open it",
		playMapTitle(m), pk.Name, playNumber(pk.Price-a.Banked))
}

// carLock is why the car or paint in the settings cannot be driven, or "".
// The settings are kept to what the player holds, so it is rarely anything.
func (p *playPage) carLock() string {
	h, set := p.s.holdings(), p.s.settings
	if !h.Drivable(set.Car) {
		return "the " + set.Car.Name() + " is not yours: choose another in the garage"
	}
	if !h.Wearable(set.Paint) {
		return "the " + paintOf(set.Paint).name + " paint is not yours: choose another in the garage"
	}
	return ""
}

// start starts what the summary says, through the shell's own routes. A start
// that cannot happen does nothing but say why, as the disabled button would.
func (p *playPage) start() {
	if why := p.why(); why != "" {
		p.list.Refused = why
		return
	}
	s := p.s
	switch p.mode {
	case playChallenge:
		sc, _ := p.scenario()
		s.customPlay = false
		p.startChallenge(sc)
	case playMulti:
		// A joiner builds the host's world; a host may host the custom map,
		// which goes to the joiner as its share code.
		s.challenge = nil
		s.customPlay = !s.joining && p.card == playCustomCard
		if s.customPlay {
			s.settings.Custom = p.customCode()
		}
		if s.joining {
			p.joinChase()
			return
		}
		p.hostChase()
	default:
		s.challenge = nil
		s.customPlay = p.card == playCustomCard
		if s.customPlay {
			s.settings.Custom = p.customCode()
		}
		p.startGame(p.auto)
	}
}

// back saves what was changed here and goes back. Settings are written when
// a screen closes, as everywhere else.
func (p *playPage) back() {
	if err := p.s.settings.Save(); err != nil {
		p.s.notice = err.Error()
	}
	p.s.backPage()
}

// editCustom opens the custom map screen on the map the Custom card shows.
func (p *playPage) editCustom() {
	p.s.settings.Custom = p.customCode()
	p.s.openPage(pageCustom)
}

// customSpec is the custom map the Custom card stands for: the one last
// edited, or, before there is one, the map the editor would open on.
func (p *playPage) customSpec() custom.Spec {
	if sp, err := custom.Decode(p.s.settings.Custom); err == nil {
		return sp
	}
	sp := custom.FromMap(p.s.settings.Map, 1)
	sp.Weather = p.s.settings.Weather
	return sp
}

// customCode is the share code of the Custom card's map.
func (p *playPage) customCode() string { return p.customSpec().Encode() }

// scenario is the chosen challenge.
func (p *playPage) scenario() (scenario.Scenario, bool) {
	if p.chal < 0 || p.chal >= len(p.scenarios) {
		return scenario.Scenario{}, false
	}
	return p.scenarios[p.chal], true
}

// step reads a frame's keys. The address field takes what is typed while it
// is selected; ←→ cross the bar between Back and the start button; the rest
// goes to the rows, and the rows are rebuilt after, so the frame drawn next
// already has the rows the keys asked for.
func (p *playPage) step(in navInput) bool {
	if p.list.SelectedKey() == "addr" && p.typeAddress() {
		return true
	}
	switch key := p.list.SelectedKey(); {
	case in.Back:
		p.back()
		return true
	case key == "back" && in.Right:
		p.list.Select("go")
	case key == "go" && in.Left:
		p.list.Select("back")
	default:
		p.list.Update(in.Input)
	}
	p.refresh()
	return true
}

// typeAddress applies what was typed to the address, reporting whether
// anything was. A frame with typing in it is used up by the typing: the keys
// that type letters also move the menu (w, a, s, d, space), and must not.
func (p *playPage) typeAddress() bool {
	chars, backspace := p.typed()
	if len(chars) == 0 && !backspace {
		return false
	}
	p.s.joinAddr = editJoinAddr(p.s.joinAddr, chars, backspace)
	p.list.Refused = ""
	return true
}

// maxJoinAddr is the longest address the field takes.
const maxJoinAddr = 64

// editJoinAddr is the address after a frame's typing: only what can be in a
// host and port is taken, up to maxJoinAddr, and backspace takes the last
// character off.
func editJoinAddr(addr string, chars []rune, backspace bool) string {
	for _, c := range chars {
		if len(addr) < maxJoinAddr && addressRune(c) {
			addr += string(c)
		}
	}
	if backspace && addr != "" {
		addr = addr[:len(addr)-1]
	}
	return addr
}

// playTyped reads the frame's typing from raylib.
func playTyped() (chars []rune, backspace bool) {
	for c := rl.GetCharPressed(); c != 0; c = rl.GetCharPressed() {
		chars = append(chars, c)
	}
	return chars, rl.IsKeyPressed(rl.KeyBackspace) || rl.IsKeyPressedRepeat(rl.KeyBackspace)
}

// hintsPlayTyping is the bar while the address field is selected.
var hintsPlayTyping = []keymap.Binding{
	{Key: "type", Action: "address"},
	{Key: "backspace", Action: "delete"},
	{Key: "enter", Action: "connect"},
	{Key: "up/down", Action: "choose"},
	{Key: "esc", Action: "back"},
}

// hints is the bar for what is selected.
func (p *playPage) hints() []keymap.Binding {
	if p.list.SelectedKey() == "addr" {
		return hintsPlayTyping
	}
	return hintsAdjust
}

// summary is the bar's one line saying what the start button will start.
func (p *playPage) summary() string {
	s := p.s
	switch {
	case p.mode == playChallenge:
		sc, ok := p.scenario()
		if !ok {
			return "no challenge"
		}
		return playJoin(sc.Name, playMapTitle(sc.CityMap()),
			fmt.Sprintf("ghost %d", p.ghosts[sc.ID].Score), "banks nothing")
	case p.mode == playMulti && p.s.joining:
		if addr := strings.TrimSpace(s.joinAddr); addr != "" {
			return playJoin("joining "+addr, s.settings.Car.Name())
		}
		return "joining: no address yet"
	case p.mode == playMulti && p.card == playCustomCard:
		return playJoin(s.hostMode.String()+" on "+p.cardTitle(p.card), s.settings.Car.Name(), "banks nothing")
	case p.mode == playMulti:
		return playJoin(s.hostMode.String()+" on "+p.cardTitle(p.card), s.settings.Car.Name())
	}
	who := "you drive"
	if p.auto {
		who = "autopilot (" + s.settings.Mode.String() + ")"
	}
	parts := []string{p.cardTitle(p.card), who}
	if p.card != playCustomCard {
		parts = append(parts, s.settings.Weather.String())
	}
	parts = append(parts, s.settings.Time.String(), s.settings.Car.Name())
	if !p.banks() {
		parts = append(parts, "banks nothing")
	}
	return playJoin(parts...)
}

// banks reports whether a free drive started now would bank: not with the
// autopilot at the wheel, and never on a custom map.
func (p *playPage) banks() bool { return !p.auto && p.card != playCustomCard }

// playCard is what one map card shows.
type playCard struct {
	Title, Season string
	// Note is the card's line under its name: the pack, how far there is to
	// go, or the share code. Note2 is a second line, for the custom map.
	Note, Note2 string
	Locked      bool
	// Progress is the bounty banked against the pack's price, for a locked map.
	Progress float64
	Chosen   bool
	// Flagged is a custom map outside what has been measured.
	Flagged bool
	Map     city.Map
	Custom  bool
}

// cards are the five map cards: every preset, then Custom.
func (p *playPage) cards() []playCard {
	a := p.s.access()
	out := make([]playCard, 0, playCustomCard+1)
	for i, m := range city.Maps() {
		pk := pack.ForMap(m)
		c := playCard{
			Title: p.cardTitle(i), Season: m.Season().Name(), Map: m,
			Chosen: i == p.card, Locked: !a.HasMap(m),
		}
		switch {
		case c.Locked:
			c.Note = playNumber(max(a.Banked, 0)) + " / " + playNumber(pk.Price) + " bounty"
			c.Progress = a.Progress(pk.ID)
		case pk.ID == pack.Base:
			c.Note = "base game"
		default:
			c.Note = pk.Name
		}
		out = append(out, c)
	}
	sp := p.customSpec()
	return append(out, playCard{
		Title: p.cardTitle(playCustomCard), Season: sp.Season().Name(),
		Note: sp.Encode(), Note2: "banks nothing", Custom: true, Map: sp.Base,
		Chosen: p.card == playCustomCard, Locked: sp.Locked(a) != nil, Flagged: sp.Flagged(),
	})
}

// cardTitle is a card's name: the map's, or Custom.
func (p *playPage) cardTitle(i int) string {
	if maps := city.Maps(); i >= 0 && i < len(maps) {
		return playMapTitle(maps[i])
	}
	return "Custom"
}

// playChalCard is what one challenge card shows.
type playChalCard struct {
	Map, Season, Name, Goal string
	Ghost, Best             string
	// Medal is the player's best medal, NoMedal if they have none.
	Medal  scenario.Medal
	Locked bool
	Chosen bool
}

// chalCards are the challenges as cards.
func (p *playPage) chalCards() []playChalCard {
	a := p.s.access()
	out := make([]playChalCard, len(p.scenarios))
	for i, sc := range p.scenarios {
		m := sc.CityMap()
		c := playChalCard{
			Map: playMapTitle(m), Season: m.Season().Name(), Name: sc.Name,
			Goal: sc.Brief(), Ghost: fmt.Sprintf("ghost %d", p.ghosts[sc.ID].Score), Best: "best -",
			Locked: !a.HasMap(m), Chosen: i == p.chal,
		}
		if b, ok := p.s.career.Challenges[sc.ID]; ok {
			c.Best, c.Medal = fmt.Sprintf("best %d %s", b.Score, b.Medal), b.Medal
		}
		out[i] = c
	}
	return out
}

// playMapTitle is a map's name as a card shows it: "Seatown".
func playMapTitle(m city.Map) string {
	n := m.Name()
	return strings.ToUpper(n[:1]) + n[1:]
}

// playNumber writes a whole number with commas, in ASCII for the default font.
func playNumber(v int) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	s := fmt.Sprint(v)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return sign + s
}

// playJoin puts a summary's parts on one line.
func playJoin(parts ...string) string { return strings.Join(parts, "  -  ") }

// playBool is 1 for true, for a two-way options row.
func playBool(b bool) int {
	if b {
		return 1
	}
	return 0
}

// playStyles names the autopilot's modes, in order.
func playStyles() []string {
	out := make([]string, autopilot.NumModes)
	for m := range autopilot.NumModes {
		out[m] = m.String()
	}
	return out
}

// playTimes names the times of day, in order.
func playTimes() []string {
	out := make([]string, NumTimes)
	for t := range NumTimes {
		out[t] = t.String()
	}
	return out
}

// playWeatherNames names weather settings.
func playWeatherNames(ws []weather.Setting) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.String()
	}
	return out
}
