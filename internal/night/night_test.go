package night_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/night"
	"github.com/danielriddell21/autobahn/internal/works"
)

func TestEveningWindowRunsDuskToOne(t *testing.T) {
	for _, tc := range []struct {
		h    float32
		n    int
		want works.Phase
	}{
		{12, 0, works.PhaseClear},
		{18.5, 0, works.PhaseClear},
		{18.8, 0, works.PhaseSigned},
		{19, 0, works.PhaseSolid},
		{23.9, 0, works.PhaseSolid},
		{24.5, 0, works.PhaseSolid},
		{24.99, 0, works.PhaseSolid},
		{25.01, 0, works.PhaseClear},
		{30, 1, works.PhaseClear},
		{24 + 19.5, 1, works.PhaseSolid},
		{-2, -1, works.PhaseSolid},
	} {
		n, ph := night.Evening(tc.h)
		if ph != tc.want || (ph != works.PhaseClear && n != tc.n) {
			t.Errorf("hour %v: night %d phase %d, want night %d phase %d", tc.h, n, ph, tc.n, tc.want)
		}
	}
}

func TestNightsDifferAndShareIsRight(t *testing.T) {
	on, total := 0, 0
	varied := false
	for i := -15; i < 15; i++ {
		for j := -15; j < 15; j++ {
			cell := [2]int{i, j}
			first := night.On(3, cell, 0)
			for n := range 20 {
				total++
				if night.On(3, cell, n) {
					on++
				}
				if night.On(3, cell, n) != first {
					varied = true
				}
			}
		}
	}
	if !varied {
		t.Error("every night is the same")
	}
	got := float32(on) / float32(total)
	if got < night.Share*0.9 || got > night.Share*1.1 {
		t.Errorf("share %.3f, want about %.2f", got, night.Share)
	}
	kinds := map[night.Kind]bool{}
	for i := range 200 {
		kinds[night.KindOf(5, [2]int{i, -i}, i)] = true
	}
	if len(kinds) != 4 {
		t.Errorf("only %d of 4 kinds turn up", len(kinds))
	}
}

// fingerprint describes the venues and closures within reach of the middle.
func fingerprint(ix *night.Index, reach float32) []string {
	ix.Update()
	var out []string
	for _, s := range ix.Sites() {
		if s.Pos.Len() > reach {
			continue
		}
		str := fmt.Sprintf("%v %.2f,%.2f/%.3f r%.1f closes=%v", s.Cell, s.Pos.X, s.Pos.Z, s.Turn, s.Radius, s.Closes)
		if s.Closes {
			str += fmt.Sprintf(" %v", s.Closure.Edge)
			for _, b := range s.Closure.Barriers {
				str += fmt.Sprintf(" B%.2f,%.2f", b.Center.X, b.Center.Z)
			}
		}
		for _, o := range s.Cars {
			str += fmt.Sprintf(" K%.2f,%.2f", o.Center.X, o.Center.Z)
		}
		out = append(out, str)
	}
	sort.Strings(out)
	return out
}

func TestGrownVenuesMatchThoseBuiltAtOnce(t *testing.T) {
	const seed, span = 7, 700
	clk := night.Fixed(22)
	for _, m := range city.Maps() {
		t.Run(m.Name(), func(t *testing.T) {
			whole := city.NewGrowing(city.ParamsFor(m, seed))
			whole.EnsureAround(mathx.V(0, 0), span)
			grown := city.NewGrowing(city.ParamsFor(m, seed))
			wi := night.NewIndex(whole, seed, clk)
			gi := night.NewIndex(grown, seed, clk)
			for _, at := range []mathx.Vec{
				mathx.V(-300, -300), mathx.V(300, -300), mathx.V(-300, 300), mathx.V(300, 300), mathx.V(0, 0),
			} {
				grown.EnsureAround(at, span/2)
				gi.Update()
			}
			grown.EnsureAround(mathx.V(0, 0), span)

			reach := float32(1e9)
			if whole.Laid() {
				reach = span * 0.55
			}
			want, got := fingerprint(wi, reach), fingerprint(gi, reach)
			if len(want) < 1 {
				t.Fatalf("only %d venues to compare", len(want))
			}
			if len(want) != len(got) {
				t.Fatalf("%d venues built at once, %d grown", len(want), len(got))
			}
			for i := range want {
				if want[i] != got[i] {
					t.Fatalf("venue differs:\n whole %s\n grown %s", want[i], got[i])
				}
			}
		})
	}
}

func TestVenuesStandOnlyOnCompleteBlocks(t *testing.T) {
	for _, m := range city.Maps() {
		c := city.NewGrowing(city.ParamsFor(m, 11))
		c.EnsureAround(mathx.V(0, 0), 400)
		ix := night.NewIndex(c, 11, night.Fixed(22))
		ix.Update()
		plan := c.Plan()
		for _, s := range ix.Sites() {
			if !c.Laid() {
				inside := false
				for _, b := range plan.Blocks {
					if s.Pos.X > b.Min.X && s.Pos.X < b.Max.X && s.Pos.Z > b.Min.Z && s.Pos.Z < b.Max.Z {
						inside = !b.Park && !b.Beach
					}
				}
				if !inside {
					t.Errorf("%s: venue %v is not in a built, non-park block", m.Name(), s.Cell)
				}
			}
			if s.Half.X < 15 || s.Half.Z < 15 {
				t.Errorf("%s: venue %v is on a block %v wide", m.Name(), s.Cell, s.Half)
			}
		}
		// A city that has been grown only a little has no venue on its frontier.
		small := city.NewGrowing(city.ParamsFor(m, 11))
		small.EnsureAround(mathx.V(0, 0), 60)
		si := night.NewIndex(small, 11, night.Fixed(22))
		si.Update()
		if len(si.Sites()) > len(ix.Sites()) {
			t.Errorf("%s: a small city has more venues than a large one", m.Name())
		}
	}
}

func TestClosuresGoThroughTheWorksAndKeepTheirRules(t *testing.T) {
	total := 0
	for _, m := range city.Maps() {
		c := city.Generate(city.ParamsFor(m, 21))
		ix := night.NewIndex(c, 21, night.Fixed(22))
		w := works.NewIndex(c, 21, works.Off, ix)
		w.Update(10)
		var cl []works.Closure
		cl = ix.Closures(10, cl)
		if len(w.Active()) != len(cl) {
			t.Fatalf("%s: index has %d closures, source gave %d", m.Name(), len(w.Active()), len(cl))
		}
		closed := map[int]bool{}
		arms := map[int]int{}
		for _, k := range cl {
			if k.Phase != works.PhaseSolid || k.Start > k.Solid || k.Solid > k.End || k.End < 10 || k.Start > 10 {
				t.Errorf("%s: closure phase %d times %v %v %v at t=10", m.Name(), k.Phase, k.Start, k.Solid, k.End)
			}
			if k.Kind != works.KindStreet {
				continue
			}
			total++
			r := c.Roads[k.Road]
			arms[r.A]++
			arms[r.B]++
			for _, id := range k.Lanes {
				closed[id] = true
				if !w.Barred(id) {
					t.Errorf("%s: lane %d is not barred through the works index", m.Name(), id)
				}
			}
			if c.Nodes[r.A].Roundabout || c.Nodes[r.B].Roundabout {
				t.Errorf("%s: closed beside a roundabout", m.Name())
			}
		}
		for n, k := range arms {
			if k > 1 {
				t.Errorf("%s: junction %d has %d closed arms", m.Name(), n, k)
			}
		}
		for _, l := range c.Lanes {
			if len(l.Succ) == 0 || c.Roads[l.Road].Ring || closed[l.ID] {
				continue
			}
			open := false
			for _, tn := range l.Succ {
				open = open || !closed[tn.Lane]
			}
			if !open {
				t.Errorf("%s: lane %d has no exit that is not closed", m.Name(), l.ID)
			}
		}
	}
	if total == 0 {
		t.Error("no street closures to test")
	}
}

func TestNothingByDayAndSignsBeforeDusk(t *testing.T) {
	c := city.Generate(city.ParamsFor(city.MapCity, 21))
	hour := float32(12)
	ix := night.NewIndex(c, 21, func() float32 { return hour })
	if got := ix.Closures(0, nil); len(got) != 0 || len(ix.Events()) != 0 || ix.SpeedScale(mathx.V(0, 0)) != 1 {
		t.Error("events by day")
	}
	hour = 18.9
	signed := ix.Closures(0, nil)
	if len(signed) == 0 {
		t.Fatal("no closures in the quarter hour before dusk")
	}
	for _, k := range signed {
		if k.Phase != works.PhaseSigned {
			t.Errorf("phase %d before dusk", k.Phase)
		}
	}
	for _, s := range ix.Sites() {
		if ix.SpeedScale(s.Pos) != 1 {
			t.Fatal("a crowd before dusk")
		}
	}
	hour = 26
	if len(ix.Closures(0, nil)) != 0 {
		t.Error("closures after one in the morning")
	}
}

func TestCrowdSlowsTrafficOnlyNearAnEvent(t *testing.T) {
	c := city.Generate(city.ParamsFor(city.MapCity, 5))
	var ix *night.Index
	var evs []night.Event
	for n := range 20 {
		ix = night.NewIndex(c, 5, night.Fixed(float32(n*24+22)))
		ix.Update()
		if evs = ix.Events(); len(evs) == 1 {
			break
		}
	}
	if len(evs) != 1 {
		t.Fatalf("no night with exactly one event in 20 tries (%d)", len(evs))
	}
	e := evs[0]
	if got := ix.SpeedScale(e.Pos); got >= 1 {
		t.Errorf("no slowing at the heart of an event: %v", got)
	}
	// Far from every event, nothing.
	far := mathx.V(1e5, 1e5)
	if got := ix.SpeedScale(far); got != 1 {
		t.Errorf("slowed %v a hundred kilometres away", got)
	}
	// And it thins out monotonically with distance from a lone event.
	prev := ix.SpeedScale(e.Pos)
	for d := float32(0); d <= e.Radius+night.Fade+1; d += 10 {
		p := e.Pos.Add(mathx.V(d, 0))
		got := ix.SpeedScale(p)
		if got < prev-1e-5 {
			t.Errorf("crowd thickens with distance at %v", d)
		}
		prev = got
	}
}

func TestShowCarsOnlyForAMeetInForce(t *testing.T) {
	c := city.Generate(city.ParamsFor(city.MapCity, 8))
	var meets, others int
	for n := range 12 {
		hour := float32(n*24 + 22)
		ix := night.NewIndex(c, 8, func() float32 { return hour })
		ix.Update()
		cars := ix.Cars(nil)
		want := 0
		for _, e := range ix.Events() {
			if e.Kind == night.Meet {
				want += len(e.Cars)
				meets++
			} else {
				others++
			}
		}
		if len(cars) != want {
			t.Fatalf("night %d: %d show cars, want %d", n, len(cars), want)
		}
	}
	if meets == 0 || others == 0 {
		t.Errorf("meets %d, others %d: both should turn up", meets, others)
	}
	by := night.NewIndex(c, 8, night.Fixed(12))
	by.Update()
	if len(by.Cars(nil)) != 0 {
		t.Error("show cars by day")
	}
}
