package game

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/pack"
)

// Every pack that adds content but the base game registers exactly once.
func TestEveryLaterPackRegistersOnce(t *testing.T) {
	seen := map[pack.ID]int{}
	for _, c := range registered {
		seen[c.ID]++
	}
	for _, id := range []pack.ID{pack.Taxi, pack.Emergency, pack.Racing, pack.Night} {
		if seen[id] != 1 {
			t.Errorf("pack %s registered %d times, want 1", pack.Of(id).Name, seen[id])
		}
	}
}

func TestApplyPacksCallsEachHook(t *testing.T) {
	saved, savedJobs := registered, jobsRegistered
	defer func() { registered, jobsRegistered = saved, savedJobs }()
	registered, jobsRegistered = nil, false
	jobs, dress, audio := 0, 0, 0
	Register(packContent{Jobs: func() { jobs++ }, Dressing: func(*Game) { dress++ }, Audio: func(*Game) { audio++ }})
	applyPacks(nil)
	applyPacks(nil)
	if jobs != 1 || dress != 2 || audio != 2 {
		t.Errorf("jobs %d, dressing %d, audio %d; want 1, 2, 2", jobs, dress, audio)
	}
}
