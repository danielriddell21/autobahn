package game

import (
	"slices"
	"strings"
	"testing"

	"github.com/danielriddell21/crucible/menu"

	"github.com/danielriddell21/autobahn/internal/feats"
	"github.com/danielriddell21/autobahn/internal/pack"
)

func careerShell(t *testing.T, banked int) (*Shell, *menuPage) {
	t.Helper()
	sh := testShell(t)
	sh.career.Banked = banked
	sh.openPage(pageCareer)
	if sh.pager.cur == nil || sh.screen != pageCareer {
		t.Fatal("Career did not open")
	}
	return sh, sh.pager.cur
}

func TestCareerIsRegistered(t *testing.T) {
	if _, stub := pageFor(pageCareer); stub {
		t.Fatal("Career is still a stub")
	}
}

func TestPackRouteIsInPriceOrderWithOneNext(t *testing.T) {
	route := packRoute(pack.Access{Banked: 4000})
	for i := 1; i < len(route); i++ {
		if route[i].Pack.Price < route[i-1].Pack.Price {
			t.Errorf("%s comes after %s but costs less", route[i].Pack.Name, route[i-1].Pack.Name)
		}
	}
	held, next, locked := 0, 0, 0
	for _, e := range route {
		switch e.State {
		case packHeld:
			held++
			if e.Left != 0 {
				t.Errorf("held %s has %d left", e.Pack.Name, e.Left)
			}
		case packNext:
			next++
		case packLocked:
			locked++
		}
	}
	if held != 2 || next != 1 || locked != len(route)-3 {
		t.Errorf("held %d next %d locked %d with 4,000 banked", held, next, locked)
	}
	n, ok := careerNextPack(route)
	if !ok || n.Pack.ID != pack.Country || n.Left != pack.Of(pack.Country).Price-4000 {
		t.Errorf("next pack = %+v, %v", n.Pack.Name, ok)
	}
	if n.status() != bountyWords(5000)+" to go" {
		t.Errorf("status %q", n.status())
	}
}

func TestEverythingHeldLeavesNoNextPack(t *testing.T) {
	route := packRoute(pack.Access{Bypass: true})
	if _, ok := careerNextPack(route); ok {
		t.Error("a player with every pack has a next one")
	}
	for _, e := range route {
		if e.State != packHeld {
			t.Errorf("%s is not held under bypass", e.Pack.Name)
		}
	}
}

func TestPacksListTheirMapsAndCars(t *testing.T) {
	var coast packEntry
	for _, e := range packRoute(pack.Access{}) {
		if e.Pack.ID == pack.Coast {
			coast = e
		}
	}
	if !slices.Contains(coast.Maps, "seatown") || len(coast.Cars) != 2 {
		t.Errorf("coast = %v / %v", coast.Maps, coast.Cars)
	}
	if !strings.Contains(coast.contents(), "seatown") {
		t.Errorf("contents %q", coast.contents())
	}
}

func TestHiddenAchievementsAreQuestionMarksUntilEarned(t *testing.T) {
	var f feats.Feats
	var hidden featEntry
	for _, e := range featGrid(f) {
		if e.Feat.Hidden {
			hidden = e
			break
		}
	}
	if hidden.Shown || hidden.name() != "???" || strings.Contains(hidden.note(), hidden.Feat.Note) {
		t.Errorf("a hidden unearned feat gives itself away: %q / %q", hidden.name(), hidden.note())
	}
	f.Unlocked = []string{hidden.Feat.ID}
	for _, e := range featGrid(f) {
		if e.Feat.ID == hidden.Feat.ID && (!e.Earned || e.name() != e.Feat.Name) {
			t.Errorf("earned hidden feat still %q", e.name())
		}
	}
}

func TestAchievementProgressIsShownWhereThereIsSome(t *testing.T) {
	f := feats.Feats{Cars: 0b111}
	seen := false
	for _, e := range featGrid(f) {
		switch e.Feat.ID {
		case "all-cars":
			have, of := f.Progress("all-cars")
			if of == 0 || e.progress() == "" || e.Have != have {
				t.Errorf("all-cars progress %q (%d/%d)", e.progress(), have, of)
			}
			seen = true
		case "first-mile":
			if e.progress() != "" {
				t.Errorf("first-mile shows progress %q", e.progress())
			}
		}
	}
	if !seen {
		t.Error("no all-cars feat")
	}
}

func TestCareerRowsAreAsciiAndEndWithBack(t *testing.T) {
	_, pg := careerShell(t, 4000)
	n := len(packRoute(pack.Access{})) + len(feats.List()) + 1
	if pg.List.Len() != n {
		t.Fatalf("%d rows, want %d", pg.List.Len(), n)
	}
	for _, v := range pg.List.Views() {
		for _, s := range []string{v.Label, v.Value, v.Desc} {
			for _, r := range s {
				if r > 126 {
					t.Errorf("row %s has non-ASCII %q", v.Key, s)
				}
			}
		}
	}
	if pg.List.Views()[n-1].Key != careerBackKey {
		t.Error("the last row is not Back")
	}
}

func TestCareerKeysWalkPacksThenAchievementsThenBack(t *testing.T) {
	sh, pg := careerShell(t, 0)
	if first := pg.List.SelectedKey(); !strings.HasPrefix(first, careerPackKey) {
		t.Fatalf("starts on %q", first)
	}
	for range len(packRoute(pack.Access{})) {
		sh.stepPage(navInput{Input: menu.Input{Down: true}})
	}
	if !strings.HasPrefix(pg.List.SelectedKey(), careerFeatKey) {
		t.Errorf("after the packs: %q", pg.List.SelectedKey())
	}
	pg.List.Select(pg.List.Views()[0].Key)
	sh.stepPage(navInput{Input: menu.Input{Up: true}}) // wraps to Back
	if pg.List.SelectedKey() != careerBackKey {
		t.Fatalf("wrapped to %q", pg.List.SelectedKey())
	}
	sh.stepPage(navInput{Input: menu.Input{Select: true}})
	if sh.screen == pageCareer {
		t.Error("Back did not leave Career")
	}
}

func TestEscLeavesCareer(t *testing.T) {
	sh, _ := careerShell(t, 0)
	sh.stepPage(navInput{Back: true})
	if sh.screen == pageCareer {
		t.Error("esc stayed on Career")
	}
}

func TestDetailSaysWhatTheSelectionNeeds(t *testing.T) {
	sh := testShell(t)
	sh.career.Banked = 4000
	c := &careerPage{s: sh}
	c.rebuild()
	c.list.Select(careerPackKey + "2") // village green
	head, line := c.detail()
	if !strings.Contains(head, "VILLAGE GREEN") || !strings.Contains(head, "NEXT") ||
		!strings.Contains(line, "9,000") || !strings.Contains(line, "5,000 to go") {
		t.Errorf("detail = %q / %q", head, line)
	}
	c.list.Select(careerFeatKey + "first-mile")
	head, line = c.detail()
	if !strings.Contains(head, "FIRST MILE") || !strings.Contains(line, "drive a mile") {
		t.Errorf("detail = %q / %q", head, line)
	}
}

func TestShowingCareerClearsNewPacks(t *testing.T) {
	markNewPacks(pack.All()[1:2])
	t.Cleanup(clearNewPacks)
	sh := testShell(t)
	sh.openPage(pageCareer)
	if hasNewPacks() {
		t.Error("NEW still showing after opening Career")
	}
	markNewPacks(pack.All()[1:2])
	sh.openPage(pageSettings)
	sh.backPage()
	if hasNewPacks() {
		t.Error("NEW still showing after coming back to Career")
	}
}

func TestCareerRebuildsWhenShownAgain(t *testing.T) {
	sh, pg := careerShell(t, 0)
	key := pg.List.SelectedKey()
	sh.career.Banked = 3000
	pg.Shown()
	if pg.List.SelectedKey() != key {
		t.Error("selection lost on refresh")
	}
	r, _ := pg.List.Row(careerPackKey + "1")
	if r.Note() != "held" {
		t.Errorf("coast after banking 3,000 says %q", r.Note())
	}
}
