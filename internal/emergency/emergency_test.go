package emergency_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/emergency"
	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

func sitesOf(c *city.City, reach float32) []string {
	ix := emergency.NewIndex(c)
	ix.Sync()
	var out []string
	for _, s := range ix.Sites() {
		if s.Pos.Len() > reach {
			continue
		}
		out = append(out, fmt.Sprintf("%s %v at %.2f,%.2f door %.2f,%.2f", s.Kind, s.Cell, s.Pos.X, s.Pos.Z, s.Door.X, s.Door.Z))
	}
	sort.Strings(out)
	return out
}

// Placement is the same whatever order the city grew in, on every map.
func TestSitesAreTheSameWhateverOrderTheCityGrows(t *testing.T) {
	const seed = 7
	for _, m := range city.Maps() {
		t.Run(m.Name(), func(t *testing.T) {
			// A super-cell is three cells wide, and the maps' cells differ.
			span := max(900, 10*float32(city.ParamsFor(m, seed).MaxSpan))
			whole := city.NewGrowing(city.ParamsFor(m, seed))
			whole.EnsureAround(mathx.V(0, 0), span)
			grown := city.NewGrowing(city.ParamsFor(m, seed))
			for _, at := range []mathx.Vec{
				mathx.V(-span/2, -span/2), mathx.V(span/2, -span/2), mathx.V(-span/2, span/2), mathx.V(span/2, span/2), mathx.V(0, 0),
			} {
				grown.EnsureAround(at, span/2)
			}
			grown.EnsureAround(mathx.V(0, 0), span)

			reach := span * 0.4
			want, got := sitesOf(whole, reach), sitesOf(grown, reach)
			if len(want) == 0 {
				t.Fatal("no sites to compare")
			}
			if fmt.Sprint(want) != fmt.Sprint(got) {
				t.Errorf("sites differ:\n whole %v\n grown %v", want, got)
			}
		})
	}
}

// A site is never decided early: growing one step at a time, a site once
// reported never moves or goes, and every one is on a complete block.
func TestSitesNeverMoveOnceFound(t *testing.T) {
	c := city.NewGrowing(city.ParamsFor(city.MapCity, 3))
	ix := emergency.NewIndex(c)
	seen := map[[2]int]emergency.Site{}
	for _, at := range []mathx.Vec{{}, {X: 300}, {X: 600}, {X: 600, Z: 300}, {Z: 700}} {
		c.EnsureAround(at, 500)
		ix.Sync()
		for _, s := range ix.Sites() {
			if old, ok := seen[s.Cell]; ok && old != s {
				t.Fatalf("site moved from %+v to %+v", old, s)
			}
			seen[s.Cell] = s
		}
		for cell := range seen {
			found := false
			for _, s := range ix.Sites() {
				found = found || s.Cell == cell
			}
			if !found {
				t.Fatalf("site %v went", cell)
			}
		}
	}
	if len(seen) < 3 {
		t.Fatalf("only %d sites found", len(seen))
	}
}

// Sites stand on blocks whose junctions are all complete, and are reachable.
func TestSitesStandOnCompleteBlocks(t *testing.T) {
	for _, m := range city.Maps() {
		c := city.NewGrowing(city.ParamsFor(m, 5))
		c.EnsureAround(mathx.V(0, 0), 700)
		ix := emergency.NewIndex(c)
		ix.Sync()
		plan := c.Plan()
		for _, s := range ix.Sites() {
			l := c.Lanes[s.Lane]
			for _, id := range []int{l.FromNode, l.ToNode} {
				if !plan.Complete(plan.Nodes[id]) {
					t.Errorf("%s: %v door lane runs to a junction that is not complete", m.Name(), s.Cell)
				}
			}
			if s.Door.DistTo(l.Point(l.Run(s.Door))) > 1 {
				t.Errorf("%s: door %v is off its lane", m.Name(), s.Door)
			}
		}
	}
}

// Nothing is found in ground that is still being built.
func TestNothingIsDecidedOnTheFrontier(t *testing.T) {
	c := city.NewGrowing(city.DefaultParams(2))
	c.EnsureAround(mathx.V(0, 0), 60)
	ix := emergency.NewIndex(c)
	ix.Sync()
	for _, s := range ix.Sites() {
		for _, l := range c.Lanes {
			if l.ID == s.Lane && (!c.Complete(c.Nodes[l.FromNode]) || !c.Complete(c.Nodes[l.ToNode])) {
				t.Fatalf("site %v on an unfinished lane", s.Cell)
			}
		}
	}
}

func world(t *testing.T) (*city.City, *emergency.Index) {
	t.Helper()
	c := city.NewGrowing(city.DefaultParams(11))
	c.EnsureAround(mathx.V(0, 0), 900)
	ix := emergency.NewIndex(c)
	ix.Sync()
	return c, ix
}

func start(t *testing.T, c *city.City) mathx.Vec {
	t.Helper()
	for _, l := range c.Lanes {
		p := l.Point(l.Length / 2)
		if p.Len() < 150 && c.Complete(c.Nodes[l.FromNode]) && c.Complete(c.Nodes[l.ToNode]) {
			return p
		}
	}
	t.Fatal("nowhere to start")
	return mathx.Vec{}
}

func TestCallOutsPlanDeterministically(t *testing.T) {
	c, _ := world(t)
	from := start(t, c)
	for _, ty := range emergency.All() {
		a, ok := ty.Plan(c, 11, 1, from, 0)
		if !ok {
			continue
		}
		b, _ := ty.Plan(c, 11, 1, from, 0)
		if fmt.Sprint(a.Goals) != fmt.Sprint(b.Goals) {
			t.Errorf("%s: the same call planned twice differs", ty.ID())
		}
		if len(a.Goals) == 0 || a.Deadline <= 0 {
			t.Errorf("%s: no goals or no clock: %+v", ty.ID(), a)
		}
	}
	if s, ok := emergency.Ambulance.Plan(c, 11, 1, from, 0); ok && len(s.Goals) != 2 {
		t.Errorf("an ambulance call has %d goals, want patient and hospital", len(s.Goals))
	}
}

func fireJob(at mathx.Vec) job.State {
	return job.State{
		Type: emergency.FireID, Goals: []job.Goal{{Pos: at, Radius: emergency.SceneRadius, Stop: true}},
		Deadline: 1e6,
	}
}

// The fire engine holds for twenty seconds; moving cancels the hold.
func TestFireEngineHoldsAtTheScene(t *testing.T) {
	at := mathx.V(100, 100)
	s := fireJob(at)
	ty := emergency.Fire
	ty.Step(&s, job.Frame{Now: 10, Pos: at, Speed: 0})
	if s.Held == 0 || s.Status != job.Active {
		t.Fatalf("hold not begun: %+v", s)
	}
	if p, hosing := emergency.Hosing(s, 20); !hosing || p <= 0 || p >= 1 {
		t.Errorf("hosing %v %v", p, hosing)
	}
	ty.Step(&s, job.Frame{Now: 29.9, Pos: at, Speed: 0})
	if s.Status != job.Active {
		t.Fatal("passed before twenty seconds")
	}
	// Moving off cancels the hold.
	ty.Step(&s, job.Frame{Now: 30, Pos: at, Speed: 3})
	if s.Held != 0 {
		t.Fatal("moving did not cancel the hold")
	}
	ty.Step(&s, job.Frame{Now: 31, Pos: at, Speed: 0})
	ty.Step(&s, job.Frame{Now: 50, Pos: at, Speed: 0})
	if s.Status != job.Active {
		t.Fatal("the hold did not start again from the beginning")
	}
	ty.Step(&s, job.Frame{Now: 51.1, Pos: at, Speed: 0})
	if s.Status != job.Passed {
		t.Fatalf("not passed after a full hold: %+v", s)
	}
}

func TestLeavingTheSceneCancelsTheHold(t *testing.T) {
	at := mathx.V(100, 100)
	s := fireJob(at)
	emergency.Fire.Step(&s, job.Frame{Now: 10, Pos: at})
	emergency.Fire.Step(&s, job.Frame{Now: 12, Pos: at.Add(mathx.V(40, 0))})
	if s.Held != 0 {
		t.Fatal("left the scene and kept the hold")
	}
}

func TestAmbulanceNeedsPatientThenHospital(t *testing.T) {
	s := job.State{Goals: []job.Goal{{Pos: mathx.V(0, 0), Radius: 6}, {Pos: mathx.V(300, 0), Radius: 8}}}
	emergency.Ambulance.Step(&s, job.Frame{Now: 1, Pos: mathx.V(300, 0)})
	if s.Next != 0 {
		t.Fatal("the hospital counted before the patient")
	}
	emergency.Ambulance.Step(&s, job.Frame{Now: 2, Pos: mathx.V(0, 0)})
	if s.Next != 1 || s.Status != job.Active {
		t.Fatalf("patient not picked up: %+v", s)
	}
	emergency.Ambulance.Step(&s, job.Frame{Now: 3, Pos: mathx.V(300, 0)})
	if s.Status != job.Passed {
		t.Fatalf("not passed at the hospital: %+v", s)
	}
}

// A failed call-out pays nothing and ends the streak; a passed one is worth
// more each time in a row.
func TestFailedCallOutPaysNothingAndEndsTheStreak(t *testing.T) {
	job.Register(emergency.Response)
	emergency.Run.Reset()
	defer emergency.Run.Reset()
	c, _ := world(t)
	from := start(t, c)

	attend := func(serial int) (job.Result, job.State) {
		b := &job.Board{}
		st, err := b.Start(emergency.Response, c, 11, serial, from, 100)
		if err != nil {
			t.Fatal(err)
		}
		at := st.Goals[0].Pos
		b.Step(job.Frame{Now: 110, Pos: at})
		r, done := b.Step(job.Frame{Now: 110 + emergency.AttendTime + 0.5, Pos: at})
		if !done {
			t.Fatalf("not done: %+v", b.Active())
		}
		return r, st
	}
	r1, s1 := attend(1)
	r2, s2 := attend(2)
	if r1.Status != job.Passed || r2.Status != job.Passed || emergency.Run.N() != 2 {
		t.Fatalf("results %+v %+v streak %d", r1, r2, emergency.Run.N())
	}
	if s1.Streak != 0 || s2.Streak != 1 {
		t.Errorf("streaks recorded %d, %d; want 0, 1", s1.Streak, s2.Streak)
	}

	b := &job.Board{}
	st, err := b.Start(emergency.Response, c, 11, 3, from, 100)
	if err != nil {
		t.Fatal(err)
	}
	if st.Streak != 2 {
		t.Errorf("the third call began with a streak of %d", st.Streak)
	}
	r, done := b.Step(job.Frame{Now: st.Deadline + 1})
	if !done || r.Status != job.Failed || r.Pay != 0 {
		t.Fatalf("a time-out: %+v", r)
	}
	if emergency.Run.N() != 0 {
		t.Errorf("streak %d after a failed call-out", emergency.Run.N())
	}
	// A police stop ends it too, and the pay is never below zero.
	emergency.Run.Reset()
	b = &job.Board{}
	b.Start(emergency.Response, c, 11, 4, from, 100)
	r, _ = b.Step(job.Frame{Now: 101, PoliceStopped: true})
	if r.Pay != 0 || emergency.Run.N() != 0 {
		t.Errorf("police stop: %+v streak %d", r, emergency.Run.N())
	}
	if w := job.Worth(job.State{Type: emergency.ResponseID, Points: 1000, Deadline: 10}); w != 0 {
		t.Errorf("worth %d with a thousand points", w)
	}
}

// A call-out the autopilot had a hand in counts for nothing, either way: it
// neither extends the streak nor ends it.
func TestAssistedCallOutLeavesTheStreakAlone(t *testing.T) {
	emergency.Run.Reset()
	defer emergency.Run.Reset()
	for _, status := range []job.Status{job.Passed, job.Failed} {
		emergency.Run.Reset()
		emergency.Ambulance.Settle(job.Result{Status: job.Passed})
		emergency.Ambulance.Settle(job.Result{Status: job.Passed})
		emergency.Ambulance.Settle(job.Result{Status: status, Assisted: true})
		if n := emergency.Run.N(); n != 2 {
			t.Errorf("an assisted %v call-out moved a streak of 2 to %d", status, n)
		}
	}
	emergency.Ambulance.Settle(job.Result{Status: job.Passed})
	if n := emergency.Run.N(); n != 3 {
		t.Errorf("the streak is %d after an unassisted pass, want 3", n)
	}
}
