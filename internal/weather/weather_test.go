package weather

import (
	"encoding/json"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
)

func TestTheSameTimeIsTheSameWeatherWhateverWasAskedBefore(t *testing.T) {
	times := []float32{0, 1000, 37, 5000.5, 12, 900, 37, 5000.5}
	first := map[float32]State{}
	for _, tm := range times {
		got := At(7, city.Autumn, Drift, tm)
		if want, seen := first[tm]; seen && got != want {
			t.Fatalf("at %.1f s the weather was %+v and then %+v", tm, want, got)
		}
		first[tm] = got
	}
	// And in the other order.
	for i := len(times) - 1; i >= 0; i-- {
		if got := At(7, city.Autumn, Drift, times[i]); got != first[times[i]] {
			t.Fatalf("asked backwards, at %.1f s the weather differs", times[i])
		}
	}
}

func TestTheWeatherNeverJumps(t *testing.T) {
	const step = 0.5
	for _, s := range city.Seasons() {
		for seed := uint64(1); seed <= 5; seed++ {
			prev := At(seed, s, Drift, 0)
			for tm := float32(step); tm < 4000; tm += step {
				st := At(seed, s, Drift, tm)
				// Sight is judged by how much air takes away, which is what is
				// smooth; the distance itself can run quickly out of clear.
				dExt := 1/st.Sight - 1/prev.Sight
				if dExt < 0 {
					dExt = -dExt
				}
				if dExt > 0.0005 {
					t.Fatalf("seed %d %s: sight went from %.1f to %.1f at %.1f s", seed, s.Name(), prev.Sight, st.Sight, tm)
				}
				if d := st.Grip - prev.Grip; d > 0.01 || d < -0.01 {
					t.Fatalf("seed %d %s: grip went from %.3f to %.3f at %.1f s", seed, s.Name(), prev.Grip, st.Grip, tm)
				}
				if d := st.Wet - prev.Wet; d > 0.02 || d < -0.02 {
					t.Fatalf("seed %d %s: wet jumped at %.1f s", seed, s.Name(), tm)
				}
				prev = st
			}
		}
	}
}

func TestEveryKindIsPossibleInEverySeasonAndSleetIsWinters(t *testing.T) {
	for _, s := range city.Seasons() {
		for _, c := range Conds() {
			if !Possible(s, c) {
				t.Errorf("%s cannot have %s", s.Name(), c.Name())
			}
			if got := At(1, s, Held(c), 100).Sleet; got != (s == city.Winter) {
				t.Errorf("%s %s: sleet is %v", s.Name(), c.Name(), got)
			}
		}
	}
	if Lightning(city.Winter) || !Lightning(city.Summer) {
		t.Error("lightning belongs to the warm seasons only")
	}
}

func TestAHeldSettingNeverChanges(t *testing.T) {
	for _, c := range Conds() {
		want := At(3, city.Spring, Held(c), 0)
		for _, tm := range []float32{1, 500, 12345, 99999} {
			if got := At(3, city.Spring, Held(c), tm); got != want {
				t.Fatalf("%s held changed at %.0f s", c.Name(), tm)
			}
		}
		if want.Cond != c {
			t.Errorf("held %s reads as %s", c.Name(), want.Cond.Name())
		}
	}
	if st := At(3, city.Spring, Held(Clear), 10); st.Sight != Unlimited || st.Grip != 1 || st.Wet != 0 {
		t.Errorf("held clear is not clear: %+v", st)
	}
	if st := At(3, city.Spring, Held(Fog), 10); st.Sight != 45 || st.Grip != 1 {
		t.Errorf("held fog gives sight %.1f, grip %.2f", st.Sight, st.Grip)
	}
	if st := At(3, city.Spring, Held(Rain), 10); st.Sight != 75 || st.Grip != 0.85 {
		t.Errorf("held rain gives sight %.1f, grip %.2f", st.Sight, st.Grip)
	}
	if st := At(3, city.Spring, Held(Storm), 10); st.Sight != 55 || st.Grip != 0.75 {
		t.Errorf("held storm gives sight %.1f, grip %.2f", st.Sight, st.Grip)
	}
}

func TestTheMixOfWeatherFollowsTheSeason(t *testing.T) {
	share := map[city.Season]map[Cond]int{}
	for _, s := range city.Seasons() {
		share[s] = map[Cond]int{}
		for seed := uint64(0); seed < 1000; seed++ {
			share[s][condOf(seed, s, 0)]++
		}
		for _, c := range Conds() {
			if share[s][c] == 0 {
				t.Errorf("%s never has %s in 1000 seeds", s.Name(), c.Name())
			}
		}
		w := weights(s)
		for _, c := range Conds() {
			if d := float32(share[s][c])/1000 - w[c]; d > 0.06 || d < -0.06 {
				t.Errorf("%s: %s came up %d times in 1000, want about %.0f", s.Name(), c.Name(), share[s][c], w[c]*1000)
			}
		}
	}
	if share[city.Summer][Storm] <= share[city.Autumn][Storm] {
		t.Error("summer should have more storms than autumn")
	}
	if share[city.Autumn][Fog] <= share[city.Summer][Fog] {
		t.Error("autumn should have more fog than summer")
	}
}

func TestSpellsLastFourToEightMinutes(t *testing.T) {
	for i := range 500 {
		if l := spellLength(9, i); l < 240 || l > 480 {
			t.Fatalf("spell %d lasts %.0f s", i, l)
		}
	}
}

func TestSettingsRoundTripAsText(t *testing.T) {
	for _, s := range append([]Setting{Drift}, Held(Clear), Held(Rain), Held(Fog), Held(Storm)) {
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var back Setting
		if err := json.Unmarshal(b, &back); err != nil || back != s {
			t.Errorf("%v came back as %v (%v)", s, back, err)
		}
	}
	var zero Setting
	if zero != Held(Clear) {
		t.Error("the zero setting should be clear")
	}
	if _, ok := ParseSetting("hail"); ok {
		t.Error("hail is not a setting")
	}
	if got, ok := ParseSetting("  Fog "); !ok || got != Held(Fog) {
		t.Error("a setting's name is read forgivingly")
	}
}

func TestStrikesComeOnlyInStormsAndNeverInWinter(t *testing.T) {
	for tm := float32(0); tm < 3000; tm += 3 {
		if _, ok := Strike(4, city.Winter, Held(Storm), tm); ok {
			t.Fatal("lightning in winter")
		}
		if _, ok := Strike(4, city.Summer, Held(Clear), tm); ok {
			t.Fatal("lightning in clear weather")
		}
	}
	n := 0
	for tm := float32(0); tm < 3000; tm += 3 {
		f, ok := Strike(4, city.Summer, Held(Storm), tm)
		if !ok {
			continue
		}
		n++
		if f.At > tm || f.Delay < 0.5 || f.Delay > 6 || f.Strength <= 0 || f.Strength > 1 {
			t.Fatalf("a strike %+v at %.0f s", f, tm)
		}
		if g, _ := Strike(4, city.Summer, Held(Storm), tm); g != f {
			t.Fatal("a strike changed when asked again")
		}
	}
	if n == 0 {
		t.Error("a storm that never strikes")
	}
}
