package game

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
)

// TestRoundaboutApronCoversTheCarriageway pins the geometry the renderer draws
// a roundabout with.
//
// The bug it guards against was cars appearing to mount the kerb: blocks were
// inset from their grid lines by the width of the roads on them, which is
// right for a crossroads and wrong for a roundabout, so the pavement ran
// metres inside the circle traffic drives around.
//
// The block layout knows about junction radii now, so the pavement clears the
// carriageway on its own and the apron is what makes the junction read as a
// circle of asphalt rather than a square gap between four blocks. Both are
// checked: the clearance because it is the bug, the apron because it is what
// the junction looks like.
func TestRoundaboutApronCoversTheCarriageway(t *testing.T) {
	c := city.Generate(city.DefaultParams(7))
	if len(c.Roundabouts()) == 0 {
		t.Fatal("this seed generated no roundabouts to check")
	}

	for _, n := range c.Roundabouts() {
		ring := n.RingRadius()
		apron := n.Radius
		island := ring - city.LaneWidth

		// The apron has to reach past the circulating lane, or cars on the
		// outside of it are still over the pavement.
		if apron <= ring+city.LaneWidth/2 {
			t.Errorf("node %d: apron %.1f does not clear the lane at %.1f", n.ID, apron, ring)
		}
		// The island has to stop short of it, or it is in the traffic's way.
		if island >= ring-city.LaneWidth/2 {
			t.Errorf("node %d: island %.1f reaches the lane at %.1f", n.ID, island, ring)
		}
		if island < 2 {
			t.Errorf("node %d: island %.1f is too small to draw", n.ID, island)
		}

		// No pavement may reach the circulating lane. This is the original
		// bug, and it is now the block layout's job rather than the
		// renderer's.
		nearest := float32(1e9)
		for _, b := range c.Blocks {
			dx := max(b.Min.X-n.Pos.X, n.Pos.X-b.Max.X, 0)
			dz := max(b.Min.Z-n.Pos.Z, n.Pos.Z-b.Max.Z, 0)
			if d := max(dx, dz); d < nearest {
				nearest = d
			}
		}
		if nearest < ring+city.LaneWidth/2 {
			t.Errorf("node %d: pavement reaches %.1f from the middle, inside the lane at %.1f",
				n.ID, nearest, ring)
		}
	}
}
