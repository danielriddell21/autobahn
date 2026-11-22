package works_test

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/works"
)

// driven is a growing city that has been driven along a line, as the world
// grows one: a piece at a time, round each point on the way.
func driven(m city.Map, seed uint64, legs int) (*city.City, []mathx.Vec) {
	c := city.NewGrowing(city.ParamsFor(m, seed))
	var path []mathx.Vec
	for i := range legs {
		at := mathx.V(float32(i)*450, float32(i%3)*150)
		c.EnsureAround(at, 700)
		path = append(path, at)
	}
	return c, path
}

// parts is every point of a closure that stands in the world.
func parts(cl works.Closure) []mathx.Vec {
	var out []mathx.Vec
	for _, c := range cl.Cones {
		out = append(out, c.Center)
	}
	for _, b := range cl.Barriers {
		out = append(out, b.Center)
	}
	for _, s := range cl.Signs {
		out = append(out, s.Pos)
	}
	return out
}

func within(cl works.Closure, r float32, at []mathx.Vec) bool {
	for _, p := range parts(cl) {
		for _, f := range at {
			if p.DistTo(f) <= r {
				return true
			}
		}
	}
	return false
}

type key struct {
	e works.Edge
	p works.Phase
}

func keys(cs []works.Closure) []key {
	var out []key
	for _, c := range cs {
		out = append(out, key{c.Edge, c.Phase})
	}
	return out
}

// Looking at the ground round the cars must find exactly what looking at the
// whole town finds there, in the same order, and nothing from the town behind.
func TestFocusedIndexFindsWhatIsInRangeAsLookingAtAllOfItDoes(t *testing.T) {
	const seed, radius = 5, 700
	for _, m := range []city.Map{city.MapCity, city.MapVillage, city.MapMotorway} {
		t.Run(m.Name(), func(t *testing.T) {
			c, path := driven(m, seed, 9)
			all := works.NewIndex(c, seed, works.Dense)
			near := works.NewIndex(c, seed, works.Dense)

			culled, compared := false, 0
			for _, at := range []float32{100, 1000, 2500, 4000} {
				for _, foci := range [][]mathx.Vec{
					{path[len(path)-1]}, {path[2]}, {path[1], path[len(path)-1]},
				} {
					all.Update(at)
					near.Focus(radius, foci...)
					near.Update(at)

					got := keys(near.Active())
					// Every closure with a part in range is there.
					have := map[key]bool{}
					for _, k := range got {
						have[k] = true
					}
					for _, cl := range all.Active() {
						if within(cl, radius, foci) {
							compared++
							if !have[key{cl.Edge, cl.Phase}] {
								t.Fatalf("t=%v: closure %v with a part in range is missing", at, cl.Edge)
							}
						}
					}
					// And what is there is the unfocused list's, in its order.
					i := 0
					for _, k := range keys(all.Active()) {
						if i < len(got) && got[i] == k {
							i++
						}
					}
					if i != len(got) {
						t.Fatalf("t=%v: the focused closures %v are not a run of %v", at, got, keys(all.Active()))
					}
					culled = culled || len(got) < len(all.Active())
				}
			}
			if compared < 10 || !culled {
				t.Errorf("compared %d closures, culled something: %v; the test is not holding anything", compared, culled)
			}
		})
	}
}

// The closures a frame looks at are the ones round the cars, not those of the
// whole drive: driving on makes the set no bigger, though the town is.
func TestFocusedClosuresDoNotGrowWithTheDrive(t *testing.T) {
	const seed = 3
	c := city.NewGrowing(city.ParamsFor(city.MapMotorway, seed))
	ix := works.NewIndex(c, seed, works.Dense)
	sched := works.NewScheduled(c, seed, 1)

	var early, late, widest int
	for i := range 24 {
		at := mathx.V(float32(i)*450, 0)
		c.EnsureAround(at, 700)
		ix.Focus(700, at)
		ix.Update(1000)
		n := len(ix.Active())
		widest = max(widest, n)
		if i == 5 {
			early = sched.Sites()
		}
		if i >= 6 && i < 12 {
			late = max(late, n)
		}
	}
	if sites := sched.Sites(); sites < 3*early {
		t.Fatalf("the town only grew from %d sites to %d: the drive was too short to prove anything", early, sites)
	}
	if widest > 3*late+10 {
		t.Errorf("%d closures in view at the end of the drive against at most %d in the middle of it", widest, late)
	}
	unfocused := works.NewIndex(c, seed, works.Dense)
	unfocused.Update(1000)
	if len(unfocused.Active()) < 3*widest {
		t.Errorf("unfocused %d closures against %d focused: the focus is not cutting anything", len(unfocused.Active()), widest)
	}
}

// BenchmarkUpdateAfterALongDrive is what a frame's roadworks cost once a long
// drive has found most of a town: by the ground round the car, and by the lot.
func BenchmarkUpdateAfterALongDrive(b *testing.B) {
	const seed = 3
	c, path := driven(city.MapMotorway, seed, 30)
	here := path[len(path)-1]
	b.Run("focused", func(b *testing.B) {
		ix := works.NewIndex(c, seed, works.Dense)
		ix.Focus(700, here)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ix.Update(1000 + float32(i%100))
		}
		b.ReportMetric(float64(len(ix.Active())), "closures")
	})
	b.Run("whole town", func(b *testing.B) {
		ix := works.NewIndex(c, seed, works.Dense)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ix.Update(1000 + float32(i%100))
		}
		b.ReportMetric(float64(len(ix.Active())), "closures")
	})
}
