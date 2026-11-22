// Package weather says what the sky is doing at a moment in a place.
//
// Like everything else in the world it is a function of where it is, and here
// that means when: [At] works the weather out from a seed and the clock and
// stores nothing, so the player, the traffic, the camera and a second player
// on the other end of a connection, each asking separately, are told the same
// thing. It has no dependence on any other part of the game beyond the
// season, and the autopilot never imports it: it learns the weather only from
// what the camera can see.
package weather

import (
	"fmt"
	"strings"

	"github.com/danielriddell21/autobahn/internal/city"
)

// Cond is a kind of weather.
type Cond uint8

// The kinds of weather. Snow is not one: it belongs to winter, and is the
// season's.
const (
	Clear Cond = iota
	Rain
	Fog
	Storm
	numConds
)

// Conds returns every kind of weather.
func Conds() []Cond {
	out := make([]Cond, 0, numConds)
	for c := range numConds {
		out = append(out, c)
	}
	return out
}

// Name returns the weather's name.
func (c Cond) Name() string {
	switch c {
	case Rain:
		return "rain"
	case Fog:
		return "fog"
	case Storm:
		return "storm"
	default:
		return "clear"
	}
}

// Setting is how the weather is chosen: left to drift through the spells a
// seed gives it, or held at one kind. The zero value is held clear, so a world
// built without asking for weather, a test, the eval gate, is dry, and only a
// caller that wants the sky to change says [Drift].
type Setting uint8

// Drift lets the weather change on its own.
const Drift Setting = Setting(numConds)

// Held returns the setting that keeps one kind of weather for good.
func Held(c Cond) Setting { return Setting(c) }

// Cond returns the kind of weather a held setting keeps, and false for one that
// drifts.
func (s Setting) Cond() (Cond, bool) {
	if s >= Drift {
		return Clear, false
	}
	return Cond(s), true
}

// String returns the setting's name.
func (s Setting) String() string {
	if c, held := s.Cond(); held {
		return c.Name()
	}
	return "drift"
}

// MarshalText writes the setting by name.
func (s Setting) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// UnmarshalText reads a setting by name, as [ParseSetting] does.
func (s *Setting) UnmarshalText(b []byte) error {
	v, ok := ParseSetting(string(b))
	if !ok {
		return fmt.Errorf("weather: no setting called %q", b)
	}
	*s = v
	return nil
}

// ParseSetting reads a setting's name, forgiving case and spaces. An empty name
// is Drift.
func ParseSetting(name string) (Setting, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || name == "drift" {
		return Drift, true
	}
	for _, c := range Conds() {
		if c.Name() == name {
			return Held(c), true
		}
	}
	return Drift, false
}

// Unlimited is the sight in clear weather, in metres. It is far beyond
// anything the camera ranges.
const Unlimited float32 = 1e6

// State is the weather at a moment. Every number runs smoothly from one spell
// to the next, so nothing jumps.
type State struct {
	// Cond is the kind of weather that is strongest now.
	Cond Cond
	// Wet is how wet the road is, from 0 to 1.
	Wet float32
	// Sight is how far a camera can pick out a thing, in metres.
	Sight float32
	// Wind is how hard it blows, from 0 to 1.
	Wind float32
	// Rain, Fog and Storm are how much of each there is, from 0 to 1. Rain is
	// whatever is falling, so a storm has it too.
	Rain, Fog, Storm float32
	// Grip is the share of a tyre's dry grip the road gives, from 0 to 1.
	Grip float32
	// Sleet is set where what falls is not rain: winter's.
	Sleet bool
}

// profile is what one kind of weather is, at full strength.
type profile struct {
	wet, wind, rain, fog, storm, sight, grip float32
}

func profileOf(c Cond) profile {
	switch c {
	case Rain:
		return profile{wet: 1, wind: 0.35, rain: 0.7, sight: 75, grip: Grip(Rain)}
	case Storm:
		return profile{wet: 1, wind: 1, rain: 1, storm: 1, sight: 55, grip: Grip(Storm)}
	case Fog:
		return profile{wet: 0.35, wind: 0.05, fog: 1, sight: 45, grip: Grip(Fog)}
	default:
		return profile{wind: 0.1, sight: Unlimited, grip: 1}
	}
}

// Grip returns how much of a tyre's dry grip the road gives in a kind of
// weather. Fog is damp but not slick.
func Grip(c Cond) float32 {
	switch c {
	case Rain:
		return 0.85
	case Storm:
		return 0.75
	default:
		return 1
	}
}

// Possible reports whether a kind of weather can happen in a season. Every one
// can anywhere: in winter rain and storms are sleet, and the storm has no
// lightning.
func Possible(s city.Season, c Cond) bool { return c < numConds }

// Lightning reports whether storms in a season flash.
func Lightning(s city.Season) bool { return s != city.Winter }

// weights are how likely each kind of weather is in a season, in the order
// Clear, Rain, Fog, Storm. Summer has the storms and autumn the fog.
func weights(s city.Season) [numConds]float32 {
	switch s {
	case city.Summer:
		return [numConds]float32{0.45, 0.20, 0.10, 0.25}
	case city.Spring:
		return [numConds]float32{0.40, 0.35, 0.15, 0.10}
	case city.Winter:
		return [numConds]float32{0.40, 0.30, 0.20, 0.10}
	default:
		return [numConds]float32{0.30, 0.30, 0.30, 0.10}
	}
}

// Timing, in seconds. A spell is the stretch of time one kind of weather is in
// charge, and the weather fades from one to the next across their boundary.
const (
	spellMin  float32 = 240
	spellSpan float32 = 240
	// fade is how long the change from one spell to the next takes, half of it
	// either side of the boundary.
	fade float32 = 45
)

// Salts keep the decisions taken for one spell from tracking each other.
const (
	saltCond   uint64 = 0x7a1c3d5e9f2b4086
	saltLength uint64 = 0x3b9d1e77c0a5f412
	saltStrike uint64 = 0x5d2f8a6b1c93e7d0
	saltOffset uint64 = 0x61e4b07a2d58c93f
	saltDelay  uint64 = 0x1f83c6d9a4e2b705
)

// chance returns a value in [0, 1) for a seed, a purpose and an index, the way
// the city does for a cell: splitmix finalising a mix of the three.
func chance(seed, salt uint64, i int) float32 {
	h := seed ^ salt
	h += 0x9e3779b97f4a7c15
	h ^= uint64(int64(i)) * 0xbf58476d1ce4e5b9
	h = (h ^ (h >> 30)) * 0xbf58476d1ce4e5b9
	h ^= uint64(int64(i)) * 0x94d049bb133111eb
	h = (h ^ (h >> 27)) * 0x94d049bb133111eb
	h ^= h >> 31
	return float32(h>>40) / float32(1<<24)
}

// Hash is [chance] for the rest of the game: a value in [0, 1) for a seed, a
// purpose and an index, so what is drawn and what is heard come from the same
// numbers.
func Hash(seed, salt uint64, i int) float32 { return chance(seed, salt, i) }

func condOf(seed uint64, s city.Season, spell int) Cond {
	r := chance(seed, saltCond, spell)
	w := weights(s)
	for c := Cond(0); c < numConds; c++ {
		if r < w[c] {
			return c
		}
		r -= w[c]
	}
	return Clear
}

func spellLength(seed uint64, spell int) float32 {
	return spellMin + spellSpan*chance(seed, saltLength, spell)
}

// spellAt returns which spell a moment is in and how far into it.
func spellAt(seed uint64, t float32) (spell int, into, length float32) {
	for {
		length = spellLength(seed, spell)
		if t < length {
			return spell, t, length
		}
		t -= length
		spell++
	}
}

// At returns the weather in a season at a moment, in seconds on the world's
// clock. The same seed and time always give the same answer, whatever else
// has been asked.
func At(seed uint64, s city.Season, set Setting, t float32) State {
	if c, held := set.Cond(); held {
		var mix [numConds]float32
		mix[c] = 1
		return stateOf(s, mix)
	}
	t = max(t, 0)
	spell, into, length := spellAt(seed, t)
	var mix [numConds]float32
	prev, next := float32(0), float32(0)
	if into < fade/2 && spell > 0 {
		prev = 0.5 * (1 - into/(fade/2))
	}
	if length-into < fade/2 {
		next = 0.5 * (1 - (length-into)/(fade/2))
	}
	mix[condOf(seed, s, spell)] += 1 - prev - next
	if prev > 0 {
		mix[condOf(seed, s, spell-1)] += prev
	}
	if next > 0 {
		mix[condOf(seed, s, spell+1)] += next
	}
	return stateOf(s, mix)
}

// stateOf blends the kinds of weather by their shares, which sum to one.
// Sight is blended as its reciprocal, how much the air takes away, so that
// clear weather with none at all fades in rather than leaping.
func stateOf(s city.Season, mix [numConds]float32) State {
	var st State
	var ext, loss float32
	best := float32(-1)
	for c := Cond(0); c < numConds; c++ {
		f := mix[c]
		if f <= 0 {
			continue
		}
		p := profileOf(c)
		st.Wet += f * p.wet
		st.Wind += f * p.wind
		st.Rain += f * p.rain
		st.Fog += f * p.fog
		st.Storm += f * p.storm
		ext += f / p.sight
		loss += f * (1 - p.grip)
		if f > best {
			best, st.Cond = f, c
		}
	}
	st.Sight = min(1/max(ext, 1/Unlimited), Unlimited)
	st.Grip = 1 - loss
	st.Sleet = s == city.Winter
	return st
}

// Flash is a lightning strike.
type Flash struct {
	// At is when it struck, in seconds on the world's clock.
	At float32
	// Delay is how long its thunder takes to arrive, in seconds.
	Delay float32
	// Strength is how bright it is, from 0 to 1.
	Strength float32
}

// strikeSlot is how often a storm gets the chance of a strike, in seconds.
const strikeSlot float32 = 6

// Strike returns the latest strike at or before a moment that is recent enough
// for its flash or its thunder to still matter, and false if there is none.
// Strikes come only in a storm, and never in winter. The flash and the thunder
// are both worked out from this, so they cannot disagree.
func Strike(seed uint64, s city.Season, set Setting, t float32) (Flash, bool) {
	if !Lightning(s) {
		return Flash{}, false
	}
	slot := int(max(t, 0) / strikeSlot)
	// Thunder takes up to six seconds, so look back far enough to hear it.
	for k := slot; k >= max(slot-3, 0); k-- {
		at := (float32(k) + chance(seed, saltOffset, k)) * strikeSlot
		if at > t {
			continue
		}
		storm := At(seed, s, set, at).Storm
		if chance(seed, saltStrike, k) >= 0.7*storm {
			continue
		}
		return Flash{
			At:       at,
			Delay:    0.5 + 5.5*chance(seed, saltDelay, k),
			Strength: 0.5 + 0.5*chance(seed, saltDelay^saltStrike, k),
		}, true
	}
	return Flash{}, false
}
