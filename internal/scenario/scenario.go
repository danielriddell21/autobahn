// Package scenario is the challenges: a fixed seed on a map under fixed rules,
// with a goal, scored against the autopilot's own run on the same seed.
//
// A scenario holds only a seed, a map and rules. Everything about the place is
// still a function of where and when, so nothing here is saved about the world.
// The autopilot's score on each scenario, the ghost, is worked out ahead of time
// by tools/eval and checked in as ghosts.json; a test in that tool proves the
// numbers still reproduce. The ghost is a number only.
//
// Only a human can pass. A run that was helped by anything else is scored as
// forfeit: the autopilot sets the score, it does not collect the medal.
//
// The package sees the world through the plain numbers of a [Frame], so it
// imports nothing of the simulation.
package scenario

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/weather"
	"github.com/danielriddell21/autobahn/internal/works"
)

// Goal kinds.
const (
	// GoalScore runs for the whole time and scores the distance driven less a
	// cost for every penalty point.
	GoalScore = "score"
	// GoalClean runs until the time is up or the first fault, and scores the
	// distance driven. It is "no faults for as long as you can".
	GoalClean = "clean"
)

// PointCost is what a penalty point takes from a GoalScore run, in metres.
const PointCost = 25

// Rules are the conditions a scenario is driven under.
type Rules struct {
	Seconds int    `json:"seconds"`
	Traffic int    `json:"traffic"`
	Police  int    `json:"police"`
	Works   string `json:"works"`
	Weather string `json:"weather"`
}

// Scenario is one challenge.
type Scenario struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Map   string `json:"map"`
	Seed  uint64 `json:"seed"`
	Goal  string `json:"goal"`
	Rules Rules  `json:"rules"`
}

// Validate says what is wrong with a scenario, if anything.
func (s Scenario) Validate() error {
	if s.ID == "" || s.Name == "" {
		return errors.New("scenario needs an id and a name")
	}
	if _, ok := city.ParseMap(s.Map); !ok {
		return fmt.Errorf("%s: unknown map %q", s.ID, s.Map)
	}
	if s.Goal != GoalScore && s.Goal != GoalClean {
		return fmt.Errorf("%s: unknown goal %q", s.ID, s.Goal)
	}
	if s.Rules.Seconds <= 0 || s.Rules.Traffic < 0 || s.Rules.Police < 0 {
		return fmt.Errorf("%s: bad rules", s.ID)
	}
	if _, ok := works.ParseLevel(s.Rules.Works); !ok {
		return fmt.Errorf("%s: unknown works level %q", s.ID, s.Rules.Works)
	}
	if _, ok := weather.ParseSetting(s.Rules.Weather); !ok {
		return fmt.Errorf("%s: unknown weather %q", s.ID, s.Rules.Weather)
	}
	return nil
}

// CityMap is the scenario's map.
func (s Scenario) CityMap() city.Map {
	m, _ := city.ParseMap(s.Map)
	return m
}

// Brief says what is asked of the player, in a line.
func (s Scenario) Brief() string {
	if s.Goal == GoalClean {
		return fmt.Sprintf("Go as far as you can in %ds without a fault", s.Rules.Seconds)
	}
	return fmt.Sprintf("Go as far as you can in %ds; a point costs %d m", s.Rules.Seconds, PointCost)
}

// File is a set of scenarios as stored.
type File struct {
	Version   int        `json:"version"`
	Scenarios []Scenario `json:"scenarios"`
}

// Encode writes scenarios as JSON.
func Encode(list []Scenario) ([]byte, error) {
	return json.MarshalIndent(File{Version: 1, Scenarios: list}, "", "  ")
}

// Decode reads scenarios back, refusing any that do not validate or that share
// an id.
func Decode(data []byte) ([]Scenario, error) {
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, s := range f.Scenarios {
		if err := s.Validate(); err != nil {
			return nil, err
		}
		if seen[s.ID] {
			return nil, fmt.Errorf("duplicate scenario %q", s.ID)
		}
		seen[s.ID] = true
	}
	return f.Scenarios, nil
}

// All is the first set of challenges, two on every map.
func All() []Scenario {
	def := func(secs int) Rules {
		return Rules{Seconds: secs, Traffic: 70, Police: 7, Works: "normal", Weather: "clear"}
	}
	rain := def(60)
	rain.Weather = "rain"
	dense := def(60)
	dense.Works = "dense"
	return []Scenario{
		{ID: "city-sprint", Name: "City sprint", Map: "city", Seed: 101, Goal: GoalScore, Rules: def(60)},
		{ID: "city-clean", Name: "City, no faults", Map: "city", Seed: 102, Goal: GoalClean, Rules: def(60)},
		{ID: "seatown-sprint", Name: "Seatown sprint", Map: "seatown", Seed: 201, Goal: GoalScore, Rules: def(60)},
		{ID: "seatown-clean", Name: "Seatown, no faults", Map: "seatown", Seed: 202, Goal: GoalClean, Rules: def(60)},
		{ID: "village-sprint", Name: "Village sprint", Map: "village", Seed: 301, Goal: GoalScore, Rules: def(60)},
		{ID: "village-rain", Name: "Village in the rain", Map: "village", Seed: 302, Goal: GoalClean, Rules: rain},
		{ID: "roadtown-sprint", Name: "Roadtown sprint", Map: "roadtown", Seed: 401, Goal: GoalScore, Rules: def(60)},
		{ID: "roadtown-works", Name: "Roadtown roadworks", Map: "roadtown", Seed: 402, Goal: GoalClean, Rules: dense},
	}
}

// Lookup finds a scenario in All by id.
func Lookup(id string) (Scenario, bool) {
	for _, s := range All() {
		if s.ID == id {
			return s, true
		}
	}
	return Scenario{}, false
}
