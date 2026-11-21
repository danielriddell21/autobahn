package game

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

func TestTheRoofFollowsTheHeight(t *testing.T) {
	c := city.Generate(city.DefaultParams(7))
	seen := map[roofKind]int{}
	for _, b := range c.Buildings {
		k := roofOf(b)
		seen[k]++
		switch {
		case b.Height > towerHeight && k != roofTower:
			t.Fatalf("a building %.0f m tall has roof %d, want a tower", b.Height, k)
		case k == roofTower && b.Height <= towerHeight:
			t.Fatalf("a building %.0f m tall has a tower on it", b.Height)
		case k == roofGable && b.Height >= gableHeight:
			t.Fatalf("a building %.0f m tall has a pitched roof", b.Height)
		case k == roofStepped && b.Height < steppedHeight:
			t.Fatalf("a building %.0f m tall is stepped", b.Height)
		}
		if roofOf(b) != k {
			t.Fatal("the same building got two roofs")
		}
	}
	for k := roofFlat; k <= roofTower; k++ {
		if seen[k] == 0 {
			t.Errorf("no building in the city has roof %d", k)
		}
	}
}

func TestStreetFurnitureStandsOnThePavement(t *testing.T) {
	c := city.Generate(city.DefaultParams(7))
	kinds := map[furnitureKind]int{}
	for _, r := range c.Roads {
		got := furnish(c, r)
		if r.Ring && len(got) != 0 {
			t.Fatalf("a roundabout has %d pieces of furniture on it", len(got))
		}
		again := furnish(c, r)
		if len(again) != len(got) {
			t.Fatalf("road %d was furnished twice and came out different", r.ID)
		}
		for i, f := range got {
			if f != again[i] {
				t.Fatalf("road %d was furnished twice and came out different", r.ID)
			}
			kinds[f.kind]++
			// Beyond the kerb, short of the shop fronts, clear of the ends.
			rel := f.at.Sub(c.Nodes[r.A].Pos)
			off, along := mathx.Abs(rel.Dot(r.Dir.Right())), rel.Dot(r.Dir)
			if off < r.HalfWidth+city.BayDepth || off > r.HalfWidth+city.PavementWidth {
				t.Errorf("road %d: furniture %.2f m from the centre line of a road %.2f m to the kerb",
					r.ID, off, r.HalfWidth)
			}
			if along < c.Nodes[r.A].Radius+furnMargin-0.01 || along > r.Length-c.Nodes[r.B].Radius-furnMargin {
				t.Errorf("road %d: furniture %.1f m along a road %.1f long", r.ID, along, r.Length)
			}
			if f.kind == furnShelter && r.Class == city.Local {
				t.Errorf("road %d is a side street with a bus shelter on it", r.ID)
			}
			if f.out.Dot(rel) <= 0 {
				t.Errorf("road %d: furniture does not know which way the road is", r.ID)
			}
		}
	}
	for k := range numFurniture {
		if kinds[k] == 0 {
			t.Errorf("the city has no furniture of kind %d", k)
		}
	}
}

func TestFurnitureForCoversEveryKind(t *testing.T) {
	seen := map[furnitureKind]bool{}
	for i := range 100 {
		seen[furnitureFor(float32(i)/100, city.Collector)] = true
		if furnitureFor(float32(i)/100, city.Local) == furnShelter {
			t.Fatal("a bus shelter on a side street")
		}
	}
	if len(seen) != int(numFurniture) {
		t.Errorf("only %d of %d kinds of furniture can be picked", len(seen), numFurniture)
	}
}

func TestOnlyAParkWithRoomHasPaths(t *testing.T) {
	big := city.Block{Min: mathx.V(0, 0), Max: mathx.V(80, 60), Park: true}
	kind, centre, ok := parkFeature(big)
	if !ok || centre != mathx.V(40, 30) || kind < 0 || kind >= numParkKinds {
		t.Fatalf("a big park has feature %d at %v, ok %v", kind, centre, ok)
	}
	if _, _, ok := parkFeature(city.Block{Min: mathx.V(0, 0), Max: mathx.V(80, 20), Park: true}); ok {
		t.Error("a park 20 m across has paths through it")
	}
	built := big
	built.Park = false
	if _, _, ok := parkFeature(built); ok {
		t.Error("a block with buildings on it has a park's paths")
	}

	if !onParkPath(big, mathx.V(40, 5)) || !onParkPath(big, mathx.V(70, 30)) || !onParkPath(big, mathx.V(45, 35)) {
		t.Error("a point on a path or the circle is not on it")
	}
	if onParkPath(big, mathx.V(15, 10)) {
		t.Error("a corner of the park counts as path")
	}
	if onParkPath(built, mathx.V(40, 30)) {
		t.Error("a block with no paths has a point on one")
	}
}

func TestNoTreeIsPlantedInAParkingBayOrOnAPath(t *testing.T) {
	g := &Game{world: sim.NewWorld(sim.Config{Seed: 7})}
	g.plantTrees()
	if len(g.trees) == 0 {
		t.Fatal("no trees were planted")
	}
	c := g.world.City
	for _, tr := range g.trees {
		for _, p := range c.Parked {
			if tr.at.DistTo(p.Pos) < 1.2 {
				t.Fatalf("a tree stands at %v, in the car parked at %v", tr.at, p.Pos)
			}
		}
		for _, b := range c.Blocks {
			// The trees along the street stand at the edge of the park; it
			// is the ones planted in it that must keep off the paths.
			const in = 5
			inside := tr.at.X > b.Min.X+in && tr.at.X < b.Max.X-in && tr.at.Z > b.Min.Z+in && tr.at.Z < b.Max.Z-in
			if b.Park && inside && onParkPath(b, tr.at) {
				t.Fatalf("a tree stands at %v, on a path", tr.at)
			}
		}
	}
}

func TestInBayCoversTheBayAndALittleEitherSide(t *testing.T) {
	bays := []city.Bay{{Along: 40, Side: 1}}
	length := city.BayLength * city.BaySlots
	for d, want := range map[float32]bool{37: false, 39: true, 45: true, 40 + length + 1: true, 40 + length + 3: false} {
		if got := inBay(bays, d, 1); got != want {
			t.Errorf("inBay at %.1f m = %v, want %v", d, got, want)
		}
	}
	if inBay(bays, 45, -1) {
		t.Error("a bay on one side of the road covers the other")
	}
}

func TestSomeShopsHaveAwningsAndSomeAreLit(t *testing.T) {
	c := city.Generate(city.DefaultParams(7))
	awnings, lit := 0, 0
	for _, b := range c.Buildings {
		h := scatter(b.Center.X, b.Center.Z, 11)
		if shopHasAwning(h) {
			awnings++
		}
		if shopLitAtNight(h) {
			lit++
		}
	}
	n := len(c.Buildings)
	if awnings < n/4 || awnings > n*3/4 {
		t.Errorf("%d of %d shops have awnings", awnings, n)
	}
	if lit < n/3 || lit == n {
		t.Errorf("%d of %d shops are lit at night", lit, n)
	}
}
