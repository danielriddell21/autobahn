package game

import (
	"strings"
	"testing"

	"github.com/danielriddell21/crucible/menu"
	"github.com/danielriddell21/crucible/netplay"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/scenario"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/vision"
)

// pauseGame is a drive with a world and no window.
func pauseGame(o Options) *Game {
	return &Game{
		opts:   o,
		world:  sim.NewWorld(sim.Config{Seed: 4, Traffic: 0, Police: 0}),
		driver: autopilot.New(vision.Camera{}),
		net:    &netplay.Session[Snapshot, Input]{},
	}
}

func tileOf(t *testing.T, tiles []pauseTile, key string) pauseTile {
	t.Helper()
	for _, x := range tiles {
		if x.Key == key {
			return x
		}
	}
	t.Fatalf("no %s tile", key)
	return pauseTile{}
}

func TestPauseTilesFreeDrive(t *testing.T) {
	g := pauseGame(DefaultOptions())
	g.world.Time = 125
	g.world.Judge.Distance = 3218.688
	g.world.Judge.Faults, g.world.Judge.Points = 3, 5
	g.world.Wanted.Banked, g.world.Wanted.Bounty, g.world.Wanted.Level = 1200, 300, 2
	tiles := pauseTiles(g)
	if len(tiles) != 6 {
		t.Fatalf("%d tiles, want six", len(tiles))
	}
	want := map[string]string{"time": "2:05", "distance": "2.0 mi", "bounty": money(1200), "wanted": "2", "faults": "3", "top": "-"}
	for k, v := range want {
		if got := tileOf(t, tiles, k).Value; got != v {
			t.Errorf("%s tile = %q, want %q", k, got, v)
		}
	}
	if n := tileOf(t, tiles, "bounty").Note; !strings.Contains(n, money(300)) {
		t.Errorf("bounty note %q does not mention the live pursuit", n)
	}
	g.world.Player.Vel.X = 20
	g.notePeakSpeed()
	g.world.Player.Vel.X = 0
	if got := tileOf(t, pauseTiles(g), "top").Value; got != "45 mph" {
		t.Errorf("top speed = %q, want 45 mph", got)
	}
}

func TestPauseTilesChallengeShowsScoreAndGhost(t *testing.T) {
	sc := scenario.All()[0]
	g := pauseGame(DefaultOptions())
	g.chal = newChallenge(sc)
	g.chal.ghost = 900
	tile := tileOf(t, pauseTiles(g), "bounty")
	if tile.Label != "Score" || tile.Note != "ghost 900" {
		t.Errorf("challenge tile = %+v, want the score with the ghost's 900", tile)
	}
	if h := pauseHeadOf(g); !strings.Contains(h.Mode, sc.Name) {
		t.Errorf("mode %q does not name the challenge %q", h.Mode, sc.Name)
	}
}

func TestPauseBankingTagInEachDrive(t *testing.T) {
	free := pauseGame(DefaultOptions())
	h := pauseHeadOf(free)
	if !h.Banking || h.Tag != "banking" || h.Mode != "free drive" {
		t.Errorf("free drive head = %+v, want banking", h)
	}
	if h.Map != "city" || h.Season != "autumn" {
		t.Errorf("head %+v, want the city in autumn", h)
	}

	o := DefaultOptions()
	o.Custom = &custom.Spec{}
	cust := pauseGame(o)
	if st := bankingState(cust); st.Banking || !strings.Contains(st.Why, "custom") {
		t.Errorf("custom map state = %+v, want not banking because custom", st)
	}
	if h := pauseHeadOf(cust); h.Map != "custom map" || h.Banking {
		t.Errorf("custom head = %+v", h)
	}

	ch := pauseGame(DefaultOptions())
	ch.chal = newChallenge(scenario.All()[0])
	if st := bankingState(ch); st.Banking || !strings.Contains(st.Why, "challenge") {
		t.Errorf("challenge state = %+v, want not banking because of the challenge", st)
	}

	auto := pauseGame(DefaultOptions())
	auto.auto = true
	if st := bankingState(auto); st.Banking || !strings.Contains(st.Why, "autopilot") {
		t.Errorf("autopilot state = %+v", st)
	}
	if h := pauseHeadOf(auto); !strings.HasPrefix(h.Mode, "autopilot, ") {
		t.Errorf("autopilot mode %q", h.Mode)
	}

	touched := pauseGame(DefaultOptions())
	touched.career.assisted = true
	if bankingState(touched).Banking {
		t.Error("a pursuit the autopilot touched is banking")
	}
}

func TestPauseCardSaysWhyOrWhat(t *testing.T) {
	g := pauseGame(DefaultOptions())
	g.world.Wanted.Banked, g.world.Wanted.Bounty = 500, 200
	text := strings.Join(pauseCardOf(g).Lines, " ")
	if !strings.Contains(text, money(500)) || !strings.Contains(text, "Abandoning") || !strings.Contains(text, money(200)) {
		t.Errorf("banking card %q lacks the bounty or what abandoning does", text)
	}
	g.chal = newChallenge(scenario.All()[0])
	g.chal.ghost = 700
	if text := strings.Join(pauseCardOf(g).Lines, " "); !strings.Contains(text, "ghost 700") {
		t.Errorf("challenge card %q does not set you against the ghost", text)
	}
	g.chal = nil
	g.auto = true
	if c := pauseCardOf(g); c.Title != "Nothing is banking" {
		t.Errorf("autopilot card %q, want the reason nothing banks", c.Title)
	}
}

// pauseShell is a shell with a running game, paused on the page.
func pauseShell(t *testing.T, g *Game) *Shell {
	t.Helper()
	s := testShell(t)
	s.game = g
	s.screen = screenPlaying
	s.openPage(pagePause)
	return s
}

func pauseRow(t *testing.T, s *Shell, key string) menuRow {
	t.Helper()
	r, ok := s.pager.cur.List.Row(key)
	if !ok {
		t.Fatalf("no %s row", key)
	}
	return r
}

func TestPauseAutopilotRowStopsBanking(t *testing.T) {
	g := pauseGame(DefaultOptions())
	s := pauseShell(t, g)
	if !s.onPage() || s.screen != pagePause || !s.pager.cur.OverGame {
		t.Fatal("the pause page is not showing over the game")
	}
	if !bankingState(g).Banking {
		t.Fatal("a free drive is not banking to begin with")
	}
	pauseRow(t, s, "autopilot").Activate()
	if !g.Autopilot() || bankingState(g).Banking {
		t.Errorf("auto %v banking %v: the autopilot row should stop the drive banking", g.Autopilot(), bankingState(g).Banking)
	}
	if !strings.Contains(pauseRow(t, s, "autopilot").Desc, "stops banking") {
		t.Error("the autopilot row does not say it stops the drive banking")
	}
	pauseRow(t, s, "autopilot").Activate()
	if g.Autopilot() {
		t.Error("the row did not turn the autopilot off")
	}
}

func TestPauseAutopilotDisabledInChallenge(t *testing.T) {
	g := pauseGame(DefaultOptions())
	g.chal = newChallenge(scenario.All()[0])
	s := pauseShell(t, g)
	if pauseRow(t, s, "autopilot").why() == "" {
		t.Error("the autopilot is offered in a challenge")
	}
}

func TestPauseQuickSettingsApply(t *testing.T) {
	g := pauseGame(DefaultOptions())
	s := pauseShell(t, g)
	pauseRow(t, s, "viewpoint").Adjust(1)
	if g.CameraMode() != CameraBonnet || s.settings.Camera != CameraBonnet {
		t.Errorf("camera %v / %v, want bonnet", g.CameraMode(), s.settings.Camera)
	}
	pauseRow(t, s, "viewpoint").Adjust(-1)
	pauseRow(t, s, "viewpoint").Adjust(-1)
	if g.CameraMode() != CameraHigh {
		t.Errorf("camera %v, want it to wrap to overhead", g.CameraMode())
	}
	pauseRow(t, s, "aicamera").Adjust(-1)
	if g.showPanel || s.settings.ShowPanel {
		t.Error("the AI camera is still on")
	}
	pauseRow(t, s, "labels").Activate()
	if g.showLabels || s.settings.ShowLabels {
		t.Error("box labels are still on")
	}
	before := s.settings.Volume
	pauseRow(t, s, "volume").Adjust(-1)
	if s.settings.Volume >= before {
		t.Error("volume did not go down")
	}
	if v := pauseRow(t, s, "volume").view(false); v.Level != s.settings.Volume || !strings.HasSuffix(v.Value, "%") {
		t.Errorf("volume row shows %+v", v)
	}
}

func TestPauseRowsAreASCII(t *testing.T) {
	s := pauseShell(t, pauseGame(DefaultOptions()))
	for _, v := range s.pager.cur.List.Views() {
		for _, x := range []string{v.Key, v.Label, v.Desc, v.Value} {
			for _, r := range x {
				if r > 126 {
					t.Errorf("row %s has non-ASCII text %q", v.Key, x)
				}
			}
		}
	}
}

func TestPauseLeftColumnOrder(t *testing.T) {
	s := pauseShell(t, pauseGame(DefaultOptions()))
	var got []string
	for _, v := range s.pager.cur.List.Views()[:pauseLeftRows] {
		got = append(got, v.Key)
	}
	want := "resume restart autopilot garage settings abandon"
	if strings.Join(got, " ") != want {
		t.Errorf("left column %v, want %s", got, want)
	}
	if v := pauseRow(t, s, "abandon").view(false); v.Tone != toneDanger {
		t.Error("Abandon is not in the danger tone")
	}
}

func TestPauseEscResumesAndLinksOpenPages(t *testing.T) {
	g := pauseGame(DefaultOptions())
	s := pauseShell(t, g)
	s.stepPage(navInput{Back: true})
	if s.screen != screenPlaying {
		t.Errorf("esc went to screen %d, want the drive", s.screen)
	}

	s = pauseShell(t, g)
	pauseRow(t, s, "garage").Activate()
	if s.screen != pageGarage {
		t.Errorf("Garage opened screen %d", s.screen)
	}
	s.backPage()
	if s.screen != pagePause {
		t.Errorf("back from the garage went to %d, want the pause page", s.screen)
	}
	pauseRow(t, s, "settings").Activate()
	if s.screen != pageSettings {
		t.Errorf("All settings opened screen %d", s.screen)
	}

	s = pauseShell(t, g)
	s.pager.cur.List.Select("resume")
	s.stepPage(navInput{Input: menu.Input{Select: true}})
	if s.screen != screenPlaying {
		t.Error("Resume did not return to the drive")
	}
}

func TestPauseRefusesANetworkedGame(t *testing.T) {
	host := &netplay.Session[Snapshot, Input]{}
	if err := host.StartHosting("127.0.0.1:0"); err != nil {
		t.Skipf("no local port to host on: %v", err)
	}
	defer host.Leave()
	g := pauseGame(DefaultOptions())
	g.net = host
	if !g.Networked() || pauseOffered(g) {
		t.Fatal("a networked game is offered the pause page")
	}
	s := testShell(t)
	s.game = g
	s.screen = screenPlaying
	s.openPage(pagePause)
	for _, k := range []string{"resume", "restart", "autopilot", "abandon"} {
		if _, ok := s.pager.cur.List.Row(k); ok {
			t.Errorf("a networked game is offered %s", k)
		}
	}
	if s.pager.cur.OverGame {
		t.Error("the refusal is drawn over a game")
	}
}
