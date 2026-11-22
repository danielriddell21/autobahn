// Package shop prices the garage's cars and paints and decides what a player
// may drive, wear and buy.
//
// Bounty banked over a career opens packs (see [pack]); earnings, a separate
// purse, buy things from the shop. A car is for sale only once its pack is
// held, and a paint bought once works on every car. The package draws nothing
// and stores nothing: whoever keeps the career hands in [Holdings].
//
// Cars are named by sim.BodyKind and paints by their index in the garage's
// list, the same integers the settings file stores. Neither list may be
// reordered, only grown at the end.
package shop

import (
	"errors"
	"fmt"
	"slices"

	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// Car prices, in earnings.
const (
	priceHatchback   = 1500
	priceVan         = 2000
	priceSaloon      = 2500
	priceEstate      = 2500
	priceTaxi        = 3500
	priceCityCar     = 3000
	priceConvertible = 6000
	price4x4         = 7000
	priceSUV         = 9000
	priceMinibus     = 8000
	priceGT          = 15000
)

// Paint prices.
const (
	pricePlain  = 300
	priceBright = 600
)

// carPrices is what each player car costs. The hot hatch is the starter.
var carPrices = map[sim.BodyKind]int{
	sim.HotHatch:    0,
	sim.Hatchback:   priceHatchback,
	sim.Van:         priceVan,
	sim.Saloon:      priceSaloon,
	sim.Estate:      priceEstate,
	sim.Taxi:        priceTaxi,
	sim.CityCar:     priceCityCar,
	sim.Convertible: priceConvertible,
	sim.FourByFour:  price4x4,
	sim.SUV:         priceSUV,
	sim.Minibus:     priceMinibus,
	sim.GrandTourer: priceGT,
}

// paintPrices is indexed by the garage's paint list: rosso, racing blue,
// arctic white, jet black, sunburst yellow, british racing green, silver,
// plum, tangerine, lime, teal, gunmetal.
var paintPrices = []int{
	0, pricePlain, pricePlain, pricePlain, priceBright, 0,
	pricePlain, priceBright, priceBright, priceBright, priceBright, pricePlain,
}

// secretPaints are never for sale: they are reserved for achievements.
var secretPaints = []int{5} // british racing green

// CarPrice is what a car costs. A shape the player cannot drive costs 0.
func CarPrice(k sim.BodyKind) int { return carPrices[k] }

// PaintPrice is what a paint costs, or 0 for one that does not exist.
func PaintPrice(i int) int {
	if i < 0 || i >= len(paintPrices) {
		return 0
	}
	return paintPrices[i]
}

// PaintCount is how many paints are priced.
func PaintCount() int { return len(paintPrices) }

// Secret reports whether a paint can only be won, never bought.
func Secret(i int) bool { return slices.Contains(secretPaints, i) }

// Kind says what an Item is.
type Kind int

// The kinds of thing for sale.
const (
	Car Kind = iota
	Paint
)

// Item is one thing for sale: a car by its BodyKind, or a paint by index.
type Item struct {
	Kind Kind
	ID   int
}

// CarItem and PaintItem build Items.
func CarItem(k sim.BodyKind) Item { return Item{Car, int(k)} }
func PaintItem(i int) Item        { return Item{Paint, i} }

// Price is what the item costs.
func (it Item) Price() int {
	if it.Kind == Car {
		return CarPrice(sim.BodyKind(it.ID))
	}
	return PaintPrice(it.ID)
}

// Refusals CanBuy gives.
var (
	ErrOwned  = errors.New("already owned")
	ErrLocked = errors.New("its pack is locked")
	ErrSecret = errors.New("not for sale")
	ErrShort  = errors.New("not enough earnings")
)

// Holdings is what a player has: the packs their bounty opened, the earnings
// in hand, and the cars and paints they own.
type Holdings struct {
	Access   pack.Access
	Earnings int
	Cars     []int
	Paints   []int
}

// OwnsCar reports whether the car is owned. What costs nothing always is.
func (h Holdings) OwnsCar(k sim.BodyKind) bool {
	return CarPrice(k) == 0 && sim.IsPlayerBody(k) || slices.Contains(h.Cars, int(k))
}

// OwnsPaint reports whether the paint is owned.
func (h Holdings) OwnsPaint(i int) bool {
	return i >= 0 && i < len(paintPrices) && (paintPrices[i] == 0 && !Secret(i) || slices.Contains(h.Paints, i))
}

// Drivable reports whether the car may be driven: its pack is held and the
// car is owned, or the access bypasses both.
func (h Holdings) Drivable(k sim.BodyKind) bool {
	if h.Access.Bypass {
		return true
	}
	return h.Access.HasBody(k) && h.OwnsCar(k)
}

// Wearable reports whether the paint may be worn.
func (h Holdings) Wearable(i int) bool {
	return h.Access.Bypass && i >= 0 && i < len(paintPrices) || h.OwnsPaint(i)
}

// Drivables returns the cars that may be driven, in the garage's order.
func (h Holdings) Drivables() []sim.BodyKind {
	var out []sim.BodyKind
	for _, k := range sim.PlayerBodies() {
		if h.Drivable(k) {
			out = append(out, k)
		}
	}
	return out
}

// Wearables returns the paints that may be worn, in the garage's order.
func (h Holdings) Wearables() []int {
	var out []int
	for i := range paintPrices {
		if h.Wearable(i) {
			out = append(out, i)
		}
	}
	return out
}

// ForSale lists what the shop offers: cars whose pack is held, and paints that
// are not secret, that the player does not own. A locked pack's cars never
// appear, and nothing is for sale to a player who bypasses it all.
func (h Holdings) ForSale() []Item {
	if h.Access.Bypass {
		return nil
	}
	var out []Item
	for _, k := range sim.PlayerBodies() {
		if h.Access.HasBody(k) && !h.OwnsCar(k) {
			out = append(out, CarItem(k))
		}
	}
	for i := range paintPrices {
		if !Secret(i) && !h.OwnsPaint(i) {
			out = append(out, PaintItem(i))
		}
	}
	return out
}

// CanBuy returns nil if the player could buy the item now, or says why not.
func (h Holdings) CanBuy(it Item) error {
	switch it.Kind {
	case Car:
		k := sim.BodyKind(it.ID)
		switch {
		case !sim.IsPlayerBody(k):
			return ErrSecret
		case h.OwnsCar(k):
			return ErrOwned
		case !h.Access.HasBody(k):
			return ErrLocked
		}
	case Paint:
		switch {
		case it.ID < 0 || it.ID >= len(paintPrices) || Secret(it.ID):
			return ErrSecret
		case h.OwnsPaint(it.ID):
			return ErrOwned
		}
	default:
		return ErrSecret
	}
	if p := it.Price(); h.Earnings < p {
		return fmt.Errorf("%w: %d short", ErrShort, p-h.Earnings)
	}
	return nil
}
