package scenario

import (
	_ "embed"
	"encoding/json"
	"math"
)

// Medal is what a run earned against the ghost.
type Medal int

// The medals, worst to best.
const (
	NoMedal Medal = iota
	Bronze
	Silver
	Gold
)

// String names the medal.
func (m Medal) String() string {
	switch m {
	case Bronze:
		return "bronze"
	case Silver:
		return "silver"
	case Gold:
		return "gold"
	}
	return "none"
}

// What share of the ghost's score each medal wants. Gold is beating the
// autopilot, which is the point.
const (
	BronzeShare = 0.5
	SilverShare = 0.8
	GoldShare   = 1.0
	// GhostFloor stops a ghost that did badly making every medal free.
	GhostFloor = 100
)

// Thresholds are the scores each medal wants against a ghost.
func Thresholds(ghost int) (bronze, silver, gold int) {
	g := float64(max(ghost, GhostFloor))
	at := func(share float64) int { return int(math.Ceil(g * share)) }
	return at(BronzeShare), at(SilverShare), at(GoldShare)
}

// MedalFor is the medal a score earns against a ghost.
func MedalFor(score, ghost int) Medal {
	b, s, g := Thresholds(ghost)
	switch {
	case score >= g:
		return Gold
	case score >= s:
		return Silver
	case score >= b:
		return Bronze
	}
	return NoMedal
}

// Frame is what the host tells a run each step. Distance, Points and Faults
// are the judge's running totals, so a run does not care when it began.
type Frame struct {
	Dt       float32
	Distance float32
	Points   int
	Faults   int
	// Helped is set when anything but the player is driving.
	Helped bool
}

// Run is one attempt at a scenario. A retry is a new Run.
type Run struct {
	sc       Scenario
	started  bool
	base     Frame
	elapsed  float32
	distance float32
	points   int
	faults   int
	helped   bool
	done     bool
}

// NewRun starts an attempt.
func NewRun(sc Scenario) *Run { return &Run{sc: sc} }

// Scenario is what is being attempted.
func (r *Run) Scenario() Scenario { return r.sc }

// Retry throws the attempt away and starts again at once.
func (r *Run) Retry() { *r = Run{sc: r.sc} }

// Step advances the run by a frame. The first frame sets the baseline the
// judge's totals are counted from.
func (r *Run) Step(f Frame) {
	if r.done {
		return
	}
	if !r.started {
		r.started, r.base = true, f
	}
	if f.Helped {
		r.helped = true
	}
	r.elapsed += f.Dt
	r.distance = max(f.Distance-r.base.Distance, 0)
	r.points = max(f.Points-r.base.Points, 0)
	r.faults = max(f.Faults-r.base.Faults, 0)
	switch {
	case r.elapsed >= float32(r.sc.Rules.Seconds):
		r.done = true
	case r.sc.Goal == GoalClean && r.faults > 0:
		r.done = true
	}
}

// Done reports whether the run has ended.
func (r *Run) Done() bool { return r.done }

// Helped reports whether anything but the player has had a hand in the run.
func (r *Run) Helped() bool { return r.helped }

// Elapsed is the time the run has taken, in seconds.
func (r *Run) Elapsed() float32 { return r.elapsed }

// Remaining is the time left, in seconds.
func (r *Run) Remaining() float32 { return max(float32(r.sc.Rules.Seconds)-r.elapsed, 0) }

// Score is the run's score so far, in metres.
func (r *Run) Score() int {
	d := int(math.Round(float64(r.distance)))
	if r.sc.Goal == GoalScore {
		d -= PointCost * r.points
	}
	return max(d, 0)
}

// Distance is how far the run has gone, in metres, rounded.
func (r *Run) Distance() int { return int(math.Round(float64(r.distance))) }

// Points and Faults are the penalties taken in the run.
func (r *Run) Points() int { return r.points }

// Faults is the number of rules broken in the run.
func (r *Run) Faults() int { return r.faults }

// Outcome is how a finished run stands against the ghost.
type Outcome struct {
	Score  int
	Ghost  int
	Medal  Medal
	Passed bool
	Reason string
}

// Outcome judges the run against a ghost's score. Nothing passes while the run
// is still going, or if the autopilot had a hand in it.
func (r *Run) Outcome(ghost int) Outcome {
	o := Outcome{Score: r.Score(), Ghost: ghost}
	switch {
	case r.helped:
		o.Reason = "the autopilot helped: forfeit"
	case !r.done:
		o.Reason = "still going"
	default:
		o.Medal = MedalFor(o.Score, ghost)
		o.Passed = o.Medal != NoMedal
		if !o.Passed {
			o.Reason = "short of the bronze mark"
		}
	}
	return o
}

// Ghost is the autopilot's run on a scenario.
type Ghost struct {
	Score    int `json:"score"`
	Distance int `json:"distance"`
	Points   int `json:"points"`
	Faults   int `json:"faults"`
}

// GhostFile is the checked-in table of ghosts.
type GhostFile struct {
	Version int              `json:"version"`
	Ghosts  map[string]Ghost `json:"ghosts"`
}

//go:embed ghosts.json
var ghostData []byte

// Ghosts returns the checked-in ghost for every scenario that has one.
func Ghosts() map[string]Ghost {
	g, _ := DecodeGhosts(ghostData)
	return g
}

// DecodeGhosts reads a ghost table.
func DecodeGhosts(data []byte) (map[string]Ghost, error) {
	var f GhostFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Ghosts == nil {
		f.Ghosts = map[string]Ghost{}
	}
	return f.Ghosts, nil
}

// EncodeGhosts writes a ghost table, in a stable order.
func EncodeGhosts(g map[string]Ghost) ([]byte, error) {
	out, err := json.MarshalIndent(GhostFile{Version: 1, Ghosts: g}, "", "  ")
	return append(out, '\n'), err
}
