package game

import (
	"image/color"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

func TestEverySeasonLooksLikeItself(t *testing.T) {
	seen := map[color.RGBA]city.Season{}
	for _, s := range city.Seasons() {
		lk := lookFor(s)
		key := toRGBAColor(lk.leaves[0])
		if other, dup := seen[key]; dup {
			t.Errorf("%s and %s have the same trees", s.Name(), other.Name())
		}
		seen[key] = s
		if lk.bare < 0 || lk.blossom < 0 || lk.bare+lk.blossom > 1 {
			t.Errorf("%s has %.2f of its trees bare and %.2f in blossom", s.Name(), lk.bare, lk.blossom)
		}
		if lk.fall > 0 && (lk.fallSpeed <= 0 || lk.fallSize <= 0) {
			t.Errorf("%s has things falling that do not fall", s.Name())
		}
	}
	if lk := lookFor(city.Winter); !lk.snow || lk.fall == 0 || lk.litter {
		t.Error("winter should be under snow, with more coming down")
	}
	if lk := lookFor(city.Autumn); !lk.litter || lk.snow || lk.fall == 0 {
		t.Error("autumn should have leaves coming down and lying about")
	}
	if lk := lookFor(city.Summer); lk.fall != 0 || lk.snow || lk.bare != 0 {
		t.Error("summer should have every tree in leaf and nothing falling")
	}
	if lk := lookFor(city.Spring); lk.blossom == 0 {
		t.Error("spring should have trees in blossom")
	}
}

// The vision system reads the camera by colour, so nothing a season puts in
// the scene may be one of the colours it has reserved for its boxes.
func TestNoSeasonUsesAReservedColour(t *testing.T) {
	reserved := map[color.RGBA]bool{}
	for _, c := range BoxColours() {
		reserved[c] = true
	}
	for _, s := range city.Seasons() {
		for _, c := range lookFor(s).colours() {
			if reserved[toRGBAColor(c)] {
				t.Errorf("%s draws in %v, which a detection box is drawn in", s.Name(), c)
			}
			// The shader holds every channel short of full, and each reserved
			// colour has one at full, so a colour here must not either.
			if c.R == 255 || c.G == 255 || c.B == 255 {
				t.Errorf("%s draws in %v, with a channel at full", s.Name(), c)
			}
		}
	}
}

func TestATreeDoesTheSameThingEveryTime(t *testing.T) {
	for _, s := range city.Seasons() {
		lk := lookFor(s)
		counts := map[treeState]int{}
		for i := range 2000 {
			at := mathx.V(float32(i)*3.7, float32(i%37)*11.3)
			st := lk.treeIn(at)
			if lk.treeIn(at) != st {
				t.Fatalf("a tree in %s changed its mind", s.Name())
			}
			counts[st]++
		}
		bare, blossom := float32(counts[treeBare])/2000, float32(counts[treeBlossom])/2000
		if mathx.Abs(bare-lk.bare) > 0.06 || mathx.Abs(blossom-lk.blossom) > 0.06 {
			t.Errorf("%s: %.2f bare and %.2f in blossom, want %.2f and %.2f",
				s.Name(), bare, blossom, lk.bare, lk.blossom)
		}
	}
}

func TestWhatFallsStaysRoundTheCameraAndComesDown(t *testing.T) {
	for _, eye := range []mathx.Vec{mathx.V(0, 0), mathx.V(-431.5, 977.2), mathx.V(12000, -8000)} {
		for i := range 200 {
			x, y, z := fallAt(i, 12.5, eye, 1.3)
			if mathx.Abs(x-eye.X) > fallReach+0.01 || mathx.Abs(z-eye.Z) > fallReach+0.01 {
				t.Fatalf("piece %d is at %.1f, %.1f with the eye at %v", i, x, z, eye)
			}
			if y < 0 || y > fallHeight {
				t.Fatalf("piece %d is %.1f m up", i, y)
			}
		}
	}
	// A moment later a piece is lower, unless it has just gone round again.
	lower := 0
	for i := range 200 {
		_, y0, _ := fallAt(i, 5, mathx.V(0, 0), 1.3)
		_, y1, _ := fallAt(i, 5.2, mathx.V(0, 0), 1.3)
		if y1 < y0 {
			lower++
		}
	}
	if lower < 190 {
		t.Errorf("only %d of 200 pieces fell", lower)
	}
	// And it stays where it is on the ground as the camera moves a little.
	x0, _, z0 := fallAt(7, 3, mathx.V(0, 0), 1.3)
	x1, _, z1 := fallAt(7, 3, mathx.V(0.5, 0.5), 1.3)
	if mathx.Abs(x0-x1) > 0.01 && mathx.Abs(mathx.Abs(x0-x1)-2*fallReach) > 0.01 {
		t.Errorf("a piece moved %.2f m sideways when the camera did", x1-x0)
	}
	if mathx.Abs(z0-z1) > 0.01 && mathx.Abs(mathx.Abs(z0-z1)-2*fallReach) > 0.01 {
		t.Errorf("a piece moved %.2f m when the camera did", z1-z0)
	}
}

func TestWinterClosesInAndNightStaysDark(t *testing.T) {
	day := lightingFor(TimeDay)
	winter := relight(day, city.Winter)
	if winter.fogRange >= day.fogRange {
		t.Error("a winter day should be closer and hazier than a clear one")
	}
	if winter.sun.X >= day.sun.X {
		t.Error("the winter sun should be weaker")
	}
	if summer := relight(day, city.Summer); summer.fogRange <= day.fogRange {
		t.Error("a summer day should be clearer")
	}
	// Whatever the season, the dark is still the dark: the things that only
	// show at night go by this number.
	night := lightingFor(TimeNight)
	for _, s := range city.Seasons() {
		got := relight(night, s)
		if got.night != night.night {
			t.Errorf("%s changed how dark the night is", s.Name())
		}
		if got.skyTop != night.skyTop {
			t.Errorf("%s changed the night sky", s.Name())
		}
	}
}

// A tier of a tree's crown is never brighter than the leaf it is made of: a
// pale colour pushed past full would wrap round into something else.
func TestACrownIsNeverBrighterThanItsLeaves(t *testing.T) {
	for _, s := range city.Seasons() {
		lk := lookFor(s)
		leaves := append(lk.leaves[:], colBlossom[:]...)
		for _, leaf := range leaves {
			for tier := range 3 {
				got := crownShade(leaf, tier)
				if got.R > leaf.R || got.G > leaf.G || got.B > leaf.B {
					t.Errorf("%s: tier %d of a tree in %v is drawn in %v", s.Name(), tier, leaf, got)
				}
			}
		}
	}
}
