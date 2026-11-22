package scenario

import (
	"reflect"
	"testing"
)

func TestFormatRoundTrip(t *testing.T) {
	data, err := Encode(All())
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, All()) {
		t.Fatal("scenarios changed in a round trip")
	}
}

func TestDecodeRefusesBad(t *testing.T) {
	for name, in := range map[string]string{
		"junk":      `{`,
		"map":       `{"scenarios":[{"id":"a","name":"A","map":"mars","goal":"score","rules":{"seconds":5,"works":"normal","weather":"clear"}}]}`,
		"goal":      `{"scenarios":[{"id":"a","name":"A","map":"city","goal":"win","rules":{"seconds":5,"works":"normal","weather":"clear"}}]}`,
		"seconds":   `{"scenarios":[{"id":"a","name":"A","map":"city","goal":"score","rules":{"seconds":0,"works":"normal","weather":"clear"}}]}`,
		"weather":   `{"scenarios":[{"id":"a","name":"A","map":"city","goal":"score","rules":{"seconds":5,"works":"normal","weather":"hail"}}]}`,
		"duplicate": `{"scenarios":[` + one + `,` + one + `]}`,
	} {
		if _, err := Decode([]byte(in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

const one = `{"id":"a","name":"A","map":"city","goal":"score","rules":{"seconds":5,"works":"normal","weather":"clear"}}`

func TestFirstSetCoversEveryMapTwice(t *testing.T) {
	per := map[string]int{}
	ids := map[string]bool{}
	for _, s := range All() {
		if err := s.Validate(); err != nil {
			t.Fatal(err)
		}
		if ids[s.ID] {
			t.Fatalf("duplicate id %s", s.ID)
		}
		ids[s.ID] = true
		per[s.Map]++
	}
	for _, m := range []string{"city", "seatown", "village", "roadtown"} {
		if per[m] < 2 {
			t.Errorf("%s has %d scenarios", m, per[m])
		}
	}
}

func TestMedals(t *testing.T) {
	cases := []struct {
		score, ghost int
		want         Medal
	}{
		{1000, 1000, Gold},
		{1500, 1000, Gold},
		{800, 1000, Silver},
		{799, 1000, Bronze},
		{500, 1000, Bronze},
		{499, 1000, NoMedal},
		{0, 0, NoMedal},
		{100, 0, Gold}, // the floor keeps a poor ghost honest
	}
	for _, c := range cases {
		if got := MedalFor(c.score, c.ghost); got != c.want {
			t.Errorf("score %d vs ghost %d: %v, want %v", c.score, c.ghost, got, c.want)
		}
	}
}

func frames(r *Run, n int, dt float32, dist float32, points, faults int, helped bool) {
	for i := 1; i <= n; i++ {
		r.Step(Frame{Dt: dt, Distance: dist * float32(i) / float32(n), Points: points, Faults: faults, Helped: helped})
	}
}

func TestScoreGoal(t *testing.T) {
	r := NewRun(Scenario{ID: "x", Goal: GoalScore, Rules: Rules{Seconds: 10}})
	frames(r, 10, 1, 1000, 4, 2, false)
	if !r.Done() {
		t.Fatal("not done after the time")
	}
	if got := r.Score(); got != 1000-4*PointCost {
		t.Fatalf("score %d", got)
	}
	o := r.Outcome(900)
	if !o.Passed || o.Medal != Gold {
		t.Fatalf("outcome %+v", o)
	}
	if r.Outcome(1500).Medal != Bronze {
		t.Fatalf("900 against 1500 should be bronze: %+v", r.Outcome(1500))
	}
	if r.Outcome(5000).Passed {
		t.Fatal("short of bronze passed")
	}
}

func TestCleanGoalEndsAtFirstFault(t *testing.T) {
	r := NewRun(Scenario{ID: "x", Goal: GoalClean, Rules: Rules{Seconds: 60}})
	r.Step(Frame{Dt: 1, Distance: 100, Faults: 3}) // baseline: earlier faults do not count
	r.Step(Frame{Dt: 1, Distance: 300, Faults: 3})
	if r.Done() {
		t.Fatal("ended without a fault")
	}
	r.Step(Frame{Dt: 1, Distance: 400, Faults: 4, Points: 3})
	if !r.Done() || r.Score() != 300 {
		t.Fatalf("done %v score %d", r.Done(), r.Score())
	}
	r.Step(Frame{Dt: 1, Distance: 9000, Faults: 4})
	if r.Score() != 300 {
		t.Fatal("a finished run moved")
	}
}

func TestOutcomeNeedsFinish(t *testing.T) {
	r := NewRun(Scenario{Goal: GoalScore, Rules: Rules{Seconds: 10}})
	frames(r, 3, 1, 5000, 0, 0, false)
	if r.Outcome(100).Passed {
		t.Fatal("passed while running")
	}
}

func TestHelpedRunCannotPass(t *testing.T) {
	r := NewRun(Scenario{Goal: GoalScore, Rules: Rules{Seconds: 10}})
	frames(r, 4, 1, 100, 0, 0, false)
	r.Step(Frame{Dt: 1, Distance: 100, Helped: true})
	frames(r, 5, 1, 100, 0, 0, false) // helping stopped, which does not undo it
	r.Step(Frame{Dt: 1, Distance: 100})
	if !r.Done() || !r.Helped() {
		t.Fatal("run should be finished and helped")
	}
	o := r.Outcome(0)
	if o.Passed || o.Medal != NoMedal {
		t.Fatalf("helped run passed: %+v", o)
	}
	if NewBests().Record("x", o) {
		t.Fatal("a helped run set a best")
	}
}

func TestRetryResetsState(t *testing.T) {
	r := NewRun(Scenario{ID: "x", Goal: GoalClean, Rules: Rules{Seconds: 10}})
	r.Step(Frame{Dt: 1, Distance: 10, Helped: true})
	r.Step(Frame{Dt: 1, Distance: 50, Faults: 1, Points: 5})
	r.Retry()
	if r.Done() || r.Helped() || r.Score() != 0 || r.Elapsed() != 0 || r.Faults() != 0 || r.Points() != 0 {
		t.Fatalf("retry left state behind: %+v", r)
	}
	if r.Scenario().ID != "x" {
		t.Fatal("retry lost the scenario")
	}
	// The new baseline is the first frame of the new attempt.
	r.Step(Frame{Dt: 1, Distance: 700, Faults: 9})
	if r.Done() || r.Faults() != 0 {
		t.Fatal("old totals leaked into the retry")
	}
}

func NewBests() Bests { return Bests{} }

func TestBestsRecord(t *testing.T) {
	b := NewBests()
	if !b.Record("a", Outcome{Passed: true, Score: 100, Medal: Bronze}) {
		t.Fatal("first pass is a best")
	}
	if b.Record("a", Outcome{Passed: true, Score: 90, Medal: Bronze}) {
		t.Fatal("worse run replaced a best")
	}
	if !b.Record("a", Outcome{Passed: true, Score: 90, Medal: Silver}) {
		t.Fatal("better medal not recorded")
	}
	if got := b["a"]; got.Score != 100 || got.Medal != Silver {
		t.Fatalf("best %+v", got)
	}
	if b.Record("b", Outcome{Score: 999}) {
		t.Fatal("a failed run set a best")
	}
}

func TestSanitise(t *testing.T) {
	ghosts := Ghosts()
	if len(ghosts) == 0 {
		t.Skip("no ghosts checked in")
	}
	id := All()[0].ID
	g := ghosts[id].Score
	in := Bests{
		id:          {Score: g * 3, Medal: Gold},
		"nowhere":   {Score: 1, Medal: Gold},
		All()[1].ID: {Score: -5, Medal: Gold},
	}
	out := in.Sanitise()
	if len(out) != 1 || out[id].Medal != Gold {
		t.Fatalf("sanitised: %+v", out)
	}
	// A medal the score does not earn is cut back.
	in = Bests{id: {Score: MinFor(g, Bronze), Medal: Gold}}
	if got := in.Sanitise()[id].Medal; got != Bronze {
		t.Fatalf("medal not cut back: %v", got)
	}
	if Bests(nil).Sanitise() == nil {
		t.Fatal("nil result")
	}
}

// MinFor is the least score that earns a medal against a ghost.
func MinFor(ghost int, m Medal) int {
	b, s, g := Thresholds(ghost)
	return map[Medal]int{Bronze: b, Silver: s, Gold: g}[m]
}

func TestGhostFileRoundTrip(t *testing.T) {
	in := map[string]Ghost{"a": {Score: 5, Distance: 9, Points: 1, Faults: 2}}
	data, err := EncodeGhosts(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecodeGhosts(data)
	if err != nil || !reflect.DeepEqual(in, out) {
		t.Fatalf("%v %+v", err, out)
	}
}
