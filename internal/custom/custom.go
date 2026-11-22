// Package custom is the custom map: one of the four presets with some of its
// numbers moved.
//
// A [Spec] is a base map and a seed, a position for each slider, and a few
// switches. [Spec.Params] turns it into city.Params by starting from
// city.ParamsFor(Base) and writing each slider over the field it names, then
// handing the result to city.Clamp; [Spec.Apply] turns it into a sim.Config.
// Nothing here is a generator of its own, and nothing in internal/city knows
// it exists beyond Clamp: a custom place is as much a function of where it is
// as any other, because it is only ever a different set of Params.
//
// The slider table is what the share code stores, so it is frozen: sliders are
// only ever appended, and never reordered, renamed in meaning or re-scaled.
package custom

import (
	"fmt"
	"math"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/weather"
)

// The sliders, in the order a code stores them.
const (
	MinSpan = iota
	MaxSpan
	Roundabouts
	Parks
	Buildings // how tall: Rise, Floor and Storeys together
	MainEvery
	Limits // 0 the usual urban limits, 1 the fast ones
	Wind
	Hills
	HillSpan
	Season  // 0 the base map's own, then autumn, summer, spring, winter
	Traffic // 0 the base map's own, then the mix of each preset in turn
	Grip    // how much of its grip the road gives
	NumSliders
)

// Slider is one knob: its steps run from Lo in increments of Inc, from step 0
// to step Max. The values at the two ends are the lowest and highest any
// preset uses, which is what city.Clamp's ranges are.
type Slider struct {
	Name string
	Lo   float64
	Inc  float64
	Max  int
	// Unit is how a menu writes the value after the number.
	Unit string
}

// Sliders is the table.
var Sliders = [NumSliders]Slider{
	MinSpan:     {"Shortest block", 70, 10, 10, "m"},
	MaxSpan:     {"Longest block", 120, 10, 17, "m"},
	Roundabouts: {"Roundabouts", 0.10, 0.02, 9, ""},
	Parks:       {"Open ground", 0.10, 0.02, 16, ""},
	Buildings:   {"Building height", 0, 1, 10, ""},
	MainEvery:   {"Main road every", 2, 1, 3, " lines"},
	Limits:      {"Speed limits", 0, 1, 1, ""},
	Wind:        {"Winding", 0, 4, 10, "m"},
	Hills:       {"Hills", 0, 1, 11, "m"},
	HillSpan:    {"Hill length", 700, 100, 19, "m"},
	Season:      {"Season", 0, 1, 4, ""},
	Traffic:     {"Traffic mix", 0, 1, 4, ""},
	Grip:        {"Road grip", 0.70, 0.05, 6, ""},
}

// Value returns what a step of a slider stands for, rounded so that the same
// step is the same number everywhere.
func (sl Slider) Value(step int) float64 {
	return math.Round((sl.Lo+float64(step)*sl.Inc)*1e4) / 1e4
}

// Nearest returns the step whose value is closest to v, within the slider.
func (sl Slider) Nearest(v float64) int {
	return min(max(int(math.Round((v-sl.Lo)/sl.Inc)), 0), sl.Max)
}

// MPH is the fast set of limits, as the roadtown has them.
var fastLimits = [3]float32{mathx.MPH(30), mathx.MPH(40), mathx.MPH(50)}

// Spec is a custom map.
type Spec struct {
	// Base is the preset the map starts from. Params.Map stays Base, so the
	// place hashes as the preset does.
	Base city.Map
	Seed uint64
	// Step is the position of every slider. A step outside 0..Max is only
	// meaningful with Unsupported, and is what makes a map unsupported.
	Step [NumSliders]int
	// Coast puts the sea along one side, as the seatown has it.
	Coast bool
	// Unsupported lets sliders go past the ranges the gate has measured. It
	// is what a code is flagged with; see [Spec.Flagged].
	Unsupported bool
	// Weather is the sky the map is driven under.
	Weather weather.Setting
}

// seasonSteps and mixSteps are the choices for the two sliders that are not
// numbers.
func seasonOf(step int, base city.Map) city.Season {
	if step >= 1 && step <= len(city.Seasons()) {
		return city.Seasons()[step-1]
	}
	return base.Season()
}

func mixOf(step int, base city.Map) sim.Mix {
	if step >= 1 && step <= len(city.Maps()) {
		return sim.MixFor(city.Maps()[step-1])
	}
	return sim.MixFor(base)
}

// FromMap is the custom map that is a preset exactly where the sliders can say
// so: every slider at the step nearest the preset's own.
func FromMap(m city.Map, seed uint64) Spec {
	if !m.Valid() {
		m = city.MapCity
	}
	p := city.ParamsFor(m, seed)
	s := Spec{Base: m, Seed: seed, Coast: p.Coast, Weather: weather.Drift}
	set := func(i int, v float64) { s.Step[i] = Sliders[i].Nearest(v) }
	set(MinSpan, float64(p.MinSpan))
	set(MaxSpan, float64(p.MaxSpan))
	set(Roundabouts, float64(p.Roundabouts))
	set(Parks, float64(p.Parks))
	set(Buildings, float64(p.Rise)*float64(Sliders[Buildings].Max))
	set(MainEvery, float64(p.MainEvery))
	set(Wind, float64(p.Wind))
	set(Hills, float64(p.Hills))
	set(HillSpan, float64(p.HillSpan)) // below the first step for a level place; unused then
	if p.Limits != [3]float32{} {
		s.Step[Limits] = 1
	}
	s.Step[Grip] = Sliders[Grip].Max
	return s
}

// Normalize brings the steps into the slider's own range, which is all of
// them but for an unsupported map's numeric ones, and the choices always.
func (s Spec) Normalize() Spec {
	if !s.Base.Valid() {
		s.Base = city.MapCity
	}
	if s.Weather > weather.Drift {
		s.Weather = weather.Drift
	}
	for i := range s.Step {
		switch {
		case i == Season || i == Traffic || i == Limits || !s.Unsupported:
			s.Step[i] = min(max(s.Step[i], 0), Sliders[i].Max)
		default:
			s.Step[i] = min(max(s.Step[i], -MaxPad), 255-padOffset)
		}
	}
	return s
}

// value is a slider's number at the spec's step.
func (s Spec) value(i int) float64 { return Sliders[i].Value(s.Step[i]) }

// Params is the city this map asks for, and whether it lies outside what the
// gate has measured.
func (s Spec) Params() (city.Params, bool) {
	s = s.Normalize()
	p := city.ParamsFor(s.Base, s.Seed)
	p.MinSpan, p.MaxSpan = float32(s.value(MinSpan)), float32(s.value(MaxSpan))
	p.Roundabouts, p.Parks = float32(s.value(Roundabouts)), float32(s.value(Parks))

	// One slider for how tall the buildings are: the village's low roofs at
	// one end and the city's towers at the other.
	t := float64(s.Step[Buildings]) / float64(Sliders[Buildings].Max)
	p.Rise = float32(t)
	p.Floor = float32(5.5 + 1.5*t)
	p.Storeys = float32(4.5 + 5.5*t)

	p.MainEvery = int(s.value(MainEvery))
	p.Limits = [3]float32{}
	if s.Step[Limits] > 0 {
		p.Limits = fastLimits
	}
	p.Wind = float32(s.value(Wind))
	p.Hills = float32(s.value(Hills))
	p.HillSpan = float32(s.value(HillSpan))
	if p.Hills <= 0 {
		p.Hills, p.HillSpan = 0, 0
	}

	// The coast is a preset's too: its row and its bluff are the seatown's.
	sea := city.ParamsFor(city.MapSeaside, s.Seed)
	p.Coast = s.Coast
	if s.Coast {
		p.CoastRow, p.Bluff = sea.CoastRow, sea.Bluff
	} else {
		p.CoastRow, p.Bluff = 0, 0
	}

	p, outside := city.Clamp(p, s.Unsupported)
	return p, outside
}

// gripScale is the grip slider's number, brought to what is supported: the
// road is never given less grip than winter's. The second result reports
// whether it had to be, or is past that and allowed.
func (s Spec) gripScale() (float32, bool) {
	scale := float32(s.value(Grip))
	season := sim.SeasonGrip(seasonOf(s.Step[Season], s.Base))
	floor := float32(math.Ceil(float64(sim.SeasonGrip(city.Winter)/season)*100-1e-6)) / 100
	scale = min(scale, 1)
	if scale >= floor-1e-6 {
		return scale, false
	}
	if s.Unsupported {
		return max(scale, 0.2), true
	}
	return floor, false
}

// Flagged reports whether the map is unsupported: allowed to leave the ranges
// and has. A flagged map is the one the HUD tags "unsupported" and takes no
// bounty or earnings from.
func (s Spec) Flagged() bool {
	s = s.Normalize()
	_, out := s.Params()
	_, gripOut := s.gripScale()
	return s.Unsupported && (out || gripOut)
}

// Apply fits a world configuration to the map: the base, the seed, the
// parameters, the traffic, the season, the grip and the weather. Everything
// else in cfg, the amount of traffic, the police, the car, is left alone.
func (s Spec) Apply(cfg sim.Config) sim.Config {
	s = s.Normalize()
	p, _ := s.Params()
	cfg.Map, cfg.Seed = s.Base, s.Seed
	cfg.Params = &p
	mix := mixOf(s.Step[Traffic], s.Base)
	cfg.Mix = &mix
	season := seasonOf(s.Step[Season], s.Base)
	cfg.Season = &season
	cfg.GripScale, _ = s.gripScale()
	cfg.Weather = s.Weather
	return cfg
}

// Season is the time of year the map is driven in.
func (s Spec) Season() city.Season {
	s = s.Normalize()
	return seasonOf(s.Step[Season], s.Base)
}

// Edge is the hardest map the ranges allow, every slider at its hardest bound
// at once: the most roundabouts, the most winding, the steepest hills the
// slope rule lets through, the longest blocks, the tallest buildings, the fast
// roads and the freight. It is what the gate drives as custom:edge, so that no
// range is wider than the autopilot has been shown to cope with. If it fails,
// narrow the ranges.
func Edge(base city.Map, seed uint64) Spec {
	s := FromMap(base, seed)
	s.Coast = false
	s.Weather = weather.Held(weather.Clear)
	s.Step[MinSpan] = Sliders[MinSpan].Max
	s.Step[MaxSpan] = Sliders[MaxSpan].Max
	s.Step[Roundabouts] = Sliders[Roundabouts].Max
	s.Step[Parks] = Sliders[Parks].Max
	s.Step[Buildings] = Sliders[Buildings].Max
	s.Step[MainEvery] = Sliders[MainEvery].Max
	s.Step[Limits] = 1
	s.Step[Wind] = Sliders[Wind].Max
	s.Step[Hills] = Sliders[Hills].Max
	// The steepest the combined rule allows for the highest hills.
	s.Step[HillSpan] = Sliders[HillSpan].Nearest(math.Ceil(float64(Sliders[Hills].Max)*850/8/100) * 100)
	s.Step[Season] = int(city.Winter) + 1
	s.Step[Traffic] = int(city.MapMotorway) + 1
	s.Step[Grip] = Sliders[Grip].Max
	return s
}

// Banks reports whether a game banks bounty, earnings and achievements. A
// custom map never does, whatever it is set to: with any slider free the
// payout would be whatever the player chose to make it. Pass the game's custom
// map, or nil.
func Banks(s *Spec) bool { return s == nil }

// Locked reports why a player who has access a may not drive the map, or nil.
// A custom map offers the base map, and the traffic of any other place it
// borrows, only to a player who has them unlocked, as the menus do. A preset's
// own traffic goes with its map. The player's car is checked where it is chosen.
func (s Spec) Locked(a pack.Access) error {
	s = s.Normalize()
	if !a.HasMap(s.Base) {
		return fmt.Errorf("%s is locked", pack.ForMap(s.Base).Name)
	}
	// A mix borrowed from another place carries that place's cars.
	if k := s.Step[Traffic]; k >= 1 && k <= len(city.Maps()) {
		if m := city.Maps()[k-1]; !a.HasMap(m) {
			return fmt.Errorf("the traffic of %s is locked with %s", m.Name(), pack.ForMap(m).Name)
		}
	}
	return nil
}
