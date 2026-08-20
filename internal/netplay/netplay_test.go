package netplay_test

import (
	"testing"
	"time"

	"github.com/danielriddell21/autobahn/internal/netplay"
)

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestControlsReachTheHostAndStateComesBack(t *testing.T) {
	h, err := netplay.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	if h.Joined() {
		t.Error("a host with nobody connected reports a player")
	}

	c, err := netplay.Join(h.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	waitFor(t, "the host to notice the player", h.Joined)

	// Controls travel from the joining player to the host.
	c.Send(netplay.Input{Throttle: 0.75, Steer: -0.5, Handbrake: true})
	waitFor(t, "controls to arrive", func() bool {
		in, ok := h.Input()
		return ok && in.Throttle == 0.75
	})
	in, _ := h.Input()
	if in.Steer != -0.5 || !in.Handbrake {
		t.Errorf("controls arrived as %+v", in)
	}

	// The world travels the other way.
	h.Send(netplay.Snapshot{
		Clock:  12.5,
		Runner: netplay.Pose{X: 3, Z: 4, Yaw: 1.5, Speed: 20},
		Agents: []netplay.Pose{{X: 1, Flags: netplay.FlagPursuing}},
		Chaser: 0, WantedLevel: 2, Elapsed: 9,
	})
	waitFor(t, "a snapshot to arrive", func() bool {
		_, ok := c.Snapshot()
		return ok
	})

	s, _ := c.Snapshot()
	if s.Clock != 12.5 || s.Runner.X != 3 || s.Runner.Speed != 20 {
		t.Errorf("runner arrived as %+v at clock %v", s.Runner, s.Clock)
	}
	if len(s.Agents) != 1 || s.Agents[0].Flags&netplay.FlagPursuing == 0 {
		t.Errorf("agents arrived as %+v", s.Agents)
	}
	if s.WantedLevel != 2 || s.Chaser != 0 {
		t.Errorf("wanted %d, chaser %d", s.WantedLevel, s.Chaser)
	}
}

// A host with nobody connected must keep running: the unit the absent player
// would drive simply goes back to the simulation.
func TestHostRunsWithNoPlayer(t *testing.T) {
	h, err := netplay.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	h.Send(netplay.Snapshot{Clock: 1}) // must not panic or block
	if _, ok := h.Input(); ok {
		t.Error("input reported available with nobody connected")
	}
}

func TestHostNoticesAPlayerLeaving(t *testing.T) {
	h, err := netplay.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	c, err := netplay.Join(h.Addr())
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the player to connect", h.Joined)

	c.Close()
	waitFor(t, "the host to notice the player has gone", func() bool { return !h.Joined() })
}

func TestJoiningNothingFails(t *testing.T) {
	if _, err := netplay.Join("127.0.0.1:1"); err == nil {
		t.Error("joining a closed port succeeded")
	}
}
