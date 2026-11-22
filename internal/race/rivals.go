package race

import (
	"fmt"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/job"
)

// rivalSalt keeps the rivals' styles off every other draw.
const rivalSalt uint64 = 0x5b8e2d9fc1a47306

// Rival is an AI racer: the traffic driver, with a racing style above the
// posted limit, sent through the same checkpoints as the player. It has no line
// of its own. Its style is how long it takes to finish as a share of the par
// time, so a smaller one is a faster driver.
type Rival struct {
	Name  string
	Style float32
}

var rivalNames = []string{"Ash", "Bryn", "Cole", "Dee", "Eli", "Fay"}

// Field draws the rivals for a race. It is a function of the race and nothing
// else, so the field is the same wherever and whenever it is asked for.
func Field(seed uint64, serial, n int) []Rival {
	out := make([]Rival, n)
	for i := range out {
		r := city.Chance(seed^uint64(serial)*0x9e3779b97f4a7c15, rivalSalt, serial, i)
		out[i] = Rival{Name: rivalNames[i%len(rivalNames)], Style: 0.97 + 0.2*r}
	}
	return out
}

// Fractions is how far through the race each goal is, from above zero to one,
// by the distance between goals. The first leg, from the start to the first
// goal, is taken as an average one.
func Fractions(goals []job.Goal) []float32 {
	if len(goals) == 0 {
		return nil
	}
	cum := make([]float32, len(goals))
	for i := 1; i < len(goals); i++ {
		cum[i] = cum[i-1] + goals[i].Pos.DistTo(goals[i-1].Pos)
	}
	first := cum[len(cum)-1] / float32(max(len(goals)-1, 1))
	if first == 0 {
		first = 1
	}
	out := make([]float32, len(goals))
	total := cum[len(cum)-1] + first
	for i := range out {
		out[i] = (cum[i] + first) / total
	}
	return out
}

// Times is when the rival reaches each goal, in seconds from the start, given
// the par time of the whole race.
func (r Rival) Times(goals []job.Goal, par float32) []float32 {
	fr := Fractions(goals)
	for i := range fr {
		fr[i] *= r.Style * par
	}
	return fr
}

// Finish is when the rival crosses the last gate.
func (r Rival) Finish(par float32) float32 { return r.Style * par }

// Place is the position a finishing time takes among the rivals', counting from
// one. A rival that finishes at the same moment is behind.
func Place(elapsed, par float32, field []Rival) int {
	p := 1
	for _, r := range field {
		if r.Finish(par) < elapsed {
			p++
		}
	}
	return p
}

// Ordinal writes a place as the player reads it.
func Ordinal(n int) string {
	switch n {
	case 1:
		return "1st"
	case 2:
		return "2nd"
	case 3:
		return "3rd"
	}
	return fmt.Sprintf("%dth", n)
}
