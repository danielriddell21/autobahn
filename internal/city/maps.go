package city

import "strings"

// Map is a place to drive: the same growing generator, tuned and dressed
// differently. A map changes what the generator is asked for and never how it
// works, so every map grows outward as the car drives and holds to the same
// rule that a place is a function of where it is.
type Map int

// The maps, in the order the title screen offers them.
const (
	// MapCity is the British city the game began with.
	MapCity Map = iota
	numMaps
)

// Maps returns every map there is.
func Maps() []Map {
	out := make([]Map, 0, numMaps)
	for m := range numMaps {
		out = append(out, m)
	}
	return out
}

// Valid reports whether m names a map.
func (m Map) Valid() bool { return m >= 0 && m < numMaps }

// Name returns the map's name, as a menu shows it and -map reads it.
func (m Map) Name() string {
	switch m { //nolint:gocritic // one map now, more to come
	default:
		return "city"
	}
}

// ParseMap reads a map's name, forgiving case and spaces.
func ParseMap(name string) (Map, bool) {
	want := squashName(name)
	for m := range numMaps {
		if squashName(m.Name()) == want {
			return m, true
		}
	}
	return MapCity, false
}

func squashName(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
}

// ParamsFor returns the generation parameters for a map. The seed chooses
// which city of that kind it is.
func ParamsFor(m Map, seed uint64) Params {
	p := DefaultParams(seed)
	if m.Valid() {
		p.Map = m
	}
	return p
}
