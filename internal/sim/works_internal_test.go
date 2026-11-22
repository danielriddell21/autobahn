package sim

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/works"
)

// solidWorld is a dense world set to a moment when roadworks are standing.
func solidWorld(t *testing.T, m city.Map, seed uint64, traffic int) *World {
	t.Helper()
	w := NewWorld(Config{Seed: seed, Map: m, Traffic: traffic, Police: 0, Works: works.Dense})
	for at := float32(100); at < 20000; at += 130 {
		w.Time = at
		w.syncWorks()
		if len(w.solid) >= 6 {
			return w
		}
	}
	t.Fatalf("%s: no roadworks standing at any time", m.Name())
	return nil
}

func TestOpenTurnsLeaveOutBarredLanes(t *testing.T) {
	w := solidWorld(t, city.MapCity, 3, 0)
	found := false
	for _, cl := range w.Works.Active() {
		if cl.Kind != works.KindStreet {
			continue
		}
		barred := map[int]bool{}
		for _, id := range cl.Lanes {
			barred[id] = true
		}
		for _, l := range w.City.Lanes {
			open := openTurns(w.Works, l)
			if len(open) == 0 && len(l.Succ) > 0 {
				t.Fatalf("lane %d was left with no turn at all", l.ID)
			}
			for _, tn := range l.Succ {
				if barred[tn.Lane] {
					found = true
				}
			}
			if len(open) < len(l.Succ) {
				for _, tn := range open {
					if barred[tn.Lane] {
						t.Errorf("lane %d is offered a turn into barred lane %d", l.ID, tn.Lane)
					}
				}
			}
		}
	}
	if !found {
		t.Skip("no lane leads into a closed street at this moment")
	}
}

func TestOpenTurnsWithNoWorksIsTheLanesOwn(t *testing.T) {
	c := city.Generate(city.ParamsFor(city.MapCity, 1))
	for _, l := range c.Lanes {
		if got := openTurns(nil, l); len(got) != len(l.Succ) {
			t.Fatalf("lane %d: %d turns of %d with no works", l.ID, len(got), len(l.Succ))
		}
	}
}

// TestTrafficAndRouteNeverTakeAClosedTurn drives a busy world through several
// closures and checks nobody has chosen to enter a street that is barred.
func TestTrafficAndRouteNeverTakeAClosedTurn(t *testing.T) {
	for _, m := range []city.Map{city.MapCity, city.MapMotorway} {
		w := solidWorld(t, m, 5, 60)
		for i, a := range w.Agents {
			a.works = w.Works
			if a.closedTurn() {
				a.chooseNext(w.City)
			}
			_ = i
		}
		entered := 0
		for step := 0; step < 1500; step++ {
			w.Update(Controls{}, 1.0/30)
			for _, a := range w.Agents {
				if a.closedTurn() {
					t.Fatalf("%s: agent %d has chosen turn into barred lane %d at t=%.0f", m.Name(), a.ID, a.next.Lane, w.Time)
				}
				if a.inTurn && w.Works.Barred(a.next.Lane) {
					entered++
				}
			}
			for _, tn := range w.Route.plan {
				if w.Works.Barred(tn.Lane) && !w.Route.crossing {
					t.Fatalf("%s: the route plans a turn into barred lane %d", m.Name(), tn.Lane)
				}
			}
		}
		// A turn made before the signs went up may still be finishing, but the
		// closures come and go slowly: nobody should be doing it by the dozen.
		if entered > 30 {
			t.Errorf("%s: %d agent-steps spent turning into closed streets", m.Name(), entered)
		}
	}
}

func TestRouteReplansWhenItsStreetCloses(t *testing.T) {
	w := solidWorld(t, city.MapCity, 3, 0)
	r := NewRoute(w.City, w.rng)
	r.works = w.Works
	for _, cl := range w.Works.Active() {
		if cl.Kind != works.KindStreet {
			continue
		}
		// A lane that leads into the closed street.
		for _, l := range w.City.Lanes {
			for _, tn := range l.Succ {
				if tn.Lane != cl.Lanes[0] {
					continue
				}
				r.lane, r.plan = l, []city.Turn{tn}
				r.revalidate()
				if len(r.plan) != 0 {
					t.Fatal("a plan into a closed street survived")
				}
				return
			}
		}
	}
	t.Skip("no lane leads into a closed street at this moment")
}

func TestConesAreSolid(t *testing.T) {
	w := solidWorld(t, city.MapMotorway, 5, 0)
	var cone mathx.OBB
	for _, cl := range w.Works.Active() {
		if len(cl.Cones) > 0 && cl.Phase == works.PhaseSolid {
			cone = cl.Cones[0]
		}
	}
	if cone == (mathx.OBB{}) {
		t.Skip("no cones standing at this moment")
	}
	v := NewVehicle(CarSpec(), cone.Center.Add(mathx.FromAngle(cone.Heading).Mul(-1.5)), cone.Heading)
	v.Vel = mathx.FromAngle(cone.Heading).Mul(8)
	for range 60 {
		v.Update(Controls{Throttle: 1}, 1.0/60)
		w.collideWorks(v, false)
		if _, depth, hit := v.Box().Penetration(cone); hit && depth > 0.05 {
			t.Fatalf("a car is sitting %.2f m inside a cone", depth)
		}
	}
	if v.Pos.Sub(cone.Center).Dot(mathx.FromAngle(cone.Heading)) > 0 {
		t.Error("a car drove straight through a cone")
	}
}

func TestTrafficQueuesBehindTheFirstCone(t *testing.T) {
	w := solidWorld(t, city.MapMotorway, 5, 0)
	for _, cl := range w.Works.Active() {
		if len(cl.Cones) == 0 {
			continue
		}
		l := w.City.Lanes[cl.Lanes[0]]
		a := NewAgent(1, w.City, l, 0, w.rng)
		a.works = w.Works
		w.Agents = append(w.Agents, a)
		first := l.Along(cl.Cones[0].Center)
		start := first - 40
		if start < 1 {
			continue
		}
		a.s = start
		a.V.Place(l.Point(start), l.HeadingAt(start))
		a.V.Vel = l.DirAt(start).Mul(8)
		a.chooseNext(w.City)
		for range 900 {
			w.Update(Controls{}, 1.0/30)
			w.Player.Place(mathx.V(1e5, 1e5), 0) // out of the way
			if a.lane != l {
				break // it moved over, which is what it should do
			}
		}
		if a.lane == l && l.Along(a.V.Pos) > first {
			t.Fatalf("a car drove past the first cone without moving over (s=%.1f, cone at %.1f)", l.Along(a.V.Pos), first)
		}
		return
	}
	t.Skip("no coned lane long enough at this moment")
}

// A frame's roadworks are the ground round the cars: driving on, with the town
// left behind growing without end, must not make the set any bigger. This is
// what the stubs, the solids and every agent's scan of them cost.
func TestRoadworksInViewDoNotGrowWithTheDrive(t *testing.T) {
	w := NewWorld(Config{Seed: 3, Map: city.MapMotorway, Traffic: 10, Works: works.Dense})
	var early, late int
	for i := range 30 {
		w.Player.Place(mathx.V(float32(i)*450, 0), 0)
		w.Time = 1000 // the same moment everywhere, so only the place differs
		w.Update(Controls{}, 1.0/60)
		n := len(w.stubs) + len(w.solid)
		if i >= 3 && i < 9 {
			early = max(early, n)
		}
		if i >= 24 {
			late = max(late, n)
		}
	}
	if early == 0 {
		t.Fatal("no roadworks in view early in the drive: the test is not holding anything")
	}
	if late > 3*early+10 {
		t.Errorf("%d stubs and solids in view after a long drive against at most %d early on", late, early)
	}

	// The same town with every closure looked at, which is what a frame cost.
	w.Works.Focus(0)
	w.Works.Update(w.Time)
	if all := len(w.Works.Solid(nil)); all < 3*late {
		t.Errorf("%d solid in the whole town against %d in view: nothing was cut", all, late)
	}
}

// Nothing is in the road until a closure is solid, and traffic does not queue
// behind what is not there.
func TestNoStubsBeforeAClosureIsSolid(t *testing.T) {
	w := solidWorld(t, city.MapMotorway, 5, 0)
	// Find a moment with a signed closure and no solid one near the player.
	signed := false
	for at := float32(0); at < 20000 && !signed; at += 7 {
		w.Time = at
		w.syncWorks()
		solid := 0
		for _, cl := range w.Works.Active() {
			switch cl.Phase {
			case works.PhaseSolid:
				solid += len(cl.Barriers)
				if len(cl.Cones) > 0 {
					solid++
				}
			case works.PhaseSigned:
				signed = true
			}
		}
		if signed {
			if len(w.stubs) != solid {
				t.Fatalf("t=%.0f: %d stubs for %d solid closure parts: a signed closure has stubs", at, len(w.stubs), solid)
			}
		}
	}
	if !signed {
		t.Skip("no closure was in its signed phase at any moment tried")
	}
}
