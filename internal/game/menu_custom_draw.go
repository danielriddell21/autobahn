package game

import (
	"fmt"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// The custom map page's drawing: the sections and rows on the left, the plan
// on the right and the share card under it. What is drawn is worked out in
// menu_custom.go and menu_custom_plan.go; nothing here decides anything.

// customFrame is where the page's parts go in a w by h window, in pixels.
type customFrame struct {
	Tabs, Rows, Plan, Card, Drive rl.Rectangle
}

// customFrameFor lays the page out. The rows are on the left under the
// section tabs, the plan above the card on the right.
func customFrameFor(w, h int32) customFrame {
	leftW := min(max(w*42/100, 420), 540)
	rightX := pageMargin + leftW + 28
	rightW := max(w-pageMargin-rightX, 200)
	const cardH, gap = 96, 12
	cardY := h - pageFoot - cardH
	f := customFrame{
		Tabs: rect(pageMargin, 84, leftW, 34),
		Rows: rect(pageMargin, 130, leftW, max(cardY+cardH-130, 0)),
		Plan: rect(rightX, 28, rightW, max(cardY-gap-28, 0)),
		Card: rect(rightX, cardY, rightW, cardH),
	}
	f.Drive = rect(rightX+rightW-196, cardY+(cardH-56)/2, 184, 56)
	return f
}

// planInk is the colours a plan is drawn in for a season. The ground is the
// season's, and the open ground and the buildings are tinted to it; the roads
// and the sea are the same all year.
type planInk struct {
	Ground, Open, Built, Tall, Sand rl.Color
}

// planInkFor is the plan's colours for a season.
func planInkFor(s city.Season) planInk {
	switch s {
	case city.Summer:
		return planInk{
			Ground: rl.NewColor(51, 64, 44, 255), Open: rl.NewColor(66, 112, 58, 255),
			Built: rl.NewColor(70, 76, 88, 255), Tall: rl.NewColor(132, 138, 150, 255), Sand: rl.NewColor(201, 185, 138, 255),
		}
	case city.Spring:
		return planInk{
			Ground: rl.NewColor(44, 62, 46, 255), Open: rl.NewColor(86, 128, 70, 255),
			Built: rl.NewColor(70, 76, 90, 255), Tall: rl.NewColor(132, 140, 154, 255), Sand: rl.NewColor(196, 184, 144, 255),
		}
	case city.Winter:
		return planInk{
			Ground: rl.NewColor(62, 68, 76, 255), Open: rl.NewColor(150, 160, 170, 255),
			Built: rl.NewColor(88, 94, 106, 255), Tall: rl.NewColor(150, 158, 170, 255), Sand: rl.NewColor(186, 186, 178, 255),
		}
	default:
		return planInk{
			Ground: rl.NewColor(58, 52, 40, 255), Open: rl.NewColor(98, 106, 52, 255),
			Built: rl.NewColor(76, 72, 74, 255), Tall: rl.NewColor(140, 134, 130, 255), Sand: rl.NewColor(194, 176, 128, 255),
		}
	}
}

var (
	planSea     = rl.NewColor(42, 90, 122, 255)
	planMain    = rl.NewColor(216, 210, 192, 255)
	planCollect = rl.NewColor(184, 182, 170, 255)
	planSide    = rl.NewColor(142, 148, 156, 255)
	planCasing  = rl.NewColor(16, 18, 22, 200)
	planPill    = rl.NewColor(12, 14, 18, 210)
)

// planView maps ground metres to pixels: the patch is scaled to cover the pane
// and centred in it, so a narrow window crops the patch rather than showing a
// gap.
type planView struct {
	pane  rl.Rectangle
	scale float32
	mid   mathx.Vec
}

func newPlanView(pl *customPlan, pane rl.Rectangle) planView {
	w, h := pl.Max.X-pl.Min.X, pl.Max.Z-pl.Min.Z
	return planView{
		pane:  pane,
		scale: max(pane.Width/w, pane.Height/h),
		mid:   mathx.V((pl.Min.X+pl.Max.X)/2, (pl.Min.Z+pl.Max.Z)/2),
	}
}

// px is where a point of the ground is on the screen.
func (v planView) px(p mathx.Vec) rl.Vector2 {
	return rl.NewVector2(
		v.pane.X+v.pane.Width/2+(p.X-v.mid.X)*v.scale,
		v.pane.Y+v.pane.Height/2+(p.Z-v.mid.Z)*v.scale,
	)
}

// across is how many kilometres wide the pane shows.
func (v planView) across() float32 { return v.pane.Width / v.scale / 1000 }

// draw is the whole page.
func (p *customPage) draw(float32) {
	w, h := int32(p.s.opts.Width), int32(p.s.opts.Height)
	f := customFrameFor(w, h)
	faceTitle.draw("CUSTOM MAP", pageMargin, 28, signInk)
	p.drawTabs(f.Tabs)
	p.drawRows(f.Rows)
	p.drawPlan(f.Plan)
	p.drawCard(f.Card, f.Drive)
}

// drawTabs draws the sections, the one the selection is in lit.
func (p *customPage) drawTabs(r rl.Rectangle) {
	rounded(r, 10, rl.ColorAlpha(signCard, 0.85))
	n := float32(len(customSections))
	w := (r.Width - 8 - 4*(n-1)) / n
	on := p.section()
	for i, sec := range customSections {
		tab := rl.NewRectangle(r.X+4+float32(i)*(w+4), r.Y+4, w, r.Height-8)
		ink := signDim
		if i == on {
			rounded(tab, 7, signBlue)
			ink = menuBackdrop
		}
		x := int32(tab.X) + (int32(tab.Width)-faceSmall.width(sec.Name))/2
		faceSmall.draw(sec.Name, x, int32(tab.Y)+(int32(tab.Height)-faceSmall.height())/2, ink)
	}
}

// drawRows draws the rows of the section the selection is in.
func (p *customPage) drawRows(r rl.Rectangle) {
	views := p.list.Views()
	idx := p.sectionRows(p.section())
	x, y, w := int32(r.X), int32(r.Y), int32(r.Width)
	sel := 0
	for k, at := range idx {
		if views[at].Selected {
			sel = k
		}
	}
	first, last := rowWindow(len(idx), sel, int(r.Height)/pageRowH)
	// A panel under the rows, so the words stand on something steady whatever
	// the menu drive shows through the fade.
	rounded(rect(x-8, y-6, w+16, int32(last-first)*pageRowH+8), 10, rl.ColorAlpha(signCard, 0.92))
	for k := first; k < last; k++ {
		drawRowView(rect(x, y, w, pageRowH-4), views[idx[k]])
		y += pageRowH
	}
}

// drawPlan draws the plan of the map, tinted for its season.
func (p *customPage) drawPlan(r rl.Rectangle) {
	v, ok := drawPlanPicture(r, p.plan, p.cs.spec.Season(), p.cache.Stale(p.cs.spec))
	if !ok {
		return
	}
	x, y := int32(r.X)+14, int32(r.Y)+12
	planChip(x, y, fmt.Sprintf("PLAN  -  %.1f KM ACROSS", v.across()), signInk)
	season := p.cs.spec.Season().Name()
	sw := faceChip.width(strings.ToUpper(season)) + 16
	planChip(int32(r.X+r.Width)-14-sw, y, season, signInk)
	if p.cs.Flagged() {
		planChip(x, int32(r.Y+r.Height)-32, "UNSUPPORTED  -  past what the gate has measured", signAmber)
	}
}

// drawPlanPicture draws a plan into r, tinted for its season: the ground in
// the season given until there is a plan, then the plan, dimmed while stale.
// It returns the view the plan was drawn with, and false before there is one.
// The custom map page and the title page's card both draw with it.
func drawPlanPicture(r rl.Rectangle, pl *customPlan, season city.Season, stale bool) (planView, bool) {
	ink := planInkFor(season)
	rounded(r, 12, ink.Ground)
	if pl == nil {
		return planView{}, false
	}
	v := newPlanView(pl, r)
	rl.BeginScissorMode(int32(r.X), int32(r.Y), int32(r.Width), int32(r.Height))
	ink = planInkFor(pl.Season)
	rl.DrawRectangleRec(r, ink.Ground)
	if pl.Sea {
		top := v.px(mathx.V(pl.Min.X, pl.SeaZ)).Y
		rl.DrawRectangle(int32(r.X), int32(top), int32(r.Width), int32(r.Y+r.Height-top)+1, planSea)
	}
	for _, b := range pl.Blocks {
		fillBlock(v, b, ink)
	}
	for _, s := range pl.Contours {
		a := 0.16
		if s.Major {
			a = 0.34
		}
		rl.DrawLineEx(v.px(s.A), v.px(s.B), 1, rl.ColorAlpha(rl.White, float32(a)))
	}
	for _, class := range [...]city.RoadClass{city.Local, city.Collector, city.Arterial} {
		for _, road := range pl.Roads {
			if road.Class == class {
				strokeRoad(v, road)
			}
		}
	}
	for _, c := range pl.Rounds {
		at, rad := v.px(c.At), max(c.R*v.scale, 5)
		rl.DrawCircleV(at, rad+1.5, planCasing)
		rl.DrawCircleV(at, rad, planMain)
		rl.DrawCircleV(at, rad*0.55, ink.Open)
	}
	if stale {
		rl.DrawRectangleRec(r, rl.ColorAlpha(ink.Ground, 0.35))
	}
	rl.EndScissorMode()
	return v, true
}

// planChip draws a tag on the plan, on a dark pill so it reads over any map.
func planChip(x, y int32, label string, ink rl.Color) {
	label = strings.ToUpper(label)
	w := faceChip.width(label) + 16
	rounded(rect(x, y, w, 20), 10, planPill)
	faceChip.draw(label, x+8, y+4, ink)
}

// fillBlock draws one block: the open ground, the sand, or the buildings shaded
// by how tall they are.
func fillBlock(v planView, b planBlock, ink planInk) {
	col := lerpColor(ink.Built, ink.Tall, b.Shade)
	switch b.Kind {
	case planOpen:
		col = ink.Open
	case planBeach:
		col = ink.Sand
	}
	pts := make([]rl.Vector2, len(b.Pts))
	var mid rl.Vector2
	for i, p := range b.Pts {
		pts[i] = v.px(p)
		mid.X += pts[i].X / float32(len(pts))
		mid.Y += pts[i].Y / float32(len(pts))
	}
	for i := range pts {
		a, c := pts[i], pts[(i+1)%len(pts)]
		// raylib wants a triangle counter-clockwise, which on the screen,
		// with y running down, is a negative cross product.
		if (a.X-mid.X)*(c.Y-mid.Y)-(a.Y-mid.Y)*(c.X-mid.X) > 0 {
			a, c = c, a
		}
		rl.DrawTriangle(mid, a, c, col)
	}
}

// strokeRoad draws a street, the main roads widest, with a dark edge.
func strokeRoad(v planView, r planRoad) {
	width, col := float32(1.6), planSide
	switch r.Class {
	case city.Arterial:
		width, col = 5, planMain
	case city.Collector:
		width, col = 3, planCollect
	}
	for pass := range 2 {
		wd, c := width+2, planCasing
		if pass == 1 {
			wd, c = width, col
		}
		for i := 0; i+1 < len(r.Pts); i++ {
			rl.DrawLineEx(v.px(r.Pts[i]), v.px(r.Pts[i+1]), wd, c)
		}
		if width >= 3 {
			for _, p := range r.Pts {
				rl.DrawCircleV(v.px(p), wd/2, c)
			}
		}
	}
}

// lerpColor is the colour t of the way from a to b.
func lerpColor(a, b rl.Color, t float32) rl.Color {
	mix := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t) }
	return rl.NewColor(mix(a.R, b.R), mix(a.G, b.G), mix(a.B, b.B), 255)
}

// drawCard draws the share code, what the map banks and the Drive button.
func (p *customPage) drawCard(r, drive rl.Rectangle) {
	drawCard(r, false)
	x, y := int32(r.X)+18, int32(r.Y)+12
	faceChip.draw("SHARE CODE  -  BANKS NOTHING", x, y, signDim)

	code := p.cs.Code()
	room := drive.X - r.X - 36
	face := typeFace{20, weightHeavy}
	for face.size > 12 && float32(face.width(code)) > room {
		face.size--
	}
	face.draw(code, x, y+20, signInk)

	if line, t := p.status(); line != "" {
		faceSmall.draw(line, x, y+50, toneInk(t))
	}

	_, selected := p.list.Selected()
	on := selected && p.list.SelectedKey() == ckDrive
	_, locked := p.driveBlocked()
	hitRow(drive, ckDrive)
	drawButton(drive, "Drive this map", on, locked, tonePlain)
}

// driveBlocked says why the map cannot be driven, and whether it cannot.
func (p *customPage) driveBlocked() (string, bool) {
	if err := p.cs.Playable(); err != nil {
		return err.Error(), true
	}
	return "", false
}
