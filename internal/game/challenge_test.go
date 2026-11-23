package game

import (
	"strings"
	"testing"

	"github.com/danielriddell21/autobahn/internal/scenario"
	"github.com/danielriddell21/autobahn/internal/sim"
)

func TestChallengeConfigIsAFunctionOfTheScenario(t *testing.T) {
	for _, sc := range scenario.All() {
		a, b := challengeConfig(sc), challengeConfig(sc)
		if a != b || a.Seed != sc.Seed || a.Map != sc.CityMap() || a.Traffic != sc.Rules.Traffic {
			t.Errorf("%s: config %+v", sc.ID, a)
		}
	}
}

func TestChallengeHelpedCannotPass(t *testing.T) {
	sc := scenario.All()[0]
	c := newChallenge(sc)
	w := sim.NewWorld(challengeConfig(sc))
	for i := 0; i < sc.Rules.Seconds*60+2 && !c.run.Done(); i++ {
		c.step(w, i == 5, 1.0/60)
	}
	o, ok := c.finished()
	if !ok || o.Passed || o.Medal != scenario.NoMedal {
		t.Fatalf("helped challenge: %+v %v", o, ok)
	}
	if _, again := c.finished(); again {
		t.Fatal("outcome reported twice")
	}
}

func TestChallengeRetry(t *testing.T) {
	sc := scenario.All()[0]
	c := newChallenge(sc)
	w := sim.NewWorld(challengeConfig(sc))
	c.step(w, true, 100)
	c.retry()
	if c.run.Done() || c.run.Helped() || c.run.Elapsed() != 0 {
		t.Fatal("retry kept state")
	}
	if len(c.readout()) == 0 || strings.Contains(strings.Join(c.readout(), " "), "helped") {
		t.Fatalf("readout %v", c.readout())
	}
}

func TestChallengeVerdict(t *testing.T) {
	title, note := verdict(scenario.Outcome{Passed: true, Medal: scenario.Silver, Score: 5, Ghost: 6})
	if title != "CHALLENGE: SILVER" || !strings.Contains(note, "5 against") {
		t.Fatal(title, note)
	}
	if title, _ := verdict(scenario.Outcome{Reason: "forfeit"}); title != "CHALLENGE FAILED" {
		t.Fatal(title)
	}
}

func TestAmbulancesGoAboutOrdinaryDrivesOnly(t *testing.T) {
	sc := scenario.All()[0]
	for name, c := range map[string]struct {
		o    Options
		want int
	}{
		"normal":    {Options{Seed: 5}, ambulancesInTraffic},
		"autopilot": {Options{Seed: 5, Autopilot: true}, ambulancesInTraffic},
		"challenge": {Options{Seed: 5, Challenge: &sc}, 0},
		"networked": {Options{Seed: 5, Networked: true}, 0},
		"hosted":    {Options{Seed: 5, Host: ":1"}, 0},
		"joined":    {Options{Seed: 5, Join: "x:1"}, 0},
	} {
		if got := worldConfig(c.o).Ambulances; got != c.want {
			t.Errorf("%s: %d ambulances, want %d", name, got, c.want)
		}
	}
}
