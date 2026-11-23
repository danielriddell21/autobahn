package game

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/sim"
)

func TestCycleBodyVisitsEveryCarAndWraps(t *testing.T) {
	all := sim.PlayerBodies()
	k := all[0]
	seen := map[sim.BodyKind]bool{}
	for range all {
		seen[k] = true
		k = cycleBody(all, k, 1)
	}
	if len(seen) != len(all) || k != all[0] {
		t.Errorf("stepping forward through %d cars saw %d and ended on %v", len(all), len(seen), k)
	}
	if got := cycleBody(all, all[0], -1); got != all[len(all)-1] {
		t.Errorf("stepping back from the first car gave %v, want the last", got)
	}
}

func TestCycleBodyStepsOffACarThatIsNotOffered(t *testing.T) {
	offered := []sim.BodyKind{sim.HotHatch, sim.Saloon}
	if got := cycleBody(offered, sim.Van, 1); got != sim.HotHatch {
		t.Errorf("stepping from a car not on offer gave %v, want the first that is", got)
	}
	if got := cycleBody(nil, sim.Van, 1); got != sim.Van {
		t.Errorf("stepping with nothing on offer moved to %v", got)
	}
}

func TestMapNamesParseBack(t *testing.T) {
	for _, name := range MapNames() {
		if _, ok := city.ParseMap(name); !ok {
			t.Errorf("-map would not read its own name %q", name)
		}
	}
}

func TestCyclePaintWraps(t *testing.T) {
	owned := []int{0, 3, 7}
	if got := cyclePaint(owned, 7, 1); got != 0 {
		t.Errorf("past the last paint gave %d, want 0", got)
	}
	if got := cyclePaint(owned, 0, -1); got != 7 {
		t.Errorf("before the first paint gave %d, want the last", got)
	}
	if got := cyclePaint(owned, 5, 1); got != 0 {
		t.Errorf("stepping from a paint not owned gave %d, want the first", got)
	}
	if got := cyclePaint(nil, 5, 1); got != 5 {
		t.Errorf("stepping with nothing owned moved to %d", got)
	}
	if paintOf(99) != playerPaints[0] || paintOf(-1) != playerPaints[0] {
		t.Error("a paint out of range should fall back to the first")
	}
}

func TestParseCarReadsTheGaragesNames(t *testing.T) {
	k, p, err := ParseCar("Hot Hatch", "British Racing Green")
	if err != nil || k != sim.HotHatch || paintOf(p).name != "british racing green" {
		t.Errorf("got %v %d %v", k, p, err)
	}
	if k, _, err := ParseCar("van", "rosso"); err != nil || k != sim.Van {
		t.Errorf("van: got %v %v", k, err)
	}
	if _, _, err := ParseCar("tank", "rosso"); err == nil {
		t.Error("a car the garage does not offer should be refused")
	}
	if _, _, err := ParseCar("saloon", "tartan"); err == nil {
		t.Error("a paint the garage does not offer should be refused")
	}
}

func TestEveryCarHasABlurb(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range sim.PlayerBodies() {
		b := carBlurb(k)
		if b == "" || seen[b] {
			t.Errorf("%s has no blurb of its own: %q", k.Name(), b)
		}
		seen[b] = true
	}
}

func TestSettingsCarryTheCar(t *testing.T) {
	s := DefaultSettings()
	s.Car, s.Paint = sim.Van, 3
	o := s.applyTo(DefaultOptions())
	if o.Car != sim.Van || o.Paint != 3 {
		t.Errorf("applyTo gave car %v paint %d", o.Car, o.Paint)
	}
	// The command line wins.
	cl := DefaultOptions()
	cl.Car, cl.Given = sim.Saloon, Given{"car": true}
	if o := s.applyTo(cl); o.Car != sim.Saloon {
		t.Errorf("a -car on the command line was overridden: %v", o.Car)
	}
	// A chase against another person keeps the shape both ends agreed.
	if o := s.applyViewTo(DefaultOptions()); o.Car != sim.HotHatch || o.Paint != 3 {
		t.Errorf("a chase took car %v paint %d, want the usual car in the chosen paint", o.Car, o.Paint)
	}
}

func TestClampRepairsTheCar(t *testing.T) {
	s := DefaultSettings()
	s.Car, s.Paint = sim.BodyKind(99), -4
	s.clamp()
	if s.Car != sim.HotHatch || s.Paint != 0 {
		t.Errorf("clamp left car %v paint %d", s.Car, s.Paint)
	}
}

// A GIF holds 256 colours. The media tool ramps each of the city's colours
// through five shades and adds every box colour as it is, plus black.
func TestTheGIFPaletteFits(t *testing.T) {
	if n := 1 + len(PaletteBases())*5 + len(BoxColours()); n > 256 {
		t.Errorf("the GIF palette needs %d colours, more than a GIF can hold", n)
	}
}
