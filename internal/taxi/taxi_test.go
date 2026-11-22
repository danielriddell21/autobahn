package taxi_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/taxi"
)

func register() {
	for _, t := range taxi.All() {
		job.Register(t)
	}
}

func world(t *testing.T) *city.City {
	t.Helper()
	register()
	c := city.NewGrowing(city.DefaultParams(11))
	c.EnsureAround(mathx.V(0, 0), 900)
	return c
}

// ---- tipping ----

func TestFaultsEatTheTipAndNeverTheBase(t *testing.T) {
	if got := taxi.Pay(100, 50, 0, 10, 0); got != 150 {
		t.Errorf("faultless pay %d, want 150", got)
	}
	if got := taxi.Pay(100, 50, 2, 10, 0); got != 130 {
		t.Errorf("two points pay %d, want 130", got)
	}
	if got := taxi.Pay(100, 50, 50, 10, 0); got != 100 {
		t.Errorf("a ruinous drive pays %d, want the base 100", got)
	}
}

func TestPayIsNeverNegative(t *testing.T) {
	for _, c := range [][5]int{{-5, -5, -5, -5, -5}, {0, 0, 99, 99, 99}, {10, 0, 1000, 10, 0}} {
		if got := taxi.Pay(c[0], c[1], c[2], c[3], c[4]); got < 0 {
			t.Errorf("Pay%v = %d", c, got)
		}
		if got := taxi.Reward(c[0], c[1], c[2], c[3], c[4]); got < 0 {
			t.Errorf("Reward%v = %d", c, got)
		}
	}
}

func TestStreakMultiplierRisesAndCaps(t *testing.T) {
	want := map[int]int{-1: 100, 0: 100, 1: 110, 3: 130, taxi.StreakCap: 100 + taxi.StreakStep*taxi.StreakCap, 40: 100 + taxi.StreakStep*taxi.StreakCap}
	for n, w := range want {
		if got := taxi.Multiplier(n); got != w {
			t.Errorf("Multiplier(%d) = %d, want %d", n, got, w)
		}
	}
	if taxi.Pay(100, 40, 0, 10, 3) <= taxi.Pay(100, 40, 0, 10, 0) {
		t.Error("a streak does not raise the pay")
	}
}

// The framework takes the cost of the points off the reward, so what it pays is
// what Pay says, with or without a tip left to eat into.
func TestFrameworkPaysWhatPayCalculates(t *testing.T) {
	register()
	for _, ty := range taxi.All() {
		for _, pts := range []int{0, 1, 5, 200} {
			s := job.State{Type: ty.ID(), Start: 0, Deadline: 200, Points: pts, Streak: 2, Status: job.Passed}
			if w := job.Worth(s); w < 0 {
				t.Errorf("%s with %d points is worth %d", ty.ID(), pts, w)
			}
			if got := job.Payout(s); got != job.Worth(s) {
				t.Errorf("%s pays %d, worth %d", ty.ID(), got, job.Worth(s))
			}
			s.Assisted = true
			if job.Payout(s) != 0 {
				t.Errorf("%s pays an assisted run", ty.ID())
			}
		}
	}
	f := job.State{Type: taxi.FareID, Deadline: 200, Points: 3, Status: job.Passed}
	ty := taxi.Fare{}
	want := taxi.Pay(80+int(2*200), (80+int(2*200))/2, 3, ty.Rules().CostPerPoint, 0)
	if got := job.Payout(f); got != want {
		t.Errorf("fare pays %d, want %d", got, want)
	}
}

// ---- risky fares ----

func TestRiskyFareIsOptionalAndPaysEarningsNotBounty(t *testing.T) {
	c := world(t)
	odd, even := 0, 0
	for serial := 1; serial <= 200; serial++ {
		if taxi.Offered(11, serial, mathx.V(0, 0)) {
			odd++
		} else {
			even++
		}
	}
	if odd == 0 || even == 0 {
		t.Fatalf("rush offers %d, none %d: it must come and go", odd, even)
	}
	fare, risky := taxi.Fare{}, taxi.Fare{Risky: true}
	if risky.Rules().MaxWanted <= fare.Rules().MaxWanted {
		t.Error("a rush fare must survive more pursuit than an ordinary one")
	}
	sf, ok1 := fare.Plan(c, 11, 1, mathx.V(0, 0), 0)
	sr, ok2 := risky.Plan(c, 11, 1, mathx.V(0, 0), 0)
	if !ok1 || !ok2 {
		t.Fatal("no fares to plan")
	}
	if sr.Deadline >= sf.Deadline {
		t.Errorf("rush deadline %.0f is not tighter than %.0f", sr.Deadline, sf.Deadline)
	}
	// Same journey and the same clock: the rush pays more.
	sf.Deadline, sr.Deadline = 300, 300
	if risky.Reward(sr) <= fare.Reward(sf) {
		t.Errorf("rush reward %d does not beat %d", risky.Reward(sr), fare.Reward(sf))
	}
	sr.Status, sr.Type = job.Passed, taxi.RiskyID
	if job.Payout(sr) <= 0 {
		t.Error("a rush fare passed pays nothing")
	}
	sr.Assisted = true
	if job.Payout(sr) != 0 {
		t.Error("a rush fare the autopilot drove paid")
	}
}

// ---- fares and streaks ----

func frame(now float32, at mathx.Vec, speed float32) job.Frame {
	return job.Frame{Now: now, Pos: at, Speed: speed}
}

// drive takes a job to its end by stopping at each goal in turn.
func drive(b *job.Board, now float32, faults int) job.Result {
	s := b.Active()
	var res job.Result
	for _, g := range s.Goals {
		now += 5
		f := frame(now, g.Pos, 0)
		if faults > 0 {
			f.Faults = []job.Fault{{Kind: "x", Points: faults}}
			faults = 0
		}
		res, _ = b.Step(f)
	}
	return res
}

func TestFareBoardsThenDropsAndStreakRisesAndResets(t *testing.T) {
	c := world(t)
	taxi.Fares.Reset()
	t.Cleanup(taxi.Fares.Reset)
	var b job.Board
	ty := taxi.Fare{}
	for i := 1; i <= 3; i++ {
		st, err := b.Start(ty, c, 11, i, mathx.V(0, 0), 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(st.Goals) != 2 || st.Streak != i-1 {
			t.Fatalf("fare %d: %d goals, streak %d", i, len(st.Goals), st.Streak)
		}
		// Not at the pick-up: nothing happens. At it and moving: nothing.
		if _, done := b.Step(frame(1, mathx.V(1e5, 1e5), 0)); done {
			t.Fatal("ended far away")
		}
		if _, done := b.Step(frame(2, st.Goals[0].Pos, 5)); done || b.Active().Next != 0 {
			t.Fatal("boarded while moving")
		}
		if _, done := b.Step(frame(3, st.Goals[0].Pos, 0)); done || !taxi.Boarded(*b.Active()) {
			t.Fatal("did not board when stopped at the pick-up")
		}
		res, done := b.Step(frame(4, st.Goals[1].Pos, 0))
		if !done || res.Status != job.Passed || res.Reason != taxi.Faultless || res.Pay <= 0 {
			t.Fatalf("fare %d: done %v result %+v", i, done, res)
		}
	}
	if n := taxi.Fares.N(); n != 3 {
		t.Fatalf("streak %d after three faultless fares, want 3", n)
	}
	// A fare passed with a fault pays less and ends the streak.
	if _, err := b.Start(ty, c, 11, 4, mathx.V(0, 0), 0); err != nil {
		t.Fatal(err)
	}
	res := drive(&b, 0, 2)
	if res.Status != job.Passed || res.Reason == taxi.Faultless || taxi.Fares.N() != 0 {
		t.Errorf("faulty pass: %+v, streak %d", res, taxi.Fares.N())
	}
	// A failure ends it too, and turning a fare down before boarding does not.
	taxi.Fares.Reset()
	for i := 5; i < 7; i++ {
		if _, err := b.Start(ty, c, 11, i, mathx.V(0, 0), 0); err != nil {
			t.Fatal(err)
		}
		drive(&b, 0, 0)
	}
	if _, err := b.Start(ty, c, 11, 7, mathx.V(0, 0), 0); err != nil {
		t.Fatal(err)
	}
	b.Cancel(taxi.Declined)
	if taxi.Fares.N() != 2 {
		t.Errorf("declining a fare changed the streak to %d", taxi.Fares.N())
	}
	if _, err := b.Start(ty, c, 11, 8, mathx.V(0, 0), 0); err != nil {
		t.Fatal(err)
	}
	b.Step(frame(1e6, mathx.V(0, 0), 0)) // out of time
	if taxi.Fares.N() != 0 {
		t.Errorf("a failed fare left the streak at %d", taxi.Fares.N())
	}
}

func TestAssistedFareNeitherPaysNorCountsToTheStreak(t *testing.T) {
	c := world(t)
	taxi.Fares.Reset()
	t.Cleanup(taxi.Fares.Reset)
	var b job.Board
	if _, err := b.Start(taxi.Fare{}, c, 11, 1, mathx.V(0, 0), 0); err != nil {
		t.Fatal(err)
	}
	g := b.Active().Goals
	f := frame(2, g[0].Pos, 0)
	f.Assisted = true
	b.Step(f)
	res, done := b.Step(frame(3, g[1].Pos, 0))
	if !done || res.Pay != 0 || taxi.Fares.N() != 0 {
		t.Errorf("assisted fare: done %v, %+v, streak %d", done, res, taxi.Fares.N())
	}
}

// ---- cargo ----

// run feeds a tracker a speed for each of n frames at 60 a second, starting
// from a car already moving at v0 and slowing by decel each second.
func run(c *taxi.Cargo, v0, decel float32, n int) float32 {
	const dt = 1.0 / 60
	var now, total float32
	c.Feed(now, v0)
	for range n {
		now += dt
		v0 = max(v0-decel*dt, 0)
		total += c.Feed(now, v0)
	}
	return total
}

func TestGentleDrivingDoesNoDamage(t *testing.T) {
	c := taxi.NewCargo(40)
	if d := run(c, 14, 2, 300); d != 0 || c.Points() != 0 {
		t.Errorf("a gentle stop did %v damage (%d points)", d, c.Points())
	}
}

func TestHardBrakingDamagesInProportionToItsExcess(t *testing.T) {
	soft, hard := taxi.NewCargo(40), taxi.NewCargo(40)
	ds, dh := run(soft, 20, 5, 120), run(hard, 25, 8, 180)
	if ds <= 0 || dh <= ds {
		t.Errorf("braking at 5 did %v and at 8 did %v", ds, dh)
	}
	if soft.Damage != ds {
		t.Errorf("damage %v is not the sum %v", soft.Damage, ds)
	}
}

func TestJoltsDamageTheLoad(t *testing.T) {
	c := taxi.NewCargo(60)
	const dt = 1.0 / 60
	now := float32(0)
	v := float32(20)
	c.Feed(now, v)
	// Cruise, then brake at 3.9 m/s^2: under the braking limit, but a jolt for
	// all that, since it comes on all at once.
	var total float32
	for i := range 90 {
		now += dt
		if i >= 30 {
			v -= 3.9 * dt
		}
		total += c.Feed(now, v)
	}
	if total <= 0 {
		t.Error("an abrupt stop did no damage")
	}
	if smooth := run(taxi.NewCargo(60), 20, 3.9, 90); smooth > total {
		t.Errorf("a stop from the start of the run did %v, more than the stamp's %v", smooth, total)
	}
}

func TestACollisionCountsInFull(t *testing.T) {
	c := taxi.NewCargo(60)
	c.Feed(0, 15)
	if d := c.Feed(1.0/60, 0); d < 15 {
		t.Errorf("hitting a wall at 15 m/s did %v, want at least 15", d)
	}
	b := taxi.NewCargo(60)
	b.Feed(0, 5)
	if d := b.Feed(1.0/60, 0); d < 5 {
		t.Errorf("a 5 m/s impact did %v", d)
	}
	if taxi.NewCargo(10).Destroyed() {
		t.Error("a fresh load is destroyed")
	}
	d := taxi.NewCargo(20)
	d.Feed(0, 20)
	d.Feed(1.0/60, 0)
	if !d.Destroyed() || d.Points() != taxi.MaxPoints {
		t.Errorf("a heavy crash: destroyed %v, %d points", d.Destroyed(), d.Points())
	}
}

func TestACollisionIsNotCountedTwiceAsAJolt(t *testing.T) {
	c := taxi.NewCargo(100)
	c.Feed(0, 12)
	hit := c.Feed(1.0/60, 0)
	if again := c.Feed(2.0/60, 0); again != 0 {
		t.Errorf("sitting still after a crash did %v more (the crash did %v)", again, hit)
	}
}

func TestAPauseIsNotAJolt(t *testing.T) {
	c := taxi.NewCargo(40)
	c.Feed(0, 20)
	if d := c.Feed(30, 0); d != 0 {
		t.Errorf("a long gap between frames did %v damage", d)
	}
}

func TestPointsComeDueOnce(t *testing.T) {
	c := taxi.NewCargo(40)
	c.Damage = 20
	if p := c.Take(); p != taxi.MaxPoints/2 {
		t.Errorf("half the load gone is %d points", p)
	}
	if p := c.Take(); p != 0 {
		t.Errorf("the same points came due again: %d", p)
	}
	c.Damage = 1000
	if p := c.Take(); p != taxi.MaxPoints/2 || c.Points() != taxi.MaxPoints {
		t.Errorf("the rest: %d, total %d", p, c.Points())
	}
}

// ---- courier ----

func TestCourierRoundDamageCutsPayAndDestroysTheLoad(t *testing.T) {
	c := world(t)
	taxi.Couriers.Reset()
	t.Cleanup(taxi.Couriers.Reset)
	ty, _ := job.Lookup(taxi.CourierID)
	var b job.Board
	st, err := b.Start(ty, c, 11, 1, mathx.V(0, 0), 0)
	if err != nil {
		t.Fatal(err)
	}
	if n := taxi.Drops(st); n < taxi.MinDrops || n > taxi.MaxDrops || len(st.Goals) != n+1 {
		t.Fatalf("%d drops in %d goals", n, len(st.Goals))
	}
	whole := job.Worth(*b.Active())
	now := float32(1)
	b.Step(frame(now, st.Goals[0].Pos, 0)) // collected
	far := st.Goals[0].Pos.Add(mathx.V(500, 500))
	for range 5 {
		now += 1.0 / 60
		if r, done := b.Step(frame(now, far, 12)); done {
			t.Fatalf("ended early: %+v", r)
		}
	}
	// Then stop in a frame.
	now += 1.0 / 60
	res, done := b.Step(frame(now, far, 0))
	if done {
		if res.Status != job.Failed || res.Pay != 0 {
			t.Errorf("destroyed load: %+v", res)
		}
		return
	}
	if p := b.Active().Points; p == 0 || job.Worth(*b.Active()) >= whole {
		t.Errorf("a crash left %d points and the pay at %d from %d", p, job.Worth(*b.Active()), whole)
	}
}

func TestCourierRoundPassesAndStreakCounts(t *testing.T) {
	c := world(t)
	taxi.Couriers.Reset()
	t.Cleanup(taxi.Couriers.Reset)
	ty, _ := job.Lookup(taxi.CourierID)
	var b job.Board
	if _, err := b.Start(ty, c, 11, 2, mathx.V(0, 0), 0); err != nil {
		t.Fatal(err)
	}
	res := drive(&b, 0, 0)
	if res.Status != job.Passed || res.Pay <= 0 || taxi.Couriers.N() != 1 {
		t.Errorf("round: %+v, streak %d", res, taxi.Couriers.N())
	}
}

func TestFragilityIsAFunctionOfTheRound(t *testing.T) {
	for s := 1; s < 50; s++ {
		f := taxi.Fragility(7, s)
		if f < taxi.MinFragility || f > taxi.MaxFragility || f != taxi.Fragility(7, s) {
			t.Errorf("fragility %v of round %d", f, s)
		}
	}
}

// ---- pick-up points ----

func spotsOf(c *city.City, reach float32) []string {
	ix := taxi.NewIndex(c)
	ix.Sync()
	var out []string
	for _, s := range ix.Spots() {
		if s.Pos.Len() > reach {
			continue
		}
		out = append(out, fmt.Sprintf("%v>%v #%d rank %v at %.2f,%.2f kerb %.2f,%.2f",
			s.From, s.To, s.Slot, s.Rank, s.Pos.X, s.Pos.Z, s.Kerb.X, s.Kerb.Z))
	}
	sort.Strings(out)
	return out
}

// Pick-up points are the same whatever order the city grew in, on every map.
func TestPickupsAreTheSameWhateverOrderTheCityGrows(t *testing.T) {
	const seed = 7
	for _, m := range city.Maps() {
		t.Run(m.Name(), func(t *testing.T) {
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
			want, got := spotsOf(whole, reach), spotsOf(grown, reach)
			if len(want) == 0 {
				t.Fatal("no pick-ups to compare")
			}
			if fmt.Sprint(want) != fmt.Sprint(got) {
				t.Errorf("pick-ups differ:\n whole %v\n grown %v", want, got)
			}
		})
	}
}

// A spot found while the city grows step by step never moves or goes, and
// every one is on a lane with both junctions complete.
func TestPickupsNeverMoveAndStandOnCompleteBlocks(t *testing.T) {
	for _, m := range city.Maps() {
		c := city.NewGrowing(city.ParamsFor(m, 3))
		ix := taxi.NewIndex(c)
		seen := map[string]taxi.Spot{}
		for _, at := range []mathx.Vec{{}, {X: 300}, {X: 600}, {X: 600, Z: 300}, {Z: 700}} {
			c.EnsureAround(at, 500)
			ix.Sync()
			for _, s := range ix.Spots() {
				k := fmt.Sprint(s.From, s.To, s.Slot)
				if old, ok := seen[k]; ok && (old.Pos != s.Pos || old.Rank != s.Rank) {
					t.Fatalf("%s: spot %s moved", m.Name(), k)
				}
				seen[k] = s
				l := c.Lanes[s.Lane]
				for _, id := range []int{l.FromNode, l.ToNode} {
					if !c.Complete(c.Nodes[id]) {
						t.Fatalf("%s: spot %s on an unfinished lane", m.Name(), k)
					}
				}
				if s.Pos.DistTo(l.Point(l.Run(s.Pos))) > 1 {
					t.Errorf("%s: spot %v is off its lane", m.Name(), s.Pos)
				}
			}
		}
		if len(seen) < 3 {
			t.Errorf("%s: only %d spots found", m.Name(), len(seen))
		}
	}
}

func TestSomeSpotsHaveRanksAndMostDoNot(t *testing.T) {
	c := world(t)
	ix := taxi.NewIndex(c)
	ix.Sync()
	all, ranks := len(ix.Spots()), len(ix.Ranks())
	if ranks == 0 || ranks >= all {
		t.Errorf("%d ranks of %d spots", ranks, all)
	}
}

func TestNothingIsPickedUpOnTheFrontier(t *testing.T) {
	c := city.NewGrowing(city.DefaultParams(2))
	c.EnsureAround(mathx.V(0, 0), 60)
	ix := taxi.NewIndex(c)
	ix.Sync()
	for _, s := range ix.Spots() {
		l := c.Lanes[s.Lane]
		if !c.Complete(c.Nodes[l.FromNode]) || !c.Complete(c.Nodes[l.ToNode]) {
			t.Fatalf("spot %v on an unfinished lane", s.From)
		}
	}
}

func TestPlansDoNotDependOnWhereTheCarStandsInACell(t *testing.T) {
	c := world(t)
	a, ok1 := taxi.Fare{}.Plan(c, 11, 3, mathx.V(10, 10), 0)
	b, ok2 := taxi.Fare{}.Plan(c, 11, 3, mathx.V(14, 12), 0)
	if !ok1 || !ok2 || a.Goals[0].Pos != b.Goals[0].Pos {
		t.Error("a few metres changed the pick-up")
	}
}
