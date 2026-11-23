package game

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/feats"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/shop"
)

func TestOldCareerLoadsWithFeatsEmptyAndBountyIntact(t *testing.T) {
	inTempConfig(t)
	path, err := careerPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// A version 1 file, from before there were achievements.
	if err := os.WriteFile(path, []byte(`{"version":1,"banked":7777,"earnings":50,"cars":[0],"paints":[0]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c := LoadCareer()
	if c.Banked != 7777 || c.Earnings != 50 || c.Version != careerVersion {
		t.Errorf("old career changed: %+v", c)
	}
	if len(c.Feats.Unlocked) != 0 || c.Feats.Distance != 0 {
		t.Errorf("feats not empty: %+v", c.Feats)
	}
}

func dirOf(p string) string {
	i := strings.LastIndexAny(p, `/\`)
	return p[:i]
}

func TestFeatsSurviveSaveAndLoad(t *testing.T) {
	inTempConfig(t)
	c := Career{Version: careerVersion, Banked: 5}
	c.RecordFeats(feats.Fact{Distance: 2000, Escapes: 1})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	back := LoadCareer()
	if back.Banked != 5 || !back.Feats.Has("first-mile") || !back.Feats.Has("first-escape") || !back.Feats.Has("clean-2k") {
		t.Errorf("feats lost: %+v", back.Feats)
	}
}

func TestSanitiseDropsUnknownFeats(t *testing.T) {
	c := Career{Feats: feats.Feats{Unlocked: []string{"bogus", "first-mile"}, Maps: 1 << 30}}
	c.sanitise()
	if !slices.Equal(c.Feats.Unlocked, []string{"first-mile"}) || c.Feats.Maps != 0 {
		t.Errorf("feats = %+v", c.Feats)
	}
}

func TestSecretFeatUnlocksPaintAndPaysNothing(t *testing.T) {
	if !shop.Secret(feats.RacingGreen) {
		t.Fatal("the achievement paint must be one the shop never sells")
	}
	c := Career{Version: careerVersion}
	c.sanitise()
	if slices.Contains(c.Paints, feats.RacingGreen) {
		t.Fatal("racing green held from the start")
	}
	var facts []feats.Fact
	for i := range city.Maps() {
		facts = append(facts, feats.Fact{Distance: 10, Map: i, Pack: int(pack.ForMap(city.Map(i)).ID)})
	}
	for _, x := range facts {
		c.RecordFeats(x)
	}
	if !c.Feats.Has("all-maps") || !slices.Contains(c.Paints, feats.RacingGreen) {
		t.Errorf("paints = %v, feats = %v", c.Paints, c.Feats.Unlocked)
	}
	if c.Banked != 0 || c.Earnings != 0 {
		t.Errorf("an achievement paid: banked %d earnings %d", c.Banked, c.Earnings)
	}
}

func TestAssistedDriveEarnsNoFeat(t *testing.T) {
	c := Career{Version: careerVersion}
	if got := c.RecordFeats(feats.Fact{Distance: 5000, Escapes: 3, Helped: true}); len(got) != 0 {
		t.Errorf("autopilot earned %v", got)
	}
	if got := c.RecordFeats(feats.Fact{Distance: 5000, Custom: true}); len(got) != 0 {
		t.Errorf("custom map earned %v", got)
	}
	b, _ := json.Marshal(c.Feats)
	if string(b) != "{}" {
		t.Errorf("counted anyway: %s", b)
	}
}

func TestFeatFeedTakesDeltas(t *testing.T) {
	var ff featFeed
	c := featCounters{Distance: 900, Faults: 2, Escapes: 1}
	if _, ok := ff.fact(c, 0); ok {
		t.Fatal("first look must only take a baseline")
	}
	c.Distance, c.Faults, c.Escapes, c.Level = 1000, 3, 2, 4
	x, ok := ff.fact(c, 80)
	if !ok || x.Distance != 100 || x.Faults != 1 || x.Escapes != 1 || x.Banked != 80 || x.WantedPeak != 4 {
		t.Errorf("fact = %+v", x)
	}
	if x, _ := ff.fact(c, 0); x.Distance != 0 || x.Faults != 0 || x.Escapes != 0 {
		t.Errorf("nothing new, got %+v", x)
	}
}

func TestFeatPackOfTheCity(t *testing.T) {
	if featPack(city.MapCity) != int(pack.Base) {
		t.Error("the city is in the base game")
	}
}
