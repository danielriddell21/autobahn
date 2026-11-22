package shop

import (
	"errors"
	"testing"

	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/sim"
)

func TestEveryPlayerCarHasAPrice(t *testing.T) {
	for _, k := range sim.PlayerBodies() {
		if _, ok := carPrices[k]; !ok {
			t.Errorf("the %s has no price", k.Name())
		}
	}
	if len(carPrices) != len(sim.PlayerBodies()) {
		t.Errorf("%d prices for %d cars", len(carPrices), len(sim.PlayerBodies()))
	}
}

func TestStartersAreFree(t *testing.T) {
	if CarPrice(sim.HotHatch) != 0 || PaintPrice(0) != 0 {
		t.Error("the hot hatch and rosso must cost nothing")
	}
	h := Holdings{}
	if !h.Drivable(sim.HotHatch) || !h.Wearable(0) {
		t.Error("a player with nothing must have the starters")
	}
}

func TestNothingForSaleWhilePackLocked(t *testing.T) {
	h := Holdings{Earnings: 1_000_000}
	for _, it := range h.ForSale() {
		if it.Kind == Car && !h.Access.HasBody(sim.BodyKind(it.ID)) {
			t.Errorf("%s is for sale with its pack locked", sim.BodyKind(it.ID).Name())
		}
	}
	if err := h.CanBuy(CarItem(sim.GrandTourer)); !errors.Is(err, ErrLocked) {
		t.Errorf("buying a locked pack's car gave %v", err)
	}
	h.Access.Banked = pack.Of(pack.Motorway).Price
	if err := h.CanBuy(CarItem(sim.GrandTourer)); err != nil {
		t.Errorf("with the pack held: %v", err)
	}
}

func TestAffordability(t *testing.T) {
	it := CarItem(sim.Van)
	h := Holdings{Earnings: CarPrice(sim.Van) - 1}
	if err := h.CanBuy(it); !errors.Is(err, ErrShort) {
		t.Errorf("one short gave %v", err)
	}
	h.Earnings++
	if err := h.CanBuy(it); err != nil {
		t.Errorf("at the price gave %v", err)
	}
	h.Cars = []int{int(sim.Van)}
	if err := h.CanBuy(it); !errors.Is(err, ErrOwned) {
		t.Errorf("an owned car gave %v", err)
	}
}

func TestBypassMakesEverythingDrivable(t *testing.T) {
	h := Holdings{Access: pack.Access{Bypass: true}}
	for _, k := range sim.PlayerBodies() {
		if !h.Drivable(k) {
			t.Errorf("%s not drivable under bypass", k.Name())
		}
	}
	for i := range PaintCount() {
		if !h.Wearable(i) {
			t.Errorf("paint %d not wearable under bypass", i)
		}
	}
	if len(h.ForSale()) != 0 {
		t.Error("nothing should be for sale under bypass")
	}
}

func TestSecretPaintsAreNeverForSale(t *testing.T) {
	h := Holdings{Earnings: 1_000_000, Access: pack.Access{Banked: 1_000_000}}
	n := 0
	for i := range PaintCount() {
		if !Secret(i) {
			continue
		}
		n++
		if err := h.CanBuy(PaintItem(i)); !errors.Is(err, ErrSecret) {
			t.Errorf("secret paint %d: %v", i, err)
		}
		for _, it := range h.ForSale() {
			if it == PaintItem(i) {
				t.Errorf("secret paint %d is for sale", i)
			}
		}
		if h.Wearable(i) {
			t.Errorf("secret paint %d is worn by someone who has not won it", i)
		}
	}
	if n == 0 {
		t.Error("no secret paint is reserved")
	}
}

func TestForSaleHoldsOnlyUnownedItems(t *testing.T) {
	h := Holdings{Cars: []int{int(sim.Van)}, Paints: []int{1}}
	for _, it := range h.ForSale() {
		if it == CarItem(sim.Van) || it == PaintItem(1) || it == CarItem(sim.HotHatch) || it == PaintItem(0) {
			t.Errorf("%v is owned but for sale", it)
		}
	}
}
