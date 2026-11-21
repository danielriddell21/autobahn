// Package pack groups the game's content into the base game and the packs
// that add to it, and decides which of them a player has.
//
// A pack is a map and the cars that come with it. Packs are unlocked by
// banking bounty: the price is a lifetime total, so a player works through
// them in order by getting away from the police. Everything ships in the one
// binary; a pack that is locked is simply not offered.
//
// The package draws nothing and stores nothing. Whoever keeps the player's
// career hands the total in as an [Access] and asks what it buys.
package pack

import (
	"slices"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// ID names a pack.
type ID int

// The packs, in the order they unlock.
const (
	// Base is the game itself: the city and the cars it has always had. It
	// is never locked.
	Base ID = iota
	Coast
	Country
	Motorway
	numPacks
)

// Pack is one group of content.
type Pack struct {
	ID   ID
	Name string
	// Blurb is a line saying what is in it.
	Blurb string
	// Price is the lifetime banked bounty that unlocks it.
	Price int
	// Maps and Bodies are what it adds: places to drive, and shapes for the
	// garage. A body no pack lists belongs to the base game.
	Maps   []city.Map
	Bodies []sim.BodyKind
}

// The prices, in banked bounty. They are set so that a player who keeps
// getting away reaches the packs after roughly a quarter of an hour, three
// quarters, and an hour and a half.
const (
	priceCoast    = 8000
	priceCountry  = 25000
	priceMotorway = 50000
)

var packs = [numPacks]Pack{
	Base: {
		ID: Base, Name: "base",
		Blurb: "the city in autumn, and the cars it has always had",
		Maps:  []city.Map{city.MapCity},
	},
	Coast: {
		ID: Coast, Name: "coast", Price: priceCoast,
		Blurb:  "a seaside town in summer",
		Maps:   []city.Map{city.MapSeaside},
		Bodies: []sim.BodyKind{sim.Convertible, sim.CityCar},
	},
	Country: {
		ID: Country, Name: "country", Price: priceCountry,
		Blurb: "a village in spring",
	},
	Motorway: {
		ID: Motorway, Name: "motorway", Price: priceMotorway,
		Blurb: "a motorway town in winter",
	},
}

// All returns every pack, the base game first.
func All() []Pack { return slices.Clone(packs[:]) }

// Of returns the pack with the given ID, or the base game for one that does
// not exist.
func Of(id ID) Pack {
	if id < 0 || id >= numPacks {
		return packs[Base]
	}
	return packs[id]
}

// ForMap returns the pack a map comes in.
func ForMap(m city.Map) Pack { return owner(func(p Pack) bool { return slices.Contains(p.Maps, m) }) }

// ForBody returns the pack a car comes in.
func ForBody(k sim.BodyKind) Pack {
	return owner(func(p Pack) bool { return slices.Contains(p.Bodies, k) })
}

// owner finds the pack that claims something. What nobody claims is the base
// game's, so content added without a pack is never locked by accident.
func owner(claims func(Pack) bool) Pack {
	for _, p := range packs {
		if claims(p) {
			return p
		}
	}
	return packs[Base]
}

// Access is what a player has earned, which is what decides the packs they
// hold.
type Access struct {
	// Banked is the bounty banked over the player's whole career.
	Banked int
	// Bypass holds every pack regardless. It is for development, for the
	// tools that have to exercise everything, and for a player who would
	// rather not earn it.
	Bypass bool
}

// Has reports whether the pack is unlocked.
func (a Access) Has(id ID) bool { return a.Bypass || a.Banked >= Of(id).Price }

// HasMap reports whether the map may be driven.
func (a Access) HasMap(m city.Map) bool { return a.Has(ForMap(m).ID) }

// HasBody reports whether the car may be driven.
func (a Access) HasBody(k sim.BodyKind) bool { return a.Has(ForBody(k).ID) }

// Maps returns the maps on offer, in the order the game lists them.
func (a Access) Maps() []city.Map {
	var out []city.Map
	for _, m := range city.Maps() {
		if a.HasMap(m) {
			out = append(out, m)
		}
	}
	return out
}

// Bodies returns the cars the garage offers, in the order it lists them.
func (a Access) Bodies() []sim.BodyKind {
	var out []sim.BodyKind
	for _, k := range sim.PlayerBodies() {
		if a.HasBody(k) {
			out = append(out, k)
		}
	}
	return out
}

// Progress is how far along the player is toward a pack, from 0 to 1.
func (a Access) Progress(id ID) float64 {
	price := Of(id).Price
	if a.Has(id) || price <= 0 {
		return 1
	}
	return float64(max(a.Banked, 0)) / float64(price)
}

// Newly returns the packs that banking took the career from one total to
// another unlocked, cheapest first.
func Newly(before, after int) []Pack {
	var out []Pack
	for _, p := range packs {
		if before < p.Price && p.Price <= after {
			out = append(out, p)
		}
	}
	slices.SortStableFunc(out, func(x, y Pack) int { return x.Price - y.Price })
	return out
}
