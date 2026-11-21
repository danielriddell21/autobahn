package city

import (
	"math"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Parking geometry, in metres.
const (
	// BayLength is one space along the kerb, and BayDepth how far a bay
	// reaches in from it. A bay is marked out on the pavement rather than in
	// the carriageway, so a parked car never stands in a lane.
	BayLength float32 = 5.6
	BayDepth  float32 = 2.4
	// BaySlots is how many spaces a bay holds.
	BaySlots = 3
	// bayInset is how far in from the kerb the middle of a parked car is.
	bayInset float32 = 1.3
	// bayMargin keeps bays clear of the junctions at either end of a road,
	// where the signs and signals stand, and bayPitch is how often along the
	// road a bay may fall.
	bayMargin float32 = 26
	bayPitch  float32 = 34
	// bayShare is how many of the places a bay could go have one, and
	// parkedShare how many of its spaces are taken.
	bayShare    float32 = 0.42
	parkedShare float32 = 0.7
)

// The salts behind where the bays fall and what is parked in them.
const (
	baySalt    = 0x6b43
	parkedSalt = 0x1f83
	shapeSalt  = 0x7a15
	paintSalt  = 0x2c9d
)

// Bay is a row of parking spaces marked on the pavement beside a road.
type Bay struct {
	// Start is the corner of the bay at the kerb, nearest the start of the
	// road. Dir runs along the kerb from there and Out away from the road.
	Start, Dir, Out mathx.Vec
	// Along is how far Start is from the start of the road, and Side which
	// side of the road the bay is on: -1 to the left of the way the road
	// runs, +1 to the right.
	Along, Side float32
}

// Slot returns the middle of the i-th space, which is where a car in it
// stands.
func (b Bay) Slot(i int) mathx.Vec {
	return b.Start.Add(b.Dir.Mul(BayLength * (float32(i) + 0.5))).Add(b.Out.Mul(bayInset))
}

// Parked is a car left in a bay. It is scenery with a footprint: nothing
// drives it, and nothing drives through it.
type Parked struct {
	Pos     mathx.Vec
	Heading float32
	// Shape and Paint are two draws for whoever builds the car to choose its
	// body and colour with, so the city need not know what cars there are.
	Shape, Paint uint8
}

// placeChance is cellChance for a point on the ground rather than a cell: a
// value in [0, 1) that depends only on the seed, the purpose and where the
// point is, to half a metre.
func placeChance(seed, salt uint64, p mathx.Vec) float32 {
	return cellChance(seed, salt, int(math.Floor(float64(p.X)*2)), int(math.Floor(float64(p.Z)*2)))
}

// Bays returns the parking bays along a road. They are worked out from where
// the road is and nothing else, so they can be asked for as soon as the road
// exists and always come out the same.
//
// The main roads have none: their pavements carry the street lamps, and a
// row of parked cars would hide the signs from a driver in the far lane.
func (c *City) Bays(r *Road) []Bay {
	if r.Ring || r.Class == Arterial {
		return nil
	}
	na, nb := c.Nodes[r.A], c.Nodes[r.B]
	from := na.Radius + bayMargin
	to := r.Length - nb.Radius - bayMargin - BayLength*BaySlots
	var out []Bay
	for d := from; d <= to; d += bayPitch {
		for _, side := range [2]float32{-1, 1} {
			start := na.Pos.Add(r.Dir.Mul(d)).Add(r.Dir.Right().Mul(side * r.HalfWidth))
			if placeChance(c.Seed, baySalt, start) >= bayShare {
				continue
			}
			out = append(out, Bay{
				Start: start, Dir: r.Dir, Out: r.Dir.Right().Mul(side),
				Along: d, Side: side,
			})
		}
	}
	return out
}

// placeParking fills the bays along a road.
func (c *City) placeParking(r *Road) {
	for _, b := range c.Bays(r) {
		// A car is left facing the way the traffic beside it runs, which on
		// the side traffic keeps to is the way the road does.
		heading := r.Dir.Angle()
		if b.Side != trafficSide() {
			heading = mathx.WrapPi(heading + math.Pi)
		}
		for i := range BaySlots {
			at := b.Slot(i)
			if placeChance(c.Seed, parkedSalt, at) >= parkedShare {
				continue
			}
			c.Parked = append(c.Parked, Parked{
				Pos: at, Heading: heading,
				Shape: uint8(placeChance(c.Seed, shapeSalt, at) * 256),
				Paint: uint8(placeChance(c.Seed, paintSalt, at) * 256),
			})
		}
	}
}
