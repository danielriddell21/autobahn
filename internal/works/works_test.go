package works_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/works"
)

// everyClosure is the closures of a city at several times, with every street
// that can be shut shut for the whole of each, so the geometry of all of them
// is looked at.
func everySite(c *city.City, seed uint64) []works.Closure {
	s := works.NewScheduled(c, seed, 1)
	// With a share of one, every slot is a closure, and a time well inside any
	// slot has every site solid or signed.
	return s.Closures(works.SlotLength*0.5+works.SolidAfter, nil)
}

func TestClosableOnlyOnTheParityPattern(t *testing.T) {
	for _, tc := range []struct {
		e    works.Edge
		want bool
	}{
		{works.Edge{I: 0, J: 0, Axis: 0}, true},
		{works.Edge{I: 4, J: 2, Axis: 0}, true},
		{works.Edge{I: -4, J: -2, Axis: 0}, true},
		{works.Edge{I: 1, J: 0, Axis: 0}, false},
		{works.Edge{I: 0, J: 1, Axis: 0}, false},
		{works.Edge{I: 2, J: 1, Axis: 1}, true},
		{works.Edge{I: -2, J: 3, Axis: 1}, true},
		{works.Edge{I: 2, J: 0, Axis: 1}, false},
		{works.Edge{I: 0, J: 1, Axis: 1}, false},
	} {
		if got := tc.e.Closable(); got != tc.want {
			t.Errorf("%v closable = %v, want %v", tc.e, got, tc.want)
		}
	}
}

func TestWorksShareOverTimeMatchesTheParameter(t *testing.T) {
	for _, share := range []float32{0.06, 0.12, 0.48} {
		on, total := 0, 0
		for i := -20; i < 20; i++ {
			for j := -20; j < 20; j++ {
				e := works.Edge{I: i, J: j}
				for slot := range 20 {
					at := (float32(slot) + 0.5) * works.SlotLength
					_, _, active := works.Window(5, e, share, at)
					total++
					if active {
						on++
					}
				}
			}
		}
		got := float32(on) / float32(total)
		if got < share*0.85 || got > share*1.15 {
			t.Errorf("share %.2f came out at %.3f over %d draws", share, got, total)
		}
	}
}

func TestPhasesAreOrderedAndWindowsAreSlotLong(t *testing.T) {
	e := works.Edge{I: 4, J: 2}
	seen := 0
	for k := range 200 {
		at := float32(k) * 37
		start, end, on := works.Window(9, e, 0.5, at)
		if at < start || at >= end {
			t.Fatalf("t=%v is outside its own window %v..%v", at, start, end)
		}
		if d := end - start; d < works.SlotLength-0.01 || d > works.SlotLength+0.01 {
			t.Fatalf("window %v..%v is not one slot long", start, end)
		}
		if !on {
			continue
		}
		seen++
		if works.PhaseAt(start, end, start-1) != works.PhaseClear ||
			works.PhaseAt(start, end, start) != works.PhaseSigned ||
			works.PhaseAt(start, end, start+works.SolidAfter-0.1) != works.PhaseSigned ||
			works.PhaseAt(start, end, start+works.SolidAfter) != works.PhaseSolid ||
			works.PhaseAt(start, end, end-0.1) != works.PhaseSolid ||
			works.PhaseAt(start, end, end) != works.PhaseClear {
			t.Fatalf("phases out of order for window %v..%v", start, end)
		}
	}
	if seen == 0 {
		t.Fatal("no works at all in 200 samples at a share of a half")
	}
}

func TestSignsGoUpBeforeBarriersAreSolid(t *testing.T) {
	for _, m := range city.Maps() {
		c := city.Generate(city.ParamsFor(m, 3))
		s := works.NewScheduled(c, 3, 1)
		// Just after a slot begins for some street: signed and not yet solid.
		var signed, solid int
		for step := float32(0); step < works.SlotLength*2; step += 5 {
			for _, cl := range s.Closures(step, nil) {
				if cl.Start > cl.Solid || cl.Solid > cl.End {
					t.Fatalf("%v: closure times out of order %v %v %v", m.Name(), cl.Start, cl.Solid, cl.End)
				}
				switch cl.Phase {
				case works.PhaseSigned:
					signed++
				case works.PhaseSolid:
					solid++
				}
			}
		}
		if signed == 0 || solid == 0 {
			t.Errorf("%s: signed %d, solid %d: both phases should turn up", m.Name(), signed, solid)
		}
	}
}

func TestNoClosureAtAnUnfinishedJunction(t *testing.T) {
	for _, m := range city.Maps() {
		c := city.NewGrowing(city.ParamsFor(m, 11))
		c.EnsureAround(mathx.V(0, 0), 350)
		s := works.NewScheduled(c, 11, 1)
		got := s.Closures(works.SlotLength*0.5+works.SolidAfter, nil)
		if len(got) == 0 {
			t.Fatalf("%s: no closures to test", m.Name())
		}
		for _, cl := range got {
			r := c.Roads[cl.Road]
			for _, n := range [2]int{r.A, r.B} {
				if !c.Complete(c.Nodes[n]) {
					t.Errorf("%s: closure on %v has an unfinished junction at %d", m.Name(), cl.Edge, n)
				}
			}
		}
		// And every junction in the city that is not finished carries none.
		for _, n := range c.Nodes {
			if c.Complete(n) {
				continue
			}
			for _, cl := range got {
				if r := c.Roads[cl.Road]; r.A == n.ID || r.B == n.ID {
					t.Errorf("%s: closure %v touches unfinished junction %d,%d", m.Name(), cl.Edge, n.GridI, n.GridJ)
				}
			}
		}
	}
}

func TestOneClosedArmPerJunctionAndEveryLaneKeepsAnExit(t *testing.T) {
	total := 0
	defer func() {
		if total == 0 {
			t.Error("no street closures anywhere to test")
		}
	}()
	for _, m := range city.Maps() {
		c := city.Generate(city.ParamsFor(m, 21))
		closed := map[int]bool{}
		arms := map[int]int{}
		got := everySite(c, 21)
		streets := 0
		for _, cl := range got {
			if cl.Kind != works.KindStreet {
				continue
			}
			streets++
			r := c.Roads[cl.Road]
			arms[r.A]++
			arms[r.B]++
			for _, id := range cl.Lanes {
				closed[id] = true
			}
			if c.Nodes[r.A].Roundabout || c.Nodes[r.B].Roundabout {
				t.Errorf("%s: %v closed beside a roundabout", m.Name(), cl.Edge)
			}
		}
		total += streets
		for n, k := range arms {
			if k > 1 {
				t.Errorf("%s: junction %d has %d closed arms", m.Name(), n, k)
			}
		}
		for _, l := range c.Lanes {
			if len(l.Succ) == 0 || c.Roads[l.Road].Ring {
				continue
			}
			open := false
			for _, tn := range l.Succ {
				if !closed[tn.Lane] {
					open = true
				}
			}
			if !open && !closed[l.ID] {
				t.Errorf("%s: lane %d has no exit that is not closed", m.Name(), l.ID)
			}
		}
	}
}

func TestClosuresStandOnTheirLanes(t *testing.T) {
	for _, m := range city.Maps() {
		c := city.Generate(city.ParamsFor(m, 4))
		for _, cl := range everySite(c, 4) {
			for _, b := range cl.Barriers {
				p := c.Project(b.Center)
				if !p.Valid || p.Dist > 0.5 {
					t.Errorf("%s: barrier of %v is %.2f m off any lane", m.Name(), cl.Edge, p.Dist)
				}
			}
			for _, o := range cl.Cones {
				p := c.Project(o.Center)
				if !p.Valid || p.Dist > 0.5 {
					t.Errorf("%s: cone of %v is %.2f m off any lane", m.Name(), cl.Edge, p.Dist)
				}
				l := c.Lanes[cl.Lanes[0]]
				if l.Length-l.Along(o.Center) < 29.9 {
					t.Errorf("%s: cone of %v within 30 m of the stop line", m.Name(), cl.Edge)
				}
			}
		}
	}
}

// fingerprint describes closures by edge and rounded geometry, within reach of
// the middle, so two builds of one place can be compared.
func fingerprint(cs []works.Closure, reach float32) []string {
	var out []string
	for _, cl := range cs {
		var parts string
		inside := true
		for _, b := range cl.Barriers {
			parts += fmt.Sprintf(" B%.2f,%.2f/%.3f", b.Center.X, b.Center.Z, b.Heading)
			inside = inside && b.Center.Len() < reach
		}
		for _, o := range cl.Cones {
			parts += fmt.Sprintf(" C%.2f,%.2f/%.3f", o.Center.X, o.Center.Z, o.Heading)
			inside = inside && o.Center.Len() < reach
		}
		for _, s := range cl.Signs {
			parts += fmt.Sprintf(" S%d@%.2f,%.2f/%.3f", s.Kind, s.Pos.X, s.Pos.Z, s.Heading)
			inside = inside && s.Pos.Len() < reach
		}
		if !inside {
			continue
		}
		out = append(out, fmt.Sprintf("%v kind=%d phase=%d %.1f/%.1f/%.1f%s",
			cl.Edge, cl.Kind, cl.Phase, cl.Start, cl.Solid, cl.End, parts))
	}
	sort.Strings(out)
	return out
}

// TestGrownWorksMatchThoseBuiltAtOnce holds roadworks to the growth rule: a
// place reached a piece at a time is dug up exactly as the same ground built in
// one pass, at every moment.
func TestGrownWorksMatchThoseBuiltAtOnce(t *testing.T) {
	const seed, span = 7, 600
	for _, m := range city.Maps() {
		t.Run(m.Name(), func(t *testing.T) {
			whole := city.NewGrowing(city.ParamsFor(m, seed))
			whole.EnsureAround(mathx.V(0, 0), span)
			grown := city.NewGrowing(city.ParamsFor(m, seed))
			ws := works.NewScheduled(whole, seed, 1)
			gs := works.NewScheduled(grown, seed, 1)

			// Reached in passes, and looked at in between, so the index has to
			// pick the new ground up without having ever seen the old.
			for _, at := range []mathx.Vec{
				mathx.V(-300, -300), mathx.V(300, -300), mathx.V(-300, 300), mathx.V(300, 300), mathx.V(0, 0),
			} {
				grown.EnsureAround(at, span/2)
				gs.Closures(100, nil)
			}
			grown.EnsureAround(mathx.V(0, 0), span)

			reach := float32(1e9)
			if whole.Laid() {
				reach = span * 0.55
			}
			compared := 0
			for _, at := range []float32{0, 70, 400, 1000, 2500, 7000} {
				want := fingerprint(ws.Closures(at, nil), reach)
				got := fingerprint(gs.Closures(at, nil), reach)
				compared += len(want)
				if len(want) != len(got) {
					t.Errorf("t=%v: %d closures built at once, %d grown", at, len(want), len(got))
					continue
				}
				for i := range want {
					if want[i] != got[i] {
						t.Errorf("t=%v differs:\n whole %s\n grown %s", at, want[i], got[i])
						break
					}
				}
			}
			if compared < 6 {
				t.Errorf("only %d closures compared; the test is not holding anything", compared)
			}
		})
	}
}

func TestGrowthDoesNotChangeWhatWasAlreadyClosed(t *testing.T) {
	c := city.NewGrowing(city.ParamsFor(city.MapMotorway, 2))
	c.EnsureAround(mathx.V(0, 0), 300)
	s := works.NewScheduled(c, 2, 1)
	before := fingerprint(s.Closures(500, nil), 250)
	c.EnsureAround(mathx.V(600, 0), 300)
	c.EnsureAround(mathx.V(0, 0), 300)
	after := fingerprint(s.Closures(500, nil), 250)
	if len(before) == 0 {
		t.Skip("nothing closed near the middle of this city")
	}
	for i := range before {
		if i >= len(after) || before[i] != after[i] {
			t.Fatalf("growth revised a closure:\n before %s", before[i])
		}
	}
}

type fixed struct{ cl works.Closure }

func (f fixed) Closures(t float32, dst []works.Closure) []works.Closure { return append(dst, f.cl) }

func TestOtherSourcesJoinTheIndex(t *testing.T) {
	c := city.Generate(city.ParamsFor(city.MapCity, 1))
	ix := works.NewIndex(c, 1, works.Off, fixed{works.Closure{
		Kind: works.KindStreet, Lanes: []int{3}, Phase: works.PhaseSolid,
	}})
	ix.Update(10)
	if !ix.Barred(3) || ix.Barred(4) {
		t.Error("a night event's closure should bar its own lane and no other")
	}
	off := works.NewIndex(c, 1, works.Off)
	off.Update(10)
	if len(off.Active()) != 0 {
		t.Error("an index at the Off level has works")
	}
}

func TestLevelsScaleTheShare(t *testing.T) {
	c := city.Generate(city.ParamsFor(city.MapMotorway, 6))
	count := func(l works.Level) int {
		ix := works.NewIndex(c, 6, l)
		n := 0
		for at := float32(0); at < 9000; at += 450 {
			ix.Update(at)
			n += len(ix.Active())
		}
		return n
	}
	off, normal, dense := count(works.Off), count(works.Normal), count(works.Dense)
	if off != 0 || normal == 0 || dense <= normal {
		t.Errorf("off %d, normal %d, dense %d", off, normal, dense)
	}
}

func TestParseLevel(t *testing.T) {
	for _, l := range []works.Level{works.Off, works.Normal, works.Dense} {
		got, ok := works.ParseLevel(l.Name())
		if !ok || got != l {
			t.Errorf("%q did not round-trip", l.Name())
		}
	}
	if _, ok := works.ParseLevel("lots"); ok {
		t.Error("an unknown level parsed")
	}
}

func TestParamsSetTheShareForEachMap(t *testing.T) {
	if city.ParamsFor(city.MapCity, 1) != city.DefaultParams(1) {
		t.Fatal("ParamsFor(MapCity) must stay equal to DefaultParams")
	}
	for m, want := range map[city.Map]float32{
		city.MapCity: 0.12, city.MapMotorway: 0.12, city.MapVillage: 0.06, city.MapSeaside: 0.06,
	} {
		if got := city.ParamsFor(m, 1).Works; got != want {
			t.Errorf("%s works share = %v, want %v", m.Name(), got, want)
		}
	}
}
