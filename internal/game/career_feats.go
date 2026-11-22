package game

import (
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/feats"
	"github.com/danielriddell21/autobahn/internal/pack"
)

func init() {
	careerSanitisers = append(careerSanitisers, func(c *Career) {
		c.Feats.Sanitise()
		// A secret paint that was earned is never lost to a hand-edited or
		// damaged paints list. Nothing is granted that was not earned.
		c.Paints = cleanIDs(append(c.Paints, c.Feats.Paints()...),
			func(i int) bool { return i >= 0 && i < len(playerPaints) })
	})
}

// featCounters are the running totals a drive's world keeps, as plain values.
// The shell reads them off the world each frame; a featFeed turns them into
// the stretch since it last looked.
type featCounters struct {
	Distance               float32 // sim.Judge.Distance
	Faults                 int     // sim.Judge.Faults
	RedsStopped, RedsRun   int     // sim.Judge
	Escapes, Roadblocks    int     // sim.Wanted
	Level                  int     // sim.Wanted.Level now
	Map, Car, Season, Cond int
	Pack                   int
	Helped, Custom         bool
}

// featFeed remembers the totals it last saw so that each look yields only
// what is new. A new session starts a new feed.
type featFeed struct {
	last featCounters
	seen bool
}

// fact is the stretch since the last look. earned is bounty banked at the
// wheel since then (Game.TakeEarned). The first look only takes a baseline,
// so a total that was already there is not counted as driven.
func (ff *featFeed) fact(c featCounters, earned int) (feats.Fact, bool) {
	if !ff.seen {
		ff.last, ff.seen = c, true
		return feats.Fact{}, false
	}
	l := ff.last
	ff.last = c
	// A total that has fallen was cleared (the judge is when the autopilot takes
	// over or hands back), so what it counts now is all new.
	pos := func(n int) int { return max(n, 0) }
	step := func(now, was int) int {
		if now < was {
			return pos(now)
		}
		return now - was
	}
	dist := c.Distance - l.Distance
	if dist < 0 {
		dist = max(c.Distance, 0)
	}
	return feats.Fact{
		Distance:    dist,
		Faults:      step(c.Faults, l.Faults),
		RedsStopped: step(c.RedsStopped, l.RedsStopped),
		RedsRun:     step(c.RedsRun, l.RedsRun),
		Escapes:     step(c.Escapes, l.Escapes),
		Roadblocks:  step(c.Roadblocks, l.Roadblocks),
		Banked:      pos(earned),
		WantedPeak:  c.Level,
		Map:         c.Map, Car: c.Car, Season: c.Season, Weather: c.Cond,
		Pack:   c.Pack,
		Helped: c.Helped, Custom: c.Custom,
	}, true
}

// RecordFeats counts a stretch of drive and returns the achievements it
// unlocked. Any secret paint they carry is put among the paints at once. The
// caller saves the career and announces the result.
func (c *Career) RecordFeats(x feats.Fact) []feats.Feat {
	t := feats.Tracker{F: c.Feats}
	got := t.Add(x)
	c.Feats = t.F
	if len(got) > 0 {
		c.sanitise()
	}
	return got
}

// featPack is the pack a fact's map belongs to, for the glue to fill in.
func featPack(m city.Map) int { return int(pack.ForMap(m).ID) }

// featCounters reads the totals the achievements are fed off the game's world.
// Anything but the player driving, in a game that counts, marks the stretch
// helped, as it does for the bounty.
func (g *Game) featCounters() featCounters {
	w := g.world
	return featCounters{
		Distance: w.Judge.Distance, Faults: w.Judge.Faults,
		RedsStopped: w.Judge.RedsStopped, RedsRun: w.Judge.RedsRun,
		Escapes: w.Wanted.Escapes, Roadblocks: w.Wanted.Roadblocks, Level: w.Wanted.Level,
		Map: int(g.opts.Map), Car: int(g.opts.Car), Season: int(g.season()),
		Cond: int(w.Weather().Cond), Pack: featPack(g.opts.Map),
		Helped: g.helped() || g.career.assisted, Custom: g.opts.Custom != nil,
	}
}
