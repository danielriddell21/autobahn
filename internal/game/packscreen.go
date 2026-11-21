package game

import (
	"fmt"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/pack"
)

// Packs screen layout, in pixels. The packs are stacked in a column the width
// of the rest of the menus rather than spread across the window.
const (
	cardWidth    = 460
	cardHeight   = 64
	cardGap      = 12
	cardPad      = 16
	cardNameSize = 22
	cardTextSize = 14
	cardBar      = 4
)

// The cards keep to the menus' own colours: grey on near-black, with the one
// blue the menus select with for the bar.
var (
	cardFace  = rl.NewColor(20, 23, 28, 255)
	cardTrack = rl.NewColor(34, 38, 46, 255)
)

// packCard is what one card on the packs screen says: the pack, and how far
// the player is toward it.
type packCard struct {
	title string
	held  bool
	// progress runs 0 to 1 toward the price, and bounty words it.
	progress float64
	bounty   string
}

// cardFor words a pack for a player with the given access.
func cardFor(p pack.Pack, a pack.Access) packCard {
	c := packCard{title: p.Name, held: a.Has(p.ID), progress: a.Progress(p.ID)}
	if !c.held {
		c.bounty = fmt.Sprintf("%s / %s", money(min(a.Banked, p.Price)), money(p.Price))
	}
	return c
}

// drawPacks stacks the packs as rows, each with how far there is to go.
func (s *Shell) drawPacks() {
	w, h := int32(s.opts.Width), int32(s.opts.Height)
	all := pack.All()
	n := int32(len(all))
	stack := n*cardHeight + (n-1)*cardGap
	top := max(h/2-stack/2+20, 150)
	x := w/2 - cardWidth/2

	centred("PACKS", w, top-menuTitleGap-24, menuTitleSize, rl.RayWhite)
	centred("what you bank getting away from the police unlocks the packs",
		w, top-menuTitleGap-24+menuTitleSize+12, menuSubSize, hudDim)
	centred("banked so far  "+money(s.career.Banked), w, top-menuTitleGap-24+menuTitleSize+30, menuSubSize, hudText)

	a := s.access()
	for i, p := range all {
		drawPackCard(cardFor(p, a), x, top+int32(i)*(cardHeight+cardGap))
	}

	back := "> Back"
	rl.DrawText(back, w/2-rl.MeasureText(back, menuItemSize)/2, top+stack+24, menuItemSize, menuSelected)
}

// drawPackCard draws one row: the name on the left, the bounty or the word
// that it is held on the right, and the bar along the foot.
func drawPackCard(c packCard, x, y int32) {
	name, label, ink := rl.RayWhite, "unlocked", hudDim
	if !c.held {
		name, label, ink = hudDim, c.bounty, hudText
	}
	rl.DrawRectangle(x, y, cardWidth, cardHeight, cardFace)
	rl.DrawRectangleLines(x, y, cardWidth, cardHeight, menuRule)

	rl.DrawText(strings.ToUpper(c.title), x+cardPad, y+cardPad, cardNameSize, name)
	right := x + cardWidth - cardPad
	if !c.held {
		padlock(right-18, y+cardPad-3, hudDim)
		right -= 30
	}
	rl.DrawText(label, right-rl.MeasureText(label, cardTextSize), y+cardPad+5, cardTextSize, ink)

	barY, barW := y+cardHeight-cardPad+2, int32(cardWidth-2*cardPad)
	rl.DrawRectangle(x+cardPad, barY, barW, cardBar, cardTrack)
	rl.DrawRectangle(x+cardPad, barY, int32(float64(barW)*c.progress), cardBar, menuSelected)
}

// padlock draws a small closed lock with its top-left corner at x, y.
func padlock(x, y int32, col rl.Color) {
	rl.DrawRectangleLines(x+4, y, 10, 12, col)
	rl.DrawRectangleLines(x+5, y+1, 8, 10, col)
	rl.DrawRectangle(x, y+9, 18, 14, col)
}
