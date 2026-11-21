package sim_test

import (
	"math/rand/v2"
	"testing"

	"github.com/danielriddell21/autobahn/internal/sim"
)

func TestTraitsRiseFromCautiousToAggressive(t *testing.T) {
	ss := []sim.Style{sim.Cautious(), sim.Normal(), sim.Brisk(), sim.Aggressive()}
	for i := 1; i < len(ss); i++ {
		a, b := ss[i-1], ss[i]
		if !(a.Overtake < b.Overtake || b.Overtake == 1) {
			t.Errorf("%s overtake %v should be below %s %v", a.Name, a.Overtake, b.Name, b.Overtake)
		}
		if !(a.Patience > b.Patience) {
			t.Errorf("%s should be more patient than %s", a.Name, b.Name)
		}
		if !(a.KeepLeft > b.KeepLeft) {
			t.Errorf("%s should keep left more than %s", a.Name, b.Name)
		}
		if !(a.Courtesy > b.Courtesy) {
			t.Errorf("%s should be more courteous than %s", a.Name, b.Name)
		}
		if !(a.Temper < b.Temper) {
			t.Errorf("%s should be calmer than %s", a.Name, b.Name)
		}
		if !(a.LaneGap > b.LaneGap) {
			t.Errorf("%s should want more room to change lane than %s", a.Name, b.Name)
		}
	}
}

func TestOnlyTheBoldPassOnTheWrongSide(t *testing.T) {
	if sim.Cautious().PassesOncoming || sim.Normal().PassesOncoming {
		t.Error("cautious and normal drivers should not use the oncoming lane")
	}
	if !sim.Brisk().PassesOncoming || !sim.Aggressive().PassesOncoming {
		t.Error("brisk and aggressive drivers should pass on a single carriageway")
	}
	if sim.Normal().Undertakes || !sim.Aggressive().Undertakes {
		t.Error("only the aggressive driver should undertake")
	}
	if sim.Normal().RollingStop || !sim.Aggressive().RollingStop {
		t.Error("only the aggressive driver should roll a stop sign")
	}
}

func TestWantsPastScalesWithKeenness(t *testing.T) {
	const want float32 = 13
	// A car doing 80% of the speed wanted.
	lead := want * 0.8
	if sim.Cautious().WantsPast(want, lead) {
		t.Error("a cautious driver should not pass a car only a fifth slower")
	}
	if !sim.Aggressive().WantsPast(want, lead) {
		t.Error("an aggressive driver should pass a car a fifth slower")
	}
	// Something crawling is worth passing for anyone who ever passes.
	if !sim.Cautious().WantsPast(want, 2) {
		t.Error("even a cautious driver should want past a car at walking pace")
	}
	if (sim.Style{}).WantsPast(want, 0) {
		t.Error("a driver who never overtakes should not want to")
	}
}

func TestJitterKeepsTraitsInRange(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for range 500 {
		s := sim.RandomStyle(rng)
		for name, v := range map[string]float32{
			"overtake": s.Overtake, "keepleft": s.KeepLeft, "attention": s.Attention,
			"courtesy": s.Courtesy, "temper": s.Temper,
		} {
			if v < 0 || v > 1 {
				t.Fatalf("%s %s = %v, outside [0,1]", s.Name, name, v)
			}
		}
		if s.Reaction <= 0 || s.Patience <= 0 {
			t.Fatalf("%s reaction %v patience %v should be positive", s.Name, s.Reaction, s.Patience)
		}
	}
}
