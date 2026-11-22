package race_test

import (
	"fmt"
	"slices"
	"sort"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/race"
)

func init() {
	for _, t := range race.All() {
		job.Register(t)
	}
}

func TestGateIsCrossedBothWaysAndOnlyOnce(t *testing.T) {
	g := race.Gate{Pos: mathx.V(10, 0), Heading: 0, Half: 4} // the street runs along +X
	for _, dir := range []float32{1, -1} {
		count := 0
		p := mathx.V(10-dir*20, 1)
		for range 400 {
			next := p.Add(mathx.V(dir*0.37, 0))
			if g.Crossed(p, next) {
				count++
			}
			p = next
		}
		if count != 1 {
			t.Errorf("direction %v: crossed %d times, want 1", dir, count)
		}
	}
	if g.Crossed(mathx.V(9, 6), mathx.V(11, 6)) {
		t.Error("a car beside the line was counted")
	}
	if g.Crossed(mathx.V(8, 0), mathx.V(9, 0)) {
		t.Error("a car short of the line was counted")
	}
	// Stopped on the line, then driving off: counted as it arrived, not again.
	if !g.Crossed(mathx.V(9, 0), mathx.V(10, 0)) || g.Crossed(mathx.V(10, 0), mathx.V(10.5, 0)) {
		t.Error("a car that ended on the line was not counted exactly once")
	}
}

func TestReached(t *testing.T) {
	if !race.Reached(mathx.V(-5, 0), mathx.V(5, 0), mathx.V(0, 2), 3) {
		t.Error("a move past the checkpoint was missed")
	}
	if race.Reached(mathx.V(-5, 0), mathx.V(5, 0), mathx.V(0, 9), 3) {
		t.Error("a move well clear of the checkpoint was counted")
	}
}

// fingerprint is a loop as the world sees it, with nothing in it that depends on
// the order the city was built in.
func fingerprint(l race.Loop) string {
	s := fmt.Sprintf("len %.1f limit %.2f corners %d", l.Length, l.Limit, l.Corners)
	for _, g := range l.Gates {
		s += fmt.Sprintf(" | %.2f,%.2f %.3f %.1f", g.Pos.X, g.Pos.Z, g.Heading, g.Half)
	}
	return s
}

func TestLoopIsTheSameWhateverOrderTheCityGrew(t *testing.T) {
	const seed = 7
	for _, m := range city.Maps() {
		t.Run(m.Name(), func(t *testing.T) {
			span := 5*city.ParamsFor(m, seed).MaxSpan + 200
			whole := city.NewGrowing(city.ParamsFor(m, seed))
			whole.EnsureAround(mathx.V(0, 0), span)
			grown := city.NewGrowing(city.ParamsFor(m, seed))
			for _, at := range []mathx.Vec{
				mathx.V(-span/2, -span/2), mathx.V(span/2, -span/2),
				mathx.V(-span/2, span/2), mathx.V(span/2, span/2), mathx.V(0, 0),
			} {
				grown.EnsureAround(at, span/2)
			}
			fresh := city.NewGrowing(city.ParamsFor(m, seed))

			for serial := 1; serial <= 3; serial++ {
				a, ok := race.PlanLoop(whole, seed, serial, mathx.V(0, 0))
				if !ok {
					t.Fatalf("serial %d: no loop on %s", serial, m.Name())
				}
				b, _ := race.PlanLoop(grown, seed, serial, mathx.V(0, 0))
				c, _ := race.PlanLoop(fresh, seed, serial, mathx.V(0, 0))
				if fingerprint(a) != fingerprint(b) || fingerprint(a) != fingerprint(c) {
					t.Errorf("serial %d: the loop depends on how the city grew:\n%s\n%s\n%s",
						serial, fingerprint(a), fingerprint(b), fingerprint(c))
				}
				if len(a.Gates) < 4 || a.Length < 700 || a.Length > 3600 {
					t.Errorf("serial %d: a loop of %d gates and %.0f m", serial, len(a.Gates), a.Length)
				}
			}
		})
	}
}

// Every gate of a loop is on a street, and every junction it passes is finished.
func TestLoopGatesAreOnCompleteStreets(t *testing.T) {
	for _, m := range city.Maps() {
		c := city.NewGrowing(city.ParamsFor(m, 3))
		l, ok := race.PlanLoop(c, 3, 1, mathx.V(0, 0))
		if !ok {
			t.Fatalf("%s: no loop", m.Name())
		}
		for i, g := range l.Gates {
			p := c.Project(g.Pos)
			if !p.Valid || !c.Complete(c.Nodes[p.Lane.FromNode]) || !c.Complete(c.Nodes[p.Lane.ToNode]) {
				t.Errorf("%s: gate %d is not on finished ground", m.Name(), i)
			}
		}
	}
}

// drive takes a car through each goal in turn at a speed, from a start, calling
// the board each frame, and returns the result if the job ended.
func drive(b *job.Board, from mathx.Vec, goals []job.Goal, speed, t0 float32) (job.Result, bool) {
	now, pos := t0, from
	for _, g := range goals {
		for pos.DistTo(g.Pos) > 0.5 {
			step := min(speed*0.05, pos.DistTo(g.Pos))
			pos = pos.Add(g.Pos.Sub(pos).Norm().Mul(step))
			now += 0.05
			if r, done := b.Step(job.Frame{Now: now, Pos: pos, Speed: speed}); done {
				return r, true
			}
		}
	}
	return job.Result{}, false
}

func TestTimeTrialPaysThroughTheBoard(t *testing.T) {
	c := city.NewGrowing(city.ParamsFor(city.MapCity, 5))
	var b job.Board
	s, err := b.Start(race.Trial, c, 5, 1, mathx.V(0, 0), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Goals) < 4 {
		t.Fatalf("only %d checkpoints", len(s.Goals))
	}
	// Gold: round in the par time, and the Ghost says what that is.
	par := race.Ghost(s)
	var length float32
	prev := mathx.V(0, 0)
	for _, g := range s.Goals {
		length += prev.DistTo(g.Pos)
		prev = g.Pos
	}
	speed := length / (par * 0.8)
	r, done := drive(&b, mathx.V(0, 0), s.Goals, speed, 0)
	if !done || r.Status != job.Passed || r.Reason != "gold" {
		t.Fatalf("result %+v done %v, want a gold pass", r, done)
	}
	if r.Pay <= 0 || r.Pay != int(par)*5 {
		t.Errorf("pay %d, want %d", r.Pay, int(par)*5)
	}
}

func TestAssistedRaceIsUnpaid(t *testing.T) {
	c := city.NewGrowing(city.ParamsFor(city.MapCity, 5))
	var b job.Board
	s, _ := b.Start(race.Trial, c, 5, 1, mathx.V(0, 0), 0)
	b.Step(job.Frame{Now: 0.01, Pos: mathx.V(0, 0), Assisted: true})
	speed := float32(30)
	r, done := drive(&b, mathx.V(0, 0), s.Goals, speed, 0.01)
	if !done || r.Status != job.Passed || r.Pay != 0 {
		t.Errorf("result %+v, want a pass that pays nothing", r)
	}
}

func TestSlowTimeTrialRunsOutOfTime(t *testing.T) {
	c := city.NewGrowing(city.ParamsFor(city.MapCity, 5))
	var b job.Board
	s, _ := b.Start(race.Trial, c, 5, 1, mathx.V(0, 0), 0)
	r, done := drive(&b, mathx.V(0, 0), s.Goals, 2, 0)
	if !done || r.Status != job.Failed {
		t.Errorf("a crawl came to %+v", r)
	}
}

func TestCheckpointsHaveToBeTakenInOrder(t *testing.T) {
	goals := []job.Goal{
		{Pos: mathx.V(0, 0), Radius: 3},
		{Pos: mathx.V(100, 0), Radius: 3},
		{Pos: mathx.V(200, 0), Radius: 3},
		{Pos: mathx.V(300, 0), Radius: 3},
	}
	at := func(s *job.State, x, z float32) {
		race.Trial.Step(s, job.Frame{Now: 1, Pos: mathx.V(x, z)})
	}
	// Driving past the third checkpoint first does not count: it is not the
	// next one.
	s := &job.State{Goals: goals, Type: race.TrialID, Deadline: 1000}
	at(s, 190, 1)
	at(s, 210, 1)
	if s.Next != 0 {
		t.Fatalf("the third gate counted first: next %d", s.Next)
	}
	// The next one is taken as the car crosses it, and only once.
	s = &job.State{Goals: goals, Type: race.TrialID, Deadline: 1000}
	at(s, -5, 0)
	at(s, 1, 0)
	if s.Next != 1 {
		t.Fatalf("the first gate was not taken: next %d", s.Next)
	}
	at(s, 2, 0)
	if s.Next != 1 {
		t.Errorf("the first gate counted twice: next %d", s.Next)
	}
	// And a gate is not skipped to: the last cannot end the race early.
	s = &job.State{Goals: goals, Type: race.TrialID, Deadline: 1000, Next: 1}
	at(s, 290, 0)
	at(s, 305, 0)
	if s.Status != job.Active || s.Next != 1 {
		t.Errorf("the last gate ended the race out of order: %+v", s)
	}
}

func TestCircuitCountsLaps(t *testing.T) {
	c := city.NewGrowing(city.ParamsFor(city.MapCity, 5))
	var b job.Board
	s, err := b.Start(race.Circuit, c, 5, 1, mathx.V(0, 0), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Goals)%3 != 0 {
		t.Fatalf("%d goals for three laps", len(s.Goals))
	}
	per := len(s.Goals) / 3
	if !slices.Equal(goalPos(s.Goals[:per]), goalPos(s.Goals[per:2*per])) {
		t.Error("a lap is not the same as the one before")
	}
	last := s.Goals[len(s.Goals)-1].Pos
	if s.Goals[per-1].Pos != last {
		t.Error("each lap does not end on the line")
	}
	for next, want := range map[int]int{0: 1, per - 1: 1, per: 2, 2*per - 1: 2, 2 * per: 3, 3*per - 1: 3} {
		s.Next = next
		if lap, of := race.Laps(s); lap != want || of != 3 {
			t.Errorf("at checkpoint %d: lap %d of %d, want %d of 3", next, lap, of, want)
		}
	}
	if lap, of := race.Laps(job.State{Type: race.TrialID}); lap != 1 || of != 1 {
		t.Errorf("a time trial is lap %d of %d", lap, of)
	}
}

func goalPos(gs []job.Goal) []mathx.Vec {
	var out []mathx.Vec
	for _, g := range gs {
		out = append(out, g.Pos)
	}
	return out
}

func TestMedalThresholds(t *testing.T) {
	const par = 100
	for _, c := range []struct {
		elapsed float32
		want    string
	}{{80, "gold"}, {100, "gold"}, {101, "silver"}, {115, "silver"}, {116, "bronze"}, {135, "bronze"}, {136, ""}} {
		if got := race.Medal(c.elapsed, par); got != c.want {
			t.Errorf("%.0f s: %q, want %q", c.elapsed, got, c.want)
		}
	}
}

func TestRewardFollowsTheVerdict(t *testing.T) {
	s := job.State{Type: race.CircuitID, Start: 0, Deadline: 160, Status: job.Passed}
	var pays []int
	for _, r := range []string{"1st", "2nd", "3rd", "4th"} {
		s.Reason = r
		pays = append(pays, race.Circuit.Reward(s))
	}
	if !sort.SliceIsSorted(pays, func(i, j int) bool { return pays[i] > pays[j] }) || pays[0] <= pays[1] || pays[3] != 0 {
		t.Errorf("pay by place %v, want falling to nothing", pays)
	}
}

func TestRivalsReachCheckpointsInOrder(t *testing.T) {
	c := city.NewGrowing(city.ParamsFor(city.MapCity, 5))
	l, ok := race.PlanLoop(c, 5, 2, mathx.V(0, 0))
	if !ok {
		t.Fatal("no loop")
	}
	var goals []job.Goal
	for _, g := range l.Gates {
		goals = append(goals, job.Goal{Pos: g.Pos, Radius: g.Half})
	}
	const par = 120
	field := race.Field(5, 2, 3)
	if !slices.Equal(field, race.Field(5, 2, 3)) {
		t.Error("the field is not a function of the race")
	}
	for _, r := range field {
		ts := r.Times(goals, par)
		if !sort.SliceIsSorted(ts, func(i, j int) bool { return ts[i] < ts[j] }) || ts[0] <= 0 {
			t.Errorf("%s reaches the checkpoints at %v", r.Name, ts)
		}
		if got := ts[len(ts)-1]; got != r.Finish(par) {
			t.Errorf("%s finishes at %.1f but the last gate is at %.1f", r.Name, r.Finish(par), got)
		}
	}
}

func TestPlaceCountsRivalsAhead(t *testing.T) {
	field := []race.Rival{{Style: 0.9}, {Style: 1.0}, {Style: 1.1}}
	for elapsed, want := range map[float32]int{80: 1, 95: 2, 105: 3, 150: 4} {
		if got := race.Place(elapsed, 100, field); got != want {
			t.Errorf("%.0f s: place %d, want %d", elapsed, got, want)
		}
	}
}

func TestCircuitResultIsAPlace(t *testing.T) {
	c := city.NewGrowing(city.ParamsFor(city.MapCity, 9))
	var b job.Board
	s, _ := b.Start(race.Circuit, c, 9, 1, mathx.V(0, 0), 0)
	par := (s.Deadline - s.Start) / 1.6
	var length float32
	prev := mathx.V(0, 0)
	for _, g := range s.Goals {
		length += prev.DistTo(g.Pos)
		prev = g.Pos
	}
	r, done := drive(&b, mathx.V(0, 0), s.Goals, length/(par*0.5), 0)
	if !done || r.Status != job.Passed || r.Reason != "1st" {
		t.Errorf("a car at twice par pace got %+v", r)
	}
}

func TestStreetRaceIsRoutedThroughStreets(t *testing.T) {
	for _, m := range city.Maps() {
		c := city.NewGrowing(city.ParamsFor(m, 4))
		c.EnsureAround(mathx.V(0, 0), 300)
		start := c.Lanes[0].Point(c.Lanes[0].Length / 2)
		for _, l := range c.Lanes {
			if !c.Roads[l.Road].Ring && c.Roads[l.Road].Class >= city.Collector {
				start = l.Point(l.Length / 2)
				break
			}
		}
		r, ok := race.PlanRoute(c, 4, 1, start)
		if !ok {
			t.Errorf("%s: no street race from %v", m.Name(), start)
			continue
		}
		if len(r.Gates) < 3 || r.Length < 1000 {
			t.Errorf("%s: %d gates over %.0f m", m.Name(), len(r.Gates), r.Length)
		}
		again, _ := race.PlanRoute(c, 4, 1, start)
		if fingerprint(race.Loop{Gates: r.Gates}) != fingerprint(race.Loop{Gates: again.Gates}) {
			t.Errorf("%s: the route changed between asking twice", m.Name())
		}
	}
}

func TestSlipAngleIsInTheCarsOwnFrame(t *testing.T) {
	if a := race.SlipAngle(mathx.V(20, 0), 0); a != 0 {
		t.Errorf("straight ahead has slip %v", a)
	}
	// Nose along +X, moving at 45 degrees to it: a slide to the right of the
	// nose is positive, and to the left negative.
	if a := race.SlipAngle(mathx.V(10, 10), 0); mathx.Abs(a-0.7854) > 0.01 {
		t.Errorf("a 45 degree slide measured %v", a)
	}
	if a := race.SlipAngle(mathx.V(10, -10), 0); mathx.Abs(a+0.7854) > 0.01 {
		t.Errorf("a 45 degree slide the other way measured %v", a)
	}
	if race.SlipAngle(mathx.V(-10, 0), 0) != 0 {
		t.Error("reversing has slip")
	}
	if race.SlipAngle(mathx.V(0.1, 0.1), 0) != 0 {
		t.Error("a crawl has slip")
	}
}

func TestDriftScoreIsSlipTimesSpeed(t *testing.T) {
	if s := race.SlipScore(0, 30, 1); s != 0 {
		t.Errorf("driving straight scored %v", s)
	}
	if s := race.SlipScore(0.05, 30, 1); s != 0 {
		t.Errorf("a gentle turn scored %v", s)
	}
	one, two := race.SlipScore(0.4, 20, 1), race.SlipScore(0.4, 40, 1)
	if one <= 0 || mathx.Abs(two-2*one) > 1e-3 {
		t.Errorf("doubling the speed took %v to %v", one, two)
	}
	if a, b := race.SlipScore(0.2, 20, 1), race.SlipScore(0.4, 20, 1); mathx.Abs(b-2*a) > 1e-3 {
		t.Errorf("doubling the slip took %v to %v", a, b)
	}
	if s := race.SlipScore(1.5, 30, 1); s != 0 {
		t.Errorf("a spin scored %v", s)
	}
	if s := race.SlipScore(-0.4, 20, 1); s != one {
		t.Errorf("slip to the left scored %v, to the right %v", s, one)
	}
}

func TestDriftEventScoresInTheZoneAndPays(t *testing.T) {
	c := city.NewGrowing(city.ParamsFor(city.MapCity, 6))
	c.EnsureAround(mathx.V(0, 0), 900)
	var b job.Board
	s, err := b.Start(race.Drift{}, c, 6, 1, mathx.V(0, 0), 0)
	if err != nil {
		t.Fatal(err)
	}
	st := b.Active()
	zone := s.Goals[0].Pos
	now := float32(0)
	// Sliding outside the zone earns nothing.
	race.Feed(st, race.Sample{Now: 1, Pos: zone.Add(mathx.V(race.DriftZone+10, 0)), Vel: mathx.V(15, 15), Step: 1})
	if race.Score(st) != 0 || st.Held != 0 {
		t.Fatalf("scored %v outside the zone", race.Score(st))
	}
	// Straight driving inside it earns nothing, and starts the clock.
	now = 2
	race.Feed(st, race.Sample{Now: now, Pos: zone, Vel: mathx.V(25, 0), Step: 1})
	if race.Score(st) != 0 || st.Held != 2 {
		t.Fatalf("straight driving scored %v, clock %v", race.Score(st), st.Held)
	}
	var res job.Result
	done := false
	for !done && now < 70 {
		now += 0.1
		race.Feed(st, race.Sample{Now: now, Pos: zone, Vel: mathx.V(20, 9), Step: 0.1})
		res, done = b.Step(job.Frame{Now: now, Pos: zone, Speed: 22})
	}
	if !done || res.Status != job.Passed || res.Pay <= 0 {
		t.Errorf("a minute of sliding came to %+v (score %v)", res, race.Score(st))
	}
}

func TestDriftEventFailsWithoutEnoughSlip(t *testing.T) {
	c := city.NewGrowing(city.ParamsFor(city.MapCity, 6))
	c.EnsureAround(mathx.V(0, 0), 900)
	var b job.Board
	s, _ := b.Start(race.Drift{}, c, 6, 1, mathx.V(0, 0), 0)
	st := b.Active()
	now := float32(1)
	var res job.Result
	done := false
	for !done && now < 80 {
		now += 0.5
		race.Feed(st, race.Sample{Now: now, Pos: s.Goals[0].Pos, Vel: mathx.V(25, 0), Step: 0.5})
		res, done = b.Step(job.Frame{Now: now, Pos: s.Goals[0].Pos, Speed: 25})
	}
	if !done || res.Status != job.Failed {
		t.Errorf("driving straight came to %+v", res)
	}
}

func TestSaltsDiffer(t *testing.T) {
	// A smoke test that the pack's draws are not the same stream as each other.
	a := city.Chance(1, 0xa3d1f4c87e2b9065, 3, 4)
	b := city.Chance(1, 0x38c2f6a1e95d704b, 3, 4)
	if a == b {
		t.Error("two salts draw the same")
	}
}

func TestAllRegistersFourTypes(t *testing.T) {
	ids := map[string]bool{}
	for _, ty := range race.All() {
		ids[ty.ID()] = true
	}
	if len(ids) != 4 {
		t.Errorf("%d distinct types, want 4", len(ids))
	}
}
