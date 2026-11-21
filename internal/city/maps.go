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

// Season is the time of year a map is driven in. Each map has one and keeps
// it: the season is part of what the place looks like, and in winter of how
// it drives.
type Season int

// The seasons.
const (
	Autumn Season = iota
	Summer
	Spring
	Winter
	numSeasons
)

// Seasons returns every season.
func Seasons() []Season {
	out := make([]Season, 0, numSeasons)
	for s := range numSeasons {
		out = append(out, s)
	}
	return out
}

// Name returns the season's name.
func (s Season) Name() string {
	switch s {
	case Summer:
		return "summer"
	case Spring:
		return "spring"
	case Winter:
		return "winter"
	default:
		return "autumn"
	}
}

// Season returns the time of year a map is driven in.
func (m Map) Season() Season {
	switch m { //nolint:gocritic // one map now, more to come
	default:
		return Autumn
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

// Map returns which place this city is.
func (c *City) Map() Map { return c.params.Map }
