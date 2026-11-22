package game

import (
	"fmt"
	"slices"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/menu"

	"github.com/danielriddell21/autobahn/internal/shop"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// The shop lists what is for sale, cheapest first, as cards in the style of
// the packs screen. Only what the player can afford is selectable. A purchase
// is made from the title or the pause menu of a single-player drive, never
// during a chase against another person (escape leaves one rather than
// pausing it), so what is bought only takes effect from the next session.

const (
	shopRows      = 7 // cards shown at once
	shopCardH     = 44
	shopCardGap   = 6
	shopCardNameS = 20
)

// holdings is what the player has, in the shop's terms.
func (s *Shell) holdings() shop.Holdings { return s.career.holdings(s.access()) }

// shopStock is what the shop lists, cheapest first, and which of it the
// player can buy now.
func (s *Shell) shopStock() (rows []shop.Item, buyable []shop.Item) {
	h := s.holdings()
	rows = h.ForSale()
	slices.SortStableFunc(rows, func(a, b shop.Item) int { return a.Price() - b.Price() })
	for _, it := range rows {
		if h.CanBuy(it) == nil {
			buyable = append(buyable, it)
		}
	}
	return rows, buyable
}

// itemName words an item for a card.
func itemName(it shop.Item) string {
	if it.Kind == shop.Car {
		return carOf(it).Name()
	}
	return paintOf(it.ID).name + " paint"
}

// carOf is the shape a car item stands for.
func carOf(it shop.Item) sim.BodyKind { return sim.BodyKind(it.ID) }

// showShop opens the shop. sel is the row to leave the cursor on.
func (s *Shell) showShop(sel int) {
	s.screen = screenShop
	rows, buyable := s.shopStock()
	sub := fmt.Sprintf("%d more for sale, earnings %s", len(rows), money(s.career.Earnings))
	m := &menu.Menu{Title: "SHOP", Subtitle: []string{sub}}
	for _, it := range buyable {
		m.Items = append(m.Items, menu.Item{
			Label:  text(itemName(it)),
			Action: func() { s.buy(it) },
		})
	}
	m.Items = append(m.Items, menu.Item{Label: text("Back"), Action: s.showTitle})
	m.Sel = min(max(sel, 0), len(m.Items)-1)
	s.menu = m
}

// buy spends earnings on an item, saves, and points the garage at it.
func (s *Shell) buy(it shop.Item) {
	var err error
	if it.Kind == shop.Car {
		err = s.career.BuyCar(s.access(), carOf(it))
	} else {
		err = s.career.BuyPaint(s.access(), it.ID)
	}
	if err != nil {
		s.notice = err.Error()
		s.showShop(s.menu.Sel)
		return
	}
	if err := s.career.Save(); err != nil {
		s.notice = err.Error()
	}
	if it.Kind == shop.Car {
		s.settings.Car = carOf(it)
	} else {
		s.settings.Paint = it.ID
	}
	if err := s.settings.Save(); err != nil {
		s.notice = err.Error()
	}
	s.applyCar()
	s.showShop(s.menu.Sel)
}

// drawShop draws the cards: what each is, and what it costs, dim when it is
// out of reach.
func (s *Shell) drawShop() {
	w, h := int32(s.opts.Width), int32(s.opts.Height)
	rows, buyable := s.shopStock()
	shown := min(len(rows), shopRows)
	stack := int32(shown)*(shopCardH+shopCardGap) + 40
	top := max(h/2-stack/2+30, 150)
	x := w/2 - cardWidth/2

	centred("SHOP", w, top-menuTitleGap-24, menuTitleSize, rl.RayWhite)
	for i, line := range s.menu.Subtitle {
		centred(line, w, top-menuTitleGap-24+menuTitleSize+12+int32(i)*18, menuSubSize, hudDim)
	}

	// The window follows the cursor.
	sel := -1
	if s.menu.Sel < len(buyable) {
		sel = slices.Index(rows, buyable[s.menu.Sel])
	}
	first := 0
	if sel >= shopRows {
		first = sel - shopRows + 1
	}
	for i := first; i < min(first+shopRows, len(rows)); i++ {
		y := top + int32(i-first)*(shopCardH+shopCardGap)
		ok := slices.Contains(buyable, rows[i])
		name, ink := rl.RayWhite, hudText
		if !ok {
			name, ink = hudDim, hudDim
		}
		rl.DrawRectangle(x, y, cardWidth, shopCardH, cardFace)
		edge := menuRule
		if i == sel {
			edge = menuSelected
		}
		rl.DrawRectangleLines(x, y, cardWidth, shopCardH, edge)
		rl.DrawText(strings.ToUpper(itemName(rows[i])), x+cardPad, y+(shopCardH-shopCardNameS)/2, shopCardNameS, name)
		price := money(rows[i].Price())
		rl.DrawText(price, x+cardWidth-cardPad-rl.MeasureText(price, cardTextSize+2), y+(shopCardH-cardTextSize)/2, cardTextSize+2, ink)
	}
	if len(rows) == 0 {
		centred("you own everything on offer", w, top, menuItemSize, hudDim)
	}

	back := "Back"
	col := hudText
	if s.menu.Sel == len(buyable) {
		back, col = "> Back", menuSelected
	}
	by := top + int32(max(shown, 1))*(shopCardH+shopCardGap) + 18
	rl.DrawText(back, w/2-rl.MeasureText(back, menuItemSize)/2, by, menuItemSize, col)
}
