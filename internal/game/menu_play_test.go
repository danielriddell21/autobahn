package game

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/scenario"
	"github.com/danielriddell21/autobahn/internal/weather"
)

// playStarts is what the Play page asked the shell to start, with what the
// shell held at the moment it asked.
type playStarts struct {
	drives     []bool // startGame's autopilot argument, one per call
	customPlay []bool // s.customPlay when startGame was called
	challenges []string
	hosts      int
	joins      int
	// typing is what the next frame's typing will be.
	typing    []rune
	backspace bool
}

// openPlay opens the Play page on sh through the registry, as the title will,
// with the routes a start takes caught rather than run: there is no window to
// drive in. It hands back the page's state so a test can read it.
func openPlay(t *testing.T, sh *Shell) (*playPage, *playStarts) {
	t.Helper()
	got := &playStarts{}
	var p *playPage
	kept := pages[pagePlay]
	pages[pagePlay] = pageEntry{hints: kept.hints, build: func(s *Shell) *menuPage {
		p = newPlayModel(s)
		p.startGame = func(ap bool) {
			got.drives = append(got.drives, ap)
			got.customPlay = append(got.customPlay, s.customPlay)
		}
		p.startChallenge = func(sc scenario.Scenario) { got.challenges = append(got.challenges, sc.ID) }
		p.hostChase = func() { got.hosts++ }
		p.joinChase = func() { got.joins++ }
		p.typed = func() ([]rune, bool) {
			c, b := got.typing, got.backspace
			got.typing, got.backspace = nil, false
			return c, b
		}
		return p.page()
	}}
	t.Cleanup(func() { pages[pagePlay] = kept })
	sh.openPage(pagePlay)
	if p == nil || sh.screen != pagePlay {
		t.Fatalf("the Play page did not open (screen %v)", sh.screen)
	}
	return p, got
}

// playPress sends the page one frame of keys.
func playPress(sh *Shell, in navInput) { sh.stepPage(in) }

// keys lists the page's rows by key.
func playKeys(p *playPage) []string {
	var out []string
	for _, v := range p.list.Views() {
		out = append(out, v.Key)
	}
	return out
}

// goRow is the start button's view.
func goRow(t *testing.T, p *playPage) rowView {
	t.Helper()
	for _, v := range p.list.Views() {
		if v.Key == "go" {
			return v
		}
	}
	t.Fatal("no start button")
	return rowView{}
}

// choose selects the row with key and presses ←→ n times (negative for ←).
func choose(t *testing.T, sh *Shell, p *playPage, key string, n int) {
	t.Helper()
	if !p.list.Select(key) {
		t.Fatalf("no %q row in %v", key, playKeys(p))
	}
	in := navInput{Input: keyRight}
	if n < 0 {
		in, n = navInput{Input: keyLeft}, -n
	}
	for range n {
		playPress(sh, in)
	}
}

func TestPlayIsRegisteredAndOpens(t *testing.T) {
	sh := testShell(t)
	if _, stub := pageFor(pagePlay); stub {
		t.Fatal("the Play page is still a stub")
	}
	sh.openPage(pagePlay)
	if sh.screen != pagePlay || !sh.onPage() || sh.pager.cur.Title != "PLAY" {
		t.Fatalf("screen %v, page %+v", sh.screen, sh.pager.cur)
	}
	if k := sh.pager.cur.List.SelectedKey(); k != "go" {
		t.Errorf("the page opens on %q, want the start button", k)
	}
	if sh.pager.cur.Draw == nil {
		t.Error("the page has no layout of its own")
	}
	// esc goes back to where it was opened from.
	playPress(sh, navInput{Back: true})
	if sh.screen != pageTitle {
		t.Errorf("esc went to %v", sh.screen)
	}
}

func TestPlayModesSwitch(t *testing.T) {
	sh := testShell(t)
	p, _ := openPlay(t, sh)
	want := func(name string, keys ...string) {
		t.Helper()
		if got := playKeys(p); !slices.Equal(got, keys) {
			t.Errorf("%s: rows %v, want %v", name, got, keys)
		}
	}
	want("free drive", "mode", "map", "who", "weather", "time", "car", "back", "go")
	if goRow(t, p).Label != "Drive" {
		t.Errorf("free drive starts with %q", goRow(t, p).Label)
	}

	choose(t, sh, p, "who", 1)
	want("the autopilot driving", "mode", "map", "who", "style", "weather", "time", "car", "back", "go")
	if goRow(t, p).Label != "Watch the autopilot" {
		t.Errorf("the autopilot starts with %q", goRow(t, p).Label)
	}
	choose(t, sh, p, "map", -1)
	want("custom", "mode", "map", "who", "style", "weather", "time", "car", "edit", "back", "go")

	choose(t, sh, p, "mode", 1)
	if p.list.SelectedKey() != "mode" {
		t.Errorf("switching modes moved the selection to %q", p.list.SelectedKey())
	}
	want("challenge", "mode", "chal", "car", "back", "go")
	if goRow(t, p).Label != "Start the challenge" {
		t.Errorf("a challenge starts with %q", goRow(t, p).Label)
	}

	choose(t, sh, p, "mode", 1)
	want("host", "mode", "role", "game", "map", "car", "back", "go")
	if goRow(t, p).Label != "Start hosting" {
		t.Errorf("hosting starts with %q", goRow(t, p).Label)
	}
	choose(t, sh, p, "role", 1)
	want("join", "mode", "role", "addr", "car", "back", "go")
	if goRow(t, p).Label != "Connect" {
		t.Errorf("joining starts with %q", goRow(t, p).Label)
	}

	// The switch wraps round.
	choose(t, sh, p, "mode", 1)
	if p.mode != playFree {
		t.Errorf("mode %d after wrapping, want free drive", p.mode)
	}
}

func TestPlayLockedMapNeverStarts(t *testing.T) {
	sh := testShell(t)
	sh.career.Banked = 1200
	p, got := openPlay(t, sh)

	choose(t, sh, p, "map", 1) // seatown, locked
	if p.card != 1 || sh.settings.Map != city.MapCity {
		t.Fatalf("card %d and settings map %v, want seatown chosen and the city kept", p.card, sh.settings.Map)
	}
	g := goRow(t, p)
	if g.Disabled == "" || !strings.Contains(g.Disabled, "seaside escape") || !strings.Contains(g.Disabled, "1,800") {
		t.Errorf("the reason is %q, want the pack and what is left to bank", g.Disabled)
	}
	p.list.Select("go")
	playPress(sh, navInput{Input: keyEnter})
	if len(got.drives) != 0 || sh.screen != pagePlay {
		t.Fatalf("a locked map started (drives %v, screen %v)", got.drives, sh.screen)
	}
	if p.list.Reason() != g.Disabled {
		t.Errorf("the refusal says %q", p.list.Reason())
	}

	// Hosting a locked map is refused the same way.
	choose(t, sh, p, "mode", 2)
	p.list.Select("go")
	playPress(sh, navInput{Input: keyEnter})
	if got.hosts != 0 || goRow(t, p).Disabled == "" {
		t.Error("a locked map was hosted")
	}

	// So is a challenge on one.
	choose(t, sh, p, "mode", 2)
	for i, sc := range p.scenarios {
		if sc.CityMap() == city.MapSeaside {
			p.chal = i
			break
		}
	}
	p.refresh()
	p.list.Select("go")
	playPress(sh, navInput{Input: keyEnter})
	if len(got.challenges) != 0 || goRow(t, p).Disabled == "" {
		t.Error("a challenge on a locked map was started")
	}

	// Once it is banked, it starts, and is the map in the settings.
	sh.career.Banked = 3000
	choose(t, sh, p, "mode", 2)
	choose(t, sh, p, "map", 0)
	choose(t, sh, p, "map", -1)
	choose(t, sh, p, "map", 1)
	if sh.settings.Map != city.MapSeaside {
		t.Errorf("settings map %v once seatown is held", sh.settings.Map)
	}
	p.list.Select("go")
	playPress(sh, navInput{Input: keyEnter})
	if !slices.Equal(got.drives, []bool{false}) {
		t.Errorf("drives %v once the map is held", got.drives)
	}
}

func TestPlayCards(t *testing.T) {
	sh := testShell(t)
	sh.career.Banked = 1200
	p, _ := openPlay(t, sh)
	cards := p.cards()
	if len(cards) != 5 {
		t.Fatalf("%d cards, want the four maps and Custom", len(cards))
	}
	titles := make([]string, len(cards))
	for i, c := range cards {
		titles[i] = c.Title
		if strings.Contains(strings.ToLower(c.Title), c.Season) {
			t.Errorf("%q has its season in its name", c.Title)
		}
	}
	if !slices.Equal(titles, []string{"City", "Seatown", "Village", "Roadtown", "Custom"}) {
		t.Errorf("cards %v", titles)
	}
	city0, sea, cust := cards[0], cards[1], cards[4]
	if city0.Locked || city0.Note != "base game" || city0.Season != "autumn" || !city0.Chosen {
		t.Errorf("the city card is %+v", city0)
	}
	if !sea.Locked || sea.Note != "£1,200 / £3,000 bounty" || sea.Progress != 0.4 || sea.Season != "summer" {
		t.Errorf("the seatown card is %+v", sea)
	}
	if !cust.Custom || cust.Note2 != "banks nothing" || cust.Locked {
		t.Errorf("the custom card is %+v", cust)
	}
	if _, err := custom.Decode(cust.Note); err != nil {
		t.Errorf("the custom card's code %q does not decode: %v", cust.Note, err)
	}

	sh.career.Banked = 3000
	if sea := p.cards()[1]; sea.Locked || sea.Note != "seaside escape" {
		t.Errorf("a held seatown card is %+v", sea)
	}
}

func TestPlaySummary(t *testing.T) {
	sh := testShell(t)
	sh.settings.Weather, sh.settings.Time = weather.Held(weather.Rain), TimeDusk
	sh.settings.Mode = autopilot.ModeBrisk
	p, _ := openPlay(t, sh)
	car := sh.settings.Car.Name()

	if got, want := p.summary(), playJoin("City", "you drive", "rain", "dusk", car); got != want {
		t.Errorf("free drive: %q, want %q", got, want)
	}
	choose(t, sh, p, "who", 1)
	if got := p.summary(); !strings.Contains(got, "autopilot (brisk)") || !strings.HasSuffix(got, "banks nothing") {
		t.Errorf("with the autopilot: %q", got)
	}
	choose(t, sh, p, "who", 1)
	choose(t, sh, p, "map", -1)
	if got := p.summary(); !strings.HasPrefix(got, "Custom") || strings.Contains(got, "rain") ||
		!strings.HasSuffix(got, "banks nothing") {
		t.Errorf("on a custom map: %q", got)
	}

	choose(t, sh, p, "mode", 1)
	sc := p.scenarios[0]
	want := playJoin(sc.Name, "City", fmt.Sprintf("ghost %d", scenario.Ghosts()[sc.ID].Score), "banks nothing")
	if got := p.summary(); got != want {
		t.Errorf("challenge: %q, want %q", got, want)
	}

	choose(t, sh, p, "mode", 1)
	choose(t, sh, p, "map", 1) // back round from custom to the city
	choose(t, sh, p, "game", 1)
	if got, want := p.summary(), playJoin("Co-op: two cars on City", car); got != want {
		t.Errorf("hosting: %q, want %q", got, want)
	}
	choose(t, sh, p, "role", 1)
	sh.joinAddr = ""
	if got := p.summary(); got != "joining: no address yet" {
		t.Errorf("joining with no address: %q", got)
	}
	sh.joinAddr = "10.0.0.2"
	if got := p.summary(); !strings.HasPrefix(got, "joining 10.0.0.2") {
		t.Errorf("joining: %q", got)
	}
}

func TestPlayStartsThroughTheShell(t *testing.T) {
	sh := testShell(t)
	p, got := openPlay(t, sh)
	stale := scenario.All()[0]
	sh.challenge = &stale // left over from somewhere, as a start must not trust

	start := func() {
		t.Helper()
		p.list.Select("go")
		playPress(sh, navInput{Input: keyEnter})
	}
	start()
	if !slices.Equal(got.drives, []bool{false}) || got.customPlay[0] || sh.challenge != nil {
		t.Errorf("a drive: drives %v, custom %v, challenge %v", got.drives, got.customPlay, sh.challenge)
	}
	choose(t, sh, p, "who", 1)
	start()
	if !slices.Equal(got.drives, []bool{false, true}) {
		t.Errorf("watching the autopilot: drives %v", got.drives)
	}

	choose(t, sh, p, "map", -1)
	sh.settings.Custom = ""
	start()
	if len(got.customPlay) != 3 || !got.customPlay[2] {
		t.Errorf("a custom map was not started as one: %v", got.customPlay)
	}
	if _, err := custom.Decode(sh.settings.Custom); err != nil {
		t.Errorf("the custom map started has no code in the settings: %v", err)
	}

	choose(t, sh, p, "mode", 1)
	choose(t, sh, p, "chal", 1)
	start()
	if !slices.Equal(got.challenges, []string{p.scenarios[1].ID}) || sh.customPlay {
		t.Errorf("challenges %v, custom %v", got.challenges, sh.customPlay)
	}

	choose(t, sh, p, "mode", 1) // multiplayer, still on the custom map
	start()
	if got.hosts != 1 || !sh.customPlay {
		t.Errorf("hosted %d times, custom %v: want the custom map hosted", got.hosts, sh.customPlay)
	}
	choose(t, sh, p, "map", 1) // the city, from custom
	start()
	if got.hosts != 2 || sh.customPlay {
		t.Errorf("hosted %d times, custom %v: want the city hosted", got.hosts, sh.customPlay)
	}
	choose(t, sh, p, "map", -1)
	choose(t, sh, p, "role", 1)
	sh.joinAddr = "10.0.0.2"
	start()
	if got.joins != 1 || sh.customPlay {
		t.Errorf("joined %d times, custom %v: a joiner builds the host's world", got.joins, sh.customPlay)
	}
}

func TestPlayRemembersTheMode(t *testing.T) {
	sh := testShell(t)
	p, _ := openPlay(t, sh)
	if p.mode != playFree {
		t.Fatalf("a first run opens on %v, want free drive", p.mode)
	}
	choose(t, sh, p, "mode", 1)
	choose(t, sh, p, "chal", 2)
	p.list.Select("back")
	playPress(sh, navInput{Input: keyEnter})
	if sh.screen != pageTitle {
		t.Fatalf("Back went to %v", sh.screen)
	}
	stored := LoadSettings()
	want := p.scenarios[2]
	if stored.PlayMode != playChallenge || stored.Challenge != want.ID {
		t.Errorf("stored mode %v, challenge %q; want a challenge, %q", stored.PlayMode, stored.Challenge, want.ID)
	}
	r, _ := sh.pager.cur.List.Row(titlePlay)
	if got := r.view(false).Value; got != "challenge, "+want.Name {
		t.Errorf("the title's Play row says %q", got)
	}

	// The page opens again where it was left.
	p, _ = openPlay(t, sh)
	if p.mode != playChallenge || p.chal != 2 {
		t.Errorf("reopened on mode %v, challenge %d", p.mode, p.chal)
	}
	choose(t, sh, p, "mode", 1)
	r, _ = sh.pager.stack[0].pg.List.Row(titlePlay)
	if got := r.view(false).Value; got != "multiplayer, host chase" {
		t.Errorf("hosting: the title's Play row says %q", got)
	}
	choose(t, sh, p, "role", 1)
	sh.joinAddr = "10.0.0.2"
	if got := r.view(false).Value; got != "multiplayer, join 10.0.0.2" {
		t.Errorf("joining: the title's Play row says %q", got)
	}
}

func TestPlayModeIsSanitised(t *testing.T) {
	s := DefaultSettings()
	s.PlayMode, s.Challenge = 7, "no-such-challenge"
	s.clamp()
	if s.PlayMode != playFree || s.Challenge != "" {
		t.Errorf("clamp left mode %v, challenge %q", s.PlayMode, s.Challenge)
	}
	s.PlayMode, s.Challenge = playMulti, scenario.All()[1].ID
	s.clamp()
	if s.PlayMode != playMulti || s.Challenge != scenario.All()[1].ID {
		t.Errorf("clamp spoilt mode %v, challenge %q", s.PlayMode, s.Challenge)
	}
}

func TestPlayHostAndJoinValidation(t *testing.T) {
	sh := testShell(t)
	p, got := openPlay(t, sh)
	choose(t, sh, p, "mode", 2)

	// A custom map can be hosted: it goes to the joiner as its share code.
	choose(t, sh, p, "map", -1)
	if why := goRow(t, p).Disabled; why != "" {
		t.Errorf("hosting a custom map is refused: %q", why)
	}
	if got := p.summary(); !strings.Contains(got, "banks nothing") {
		t.Errorf("hosting a custom map does not say it banks nothing: %q", got)
	}
	choose(t, sh, p, "map", 1)
	if why := goRow(t, p).Disabled; why != "" {
		t.Errorf("hosting the city is refused: %q", why)
	}
	choose(t, sh, p, "game", 1)
	if sh.hostMode != ModeCoopCars {
		t.Errorf("host mode %v", sh.hostMode)
	}

	choose(t, sh, p, "role", 1)
	sh.joinAddr = ""
	if why := goRow(t, p).Disabled; why != "type the host's address first" {
		t.Errorf("an empty address: %q", why)
	}
	p.list.Select("addr")
	playPress(sh, navInput{Input: keyEnter})
	p.list.Select("go")
	playPress(sh, navInput{Input: keyEnter})
	if got.joins != 0 {
		t.Fatal("connected with no address")
	}

	// Typing goes into the field, by the old join screen's rules, and a
	// frame with typing in it moves nothing: s and d are keys that type.
	p.list.Select("addr")
	got.typing = []rune("10.0.0.2 x!")
	playPress(sh, navInput{Input: keyDown})
	if sh.joinAddr != "10.0.0.2x" || p.list.SelectedKey() != "addr" {
		t.Errorf("address %q, selection %q", sh.joinAddr, p.list.SelectedKey())
	}
	got.backspace = true
	playPress(sh, navInput{})
	if sh.joinAddr != "10.0.0.2" {
		t.Errorf("after backspace %q", sh.joinAddr)
	}
	if !slices.Equal(p.hints(), hintsPlayTyping) {
		t.Error("the bar does not say the field takes typing")
	}
	// Away from the field, typing is not read.
	p.list.Select("car")
	got.typing = []rune("9")
	playPress(sh, navInput{})
	if sh.joinAddr != "10.0.0.2" {
		t.Errorf("typing reached the field from elsewhere: %q", sh.joinAddr)
	}
	got.typing = nil

	p.list.Select("addr")
	playPress(sh, navInput{Input: keyEnter})
	if got.joins != 1 {
		t.Errorf("enter in the field joined %d times", got.joins)
	}
}

func TestEditJoinAddr(t *testing.T) {
	if got := editJoinAddr("", []rune("host-1.lan:7777 /"), false); got != "host-1.lan:7777" {
		t.Errorf("got %q", got)
	}
	if got := editJoinAddr("ab", nil, true); got != "a" {
		t.Errorf("backspace gave %q", got)
	}
	if got := editJoinAddr("", nil, true); got != "" {
		t.Errorf("backspace on nothing gave %q", got)
	}
	long := strings.Repeat("a", maxJoinAddr)
	if got := editJoinAddr(long, []rune("b"), false); got != long {
		t.Error("the field took more than its length")
	}
}

func TestPlayBarCrossesBetweenBackAndStart(t *testing.T) {
	sh := testShell(t)
	p, _ := openPlay(t, sh)
	p.list.Select("back")
	playPress(sh, navInput{Input: keyRight})
	if p.list.SelectedKey() != "go" {
		t.Errorf("right from Back went to %q", p.list.SelectedKey())
	}
	playPress(sh, navInput{Input: keyLeft})
	if p.list.SelectedKey() != "back" {
		t.Errorf("left from the start went to %q", p.list.SelectedKey())
	}
}

func TestPlayOpensTheGarageAndTheCustomMap(t *testing.T) {
	sh := testShell(t)
	p, _ := openPlay(t, sh)
	page := sh.pager.cur

	p.list.Select("car")
	playPress(sh, navInput{Input: keyEnter})
	if sh.screen != pageGarage {
		t.Fatalf("the car row opened %v", sh.screen)
	}
	sh.backPage()
	if sh.screen != pagePlay || sh.pager.cur != page {
		t.Errorf("back from the garage gave %v", sh.screen)
	}

	sh.settings.Custom = ""
	choose(t, sh, p, "map", -1)
	p.list.Select("edit")
	playPress(sh, navInput{Input: keyEnter})
	if sh.screen != pageCustom {
		t.Fatalf("the edit row opened %v", sh.screen)
	}
	if _, err := custom.Decode(sh.settings.Custom); err != nil {
		t.Errorf("the custom map screen was opened with no map: %v", err)
	}
}

func TestPlayBackSavesTheChoices(t *testing.T) {
	sh := testShell(t)
	p, _ := openPlay(t, sh)
	choose(t, sh, p, "weather", 2) // drift, clear, rain
	choose(t, sh, p, "time", 1)
	p.list.Select("back")
	playPress(sh, navInput{Input: keyEnter})
	if sh.screen != pageTitle {
		t.Errorf("Back went to %v", sh.screen)
	}
	got := LoadSettings()
	if got.Weather != weather.Held(weather.Rain) || got.Time != TimeDusk {
		t.Errorf("saved weather %v and time %v", got.Weather, got.Time)
	}

	// The custom map brings its own weather, so the row is shut there.
	p, _ = openPlay(t, sh)
	choose(t, sh, p, "map", -1)
	before := sh.settings.Weather
	choose(t, sh, p, "weather", 1)
	if sh.settings.Weather != before || !strings.Contains(p.list.Reason(), "its own weather") {
		t.Errorf("weather %v on a custom map, reason %q", sh.settings.Weather, p.list.Reason())
	}
}

func TestPlayChallengeCards(t *testing.T) {
	sh := testShell(t)
	sh.career.Challenges = scenario.Bests{"city-sprint": {Score: 900, Medal: scenario.Gold}}
	p, _ := openPlay(t, sh)
	cards := p.chalCards()
	if len(cards) != len(scenario.All()) {
		t.Fatalf("%d cards", len(cards))
	}
	first := cards[0]
	if first.Name != "City sprint" || first.Map != "City" || first.Season != "autumn" ||
		first.Best != "best 900 gold" || first.Medal != scenario.Gold || !first.Chosen || first.Locked {
		t.Errorf("the first card is %+v", first)
	}
	if want := fmt.Sprintf("ghost %d", scenario.Ghosts()["city-sprint"].Score); first.Ghost != want {
		t.Errorf("ghost %q, want %q", first.Ghost, want)
	}
	if cards[1].Best != "best -" || cards[1].Goal == "" {
		t.Errorf("a card with no best is %+v", cards[1])
	}
	if !cards[2].Locked {
		t.Error("a seatown challenge is open with nothing banked")
	}
}

// TestPlayDraws draws the page in every mode and state, at the default size
// and a small one, which catches a layout the model cannot satisfy.
func TestPlayDraws(t *testing.T) {
	inTempConfig(t)
	openTestWindow(t, 1280, 720)

	o := DefaultOptions()
	o.Mute = true
	sh := NewShell(o)
	defer sh.Close()
	sh.career.Banked = 1200
	sh.career.Challenges = scenario.Bests{"city-sprint": {Score: 900, Medal: scenario.Gold}}
	p, _ := openPlay(t, sh)

	frame := func() {
		for _, size := range [][2]int{{1280, 720}, {800, 450}} {
			sh.opts.Width, sh.opts.Height = size[0], size[1]
			sh.drawScreen(1.0 / 60)
		}
	}
	for _, k := range playKeys(p) {
		p.list.Select(k)
		frame()
	}
	choose(t, sh, p, "who", 1)
	choose(t, sh, p, "map", 1) // seatown, locked
	frame()
	choose(t, sh, p, "map", -2) // custom
	frame()
	for range 2 {
		choose(t, sh, p, "mode", 1)
		for _, k := range playKeys(p) {
			p.list.Select(k)
			frame()
		}
	}
	choose(t, sh, p, "role", 1)
	for _, addr := range []string{"", "192.168.1.20"} {
		sh.joinAddr = addr
		for _, k := range playKeys(p) {
			p.list.Select(k)
			frame()
		}
	}
}
