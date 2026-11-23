package city

import (
	"strings"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

// Map is a place to drive: the same growing generator, tuned and dressed
// differently. A map changes what the generator is asked for and never how it
// works, so every map grows outward as the car drives and holds to the same
// rule that a place is a function of where it is.
type Map int

// The maps, in the order the menus offer them (Play's map cards). They are
// stored as integers, in the settings and the share codes, so a new map goes
// on the end.
const (
	// MapCity is the British city the game began with.
	MapCity Map = iota
	// MapSeaside is a seaside town: low terraces, and the sea along one
	// side with a beach below the front.
	MapSeaside
	// MapVillage is a village in the country: cottages along long lanes,
	// with fields between them.
	MapVillage
	// MapMotorway is a town built round its dual carriageways: fast roads
	// a long way apart, big junctions, and sheds rather than streets.
	MapMotorway
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
	switch m {
	case MapSeaside:
		return "seatown"
	case MapVillage:
		return "village"
	case MapMotorway:
		return "roadtown"
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
	switch m {
	case MapSeaside:
		return Summer
	case MapVillage:
		return Spring
	case MapMotorway:
		return Winter
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
	if !m.Valid() {
		return p
	}
	p.Map = m
	if m == MapSeaside {
		// Shorter blocks of low terraces with a few more greens among them,
		// and the front three streets down from where a drive begins. That
		// grid line is one of the main roads, so the seafront is a proper
		// promenade rather than a back street.
		p.MinSpan, p.MaxSpan = 70, 120
		p.Roundabouts, p.Parks, p.Rise = 0.2, 0.14, 0.22
		p.Coast, p.CoastRow = true, 3
		p.Wind = 26
		p.Works = 0.06
		// The town comes down a hill to the front.
		p.Hills, p.HillSpan, p.Bluff = 3.5, 700, 20
	}
	if m == MapVillage {
		// Long lanes with a main road only every fifth line, so nearly every
		// junction is a give way rather than lights; two blocks in five left
		// as fields, and nothing over three storeys. Roundabouts are the
		// exception here as on the motorway, and for the same reason: they
		// are where the autopilot comes to grief.
		p.MinSpan, p.MaxSpan = 110, 200
		p.Roundabouts, p.Parks, p.Rise = 0.15, 0.42, 0
		p.Floor, p.Storeys, p.MainEvery, p.Balanced = 5.5, 4.5, 5, true
		// Nothing in a village runs straight for long.
		p.Wind = 40
		p.Works = 0.06
		// And nothing is level either: rolling country.
		p.Hills, p.HillSpan = 8, 850
	}
	if m == MapMotorway {
		// Every other grid line is a dual carriageway and the ones between
		// are its feeders, a long way apart, with no side streets at all.
		// Nearly every junction is under lights: a roundabout between two
		// dual carriageways is the one place the autopilot comes to grief,
		// and a town built of them fails the gate.
		// The limits are the fast ones: fifty on the main roads, forty off
		// them.
		p.MinSpan, p.MaxSpan = 170, 290
		p.Roundabouts, p.Parks, p.Rise = 0.1, 0.14, 0.12
		p.Floor, p.Storeys, p.MainEvery, p.Balanced = 6, 6, 2, true
		p.Limits = [3]float32{mathx.MPH(30), mathx.MPH(40), mathx.MPH(50)}
		// Fast roads sweep rather than wind.
		p.Wind = 18
		// Long grades, as a fast road takes a hill.
		p.Hills, p.HillSpan = 11, 2600
	}
	return p
}

// Map returns which place this city is.
func (c *City) Map() Map { return c.params.Map }

// Params returns the parameters the city was generated from.
func (c *City) Params() Params { return c.params }
