package main

import (
	"math"
	"os"
	"runtime"
	"testing"

	"github.com/danielriddell21/autobahn/internal/scenario"
)

// TestGhostsReproduce proves the checked-in ghosts are what the autopilot
// scores now. Set UPDATE_GHOSTS=1 to rewrite the table after a change that is
// meant to move them.
func TestGhostsReproduce(t *testing.T) {
	if testing.Short() {
		t.Skip("drives every scenario")
	}
	list := scenario.All()
	got := ghostsFor(list, runtime.NumCPU())
	if os.Getenv("UPDATE_GHOSTS") != "" {
		if err := writeGhosts("../../internal/scenario/ghosts.json", got); err != nil {
			t.Fatal(err)
		}
		return
	}
	want := scenario.Ghosts()
	if len(want) != len(list) {
		t.Fatalf("%d ghosts checked in for %d scenarios; run with UPDATE_GHOSTS=1", len(want), len(list))
	}
	for _, sc := range list {
		w, ok := want[sc.ID]
		if !ok {
			t.Errorf("%s: no ghost checked in", sc.ID)
			continue
		}
		g := got[sc.ID]
		// Distance is float arithmetic, so allow a metre across platforms;
		// what the autopilot did wrong is whole numbers and must match.
		if g.Points != w.Points || g.Faults != w.Faults ||
			math.Abs(float64(g.Distance-w.Distance)) > 1 || math.Abs(float64(g.Score-w.Score)) > 1 {
			t.Errorf("%s: ghost is %+v, checked in as %+v", sc.ID, g, w)
		}
	}
}

func TestGhostsAreWorthBeating(t *testing.T) {
	for id, g := range scenario.Ghosts() {
		if g.Distance <= 0 {
			t.Errorf("%s: the ghost went nowhere", id)
		}
	}
}

func TestGhostIsDeterministic(t *testing.T) {
	if testing.Short() {
		t.Skip("drives a scenario twice")
	}
	sc := scenario.All()[0]
	if a, b := ghostOf(sc), ghostOf(sc); a != b {
		t.Fatalf("two runs differ: %+v %+v", a, b)
	}
}
