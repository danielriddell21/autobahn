package game

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/keymap"

	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/shop"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// The garage page (docs/menus.md, "Garage"): the player's cars and paints, and
// the shop folded into them. The car stands on the turntable at the top left
// with its tag, its place in the line-up and four bars; paint and buying are
// on the right; every player car runs along the bottom as a card.
//
// It is the garage and the shop in one place and changes none of
// their rules: what is owned, what is for sale once its pack is held and what
// is never sold all come from internal/shop, and a purchase goes through
// Career.BuyCar and Career.BuyPaint as it always did.
//
// Two things are kept apart. What is *viewed* is where browsing has got to,
// and may be a car the player cannot drive or a paint they do not own. What is
// *driven* is Settings.Car and Settings.Paint, which move only onto something
// owned, so wandering through the shop never leaves the player in a car they
// have not paid for.

var _ = registerPage(pageGarage, buildGaragePage, hintsGarage...)

// hintsGarage is the bar along the bottom of the garage.
var hintsGarage = []keymap.Binding{
	{Key: "up/down", Action: "choose"},
	{Key: "left/right", Action: "browse"},
	{Key: "enter", Action: "buy"},
	{Key: "esc", Action: "done"},
}

// The filters over the line of cars, in the order ←→ steps through them.
const (
	garageAll = iota
	garageYours
	garageForSale
)

var garageFilters = []string{"everything", "yours", "for sale"}

// The rows the garage's keys move between, in the order ↑↓ visit them. The buy
// rows come and go with what is on show.
const (
	rowCar      = "car"
	rowPaint    = "paint"
	rowBuyCar   = "buycar"
	rowBuyPaint = "buypaint"
	rowFilter   = "filter"
	rowDone     = "done"
)

// fixedShapeNote is what the garage says in a chase against another person.
const fixedShapeNote = "the shape of the car is fixed for this chase"

// standing is where a car stands with the player.
type standing int

const (
	// standDriving is the car being driven now.
	standDriving standing = iota
	// standYours is a car the player owns and can drive.
	standYours
	// standForSale is a car whose pack is held and that is not owned yet.
	standForSale
	// standLocked is a car whose pack is not held.
	standLocked
)

// tag is the word on the stage and the tone it is drawn in.
func (st standing) tag() (string, tone) {
	switch st {
	case standDriving:
		return "DRIVING", toneYours
	case standYours:
		return "YOURS", toneYours
	case standForSale:
		return "FOR SALE", toneOffer
	default:
		return "LOCKED", toneLocked
	}
}

// garageStat is one of the four bars: its name, what it says in words, how far
// along the bar it runs, and where the car being driven stands on the same bar.
type garageStat struct {
	Name, Words string
	Level, Mark float64
}

// garageMassCap is where the weight bar ends: a fire engine is not three times
// as heavy as everything else on the bar, it is off the end of it.
const garageMassCap = 4000

// garageGrip is how much tyre a car has for its weight: the stiffness of both
// axles over the mass they carry. Taken whole, a fire engine's huge tyres
// would put it far ahead of every car on the bar.
func garageGrip(s sim.Spec) float32 { return (s.GripFront + s.GripRear) / 2 / s.Mass }

// garageScale is the best of each figure among the player's cars, which is
// what every bar is measured against, so a car a pack adds later still fits.
type garageScale struct {
	speed, accel, grip, mass float32
	// length and height are the longest and tallest, for the cards' pictures.
	length, height float32
}

var garageScaleOnce = sync.OnceValue(func() garageScale {
	var sc garageScale
	for _, k := range sim.PlayerBodies() {
		b := sim.PlayerBodyOf(k)
		sp := b.Spec
		sc.speed = max(sc.speed, sp.MaxSpeed)
		sc.accel = max(sc.accel, sp.EngineForce/sp.Mass)
		sc.grip = max(sc.grip, garageGrip(sp))
		sc.mass = max(sc.mass, min(sp.Mass, garageMassCap))
		sc.length = max(sc.length, 2*sp.HalfLength)
		sc.height = max(sc.height, sp.Height)
	}
	return sc
})

// garageStats works out the four bars for car k, with the tick at the car
// being driven. The figures are the car's own: top speed, how hard the engine
// pulls for its weight, how stiff its tyres are, and its mass.
func garageStats(k, driven sim.BodyKind) []garageStat {
	sc := garageScaleOnce()
	level := func(body sim.BodyKind, f func(sim.Spec) float32, best float32) float64 {
		return float64(min(f(sim.PlayerBodyOf(body).Spec)/best, 1))
	}
	speed := func(s sim.Spec) float32 { return s.MaxSpeed }
	accel := func(s sim.Spec) float32 { return s.EngineForce / s.Mass }
	grip := garageGrip
	mass := func(s sim.Spec) float32 { return min(s.Mass, garageMassCap) }

	sp := sim.PlayerBodyOf(k).Spec
	stat := func(name, words string, f func(sim.Spec) float32, best float32) garageStat {
		return garageStat{
			Name: name, Words: words,
			Level: level(k, f, best), Mark: level(driven, f, best),
		}
	}
	return []garageStat{
		stat("Top speed", fmt.Sprintf("%.0f mph", mathx.ToMPH(sp.MaxSpeed)), speed, sc.speed),
		stat("Acceleration", fmt.Sprintf("%.1f m/s2", accel(sp)), accel, sc.accel),
		stat("Grip", fmt.Sprintf("%.0f%%", 100*level(k, grip, sc.grip)), grip, sc.grip),
		stat("Weight", fmt.Sprintf("%.0f kg", sp.Mass), mass, sc.mass),
	}
}

// garagePage is the garage's state: what is viewed, which filter is on, and the
// rows ↑↓ move between.
type garagePage struct {
	s *Shell
	// view is the car on the turntable and paint the swatch ringed; neither is
	// necessarily owned.
	view  sim.BodyKind
	paint int
	// filter is which of garageAll, garageYours and garageForSale is on.
	filter int
	// msg is what the last purchase said.
	msg  string
	list *rowList
	// refocus names the row to put the selection on at the next rebuild, for
	// when the row it was on has gone (a car bought has no buy button).
	refocus string
	// age is how long the page has been showing, for the fade in.
	age float32
	// fixed overrides whether the shape of the car is fixed, for a test; nil
	// asks the running game.
	fixed func() bool
}

// buildGaragePage is the registered builder.
func buildGaragePage(s *Shell) *menuPage { return newGaragePage(s).page() }

// newGaragePage opens the garage on the car and paint being driven.
func newGaragePage(s *Shell) *garagePage {
	g := &garagePage{s: s, paint: s.settings.Paint}
	g.view = g.driven()
	g.list = newRowList()
	g.sync()
	return g
}

// page is the garage as the registry shows it.
func (g *garagePage) page() *menuPage {
	return &menuPage{
		Title: "GARAGE",
		List:  g.list,
		Step:  g.step,
		Draw:  g.draw,
		Back:  g.leave,
		Shown: g.sync,
		// The garage is a room of its own, which would only hide the menu
		// drive: it stands on the plain backdrop and draws its own.
		Backdrop: backdropPlain,
	}
}

// chase reports whether the car's shape is fixed: the drive under way is a
// chase against another person, which has to agree on the car's size.
func (g *garagePage) chase() bool {
	if g.fixed != nil {
		return g.fixed()
	}
	return g.s.game != nil && g.s.game.Networked()
}

// driven is the car being driven now.
func (g *garagePage) driven() sim.BodyKind {
	if g.s.game != nil && g.chase() {
		return g.s.game.opts.Car
	}
	return g.s.settings.Car
}

// holdings is what the player has.
func (g *garagePage) holdings() shop.Holdings { return g.s.holdings() }

// standingOf is where car k stands with the player.
func (g *garagePage) standingOf(k sim.BodyKind) standing {
	h := g.holdings()
	switch {
	case h.Drivable(k):
		if k == g.driven() {
			return standDriving
		}
		return standYours
	case h.Access.HasBody(k) && !h.OwnsCar(k):
		return standForSale
	default:
		return standLocked
	}
}

// fits reports whether car k passes the filter.
func (g *garagePage) fits(k sim.BodyKind) bool {
	switch g.filter {
	case garageYours:
		st := g.standingOf(k)
		return st == standDriving || st == standYours
	case garageForSale:
		return g.standingOf(k) == standForSale
	default:
		return true
	}
}

// cars is the line of cars the filter lets through, in the garage's order.
func (g *garagePage) cars() []sim.BodyKind {
	var out []sim.BodyKind
	for _, k := range sim.PlayerBodies() {
		if g.fits(k) {
			out = append(out, k)
		}
	}
	return out
}

// browse steps the viewed car through the line, wrapping. A car that is owned
// is driven as soon as it is viewed; one that is not is only looked at.
func (g *garagePage) browse(dir int) {
	g.view = cycleBody(g.cars(), g.view, dir)
	g.drive()
}

// drive makes the viewed car the one driven, when it may be: it is owned, its
// pack is held, and the chase (if there is one) has not fixed the shape.
func (g *garagePage) drive() {
	if g.chase() || !g.holdings().Drivable(g.view) || g.s.settings.Car == g.view {
		return
	}
	g.s.settings.Car = g.view
	g.s.applyCar()
}

// browsePaint steps the viewed paint through all of them, wrapping. One that
// may be worn is worn at once.
func (g *garagePage) browsePaint(dir int) {
	all := make([]int, len(playerPaints))
	for i := range all {
		all[i] = i
	}
	g.paint = cyclePaint(all, g.paint, dir)
	g.wear()
}

// wear makes the viewed paint the one worn, when it may be.
func (g *garagePage) wear() {
	if !g.holdings().Wearable(g.paint) || g.s.settings.Paint == g.paint {
		return
	}
	g.s.settings.Paint = g.paint
	g.s.applyCar()
}

// setFilter turns a filter on, and moves the view onto a car that passes it if
// the one viewed does not. With nothing passing, the view stays where it is.
func (g *garagePage) setFilter(f int) {
	g.filter = min(max(f, 0), len(garageFilters)-1)
	if g.fits(g.view) {
		return
	}
	if all := g.cars(); len(all) > 0 {
		g.view = all[0]
		g.drive()
	}
}

// carForSale reports whether the viewed car is one the shop sells now.
func (g *garagePage) carForSale() bool { return g.standingOf(g.view) == standForSale }

// paintForSale reports whether the viewed paint is one the shop sells and the
// player does not have.
func (g *garagePage) paintForSale() bool {
	h := g.holdings()
	if h.Wearable(g.paint) {
		return false
	}
	err := h.CanBuy(shop.PaintItem(g.paint))
	return err == nil || errors.Is(err, shop.ErrShort)
}

// paintNever reports whether the viewed paint is one that is never sold.
func (g *garagePage) paintNever() bool {
	return !g.holdings().Wearable(g.paint) && shop.Secret(g.paint)
}

// short is how much more than the player has an item costs, or "".
func (g *garagePage) short(price int) string {
	if d := price - g.s.career.Earnings; d > 0 {
		return fmt.Sprintf("need %s more earnings", money(d))
	}
	return ""
}

// shortOfView is how far earnings are from what the viewed car, or else the
// viewed paint, costs, when it is for sale and they fall short.
func (g *garagePage) shortOfView() string {
	switch {
	case g.carForSale() && !g.chase():
		return g.short(shop.CarPrice(g.view))
	case g.paintForSale():
		return g.short(shop.PaintPrice(g.paint))
	}
	return ""
}

// rows is the page's rows as the viewed car and paint leave them.
func (g *garagePage) rows() []menuRow {
	car := actionRow(rowCar, "Car", nil).noted(func() string { return g.view.Name() })
	car.Adjust = g.browse
	paint := actionRow(rowPaint, "Paint", nil).noted(func() string { return paintOf(g.paint).name })
	paint.Adjust = g.browsePaint
	rows := []menuRow{car, paint}

	if g.carForSale() {
		price := shop.CarPrice(g.view)
		rows = append(rows, actionRow(rowBuyCar, "Buy the "+g.view.Name(), g.buyCar).
			noted(func() string { return money(price) }).
			disabledBy(func() string {
				if g.chase() {
					return fixedShapeNote
				}
				return g.short(price)
			}))
	}
	if g.paintForSale() {
		price := shop.PaintPrice(g.paint)
		rows = append(rows, actionRow(rowBuyPaint, "Buy paint", g.buyPaint).
			noted(func() string { return money(price) }).
			disabledBy(func() string { return g.short(price) }))
	}

	rows = append(rows,
		optionsRow(rowFilter, "Show", garageFilters, func() int { return g.filter }, g.setFilter),
		actionRow(rowDone, "Done", g.leave),
	)
	return rows
}

// sync rebuilds the rows, which change with what is viewed, and keeps the
// selection where it was or where a purchase asked for it.
func (g *garagePage) sync() {
	g.list.SetRows(g.rows())
	if g.refocus != "" {
		g.list.Select(g.refocus)
		g.refocus = ""
	}
}

// step reads a frame's keys. Everything but esc goes to the rows, and the
// rows are rebuilt after, since a key may have changed what there is to buy.
func (g *garagePage) step(in navInput) bool {
	if in.Back {
		return false
	}
	if in.Up || in.Down || in.Left || in.Right {
		g.msg = ""
	}
	g.list.Update(in.Input)
	g.sync()
	return true
}

// leave is Done and esc: the car and paint are written down, and the page goes
// back to where it was opened from.
func (g *garagePage) leave() {
	g.saveSettings()
	g.s.backPage()
}

func (g *garagePage) saveSettings() {
	if err := g.s.settings.Save(); err != nil {
		g.s.notice = err.Error()
	}
}

// fail says why a purchase did not happen, on the page. It is not the shell's
// notice, which the title page would go on showing long after the garage was
// left; a file that will not save still is (saveSettings, Shell.saveCareer).
func (g *garagePage) fail(err error) {
	g.msg = err.Error()
}

// buyCar spends earnings on the viewed car, which is driven from then on.
func (g *garagePage) buyCar() {
	if g.chase() {
		g.msg = fixedShapeNote
		return
	}
	k := g.view
	if err := g.s.career.BuyCar(g.s.access(), k); err != nil {
		g.fail(err)
		return
	}
	g.s.saveCareer()
	g.s.settings.Car = k
	g.saveSettings()
	g.s.applyCar()
	g.msg = "you bought the " + k.Name()
	g.refocus = rowCar
}

// buyPaint spends earnings on the viewed paint, which is worn from then on.
func (g *garagePage) buyPaint() {
	i := g.paint
	if err := g.s.career.BuyPaint(g.s.access(), i); err != nil {
		g.fail(err)
		return
	}
	g.s.saveCareer()
	g.s.settings.Paint = i
	g.saveSettings()
	g.s.applyCar()
	g.msg = "you bought " + paintOf(i).name
	g.refocus = rowPaint
}

// place is "3 of 20": where the viewed car is in the whole line-up.
func (g *garagePage) place() string {
	all := sim.PlayerBodies()
	return fmt.Sprintf("%d of %d", slices.Index(all, g.view)+1, len(all))
}

// garagePackName is the pack a car comes in, in words.
func garagePackName(k sim.BodyKind) string {
	if p := pack.ForBody(k); p.ID != pack.Base {
		return garageCap(p.Name) + " pack"
	}
	return "base game"
}

// status is the line on the stage saying where the viewed car stands.
func (g *garagePage) status() (string, tone) {
	k := g.view
	switch g.standingOf(k) {
	case standDriving:
		return "the car you drive now", toneYours
	case standYours:
		return "in your garage", toneYours
	case standForSale:
		return "for sale: " + money(shop.CarPrice(k)), toneOffer
	default:
		p := pack.ForBody(k)
		return fmt.Sprintf("bank %s bounty for %s", money(p.Price), p.Name), toneLocked
	}
}

// paintStatus is the word beside the paint's name.
func (g *garagePage) paintStatus() (string, tone) {
	switch {
	case g.holdings().Wearable(g.paint):
		return "yours", toneYours
	case shop.Secret(g.paint):
		return "never sold", toneLocked
	default:
		return money(shop.PaintPrice(g.paint)), toneOffer
	}
}

// buyHead and notes: what the buying card says under its buttons.
func (g *garagePage) buyHead() string {
	switch {
	case g.carForSale() || g.paintForSale():
		return "BUY"
	case g.standingOf(g.view) == standLocked:
		return "LOCKED"
	default:
		return "YOURS"
	}
}

// notes is what the buying card says: the reason the selected button is
// greyed or the last purchase first, then what the viewed car and paint need.
func (g *garagePage) notes() []string {
	var out []string
	if why := g.list.Reason(); why != "" {
		out = append(out, why)
	} else if g.msg != "" {
		out = append(out, g.msg)
	} else if short := g.shortOfView(); short != "" {
		// The greyed button says so without being selected.
		out = append(out, short)
	}
	if g.standingOf(g.view) == standLocked {
		p := pack.ForBody(g.view)
		out = append(out, fmt.Sprintf("Comes with the %s pack: bank %s bounty to open it. You have %s.",
			p.Name, money(p.Price), money(g.s.career.Banked)))
	}
	if g.paintNever() {
		out = append(out, garageCap(paintOf(g.paint).name)+" is earned by an achievement and is never sold.")
	}
	if len(out) == 0 {
		switch {
		case g.carForSale() || g.paintForSale():
			out = append(out, "Bought with earnings, and kept for good.")
		default:
			out = append(out, "Nothing to buy: this car and paint are yours.")
		}
	}
	return out
}

// garageCap upper-cases the first letter of each word.
func garageCap(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// garageFit shortens s with ".." until it fits in w pixels.
func garageFit(f typeFace, s string, w int32) string {
	if f.width(s) <= w {
		return s
	}
	for len(s) > 1 && f.width(s+"..") > w {
		s = s[:len(s)-1]
	}
	return s + ".."
}

// garageWrap breaks s into lines no wider than w pixels.
func garageWrap(f typeFace, s string, w int32) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		next := word
		if line != "" {
			next = line + " " + word
		}
		if line != "" && f.width(next) > w {
			lines = append(lines, line)
			next = word
		}
		line = next
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// Drawing. The layout is worked out from the window's size alone.

// garageRects is where everything on the page goes.
type garageRects struct {
	stage, panel, paint, buy rl.Rectangle
	filterY                  int32
	stripX, stripY           int32
	stripW, cardW, cardH     int32
	swatch                   int32
}

const (
	garageMargin  = 56
	garageTop     = 84
	garageGap     = 14
	garageCards   = 7
	garageCardGap = 10
)

// garageLayout divides a w by h window between the stage, the panel, the
// filter and the cards.
func garageLayout(w, h int32) garageRects {
	var r garageRects
	panelW := int32(330)
	if w < 1000 {
		panelW = 260
	}
	r.cardH = 92
	if h < 640 {
		r.cardH = 74
	}
	r.stripX, r.stripW = garageMargin, w-2*garageMargin
	r.cardW = (r.stripW - (garageCards-1)*garageCardGap) / garageCards
	r.stripY = h - pageFoot - r.cardH
	r.filterY = r.stripY - 36
	bottom := r.filterY - garageGap
	stageW := w - 2*garageMargin - panelW - 20
	r.stage = rl.NewRectangle(garageMargin, garageTop, float32(stageW), float32(max(bottom-garageTop, 120)))

	px := float32(garageMargin + stageW + 20)
	r.panel = rl.NewRectangle(px, garageTop, float32(panelW), r.stage.Height)
	r.swatch = min(34, (panelW-36-5*8)/6)
	paintH := 16 + 14 + 6 + 24 + 10 + 2*r.swatch + 8 + 16
	r.paint = rl.NewRectangle(px, garageTop, float32(panelW), float32(paintH))
	buyY := float32(garageTop+paintH) + garageGap
	r.buy = rl.NewRectangle(px, buyY, float32(panelW), max(r.panel.Y+r.panel.Height-buyY, 100))
	return r
}

var (
	faceName    = typeFace{30, weightHeavy}
	faceHeading = typeFace{12, weightBold}
)

// draw is the whole page: the backdrop, the stage, the panel and the line of
// cars, faded in from black.
func (g *garagePage) draw(dt float32) {
	g.age += dt
	w, h := int32(g.s.opts.Width), int32(g.s.opts.Height)
	r := garageLayout(w, h)
	garageBackdrop(w, h)

	faceTitle.draw("GARAGE", garageMargin, 28, signInk)
	lede := fmt.Sprintf("driving the %s in %s", g.driven().Name(), paintOf(g.s.settings.Paint).name)
	if g.chase() {
		lede = fixedShapeNote
	}
	faceBody.draw(lede, garageMargin+faceTitle.width("GARAGE")+18, 28+faceTitle.height()-faceBody.height()-4, signDim)
	earn := "earnings " + money(g.s.career.Earnings)
	ew := faceBody.width(earn) + 28
	rounded(rect(w-garageMargin-ew, 34, ew, 30), 15, signCard)
	faceBody.draw(earn, w-garageMargin-ew+14, 34+(30-faceBody.height())/2, signInk)

	sel := g.list.SelectedKey()
	g.drawStage(r, dt)
	g.drawPaint(r, sel == rowPaint)
	g.drawBuy(r, sel)
	g.drawFilter(r, sel == rowFilter)
	g.drawStrip(r, sel == rowCar)

	if fade := 1 - min(g.age/0.45, 1); fade > 0 {
		rl.DrawRectangle(0, 0, w, h, rl.ColorAlpha(rl.Black, fade))
	}
}

// garageBackdrop is a plain garage: a wall, a floor and the faint lines of
// the panelling.
func garageBackdrop(w, h int32) {
	wall := h * 58 / 100
	rl.DrawRectangleGradientV(0, 0, w, wall, rl.NewColor(15, 19, 25, 255), rl.NewColor(21, 26, 34, 255))
	rl.DrawRectangleGradientV(0, wall, w, h-wall, rl.NewColor(29, 35, 45, 255), rl.NewColor(18, 22, 28, 255))
	for x := int32(0); x < w; x += 160 {
		rl.DrawRectangle(x, 0, 1, wall, rl.NewColor(255, 255, 255, 8))
	}
	rl.DrawRectangle(120, 0, max(w-640, 100), 5, rl.NewColor(230, 232, 236, 40))
}

// drawStage draws the car on its turntable, its tag, its name and its bars.
func (g *garagePage) drawStage(r garageRects, dt float32) {
	st := r.stage
	x, y, w, h := int32(st.X), int32(st.Y), int32(st.Width), int32(st.Height)
	rounded(st, 10, rl.NewColor(20, 24, 31, 255))

	// The bars, in a row if the stage is wide enough and two by two if not.
	inner := w - 48
	cols := int32(4)
	if inner/4 < 150 {
		cols = 2
	}
	rowsN := 4 / cols
	const pitch = 36
	infoH := 34 + 12 + rowsN*pitch + 20
	prevH := max(h-infoH-16, 40)

	// The turntable, fitted to the room above the name.
	rl.DrawRectangleGradientV(x+10, y+10, w-20, prevH-4, rl.NewColor(30, 34, 42, 255), rl.NewColor(14, 16, 20, 255))
	tex := g.s.showroom.render(sim.PlayerBodyOf(g.view), paintOf(g.paint), dt)
	scale := min(float32(w-16)/previewW, float32(prevH)/previewH)
	dw, dh := previewW*scale, previewH*scale
	dx, dy := float32(x)+(float32(w)-dw)/2, float32(y+8)+(float32(prevH)-dh)/2
	tint := rl.White
	if g.standingOf(g.view) == standLocked {
		tint = rl.ColorAlpha(rl.White, 0.5)
	}
	src := rl.NewRectangle(0, 0, previewW, -previewH) // render targets are upside down
	rl.DrawTexturePro(tex, src, rl.NewRectangle(dx, dy, dw, dh), rl.NewVector2(0, 0), 0, tint)

	tag, tn := g.standingOf(g.view).tag()
	cw := drawChip(x+16, y+14, tag, tn)
	if g.standingOf(g.view) == standLocked {
		padlock(x+16+cw+10, y+10, signGrey)
	}
	place := g.place()
	faceSmall.draw(place, x+w-16-faceSmall.width(place), y+17, signDim)
	faceSmall.draw(garageFit(faceSmall, carBlurb(g.view), w-32), x+16, y+44, signDim)

	// The name, the pack and where it stands.
	ny := y + 8 + prevH + 6
	name := garageCap(g.view.Name())
	faceName.draw(name, x+24, ny, signInk)
	px := x + 24 + faceName.width(name) + 12
	faceBody.draw(garagePackName(g.view), px, ny+faceName.height()-faceBody.height()-3, signDim)
	status, stone := g.status()
	status = garageFit(faceSmall, status, w-48-(px-x-24)-faceBody.width(garagePackName(g.view))-24)
	faceSmall.draw(status, x+w-24-faceSmall.width(status), ny+faceName.height()-faceSmall.height()-3, toneInk(stone))

	// The bars, with a tick for the car being driven when it is not this one.
	stats := garageStats(g.view, g.driven())
	showMark := g.view != g.driven()
	colW := (inner - (cols-1)*20) / cols
	by := ny + faceName.height() + 12
	for i, s := range stats {
		bx := x + 24 + int32(i)%cols*(colW+20)
		yy := by + int32(i)/cols*pitch
		faceSmall.draw(s.Name, bx, yy, signDim)
		faceSmall.draw(s.Words, bx+colW-faceSmall.width(s.Words), yy, signInk)
		gy := yy + faceSmall.height() + 6
		drawBar(bx, gy, colW, s.Level, signBlue)
		if showMark {
			rl.DrawRectangle(bx+int32(float64(colW)*s.Mark)-1, gy-3, 2, barH+6, signInk)
		}
	}
	note := "white tick: the car you drive now"
	if !showMark {
		note = "the car you drive now"
	}
	if g.chase() {
		note = fixedShapeNote
	}
	faceChip.draw(note, x+24, y+h-12-faceChip.height(), signDim)
}

// drawPaint draws the paint card: its name, its status and the swatches.
func (g *garagePage) drawPaint(r garageRects, lit bool) {
	c := r.paint
	x, y, w := int32(c.X), int32(c.Y), int32(c.Width)
	drawCard(c, lit)
	faceHeading.draw("PAINT", x+18, y+16, signDim)
	status, tn := g.paintStatus()
	faceSmall.draw(status, x+w-18-faceSmall.width(status), y+14, toneInk(tn))
	faceRow.draw(garageCap(paintOf(g.paint).name), x+18, y+16+14+6, signInk)

	h := g.holdings()
	gx := x + (w-(6*r.swatch+5*8))/2
	gy := y + 16 + 14 + 6 + 24 + 10
	for i, p := range playerPaints {
		cx := gx + int32(i%6)*(r.swatch+8) + r.swatch/2
		cy := gy + int32(i/6)*(r.swatch+8) + r.swatch/2
		col := p.col
		if !h.Wearable(i) {
			col = rl.ColorAlpha(col, 0.38)
		}
		rl.DrawCircle(cx, cy, float32(r.swatch)/2, col)
		if i == g.paint {
			ringCol := signInk
			if lit {
				ringCol = signBlue
			}
			rl.DrawRing(rl.NewVector2(float32(cx), float32(cy)), float32(r.swatch)/2+3, float32(r.swatch)/2+5, 0, 360, 32, ringCol)
		}
	}
}

// drawBuy draws the buying card: the buy buttons, what the card has to say and
// Done.
func (g *garagePage) drawBuy(r garageRects, sel string) {
	c := r.buy
	x, y, w, h := int32(c.X), int32(c.Y), int32(c.Width), int32(c.Height)
	rounded(c, 10, signCard)
	ring(c, 10, 1, signRule)
	faceHeading.draw(g.buyHead(), x+18, y+14, signDim)

	by := y + 36
	for _, v := range g.list.Views() {
		if v.Key != rowBuyCar && v.Key != rowBuyPaint {
			continue
		}
		drawBuyButton(rect(x+18, by, w-36, 40), v)
		by += 48
	}
	doneY := y + h - 16 - 44
	for _, line := range garageWrap(faceSmall, strings.Join(g.notes(), " "), w-36) {
		if by+faceSmall.height() > doneY-4 {
			break
		}
		faceSmall.draw(line, x+18, by+2, signDim)
		by += faceSmall.height() + 4
	}
	drawButton(rect(x+18, doneY, w-36, 44), "Done", sel == rowDone, false, tonePlain)
}

// drawBuyButton draws a buy button: what it buys on the left and its price on
// the right, greyed when earnings fall short.
func drawBuyButton(b rl.Rectangle, v rowView) {
	x, y, w, h := int32(b.X), int32(b.Y), int32(b.Width), int32(b.Height)
	ink := signInk
	switch {
	case v.Disabled != "":
		rounded(b, 6, rl.NewColor(26, 29, 36, 255))
		ring(b, 6, 1, signRule)
		if v.Selected {
			ring(b, 6, 2, signBlue)
		}
		ink = signGrey
	case v.Selected:
		rounded(b, 6, signBlue)
		ink = menuBackdrop
	default:
		rounded(b, 6, signLift)
		ring(b, 6, 1, signRule)
	}
	label := garageFit(faceRow, v.Label, w-32-faceRow.width(v.Value)-12)
	faceRow.draw(label, x+14, y+(h-faceRow.height())/2, ink)
	faceRow.draw(v.Value, x+w-14-faceRow.width(v.Value), y+(h-faceRow.height())/2, ink)
}

// drawFilter draws CARS, the three filters and how many cars pass.
func (g *garagePage) drawFilter(r garageRects, lit bool) {
	x, y := r.stripX, r.filterY
	faceHeading.draw("CARS", x, y+8, signDim)
	x += faceHeading.width("CARS") + 16
	for i, name := range garageFilters {
		w := faceSmall.width(name) + 26
		pill := rect(x, y, w, 26)
		ink := signDim
		if i == g.filter {
			rounded(pill, 13, signInk)
			ink = menuBackdrop
			if lit {
				ring(rect(x-3, y-3, w+6, 32), 16, 2, signBlue)
			}
		} else {
			rounded(pill, 13, signCard)
		}
		faceSmall.draw(name, x+13, y+(26-faceSmall.height())/2, ink)
		x += w + 8
	}
	cars := g.cars()
	count := fmt.Sprintf("%d %s", len(cars), []string{"cars", "yours", "for sale"}[g.filter])
	if at := slices.Index(cars, g.view); at >= 0 {
		count = fmt.Sprintf("%d of %s", at+1, count)
	}
	count += "  |  left/right browse"
	faceChip.draw(count, r.stripX+r.stripW-faceChip.width(count), y+8, signDim)
}

// drawStrip draws the cars along the bottom, seven at a time, the viewed one
// kept in view.
func (g *garagePage) drawStrip(r garageRects, lit bool) {
	cars := g.cars()
	if len(cars) == 0 {
		faceBody.draw("nothing here yet", r.stripX+6, r.stripY+r.cardH/2-faceBody.height()/2, signDim)
		return
	}
	first, last := rowWindow(len(cars), max(slices.Index(cars, g.view), 0), garageCards)
	h := g.holdings()
	for i := first; i < last; i++ {
		k := cars[i]
		x := r.stripX + int32(i-first)*(r.cardW+garageCardGap)
		card := rect(x, r.stripY, r.cardW, r.cardH)
		on := k == g.view
		st := g.standingOf(k)
		rounded(card, 8, signCard)
		switch {
		case on && lit:
			ring(card, 8, 2, signBlue)
		case on:
			ring(card, 8, 2, signInk)
		default:
			ring(card, 8, 1, signRule)
		}

		col := rl.NewColor(92, 99, 112, 255)
		if h.Drivable(k) {
			col = paintOf(g.s.settings.Paint).col
		}
		if on {
			col = paintOf(g.paint).col
		}
		alpha := float32(1)
		if st == standLocked {
			alpha = 0.4
		}
		garageGlyph(x+10, r.stripY+8, r.cardW-20, r.cardH-8-34, sim.PlayerBodyOf(k), rl.ColorAlpha(col, alpha))

		if st == standLocked {
			padlock(x+r.cardW-28, r.stripY+8, signGrey)
		}
		tag, tn := garageCardStatus(st, k)
		ny := r.stripY + r.cardH - 34
		name := garageFit(faceSmall, garageCap(k.Name()), r.cardW-16)
		faceSmall.draw(name, x+8, ny, signInk)
		faceChip.draw(garageFit(faceChip, tag, r.cardW-16), x+8, ny+faceSmall.height()+3, toneInk(tn))
	}
}

// garageCardStatus is the word under a card's name.
func garageCardStatus(st standing, k sim.BodyKind) (string, tone) {
	switch st {
	case standDriving:
		return "driving", toneYours
	case standYours:
		return "yours", toneYours
	case standForSale:
		return money(shop.CarPrice(k)), toneOffer
	default:
		return "locked", toneLocked
	}
}

// garageGlyph draws a car side-on in the box x, y, w by h: a body, a cabin and
// two wheels, in the proportions of the shape.
func garageGlyph(x, y, w, h int32, b sim.Body, col rl.Color) {
	sc := garageScaleOnce()
	k := min(float32(w)/sc.length, float32(h)/sc.height)
	length := int32(2 * b.Spec.HalfLength * k)
	ht := int32(b.Spec.Height * k)
	left := x + (w-length)/2
	base := y + h
	wheel := max(ht/7, 3)

	waist := max(ht*45/100, 4)
	cabinW := int32(float32(length) * min(max(b.RoofFrac, 0.3), 1))
	cabinX := left + (length-cabinW)/2 + length/20
	if b.Boxy {
		cabinX = left + length/10
	}
	rounded(rect(cabinX, base-ht, cabinW, ht-waist/2), 3, col)
	rl.DrawRectangle(cabinX+cabinW/8, base-ht+ht/10, cabinW*3/4, max(ht/5, 2), rl.NewColor(28, 35, 48, 255))
	rounded(rect(left, base-wheel-waist, length, waist), 3, col)
	for _, fx := range []int32{length * 22 / 100, length * 78 / 100} {
		rl.DrawCircle(left+fx, base-wheel, float32(wheel), rl.NewColor(18, 20, 24, 255))
	}
}
