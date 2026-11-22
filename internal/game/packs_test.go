package game

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/emergency"
	"github.com/danielriddell21/autobahn/internal/job"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/taxi"
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

// Streaks are process-wide, so every game starts them afresh.
func TestEachGameStartsWithNoStreaks(t *testing.T) {
	saved, savedJobs := registered, jobsRegistered
	defer func() { registered, jobsRegistered = saved, savedJobs }()
	registered, jobsRegistered = nil, true

	taxi.Fares.Reset()
	for range 3 {
		taxi.Fare{}.Settle(job.Result{Status: job.Passed, Reason: taxi.Faultless})
	}
	(&taxi.Courier{}).Settle(job.Result{Status: job.Passed, Reason: taxi.Faultless})
	emergency.Ambulance.Settle(job.Result{Status: job.Passed})
	if taxi.Fares.N() != 3 || taxi.Couriers.N() != 1 || emergency.Run.N() != 1 {
		t.Fatalf("setup: streaks %d, %d, %d", taxi.Fares.N(), taxi.Couriers.N(), emergency.Run.N())
	}
	applyPacks(nil)
	if taxi.Fares.N() != 0 || taxi.Couriers.N() != 0 || emergency.Run.N() != 0 {
		t.Errorf("streaks %d, %d, %d carried into a new game", taxi.Fares.N(), taxi.Couriers.N(), emergency.Run.N())
	}
}
