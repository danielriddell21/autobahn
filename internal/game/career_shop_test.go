package game

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/shop"
	"github.com/danielriddell21/autobahn/internal/sim"
)

func inTempConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("AppData", dir)
}

func TestMigrationFromEachVersion(t *testing.T) {
	gt := DefaultSettings()
	gt.Car, gt.Paint = sim.GrandTourer, 6
	tests := []struct {
		name       string
		file       string
		set        Settings
		banked     int
		cars       []int
		paints     []int
		earnings   int
		hasVersion bool
	}{
		{
			"v0 keeps bounty and the chosen car and paint", `{"banked":4321}`, gt, 4321,
			[]int{int(sim.HotHatch), int(sim.GrandTourer)},
			[]int{0, 6},
			0, true,
		},
		{
			"v0 with default settings is the starter only", `{"banked":5}`, DefaultSettings(), 5,
			[]int{int(sim.HotHatch)},
			[]int{0},
			0, true,
		},
		{
			"v1 is left as saved", `{"version":1,"banked":9,"earnings":77,"cars":[3],"paints":[2]}`, gt, 9,
			[]int{int(sim.HotHatch), int(sim.Van)},
			[]int{0, 2},
			77, true,
		},
		{
			"negatives and unknown ids are cleaned", `{"version":1,"banked":-4,"earnings":-1,"cars":[3,3,99,-2,5],"paints":[2,2,99,-1]}`, gt, 0,
			[]int{int(sim.HotHatch), int(sim.Van)},
			[]int{0, 2},
			0, true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c Career
			if err := json.Unmarshal([]byte(tt.file), &c); err != nil {
				t.Fatal(err)
			}
			c = migrate(c, tt.set)
			if c.Version != careerVersion || c.Banked != tt.banked || c.Earnings != tt.earnings {
				t.Errorf("got %+v", c)
			}
			if !slices.Equal(c.Cars, tt.cars) || !slices.Equal(c.Paints, tt.paints) {
				t.Errorf("cars %v paints %v, want %v %v", c.Cars, c.Paints, tt.cars, tt.paints)
			}
		})
	}
}

func TestCorruptCareerFileIsTheStarterOnly(t *testing.T) {
	inTempConfig(t)
	path, err := careerPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	set := DefaultSettings()
	set.Car, set.Paint = sim.Van, 2
	c := loadCareerFor(set)
	if c.Banked != 0 || !slices.Equal(c.Cars, []int{int(sim.HotHatch)}) || !slices.Equal(c.Paints, []int{0}) {
		t.Errorf("corrupt file gave %+v", c)
	}
}

func TestCareerV1SurvivesSaveAndLoad(t *testing.T) {
	inTempConfig(t)
	c := Career{Banked: 12, Earnings: 345, Cars: []int{int(sim.HotHatch), int(sim.Taxi)}, Paints: []int{0, 3}}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	got := LoadCareer()
	c.Version = careerVersion
	if got.Banked != c.Banked || got.Earnings != c.Earnings || got.Version != careerVersion ||
		!slices.Equal(got.Cars, c.Cars) || !slices.Equal(got.Paints, c.Paints) {
		t.Errorf("loaded %+v, saved %+v", got, c)
	}
}

func TestBuyingSpendsOnceAndFailsWhenShort(t *testing.T) {
	held := pack.Access{}
	c := migrate(Career{Earnings: shop.CarPrice(sim.Van) + 10}, DefaultSettings())
	if err := c.BuyCar(held, sim.Van); err != nil {
		t.Fatal(err)
	}
	if c.Earnings != 10 || !slices.Contains(c.Cars, int(sim.Van)) {
		t.Errorf("after buying: %+v", c)
	}
	if err := c.BuyCar(held, sim.Van); !errors.Is(err, shop.ErrOwned) || c.Earnings != 10 {
		t.Errorf("buying twice gave %v with %d left", err, c.Earnings)
	}
	if err := c.BuyCar(held, sim.Saloon); !errors.Is(err, shop.ErrShort) || c.Earnings != 10 {
		t.Errorf("buying short gave %v with %d left", err, c.Earnings)
	}
	if err := c.BuyPaint(held, 5); !errors.Is(err, shop.ErrSecret) {
		t.Errorf("a secret paint gave %v", err)
	}
	c.Earn(-50)
	c.Earn(0)
	if c.Earnings != 10 {
		t.Errorf("Earn of nothing or less changed the purse to %d", c.Earnings)
	}
	c.Earn(290)
	if err := c.BuyPaint(held, 2); err != nil || c.Earnings != 0 || !slices.Contains(c.Paints, 2) {
		t.Errorf("buying a paint: %v, %+v", err, c)
	}
}

func TestLockedRefusesWhatIsNotOwnedUnlessBypassed(t *testing.T) {
	c := migrate(Career{}, DefaultSettings())
	o := Options{Map: 0, Car: sim.Van, Paint: 0}
	if err := o.lockedFor(c.holdings(pack.Access{})); err == nil {
		t.Error("an unowned car was allowed")
	}
	o = Options{Car: sim.HotHatch, Paint: 2}
	if err := o.lockedFor(c.holdings(pack.Access{})); err == nil {
		t.Error("an unowned paint was allowed")
	}
	o = Options{Car: sim.Van, Paint: 2}
	if err := o.lockedFor(c.holdings(pack.Access{Bypass: true})); err != nil {
		t.Errorf("bypass refused: %v", err)
	}
	c.Cars = append(c.Cars, int(sim.Van))
	c.Paints = append(c.Paints, 2)
	if err := o.lockedFor(c.holdings(pack.Access{})); err != nil {
		t.Errorf("owned car and paint refused: %v", err)
	}
}

func TestKeepToDropsWhatIsNotOwned(t *testing.T) {
	s := DefaultSettings()
	s.Car, s.Paint = sim.Van, 2
	s.keepTo(shop.Holdings{})
	if s.Car != sim.HotHatch || s.Paint != 0 {
		t.Errorf("kept %v and paint %d", s.Car, s.Paint)
	}
	s.Car, s.Paint = sim.Van, 2
	s.keepTo(shop.Holdings{Cars: []int{int(sim.Van)}, Paints: []int{2}})
	if s.Car != sim.Van || s.Paint != 2 {
		t.Error("dropped what the player owns")
	}
}

// The career and settings files store cars and paints as integers, so the
// numbers are part of the file format. New ones go on the end.
func TestStoredIdentifiersAreStable(t *testing.T) {
	pinned := map[sim.BodyKind]int{
		sim.Hatchback: 0, sim.Saloon: 1, sim.Estate: 2, sim.Van: 3, sim.Taxi: 4, sim.HotHatch: 5,
		sim.Convertible: 8, sim.CityCar: 9, sim.FourByFour: 12, sim.SUV: 13, sim.Minibus: 15, sim.GrandTourer: 16,
	}
	for k, n := range pinned {
		if int(k) != n {
			t.Errorf("the %s is %d, was %d: saved files would change meaning", k.Name(), int(k), n)
		}
	}
	names := []string{
		"rosso", "racing blue", "arctic white", "jet black", "sunburst yellow",
		"british racing green", "silver", "plum", "tangerine", "lime", "teal", "gunmetal",
	}
	for i, n := range names {
		if playerPaints[i].name != n {
			t.Errorf("paint %d is %q, was %q", i, playerPaints[i].name, n)
		}
	}
	if len(playerPaints) < len(names) || shop.PaintCount() != len(playerPaints) {
		t.Errorf("%d paints, %d priced, %d pinned", len(playerPaints), shop.PaintCount(), len(names))
	}
}

func TestPayJobsEarnsOnlyWhatWasPaid(t *testing.T) {
	s := &Shell{}
	if s.payJobs([]job.Result{{Pay: 0}, {Pay: -5}}) || s.career.Earnings != 0 {
		t.Errorf("unpaid jobs changed the purse to %d", s.career.Earnings)
	}
	if !s.payJobs([]job.Result{{Pay: 40}, {Pay: 0}, {Pay: 25}}) || s.career.Earnings != 65 {
		t.Errorf("paid jobs left the purse at %d, want 65", s.career.Earnings)
	}
	if s.career.Banked != 0 {
		t.Error("job pay was banked as bounty")
	}
}
