package game

import (
	"strings"
	"testing"

	"github.com/danielriddell21/crucible/netplay"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/scenario"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// bareGame is a game with a world and nothing drawn: enough for the helpers
// the shell and the HUD ask of it.
func bareGame(o Options) *Game {
	return &Game{
		opts: o, world: sim.NewWorld(sim.Config{Seed: 7, Traffic: 5, Police: 1}),
		net: &netplay.Session[Snapshot, Input]{},
	}
}

func TestACustomGameOrAChallengeBanksNothing(t *testing.T) {
	spec := custom.FromMap(city.MapCity, 3)
	if !bareGame(Options{}).banks() {
		t.Error("an ordinary game does not bank")
	}
	if bareGame(Options{Custom: &spec}).banks() {
		t.Error("a custom map banks")
	}
	g := bareGame(Options{})
	g.chal = newChallenge(scenario.All()[0])
	if g.banks() {
		t.Error("a challenge banks")
	}
}

func TestSeasonIsTheCustomMapsOwn(t *testing.T) {
	spec := custom.FromMap(city.MapCity, 3)
	spec.Step[custom.Season] = int(city.Winter) + 1
	if got := bareGame(Options{Custom: &spec}).season(); got != city.Winter {
		t.Errorf("custom season = %v, want winter", got)
	}
	if got, want := bareGame(Options{Map: city.MapVillage}).season(), city.MapVillage.Season(); got != want {
		t.Errorf("preset season = %v, want %v", got, want)
	}
}

func TestFeatCountersReadTheWorld(t *testing.T) {
	g := bareGame(Options{Map: city.MapVillage})
	g.world.Judge.Distance, g.world.Judge.Faults = 1234, 2
	g.world.Judge.RedsStopped, g.world.Judge.RedsRun = 3, 1
	g.world.Wanted.Escapes, g.world.Wanted.Roadblocks, g.world.Wanted.Level = 4, 5, 2
	c := g.featCounters()
	if c.Distance != 1234 || c.Faults != 2 || c.RedsStopped != 3 || c.RedsRun != 1 ||
		c.Escapes != 4 || c.Roadblocks != 5 || c.Level != 2 || c.Helped || c.Custom ||
		c.Map != int(city.MapVillage) || c.Pack != featPack(city.MapVillage) {
		t.Errorf("counters = %+v", c)
	}
	g.career.assisted = true
	if !g.featCounters().Helped {
		t.Error("a pursuit the autopilot touched is not helped")
	}
	spec := custom.FromMap(city.MapCity, 3)
	g.opts.Custom = &spec
	if !g.featCounters().Custom {
		t.Error("a custom map is not marked custom")
	}
}

func TestFeatFeedStartsAgainWhenTheJudgeIsCleared(t *testing.T) {
	var ff featFeed
	ff.fact(featCounters{Distance: 3000, Faults: 4, RedsStopped: 9}, 0)
	x, _ := ff.fact(featCounters{Distance: 120, Faults: 1, RedsStopped: 2}, 0)
	if x.Distance != 120 || x.Faults != 1 || x.RedsStopped != 2 {
		t.Errorf("a cleared judge lost what came after: %+v", x)
	}
}

func TestCollectBanksNothingOnACustomMap(t *testing.T) {
	inTempConfig(t)
	o := DefaultOptions()
	o.Mute = true
	sh := NewShell(o)
	spec := custom.FromMap(city.MapCity, 3)
	sh.game = bareGame(Options{Custom: &spec})
	sh.game.career.earned = 500
	sh.game.jobs.results = nil
	sh.collect() // establishes nothing; a custom map is skipped outright
	sh.game.world.Judge.Distance = 5000
	sh.collect()
	if sh.career.Banked != 0 || len(sh.career.Feats.Unlocked) != 0 || sh.career.Feats.Distance != 0 {
		t.Errorf("a custom map counted: %+v", sh.career)
	}
	if sh.game.career.earned != 0 {
		t.Error("the bounty was left to bank later")
	}
}

func TestCollectGivesAchievementsAndBounty(t *testing.T) {
	inTempConfig(t)
	o := DefaultOptions()
	o.Mute = true
	sh := NewShell(o)
	sh.game = bareGame(Options{})
	sh.collect() // the first look is a baseline
	sh.game.world.Judge.Distance = 2000
	sh.game.career.earned = 300
	sh.collect()
	if !sh.career.Feats.Has("first-mile") || sh.career.Banked != 300 {
		t.Errorf("career = banked %d, feats %v", sh.career.Banked, sh.career.Feats.Unlocked)
	}
	if sh.game.banner != "ACHIEVEMENT" {
		t.Errorf("banner = %q", sh.game.banner)
	}
}

func TestStepChallengeAnnouncesAndHandsTheOutcomeOver(t *testing.T) {
	sc := scenario.All()[0]
	g := bareGame(Options{})
	g.world = sim.NewWorld(challengeConfig(sc))
	g.chal = newChallenge(sc)
	if _, ok := g.TakeChallenge(); ok {
		t.Fatal("an outcome before the run ended")
	}
	g.stepChallenge(float32(sc.Rules.Seconds) + 10)
	if g.bannerFor <= 0 || !strings.HasPrefix(g.banner, "CHALLENGE") {
		t.Errorf("banner = %q", g.banner)
	}
	if _, ok := g.TakeChallenge(); !ok {
		t.Fatal("no outcome after the run ended")
	}
	if _, ok := g.TakeChallenge(); ok {
		t.Error("the outcome was handed over twice")
	}
}

func TestChallengesAreRecordedInTheCareer(t *testing.T) {
	inTempConfig(t)
	sc := scenario.All()[0]
	gold := scenario.Ghosts()[sc.ID].Score * 2
	c := Career{Version: careerVersion, Challenges: scenario.Bests{sc.ID: {Score: gold, Medal: scenario.Gold}, "bogus": {Score: 5}}}
	c.sanitise()
	if _, ok := c.Challenges["bogus"]; ok {
		t.Error("an unknown scenario survived")
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if back := LoadCareer(); back.Challenges[sc.ID].Score == 0 {
		t.Errorf("the best was lost: %+v", back.Challenges)
	}
}

func TestLockedLooksAtTheCustomBaseAndTheScenarioMap(t *testing.T) {
	var none pack.Access
	spec := custom.FromMap(city.MapSeaside, 3)
	locked := Options{Custom: &spec}.locked(none) != nil
	if want := !none.HasMap(city.MapSeaside); locked != want {
		t.Errorf("custom base locked = %v, want %v", locked, want)
	}
	for _, sc := range scenario.All() {
		got := Options{Challenge: &sc}.locked(none) != nil
		if want := !none.HasMap(sc.CityMap()); got != want {
			t.Errorf("%s: locked = %v, want %v", sc.ID, got, want)
		}
	}
}

func TestTheSnapshotCarriesTheCustomMap(t *testing.T) {
	spec := custom.FromMap(city.MapVillage, 11)
	spec.Step[custom.Hills] = 3
	g := bareGame(Options{Custom: &spec})
	snap := g.snapshot()
	if snap.Proto != Proto || snap.Custom == "" {
		t.Fatalf("snapshot = proto %d, custom %q", snap.Proto, snap.Custom)
	}
	if got := customOf(snap.Custom); got == nil || *got != spec.Normalize() {
		t.Errorf("the joiner builds %+v, the host has %+v", got, spec)
	}
	if bareGame(Options{}).snapshot().Custom != "" || customOf("") != nil {
		t.Error("a preset claims a custom map")
	}
}

func TestHostModeIsRemembered(t *testing.T) {
	inTempConfig(t)
	set := DefaultSettings()
	set.HostMode = ModeCoopCar
	if err := set.Save(); err != nil {
		t.Fatal(err)
	}
	o := DefaultOptions()
	o.Mute = true
	if sh := NewShell(o); sh.hostMode != ModeCoopCar {
		t.Errorf("host mode = %v", sh.hostMode)
	}
	set.HostMode = 9
	set.clamp()
	if set.HostMode != ModeChase {
		t.Errorf("a bad host mode survived: %v", set.HostMode)
	}
}

func TestParseNetMode(t *testing.T) {
	for name, want := range map[string]NetMode{"chase": ModeChase, "coop": ModeCoopCars, "coop-car": ModeCoopCar} {
		if got, ok := ParseNetMode(name); !ok || got != want {
			t.Errorf("%q = %v, %v", name, got, ok)
		}
	}
	if _, ok := ParseNetMode("race"); ok {
		t.Error("an unknown mode parsed")
	}
}

func TestNextPresetNameSkipsTakenOnes(t *testing.T) {
	p := custom.Presets{"custom 1": "x", "custom 2": "y"}
	if got := nextPresetName(p); got != "custom 3" {
		t.Errorf("name = %q", got)
	}
}
