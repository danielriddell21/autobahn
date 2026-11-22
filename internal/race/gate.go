// Package race is the racing pack's rules: checkpoint gates on the existing
// streets, the loop a circuit runs round, the clock and medals of a time trial,
// the field of rivals in a race, and the score of a drift. It is pure: it reads
// the city and plain job data, and drives nothing itself.
//
// A race is run on the streets as they are, so it works on every map and under
// the growth rule: a loop is chosen on the plan from junctions that are all
// complete, and its gates sit at junction exits, which never move. Traffic
// stays on the road for every kind of race; the medal times allow for it.
package race

import "github.com/danielriddell21/autobahn/internal/mathx"

// Gate is a line across a street that a car has to cross, in the simulation
// only: nothing about it is seen by the vision system.
type Gate struct {
	Pos mathx.Vec
	// Heading is the way the street runs through the gate. The line is square
	// to it.
	Heading float32
	// Half is half the length of the line, in metres.
	Half float32
}

// Ends returns the two ends of the line.
func (g Gate) Ends() (a, b mathx.Vec) {
	side := mathx.FromAngle(g.Heading).Right().Mul(g.Half)
	return g.Pos.Sub(side), g.Pos.Add(side)
}

// Crossed reports whether a car that moved from prev to cur went over the
// line, in either direction. A frame's move is a short segment and the next
// frame's begins where it ended, so one crossing is seen by one frame only: a
// move that ends exactly on the line counts, one that begins on it does not.
func (g Gate) Crossed(prev, cur mathx.Vec) bool {
	along := mathx.FromAngle(g.Heading) // square to the line
	d1, d2 := prev.Sub(g.Pos).Dot(along), cur.Sub(g.Pos).Dot(along)
	if !(d1 < 0 && d2 >= 0 || d1 > 0 && d2 <= 0) {
		return false
	}
	at := prev.Add(cur.Sub(prev).Mul(d1 / (d1 - d2)))
	return mathx.Abs(at.Sub(g.Pos).Dot(along.Right())) <= g.Half
}

// Reached is the test a job makes of a checkpoint it holds only as a place and
// a radius: whether the move from prev to cur came within radius of it.
func Reached(prev, cur, pos mathx.Vec, radius float32) bool {
	d := cur.Sub(prev)
	l := d.LenSq()
	t := float32(0)
	if l > 0 {
		t = mathx.Clamp(pos.Sub(prev).Dot(d)/l, 0, 1)
	}
	return prev.Add(d.Mul(t)).DistTo(pos) <= radius
}
