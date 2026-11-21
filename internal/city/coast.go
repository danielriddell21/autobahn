package city

import (
	"github.com/danielriddell21/crucible/worldgen"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

// A map with a coast has the sea along one side of it. The lattice still runs
// on under the water, because a junction settles against its neighbours and
// they have to exist; but none of the streets beyond the seafront are laid,
// and no block is built there. The city is as endless as ever in the other
// three directions.
//
// Where the coast falls is a grid line, so like everything else it is a
// function of where a place is: the seafront is the same road however the
// ground around it was reached.

// edgesOf returns the streets planned at a junction: the lattice's edges,
// less any that would run out to sea.
func (c *City) edgesOf(ln *worldgen.LatticeNode) []int {
	if !c.params.Coast {
		return ln.Edges
	}
	kept := make([]int, 0, len(ln.Edges))
	for _, id := range ln.Edges {
		e := c.lat.Edges[id]
		if c.lat.Nodes[e.A].Row <= c.params.CoastRow && c.lat.Nodes[e.B].Row <= c.params.CoastRow {
			kept = append(kept, id)
		}
	}
	return kept
}

// fillShore lays the ground in a cell at or beyond the seafront. The first
// row is the beach, set back from the seafront road and running flush into
// its neighbours either side; past that there is only water, and nothing to
// lay at all.
func (c *City) fillShore(row int, corners [4]*Node) {
	if row != c.params.CoastRow {
		return
	}
	front := c.streetHalfWidth(corners[0], corners[1])
	c.Blocks = append(c.Blocks, Block{
		Min:   mathx.V(corners[0].Pos.X, corners[0].Pos.Z+front),
		Max:   corners[3].Pos,
		Beach: true,
	})
}

// Shoreline returns where the coast is: front is the line the seafront road
// runs along, and sea the line the water starts at, with the beach between
// them. ok is false for a map with no coast.
func (c *City) Shoreline() (front, sea float32, ok bool) {
	if !c.params.Coast {
		return 0, 0, false
	}
	return float32(c.plan.Y(c.params.CoastRow)), float32(c.plan.Y(c.params.CoastRow + 1)), true
}

// AtSea reports whether a point is out past the beach, in the water.
func (c *City) AtSea(p mathx.Vec) bool {
	_, sea, ok := c.Shoreline()
	return ok && p.Z > sea
}
