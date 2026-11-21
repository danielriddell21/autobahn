package city_test

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// laidMaps are the maps whose ground wanders from their plan.
func laidMaps(t *testing.T) []city.Map {
	t.Helper()
	var out []city.Map
	for _, m := range city.Maps() {
		if city.ParamsFor(m, 7).Wind > 0 {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		t.Skip("no map is laid differently from its plan")
	}
	return out
}

func grown(m city.Map, seed uint64) *city.City {
	c := city.NewGrowing(city.ParamsFor(m, seed))
	c.EnsureAround(mathx.V(0, 0), 900)
	return c
}

func TestTheCityIsLaidAsItWasDrawn(t *testing.T) {
	c := build(7)
	if c.Laid() || c.Plan() != c {
		t.Fatal("the city's ground wanders from its plan")
	}
	p := mathx.V(123.4, -567.8)
	if on, turn := c.Lay(p); on != p || turn != 0 {
		t.Errorf("Lay(%v) = %v turned %v in a place laid as drawn", p, on, turn)
	}
	for _, l := range c.Lanes {
		if l.Bend(0, l.Length) != 0 {
			t.Fatalf("lane %d in the city bends", l.ID)
		}
	}
}

// Where a point lies depends on where it is and nothing else: not on what
// has been built, nor on the order it was built in.
func TestWhereAPointLiesDoesNotDependOnWhatIsBuilt(t *testing.T) {
	for _, m := range laidMaps(t) {
		empty := city.NewGrowing(city.ParamsFor(m, 7))
		full := grown(m, 7)
		if !full.Laid() || full.Plan() == full {
			t.Fatalf("%s is not laid", m.Name())
		}
		for i := range 400 {
			p := mathx.V(float32(i%20)*61-580, float32(i/20)*47-420)
			a, at := empty.Lay(p)
			b, bt := full.Lay(p)
			if a != b || at != bt {
				t.Fatalf("%s: %v lies at %v before anything is built and %v after", m.Name(), p, a, b)
			}
		}
	}
}

// A street keeps its width round a bend: two points the plan puts either
// side of its centre line lie exactly as far apart on the ground. And the
// ground is all of a piece across it, since both sides are laid against it.
func TestAStreetKeepsItsWidthRoundABend(t *testing.T) {
	for _, m := range laidMaps(t) {
		c := grown(m, 7)
		plan := c.Plan()
		checked := 0
		for _, r := range plan.Roads {
			if r.Ring {
				continue
			}
			a := plan.Nodes[r.A].Pos
			// Clear of the junctions at either end, where a point to one side
			// may be nearer the street round the corner than this one.
			for d := float32(24); d < r.Length-24; d += 13 {
				mid := a.Add(r.Dir.Mul(d))
				for _, off := range []float32{0.01, 3.6, 9, 18} {
					left, _ := c.Lay(mid.Sub(r.Dir.Right().Mul(off)))
					right, _ := c.Lay(mid.Add(r.Dir.Right().Mul(off)))
					if got := left.DistTo(right); mathx.Abs(got-2*off) > 0.02 {
						t.Fatalf("%s: two points %.2f m apart across a street lie %.2f m apart", m.Name(), 2*off, got)
					}
				}
				checked++
			}
		}
		if checked < 60 {
			t.Fatalf("%s: only %d places along its streets were looked at", m.Name(), checked)
		}
	}
}

// A junction is laid rigidly: exactly the shape the plan drew, moved and
// turned. So its arms are still square to each other.
func TestAJunctionIsLaidSquare(t *testing.T) {
	for _, m := range laidMaps(t) {
		c := grown(m, 7)
		plan := c.Plan()
		turned := 0
		for _, n := range c.Nodes {
			if !c.Complete(n) {
				continue
			}
			at := plan.Nodes[n.ID].Pos
			const r = 11
			east, _ := c.Lay(at.Add(mathx.V(r, 0)))
			south, _ := c.Lay(at.Add(mathx.V(0, r)))
			e, s := east.Sub(n.Pos), south.Sub(n.Pos)
			if mathx.Abs(e.Len()-r) > 0.02 || mathx.Abs(s.Len()-r) > 0.02 {
				t.Fatalf("%s: a junction's arms are %.2f and %.2f m long at %d m", m.Name(), e.Len(), s.Len(), r)
			}
			if dot := e.Norm().Dot(s.Norm()); mathx.Abs(dot) > 0.01 {
				t.Fatalf("%s: a junction's arms meet at a cosine of %.3f, not square", m.Name(), dot)
			}
			if mathx.Abs(mathx.AngleDiff(e.Angle(), n.Rot)) > 0.01 {
				t.Fatalf("%s: a junction says it has turned %.3f and its arm points %.3f", m.Name(), n.Rot, e.Angle())
			}
			if mathx.Abs(n.Rot) > 0.02 {
				turned++
			}
		}
		if turned == 0 {
			t.Errorf("%s: no junction has turned at all", m.Name())
		}
	}
}

func TestLaidLanesAreCurvesThatCanBeDriven(t *testing.T) {
	for _, m := range laidMaps(t) {
		c := grown(m, 7)
		plan := c.Plan()
		if len(c.Lanes) != len(plan.Lanes) || len(c.Nodes) != len(plan.Nodes) || len(c.Buildings) != len(plan.Buildings) {
			t.Fatalf("%s: the ground and the plan disagree about how much there is", m.Name())
		}
		bent, tightest := 0, float32(0)
		for _, l := range c.Lanes {
			pl := plan.Lanes[l.ID]
			a, _ := c.Lay(pl.A)
			b, _ := c.Lay(pl.B)
			if l.A.DistTo(a) > 0.01 || l.B.DistTo(b) > 0.01 {
				t.Fatalf("%s: lane %d does not start and end where the plan's lies", m.Name(), l.ID)
			}
			if l.Point(0).DistTo(l.A) > 0.01 || l.Point(l.Length).DistTo(l.B) > 0.01 {
				t.Fatalf("%s: lane %d does not run from its start to its end", m.Name(), l.ID)
			}
			if l.Length < l.A.DistTo(l.B)-0.01 {
				t.Fatalf("%s: lane %d is %.2f m long between ends %.2f m apart", m.Name(), l.ID, l.Length, l.A.DistTo(l.B))
			}
			if l.Fwd.Dot(l.DirAt(l.Length)) < 0.999 || mathx.Abs(l.Fwd.Len()-1) > 0.001 {
				t.Fatalf("%s: lane %d does not point the way it ends", m.Name(), l.ID)
			}
			// Somewhere along it, the lane finds itself: a point on it is that
			// far along and no way to the side, and one to its right is to its
			// right.
			s := l.Length * 0.4
			on := l.Point(s)
			if got, lat := l.Locate(on); mathx.Abs(got-s) > 0.05 || mathx.Abs(lat) > 0.01 {
				t.Fatalf("%s: lane %d puts its own point at %.2f along and %.2f aside, want %.2f and 0", m.Name(), l.ID, got, lat, s)
			}
			if _, lat := l.Locate(on.Add(l.DirAt(s).Right().Mul(1.5))); mathx.Abs(lat-1.5) > 0.05 {
				t.Fatalf("%s: lane %d puts a point 1.5 m to its right %.2f m aside", m.Name(), l.ID, lat)
			}
			if l.Run(l.B.Add(l.Fwd.Mul(4))) < l.Length+3.9 || l.Run(l.A.Sub(l.StartDir().Mul(4))) > -3.9 {
				t.Fatalf("%s: lane %d does not carry on past its ends", m.Name(), l.ID)
			}
			if c.Roads[l.Road].Ring {
				continue
			}
			bend := l.Bend(0, l.Length)
			tightest = max(tightest, bend)
			if bend > 1e-4 {
				bent++
			}
		}
		if bent < len(c.Lanes)/4 {
			t.Errorf("%s: only %d of %d lanes bend", m.Name(), bent, len(c.Lanes))
		}
		// Nothing so tight a car could not get round it at a crawl.
		if tightest > 1.0/12 {
			t.Errorf("%s: the tightest bend has a radius of %.1f m", m.Name(), 1/tightest)
		}
		t.Logf("%s: %d of %d lanes bend, the tightest to a radius of %.0f m", m.Name(), bent, len(c.Lanes), 1/tightest)
	}
}

// Two lanes side by side stay a lane's width apart all the way along, and a
// turn through a junction starts where one lane ends and ends where the next
// begins.
func TestLanesStayALaneApartAndTurnsJoinUp(t *testing.T) {
	for _, m := range laidMaps(t) {
		c := grown(m, 7)
		pairs, turns := 0, 0
		for _, r := range c.Roads {
			if r.Ring {
				continue
			}
			for _, ids := range [][]int{r.LanesAB, r.LanesBA} {
				if len(ids) < 2 {
					continue
				}
				inner, outer := c.Lanes[ids[0]], c.Lanes[ids[1]]
				for _, f := range []float32{0.1, 0.5, 0.9} {
					on := inner.Point(inner.Length * f)
					if _, lat := outer.Locate(on); mathx.Abs(mathx.Abs(lat)-city.LaneWidth) > 0.06 {
						t.Fatalf("%s: two lanes of road %d are %.2f m apart, want %.2f", m.Name(), r.ID, mathx.Abs(lat), city.LaneWidth)
					}
				}
				pairs++
			}
		}
		for _, l := range c.Lanes {
			for _, turn := range l.Succ {
				turns++
				if c.TurnPoint(l, turn, 0).DistTo(l.B) > 0.01 || c.TurnPoint(l, turn, 1).DistTo(c.Lanes[turn.Lane].A) > 0.01 {
					t.Fatalf("%s: a turn from lane %d does not join it to lane %d", m.Name(), l.ID, turn.Lane)
				}
				if turn.Len < l.B.DistTo(c.Lanes[turn.Lane].A)-0.01 {
					t.Fatalf("%s: a turn from lane %d is shorter than the gap it crosses", m.Name(), l.ID)
				}
			}
		}
		if turns == 0 {
			t.Fatalf("%s: no lane leads anywhere", m.Name())
		}
		if pairs == 0 && m != city.MapVillage {
			t.Errorf("%s: no road with two lanes each way to look at", m.Name())
		}
	}
}

// Whatever stands beside a road still stands beside it once the road bends:
// no building reaches the carriageway, and no parked car stands in a lane.
func TestNothingIsLaidInTheRoad(t *testing.T) {
	for _, m := range laidMaps(t) {
		c := grown(m, 7)
		for _, b := range c.Buildings {
			if b.Center.Len() > 700 {
				continue
			}
			along, across := mathx.FromAngle(b.Heading).Mul(b.W/2), mathx.FromAngle(b.Heading).Right().Mul(b.D/2)
			for _, corner := range []mathx.Vec{
				b.Center.Add(along).Add(across), b.Center.Add(along).Sub(across),
				b.Center.Sub(along).Add(across), b.Center.Sub(along).Sub(across),
			} {
				if pr := c.Project(corner); pr.Valid && pr.Dist < city.LaneWidth/2+1.5 {
					t.Fatalf("%s: the corner of a building at %v is %.2f m from the middle of a lane", m.Name(), corner, pr.Dist)
				}
			}
		}
		for _, p := range c.Parked {
			if pr := c.Project(p.Pos); pr.Valid && pr.Dist < city.LaneWidth/2+1.2 {
				t.Fatalf("%s: a car is parked %.2f m from the middle of a lane", m.Name(), pr.Dist)
			}
		}
		if len(c.Parked) == 0 || len(c.Buildings) == 0 {
			t.Fatalf("%s: nothing was laid beside its roads at all", m.Name())
		}
	}
}

// The land does not wander by the sea, so a seafront is as straight as it
// was drawn and the beach below it is where the plan put it.
func TestTheLandIsStillByTheSea(t *testing.T) {
	c := grown(city.MapSeaside, 7)
	if !c.Laid() {
		t.Skip("the seaside is laid as drawn")
	}
	front, sea, _ := c.Shoreline()
	for x := float32(-600); x <= 600; x += 37 {
		for _, z := range []float32{front - 20, front, front + 20, sea, sea + 100} {
			p := mathx.V(x, z)
			if on, turn := c.Lay(p); on.DistTo(p) > 0.001 || mathx.Abs(turn) > 0.001 {
				t.Fatalf("the ground at %v, by the sea, has wandered to %v", p, on)
			}
		}
	}
	inland, _ := c.Lay(mathx.V(100, front-700))
	if inland.DistTo(mathx.V(100, front-700)) < 1 {
		t.Error("the ground well inland has not wandered at all")
	}
	for _, l := range c.Lanes {
		if mathx.Abs(l.A.Z-front) < 8 && mathx.Abs(l.B.Z-front) < 8 && !c.Roads[l.Road].Ring && l.Bend(0, l.Length) > 1e-4 {
			t.Fatalf("lane %d along the front bends", l.ID)
		}
	}
}

// A sign or a signal is turned with the junction it stands at, so it still
// faces the traffic it governs.
func TestSignsFaceTheTrafficTheyGovern(t *testing.T) {
	for _, m := range laidMaps(t) {
		c := grown(m, 7)
		checked := 0
		for _, p := range c.Props {
			if p.Kind != city.PropStopSign && p.Kind != city.PropGiveWaySign {
				continue
			}
			pr := c.ProjectAlong(p.Pos, mathx.FromAngle(p.Heading))
			if !pr.Valid || pr.Dist > 12 {
				continue
			}
			checked++
			if dot := mathx.FromAngle(p.Heading).Dot(pr.Lane.DirAt(pr.S)); dot < 0.995 {
				t.Fatalf("%s: a sign at %v faces a cosine of %.4f off the lane it governs", m.Name(), p.Pos, dot)
			}
		}
		if checked == 0 && m != city.MapMotorway {
			t.Errorf("%s: no sign was found beside a lane to check", m.Name())
		}
	}
}
