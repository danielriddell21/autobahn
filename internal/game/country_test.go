package game

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

func villageFields(t *testing.T) []city.Block {
	t.Helper()
	c := city.NewGrowing(city.ParamsFor(city.MapVillage, 7))
	c.EnsureAround(mathx.V(0, 0), 1500)
	var out []city.Block
	for _, b := range c.Blocks {
		if b.Park {
			out = append(out, b)
		}
	}
	if len(out) < 20 {
		t.Fatalf("only %d blocks of open ground to look at", len(out))
	}
	return out
}

func TestOnlyTheVillageHasFields(t *testing.T) {
	kinds := map[fieldKind]int{}
	for _, b := range villageFields(t) {
		k := fieldOf(b, city.MapVillage)
		kinds[k]++
		if fieldOf(b, city.MapVillage) != k {
			t.Fatal("a field changed what it grows")
		}
		if fieldOf(b, city.MapCity) != fieldNone || fieldOf(b, city.MapSeaside) != fieldNone {
			t.Fatal("open ground outside the village is a field")
		}
		built := b
		built.Park = false
		if fieldOf(built, city.MapVillage) != fieldNone {
			t.Fatal("a block with houses on it is a field")
		}
	}
	for k := fieldNone; k <= fieldPlough; k++ {
		if kinds[k] == 0 {
			t.Errorf("the village has no open ground of kind %d", k)
		}
	}
	// Most of it is farmed; the green is the exception.
	if kinds[fieldNone]*3 > len(villageFields(t)) {
		t.Errorf("%d of the village's open blocks are greens", kinds[fieldNone])
	}
}

func TestAHedgeKeepsInsideItsFieldAndClearOfRoundabouts(t *testing.T) {
	for _, b := range villageFields(t) {
		runs := hedges(b)
		if len(runs) == 0 {
			t.Fatalf("the field %v..%v has no hedge", b.Min, b.Max)
		}
		for _, r := range runs {
			for _, p := range []mathx.Vec{r.a, r.b} {
				if p.X < b.Min.X+hedgeInset-0.01 || p.X > b.Max.X-hedgeInset+0.01 ||
					p.Z < b.Min.Z+hedgeInset-0.01 || p.Z > b.Max.Z-hedgeInset+0.01 {
					t.Fatalf("a hedge end at %v is outside the field %v..%v", p, b.Min, b.Max)
				}
				for _, c := range b.Corners {
					if c.Bite > 0 && p.DistTo(c.Centre) < c.Bite {
						t.Fatalf("a hedge end at %v is on the roundabout at %v", p, c.Centre)
					}
				}
			}
			if r.a.X != r.b.X && r.a.Z != r.b.Z {
				t.Fatalf("a hedge runs from %v to %v, across the field", r.a, r.b)
			}
		}
	}
}

func TestSheepAndHedgerowTreesStandInTheField(t *testing.T) {
	sheep, trees := 0, 0
	for _, b := range villageFields(t) {
		for _, at := range flock(b) {
			sheep++
			if at.X < b.Min.X+hedgeInset || at.X > b.Max.X-hedgeInset || at.Z < b.Min.Z+hedgeInset || at.Z > b.Max.Z-hedgeInset {
				t.Fatalf("a sheep at %v has got out of the field %v..%v", at, b.Min, b.Max)
			}
		}
		for _, tr := range hedgerow(b) {
			trees++
			if tr.at.X < b.Min.X || tr.at.X > b.Max.X || tr.at.Z < b.Min.Z || tr.at.Z > b.Max.Z {
				t.Fatalf("a hedgerow tree at %v is outside the field %v..%v", tr.at, b.Min, b.Max)
			}
		}
	}
	if sheep == 0 || trees == 0 {
		t.Errorf("%d sheep and %d hedgerow trees in the whole village", sheep, trees)
	}
}

func TestAFieldIsTheColourOfWhatGrowsInIt(t *testing.T) {
	lk := lookFor(city.Spring)
	if fieldShade(fieldPasture, lk) != lk.park || fieldShade(fieldNone, lk) != lk.park {
		t.Error("pasture and the green should be the season's grass")
	}
	if fieldShade(fieldCrop, lk) == lk.park || fieldShade(fieldPlough, lk) == fieldShade(fieldCrop, lk) {
		t.Error("a crop and a ploughed field should each be their own colour")
	}
}
