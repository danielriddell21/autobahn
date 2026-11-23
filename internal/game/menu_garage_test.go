package game

import (
	"slices"
	"strings"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/shop"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// garageFor opens the garage on a fresh shell with the given earnings and
// banked bounty, through the registry as a player would reach it.
func garageFor(t *testing.T, earnings, banked int) (*Shell, *garagePage) {
	t.Helper()
	sh := testShell(t)
	sh.career.Earnings, sh.career.Banked = earnings, banked
	return sh, showGarage(sh)
}

// showGarage puts a garage page on the shell and returns its state, so a test
// can look inside the page the registry would have built.
func showGarage(sh *Shell) *garagePage {
	g := newGaragePage(sh)
	sh.pager.stack = []pageFrame{{id: screenTitle}}
	sh.showPage(pageGarage, g.page())
	return g
}

// press feeds the garage one frame of keys through the shell, as the window does.
func press(sh *Shell, in navInput) { sh.stepPage(in) }

// goTo presses ↓ until the selection is on key.
func goTo(t *testing.T, sh *Shell, g *garagePage, key string) {
	t.Helper()
	for range 12 {
		if g.list.SelectedKey() == key {
			return
		}
		press(sh, navInput{Input: keyDown})
	}
	t.Fatalf("↓ never reached %q; stuck on %q", key, g.list.SelectedKey())
}

func keysOf(l *rowList) []string {
	var out []string
	for _, v := range l.Views() {
		out = append(out, v.Key)
	}
	return out
}

func TestGarageIsRegistered(t *testing.T) {
	if _, stub := pageFor(pageGarage); stub {
		t.Fatal("the garage is still a stub")
	}
	sh := testShell(t)
	sh.openPage(pageGarage)
	if sh.screen != pageGarage || sh.pager.cur == nil || sh.pager.cur.Title != "GARAGE" {
		t.Fatalf("opening the garage gave screen %d", sh.screen)
	}
}

func TestGarageStats(t *testing.T) {
	for _, k := range sim.PlayerBodies() {
		stats := garageStats(k, sim.HotHatch)
		if len(stats) != 4 {
			t.Fatalf("the %s has %d bars, want 4", k.Name(), len(stats))
		}
		for _, s := range stats {
			if s.Level <= 0 || s.Level > 1 || s.Mark <= 0 || s.Mark > 1 {
				t.Errorf("the %s's %s bar is %.2f with a tick at %.2f", k.Name(), s.Name, s.Level, s.Mark)
			}
			if s.Words == "" {
				t.Errorf("the %s's %s says nothing", k.Name(), s.Name)
			}
		}
	}

	// The tick is the car being driven: on its own bars it sits at the end of
	// the fill, and on another car's it marks where the driven car stands.
	own := garageStats(sim.Van, sim.Van)
	for _, s := range own {
		if s.Level != s.Mark {
			t.Errorf("the van's %s tick is at %.3f against a fill of %.3f", s.Name, s.Mark, s.Level)
		}
	}
	van, hatch := garageStats(sim.Van, sim.HotHatch), garageStats(sim.HotHatch, sim.HotHatch)
	byName := func(ss []garageStat, n string) garageStat {
		for _, s := range ss {
			if s.Name == n {
				return s
			}
		}
		t.Fatalf("no %s bar", n)
		return garageStat{}
	}
	if byName(van, "Weight").Level <= byName(hatch, "Weight").Level {
		t.Error("a van does not weigh more than a hot hatch")
	}
	if byName(van, "Top speed").Level >= byName(hatch, "Top speed").Level {
		t.Error("a van is not slower than a hot hatch")
	}
	if byName(van, "Weight").Mark != byName(hatch, "Weight").Level {
		t.Error("the tick on the van's bar is not where the hot hatch's fill ends")
	}
	if !strings.HasSuffix(byName(hatch, "Top speed").Words, "mph") || !strings.HasSuffix(byName(hatch, "Weight").Words, "kg") {
		t.Errorf("the words are %q and %q", byName(hatch, "Top speed").Words, byName(hatch, "Weight").Words)
	}
}

func TestGarageTags(t *testing.T) {
	_, g := garageFor(t, 0, 0)
	tests := []struct {
		car  sim.BodyKind
		want standing
		tag  string
	}{
		{sim.HotHatch, standDriving, "DRIVING"},
		{sim.Saloon, standForSale, "FOR SALE"},
		{sim.Convertible, standLocked, "LOCKED"},
		{sim.GrandTourer, standLocked, "LOCKED"},
	}
	for _, tt := range tests {
		st := g.standingOf(tt.car)
		if tag, _ := st.tag(); st != tt.want || tag != tt.tag {
			t.Errorf("the %s is %q, want %q", tt.car.Name(), tag, tt.tag)
		}
	}

	g.s.career.Cars = append(g.s.career.Cars, int(sim.Saloon))
	if st := g.standingOf(sim.Saloon); st != standYours {
		t.Errorf("an owned car that is not driven is %v, want YOURS", st)
	}
	g.s.career.Banked = pack.Of(pack.Coast).Price
	if st := g.standingOf(sim.Convertible); st != standForSale {
		t.Errorf("a car whose pack has been opened is %v, want FOR SALE", st)
	}
}

func TestGarageOpensOnTheCarDriven(t *testing.T) {
	sh := testShell(t)
	sh.settings.Car, sh.settings.Paint = sim.Saloon, 0
	sh.career.Cars = append(sh.career.Cars, int(sim.Saloon))
	g := newGaragePage(sh)
	if g.view != sim.Saloon || g.standingOf(sim.Saloon) != standDriving {
		t.Errorf("the garage opens on the %s, tagged %v", g.view.Name(), g.standingOf(g.view))
	}
}

func TestBrowsingNeverChangesTheCarDriven(t *testing.T) {
	sh, g := garageFor(t, 100000, 0)
	all := sim.PlayerBodies()
	for range len(all) + 1 {
		press(sh, navInput{Input: keyRight})
		if sh.settings.Car != sim.HotHatch {
			t.Fatalf("browsing onto the %s (%s) made it the car driven", g.view.Name(), g.standingOf(g.view).tagWord())
		}
	}
	if g.view != all[1] {
		t.Errorf("after a lap and a step the view is the %s, want the %s", g.view.Name(), all[1].Name())
	}

	// Once a car is owned, browsing onto it drives it; browsing past an unowned
	// one afterwards leaves that one driven.
	sh.career.Cars = append(sh.career.Cars, int(sim.Estate))
	g.view = sim.Saloon
	press(sh, navInput{Input: keyRight})
	if g.view != sim.Estate || sh.settings.Car != sim.Estate {
		t.Errorf("onto an owned car: viewing %s, driving %s", g.view.Name(), sh.settings.Car.Name())
	}
	press(sh, navInput{Input: keyRight})
	if g.view != sim.Taxi || sh.settings.Car != sim.Estate {
		t.Errorf("onto an unowned car: viewing %s, driving %s", g.view.Name(), sh.settings.Car.Name())
	}
	press(sh, navInput{Input: keyLeft})
	press(sh, navInput{Input: keyLeft})
	if g.view != sim.Saloon || sh.settings.Car != sim.Estate {
		t.Errorf("back past: viewing %s, driving %s", g.view.Name(), sh.settings.Car.Name())
	}
}

// tagWord is the tag as a word, for messages.
func (st standing) tagWord() string { s, _ := st.tag(); return s }

func TestALockedCarCannotBeDriven(t *testing.T) {
	sh, g := garageFor(t, 1_000_000, 0)
	g.view = sim.GrandTourer
	g.drive()
	if sh.settings.Car != sim.HotHatch {
		t.Fatalf("a car in a locked pack was made the car driven: %s", sh.settings.Car.Name())
	}
	g.sync()
	if _, ok := g.list.Row(rowBuyCar); ok {
		t.Error("a car in a locked pack has a buy button")
	}
	// Even with the earnings in hand, buying it is refused by the shop itself.
	if err := sh.career.BuyCar(sh.access(), sim.GrandTourer); err == nil {
		t.Error("the shop sold a car in a locked pack")
	}
	g.buyCar()
	if slices.Contains(sh.career.Cars, int(sim.GrandTourer)) || sh.settings.Car != sim.HotHatch {
		t.Error("buying a locked car through the page did something")
	}

	// The card says which pack opens it and how much bounty that takes.
	notes := strings.Join(g.notes(), " ")
	p := pack.ForBody(sim.GrandTourer)
	for _, want := range []string{p.Name, money(p.Price)} {
		if !strings.Contains(notes, want) {
			t.Errorf("the note %q does not say %q", notes, want)
		}
	}
	if g.buyHead() != "LOCKED" {
		t.Errorf("the buying card is headed %q", g.buyHead())
	}
}

func TestBuyingWithTooLittle(t *testing.T) {
	sh, g := garageFor(t, 100, 0)
	g.view = sim.Hatchback
	g.sync()
	v := rowOf(t, g.list, rowBuyCar)
	if v.Disabled == "" || !strings.Contains(v.Disabled, money(shop.CarPrice(sim.Hatchback)-100)) {
		t.Fatalf("the buy button is %q when short, want a reason naming what is missing", v.Disabled)
	}
	if v.Value != money(shop.CarPrice(sim.Hatchback)) {
		t.Errorf("the button shows %q", v.Value)
	}

	goTo(t, sh, g, rowBuyCar)
	press(sh, navInput{Input: keyEnter})
	if len(sh.career.Cars) != 0 && slices.Contains(sh.career.Cars, int(sim.Hatchback)) || sh.career.Earnings != 100 {
		t.Errorf("a car was bought with too little: cars %v, earnings %d", sh.career.Cars, sh.career.Earnings)
	}
	if sh.settings.Car != sim.HotHatch {
		t.Errorf("the car driven changed to %s", sh.settings.Car.Name())
	}
	if why := g.list.Reason(); why == "" || !strings.Contains(strings.Join(g.notes(), " "), why) {
		t.Errorf("the page does not say why: reason %q, notes %q", why, g.notes())
	}
	if g.list.SelectedKey() != rowBuyCar {
		t.Errorf("a refused purchase moved the selection to %q", g.list.SelectedKey())
	}
}

func TestBuyingACarOwnsItDrivesItAndSaves(t *testing.T) {
	sh, g := garageFor(t, 2000, 0)
	g.view = sim.Hatchback
	g.sync()
	price := shop.CarPrice(sim.Hatchback)

	goTo(t, sh, g, rowBuyCar)
	press(sh, navInput{Input: keyEnter})

	if !slices.Contains(sh.career.Cars, int(sim.Hatchback)) {
		t.Fatalf("the hatchback is not owned: %v", sh.career.Cars)
	}
	if sh.career.Earnings != 2000-price {
		t.Errorf("earnings are %d, want %d", sh.career.Earnings, 2000-price)
	}
	if sh.settings.Car != sim.Hatchback {
		t.Errorf("the car driven is the %s", sh.settings.Car.Name())
	}
	if _, ok := g.list.Row(rowBuyCar); ok {
		t.Error("an owned car still has a buy button")
	}
	if g.list.SelectedKey() != rowCar {
		t.Errorf("after buying the selection is on %q, want the car", g.list.SelectedKey())
	}
	if g.standingOf(sim.Hatchback) != standDriving {
		t.Error("the car just bought is not tagged DRIVING")
	}
	if g.msg == "" {
		t.Error("the page says nothing about what was bought")
	}

	// Both files were written, so the car is still owned and driven next time.
	set := LoadSettings()
	career, _ := readCareer(set)
	if set.Car != sim.Hatchback || !slices.Contains(career.Cars, int(sim.Hatchback)) || career.Earnings != 2000-price {
		t.Errorf("saved car %s, cars %v, earnings %d", set.Car.Name(), career.Cars, career.Earnings)
	}

	// Buying it again is refused by the shop, not by the page.
	if err := sh.career.BuyCar(sh.access(), sim.Hatchback); err == nil {
		t.Error("the shop sold the same car twice")
	}
}

func TestBuyingAPaintWearsIt(t *testing.T) {
	sh, g := garageFor(t, 1000, 0)
	press(sh, navInput{Input: keyDown}) // onto the paint
	if g.list.SelectedKey() != rowPaint {
		t.Fatalf("↓ from the car reached %q", g.list.SelectedKey())
	}
	press(sh, navInput{Input: keyRight}) // racing blue, which is not owned
	if g.paint != 1 || sh.settings.Paint != 0 {
		t.Fatalf("viewing paint %d, wearing %d: browsing an unowned paint must only look", g.paint, sh.settings.Paint)
	}
	v := rowOf(t, g.list, rowBuyPaint)
	if v.Disabled != "" || v.Value != money(shop.PaintPrice(1)) {
		t.Fatalf("buy paint row: %+v", v)
	}
	goTo(t, sh, g, rowBuyPaint)
	press(sh, navInput{Input: keyEnter})
	if !slices.Contains(sh.career.Paints, 1) || sh.settings.Paint != 1 || sh.career.Earnings != 1000-shop.PaintPrice(1) {
		t.Errorf("paints %v, worn %d, earnings %d", sh.career.Paints, sh.settings.Paint, sh.career.Earnings)
	}
	if _, ok := g.list.Row(rowBuyPaint); ok {
		t.Error("an owned paint still has a buy button")
	}
	if got := LoadSettings().Paint; got != 1 {
		t.Errorf("the paint saved is %d", got)
	}
}

func TestAPaintNeverSoldStaysNeverSold(t *testing.T) {
	sh, g := garageFor(t, 1_000_000, 1_000_000)
	const racingGreen = 5
	if !shop.Secret(racingGreen) {
		t.Fatal("the test needs a paint that is never sold")
	}
	g.paint = racingGreen
	g.sync()
	if _, ok := g.list.Row(rowBuyPaint); ok {
		t.Error("a paint that is never sold has a buy button")
	}
	if err := sh.career.BuyPaint(sh.access(), racingGreen); err == nil {
		t.Error("the shop sold a paint that is never sold")
	}
	g.buyPaint()
	if slices.Contains(sh.career.Paints, racingGreen) || sh.career.Earnings != 1_000_000 {
		t.Error("buying through the page spent earnings on a paint that is never sold")
	}
	if !strings.Contains(strings.Join(g.notes(), " "), "never sold") {
		t.Errorf("the card does not say it is never sold: %q", g.notes())
	}
	if status, _ := g.paintStatus(); status != "never sold" {
		t.Errorf("the paint status is %q", status)
	}

	// Looking at it does not put it on the car.
	g.paint = 0
	goTo(t, sh, g, rowPaint)
	for range racingGreen {
		press(sh, navInput{Input: keyRight})
	}
	if g.paint != racingGreen || sh.settings.Paint != 0 {
		t.Errorf("viewing %d, wearing %d", g.paint, sh.settings.Paint)
	}
	// An owned one is worn as soon as it is viewed.
	sh.career.Paints = append(sh.career.Paints, 2)
	g.paint = 1
	press(sh, navInput{Input: keyRight})
	if g.paint != 2 || sh.settings.Paint != 2 {
		t.Errorf("onto an owned paint: viewing %d, wearing %d", g.paint, sh.settings.Paint)
	}
}

func TestGarageFilter(t *testing.T) {
	_, g := garageFor(t, 0, 0)
	g.s.career.Cars = append(g.s.career.Cars, int(sim.Van))

	all := g.cars()
	if len(all) != len(sim.PlayerBodies()) {
		t.Errorf("everything shows %d of %d cars", len(all), len(sim.PlayerBodies()))
	}

	g.setFilter(garageYours)
	if got := g.cars(); !slices.Equal(got, []sim.BodyKind{sim.HotHatch, sim.Van}) {
		t.Errorf("yours shows %v", got)
	}
	g.setFilter(garageForSale)
	sale := g.cars()
	for _, k := range sale {
		if g.standingOf(k) != standForSale {
			t.Errorf("the %s is listed as for sale", k.Name())
		}
	}
	// With no pack held, only the base game's cars are for sale.
	if want := []sim.BodyKind{sim.Hatchback, sim.Saloon, sim.Estate, sim.Taxi}; !slices.Equal(sale, want) {
		t.Errorf("for sale shows %v, want %v", sale, want)
	}
	if !g.fits(g.view) || g.standingOf(g.view) != standForSale {
		t.Errorf("switching filter left the view on the %s, which does not pass", g.view.Name())
	}
	if g.s.settings.Car != sim.HotHatch {
		t.Errorf("filtering to what is for sale changed the car driven to %s", g.s.settings.Car.Name())
	}

	// Browsing stays inside the filter and wraps.
	g.view = sale[len(sale)-1]
	g.browse(1)
	if g.view != sale[0] {
		t.Errorf("past the last for-sale car is the %s", g.view.Name())
	}

	// An empty filter leaves the view alone rather than inventing a car.
	g.s.career.Banked = 1_000_000
	for _, k := range sim.PlayerBodies() {
		if !slices.Contains(g.s.career.Cars, int(k)) {
			g.s.career.Cars = append(g.s.career.Cars, int(k))
		}
	}
	g.setFilter(garageYours)
	g.setFilter(garageForSale)
	if len(g.cars()) != 0 {
		t.Fatalf("with everything owned %d cars are for sale", len(g.cars()))
	}
	before := g.view
	g.browse(1)
	if g.view != before {
		t.Error("browsing an empty line moved the view")
	}
}

func TestGarageFilterOnTheRow(t *testing.T) {
	sh, g := garageFor(t, 0, 0)
	goTo(t, sh, g, rowFilter)
	press(sh, navInput{Input: keyRight})
	if g.filter != garageYours || g.view != sim.HotHatch {
		t.Errorf("→ on the filter gave %d viewing %s", g.filter, g.view.Name())
	}
	press(sh, navInput{Input: keyRight})
	if g.filter != garageForSale || g.standingOf(g.view) != standForSale {
		t.Errorf("→ again gave %d viewing %s", g.filter, g.view.Name())
	}
	press(sh, navInput{Input: keyRight})
	press(sh, navInput{Input: keyRight})
	if g.filter != garageYours {
		t.Errorf("the filter wraps to %d, want yours", g.filter)
	}
	if rowOf(t, g.list, rowFilter).Value != "yours" {
		t.Errorf("the row says %q", rowOf(t, g.list, rowFilter).Value)
	}
}

func TestGarageKeysMoveBetweenTheRows(t *testing.T) {
	sh, g := garageFor(t, 100, 0)
	g.view = sim.Hatchback
	g.paint = 1
	g.sync()
	want := []string{rowCar, rowPaint, rowBuyCar, rowBuyPaint, rowFilter, rowDone}
	if got := keysOf(g.list); !slices.Equal(got, want) {
		t.Fatalf("rows are %v, want %v", got, want)
	}
	g.list.Select(rowCar)
	for i, key := range want {
		if got := g.list.SelectedKey(); got != key {
			t.Fatalf("stop %d is on %q, want %q", i, got, key)
		}
		press(sh, navInput{Input: keyDown})
	}
	if g.list.SelectedKey() != rowCar {
		t.Errorf("↓ past Done lands on %q, want the top again", g.list.SelectedKey())
	}
	press(sh, navInput{Input: keyUp})
	if g.list.SelectedKey() != rowDone {
		t.Errorf("↑ from the top lands on %q", g.list.SelectedKey())
	}

	// With nothing to buy, the buy buttons are not there to stop on.
	g.view, g.paint = sim.HotHatch, 0
	g.sync()
	if got := keysOf(g.list); !slices.Equal(got, []string{rowCar, rowPaint, rowFilter, rowDone}) {
		t.Errorf("rows with nothing to buy are %v", got)
	}
}

func TestDoneAndEscGoBackAndSave(t *testing.T) {
	sh := testShell(t)
	sh.career.Cars = append(sh.career.Cars, int(sim.Van))
	sh.openPage(pageGarage)
	if sh.screen != pageGarage {
		t.Fatal("the garage did not open")
	}
	// ←→ on the car row drives the van, and leaving writes it down.
	for range 5 {
		press(sh, navInput{Input: keyRight})
	}
	if sh.settings.Car != sim.Van {
		t.Fatalf("the car driven is the %s", sh.settings.Car.Name())
	}
	press(sh, navInput{Back: true})
	if sh.screen != screenTitle || sh.onPage() {
		t.Errorf("esc left the garage for screen %d", sh.screen)
	}
	if LoadSettings().Car != sim.Van {
		t.Error("esc did not save the car")
	}

	sh.openPage(pageGarage)
	g := sh.pager.cur
	for range 12 {
		if g.List.SelectedKey() == rowDone {
			break
		}
		press(sh, navInput{Input: keyDown})
	}
	press(sh, navInput{Input: keyEnter})
	if sh.screen != screenTitle {
		t.Errorf("Done left the garage for screen %d", sh.screen)
	}
}

func TestGarageInAChaseKeepsTheShapeFixed(t *testing.T) {
	sh, g := garageFor(t, 1_000_000, 1_000_000)
	g.fixed = func() bool { return true }
	sh.career.Cars = append(sh.career.Cars, int(sim.Van))
	g.sync()

	press(sh, navInput{Input: keyRight}) // onto the hatchback, for sale
	g.view = sim.Van
	g.drive()
	if sh.settings.Car != sim.HotHatch {
		t.Errorf("the garage changed the car in a chase to the %s", sh.settings.Car.Name())
	}
	g.view = sim.Hatchback
	g.sync()
	v := rowOf(t, g.list, rowBuyCar)
	if v.Disabled != fixedShapeNote {
		t.Errorf("the buy button in a chase says %q", v.Disabled)
	}
	g.buyCar()
	if slices.Contains(sh.career.Cars, int(sim.Hatchback)) || sh.career.Earnings != 1_000_000 {
		t.Error("a car was bought in a chase")
	}
	if g.driven() != sim.HotHatch {
		t.Errorf("driven is %s", g.driven().Name())
	}

	// Paint is not part of the shape and still works.
	g.paint = 1
	g.sync()
	goTo(t, sh, g, rowBuyPaint)
	press(sh, navInput{Input: keyEnter})
	if sh.settings.Paint != 1 {
		t.Errorf("a paint bought in a chase was not worn: %d", sh.settings.Paint)
	}
}

func TestUnlockAllOwnsEveryCar(t *testing.T) {
	sh := testShell(t)
	sh.opts.UnlockAll = true
	g := newGaragePage(sh)
	owned := slices.Clone(sh.career.Cars)
	for _, k := range sim.PlayerBodies() {
		if g.standingOf(k) == standLocked || g.standingOf(k) == standForSale {
			t.Errorf("the %s is %v under -unlock-all", k.Name(), g.standingOf(k).tagWord())
		}
	}
	g.setFilter(garageForSale)
	if len(g.cars()) != 0 {
		t.Error("something is for sale under -unlock-all")
	}
	g.setFilter(garageAll)
	g.browse(1)
	if sh.settings.Car != g.view || g.view == sim.HotHatch {
		t.Errorf("browsing under -unlock-all: viewing %s, driving %s", g.view.Name(), sh.settings.Car.Name())
	}
	if !slices.Equal(sh.career.Cars, owned) {
		t.Error("browsing under -unlock-all recorded ownership")
	}
}

func TestGarageRowsStayASCII(t *testing.T) {
	_, g := garageFor(t, 10, 0)
	check := func(s string) {
		for _, r := range s {
			if r > 126 && r != '£' {
				t.Errorf("%q has the non-ASCII %q", s, r)
			}
		}
	}
	for _, k := range sim.PlayerBodies() {
		g.view = k
		for p := range playerPaints {
			g.paint = p
			g.sync()
			for _, v := range g.list.Views() {
				check(v.Label + v.Value + v.Disabled)
			}
			for _, n := range g.notes() {
				check(n)
			}
			st, _ := g.status()
			check(st)
			ps, _ := g.paintStatus()
			check(ps)
			check(garagePackName(k))
		}
	}
}

func TestGarageLayoutFits(t *testing.T) {
	for _, size := range [][2]int32{{1280, 720}, {1920, 1080}, {800, 450}} {
		w, h := size[0], size[1]
		r := garageLayout(w, h)
		inside := func(name string, c rl.Rectangle) {
			if c.X < 0 || c.Y < 0 || c.X+c.Width > float32(w) || c.Y+c.Height > float32(h-pageFoot+8) {
				t.Errorf("%dx%d: the %s %+v is not inside the window", w, h, name, c)
			}
		}
		inside("stage", r.stage)
		inside("panel", r.panel)
		inside("paint card", r.paint)
		inside("buy card", r.buy)
		if r.stage.X+r.stage.Width > r.panel.X {
			t.Errorf("%dx%d: the stage runs into the panel", w, h)
		}
		if r.paint.Y+r.paint.Height > r.buy.Y {
			t.Errorf("%dx%d: the paint card runs into the buy card", w, h)
		}
		if r.stage.Y+r.stage.Height > float32(r.filterY) {
			t.Errorf("%dx%d: the stage runs into the filter", w, h)
		}
		if got := r.cardW*garageCards + garageCardGap*(garageCards-1); got > r.stripW {
			t.Errorf("%dx%d: seven cards are %d wide in a strip of %d", w, h, got, r.stripW)
		}
	}
}

func TestGarageWrapAndFit(t *testing.T) {
	requireDisplay(t)
	rl.SetTraceLogLevel(rl.LogError)
	rl.InitWindow(320, 200, "autobahn test")
	defer rl.CloseWindow()

	for _, line := range garageWrap(faceSmall, "one two three four five six seven eight nine ten", 120) {
		if faceSmall.width(line) > 120 {
			t.Errorf("%q is wider than the box", line)
		}
	}
	if got := garageFit(faceSmall, "race roadster", 40); faceSmall.width(got) > 40 || !strings.HasSuffix(got, "..") {
		t.Errorf("fit gave %q", got)
	}
	if got := garageFit(faceSmall, "van", 400); got != "van" {
		t.Errorf("fit changed a word that fits: %q", got)
	}
}

// TestGarageDraws draws the page in each state it has, which catches a draw
// the model cannot satisfy.
func TestGarageDraws(t *testing.T) {
	requireDisplay(t)
	inTempConfig(t)

	rl.SetTraceLogLevel(rl.LogError)
	rl.InitWindow(1280, 720, "autobahn test")
	defer rl.CloseWindow()

	o := DefaultOptions()
	o.Mute = true
	sh := NewShell(o)
	defer sh.Close()
	sh.career.Earnings = 2000

	g := showGarage(sh)
	page := func() *garagePage { return g }

	frames := []func(){
		func() {}, // the car driven, nothing to buy
		func() { page().view = sim.Hatchback; page().paint = 1; page().sync() },       // two buy buttons
		func() { page().view = sim.GrandTourer; page().sync() },                       // locked
		func() { page().paint = 5; page().sync() },                                    // never sold
		func() { page().setFilter(garageForSale) },                                    // a short line
		func() { page().setFilter(garageYours) },                                      // owned only
		func() { page().fixed = func() bool { return true }; page().sync() },          // a chase
		func() { page().age = 0 },                                                     // mid fade
		func() { page().list.Select(rowDone); page().list.Select(rowPaint) },          // other rows lit
		func() { page().setFilter(garageAll); page().list.Select(rowFilter) },         // the filter lit
		func() { page().view = sim.RaceProto; page().list.Select(rowCar) },            // the end of the line
		func() { sh.career.Earnings = 0; page().view = sim.Hatchback; page().sync() }, // greyed
	}
	for _, set := range frames {
		set()
		for range 2 {
			sh.drawScreen(1.0 / 60)
		}
	}
}
