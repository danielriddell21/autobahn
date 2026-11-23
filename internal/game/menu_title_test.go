package game

import (
	"slices"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/pack"
)

func TestTitleRowsAndOpening(t *testing.T) {
	clearNewPacks()
	t.Cleanup(clearNewPacks)
	sh := testShell(t)
	sh.openPage(pageTitle)
	if sh.screen != pageTitle || !sh.onPage() {
		t.Fatal("openPage(pageTitle) did not show the title page")
	}
	var got []string
	for _, v := range sh.pager.cur.List.Views() {
		got = append(got, v.Key)
		for _, r := range v.Label + v.Value {
			if r > 127 {
				t.Errorf("row %s has non-ASCII text %q", v.Key, v.Label+v.Value)
			}
		}
	}
	want := []string{"play", "garage", "career", "settings", "quit"}
	if !slices.Equal(got, want) {
		t.Fatalf("rows %v, want %v", got, want)
	}
	for key, id := range map[string]screen{"play": pagePlay, "garage": pageGarage, "career": pageCareer, "settings": pageSettings, "quit": pageQuit} {
		sh.openPage(pageTitle)
		r, _ := sh.pager.cur.List.Row(key)
		r.Activate()
		if sh.screen != id {
			t.Errorf("%s opened screen %d, want %d", key, sh.screen, id)
		}
	}
}

func TestTitleNotes(t *testing.T) {
	clearNewPacks()
	t.Cleanup(clearNewPacks)
	sh := testShell(t)
	sh.settings.Map = city.MapSeaside
	sh.openPage(pageTitle)
	l := sh.pager.cur.List
	r, _ := l.Row("play")
	if got := r.view(false).Value; got != "free drive, seatown" {
		t.Errorf("play note %q", got)
	}
	r, _ = l.Row("garage")
	if v := r.view(false).Value; v == "" || v == "-" {
		t.Errorf("garage note %q", v)
	}
	r, _ = l.Row("career")
	if v := r.view(false).Value; v != "" {
		t.Errorf("career note %q with nothing unlocked", v)
	}
	markNewPacks([]pack.Pack{pack.Of(pack.Coast)})
	if v := r.view(false).Value; v != "NEW" {
		t.Errorf("career note %q after an unlock, want NEW", v)
	}
	clearNewPacks()
	if v := r.view(false).Value; v != "" {
		t.Errorf("career note %q after clearing", v)
	}
}

func TestTitleCards(t *testing.T) {
	clearNewPacks()
	sh := testShell(t)
	sh.settings.Map = city.MapSeaside
	c := sh.titleCardFor("play")
	if c.Head != "SEATOWN" || c.Tag != "summer" {
		t.Errorf("play card %+v: the season is a tag, not part of the name", c)
	}
	if c := sh.titleCardFor("garage"); c.Head == "" || len(c.Lines) == 0 {
		t.Errorf("garage card %+v", c)
	}
	c = sh.titleCardFor("career")
	if len(c.Lines) != 2 || c.Lines[0] != "Bounty banked: 0" {
		t.Errorf("career card %+v", c)
	}
}

func TestTitleEscOpensQuit(t *testing.T) {
	sh := testShell(t)
	sh.openPage(pageTitle)
	sh.stepPage(navInput{Back: true})
	if sh.screen != pageQuit {
		t.Fatalf("esc on the title opened screen %d, want the quit dialog", sh.screen)
	}
}

func TestNextPackIsCheapestUnheld(t *testing.T) {
	p, ok := nextPack(pack.Access{})
	if !ok || p.Price <= 0 {
		t.Fatal("no next pack for a new player")
	}
	for _, q := range pack.All() {
		if q.Price > 0 && q.Price < p.Price {
			t.Errorf("%s is cheaper than %s", q.Name, p.Name)
		}
	}
	if _, ok := nextPack(pack.Access{Bypass: true}); ok {
		t.Error("a bypassed player has a next pack")
	}
}
