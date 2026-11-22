package sim

import (
	"math/rand/v2"
	"slices"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// BodyKind is the shape of a vehicle. It changes how a car looks, how big it
// is, and how it drives: a van is slow and tall and takes an age to stop, a
// hot hatch is neither.
type BodyKind int

// The vehicle shapes on the road.
const (
	Hatchback BodyKind = iota
	Saloon
	Estate
	Van
	Taxi
	HotHatch
	// Bus is a double-decker, and Lorry a rigid box lorry: long, tall, slow
	// to get going and slower to stop. Only traffic drives them.
	Bus
	Lorry
	// Convertible is a low open two-seater and CityCar the smallest thing on
	// the road. They come with the coast, as do the two that only traffic
	// drives there: the ice cream van and the camper.
	Convertible
	CityCar
	IceCreamVan
	Camper
	// FourByFour is a boxy off-roader and SUV a big, quick, comfortable one.
	// They come with the country, where a tractor is also to be met.
	FourByFour
	SUV
	Tractor
	// Minibus carries a dozen people slowly and GrandTourer two of them very
	// fast. They come with the motorway, whose roads a gritter keeps open.
	Minibus
	GrandTourer
	Gritter
	// The cars of the taxi, emergency and racing packs. Stored as integers by
	// the settings and the career, so they stay at the end: never insert above.
	// Each pack's placeholder shape lives in its own body_ file.
	BlackCab
	CourierVan
	Ambulance
	FireEngine
	ResponseCar
	RaceCoupe
	RaceRoadster
	RaceProto
	numBodies
)

// bodyNames are what each shape is called.
var bodyNames = [numBodies]string{
	Hatchback:    "hatchback",
	Saloon:       "saloon",
	Estate:       "estate",
	Van:          "van",
	Taxi:         "taxi",
	HotHatch:     "hot hatch",
	Bus:          "bus",
	Lorry:        "lorry",
	Convertible:  "convertible",
	CityCar:      "city car",
	IceCreamVan:  "ice cream van",
	Camper:       "camper",
	FourByFour:   "4x4",
	SUV:          "suv",
	Tractor:      "tractor",
	Minibus:      "minibus",
	GrandTourer:  "grand tourer",
	Gritter:      "gritter",
	BlackCab:     "black cab",
	CourierVan:   "courier van",
	Ambulance:    "ambulance",
	FireEngine:   "fire engine",
	ResponseCar:  "response car",
	RaceCoupe:    "race coupe",
	RaceRoadster: "race roadster",
	RaceProto:    "race proto",
}

// Name returns the shape's name.
func (b BodyKind) Name() string {
	if b < 0 || b >= numBodies {
		return bodyNames[Hatchback]
	}
	return bodyNames[b]
}

// Body describes one shape: its dimensions, how it drives, and how the
// renderer should build it.
type Body struct {
	Kind BodyKind
	Spec Spec
	// RoofFrac and RoofRise set how much of the length the cabin covers and how
	// far it stands above the waist, which is most of what tells one silhouette
	// from another at a glance.
	RoofFrac float32
	RoofRise float32
	// Boxy vans get a squared-off cabin running the full width.
	Boxy bool
	// Livery picks a fixed paint scheme instead of a random one, for the shapes
	// that have one.
	Livery int
	// Ride is how far the sill stands above the road, and Wheel the radius of
	// the wheels under it.
	Ride, Wheel float32
}

// The stance of an ordinary car, in metres.
const (
	carRide  float32 = 0.27
	carWheel float32 = 0.33
)

// Bodies returns every shape there is, the ones only traffic drives included.
func Bodies() []BodyKind {
	out := make([]BodyKind, 0, numBodies)
	for k := range numBodies {
		out = append(out, k)
	}
	return out
}

// shapes is what turns the ordinary traffic car into each shape: its size,
// its weight and what it has to pull it, and the proportions the renderer
// builds it to. The base game's are here; each pack's are filled from init in
// its own body_ file.
var shapes = [numBodies]func(b *Body){
	Saloon: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.42, 0.95, 1.44
		b.Spec.Mass, b.Spec.EngineForce = 1480, 7600
		b.RoofFrac, b.RoofRise = 0.46, 0.42
	},
	Estate: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.55, 0.96, 1.52
		b.Spec.Mass, b.Spec.EngineForce = 1580, 7400
		b.RoofFrac, b.RoofRise = 0.62, 0.46
	},
	Van: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.75, 1.05, 2.05
		b.Spec.Mass, b.Spec.EngineForce = 2100, 7000
		b.Spec.MaxSpeed, b.Spec.BrakeForce = 32, 13000
		b.Spec.GripFront, b.Spec.GripRear = 78000, 84000
		b.RoofFrac, b.RoofRise, b.Boxy = 0.7, 0.72, true
	},
	Taxi: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.35, 0.98, 1.72
		b.Spec.Mass, b.Spec.EngineForce = 1700, 7200
		b.Spec.MaxSpeed = 34
		b.RoofFrac, b.RoofRise, b.Boxy = 0.6, 0.62, true
		b.Livery = 3 // the black cab
	},
	HotHatch: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.12, 0.93, 1.38
		b.Spec.Mass, b.Spec.EngineForce = 1250, 9200
		b.Spec.MaxSpeed = 46
		b.Spec.GripFront, b.Spec.GripRear = 94000, 99000
		b.RoofFrac, b.RoofRise = 0.48, 0.38
	},
	Bus: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 5.2, 1.25, 4.3
		b.Spec.FrontAxle, b.Spec.RearAxle = 2.9, 2.6
		b.Spec.haul(5600, 15000)
		b.Spec.MaxSpeed = 22
		b.RoofFrac, b.RoofRise, b.Boxy = 1, 3.2, true
		b.Ride, b.Wheel = 0.36, 0.5
		b.Livery = 0 // the red bus
	},
	Lorry: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 4.3, 1.25, 3.6
		b.Spec.FrontAxle, b.Spec.RearAxle = 2.6, 2.2
		b.Spec.haul(6400, 16000)
		b.Spec.MaxSpeed = 25
		b.RoofFrac, b.RoofRise, b.Boxy = 0.22, 2.45, true
		b.Ride, b.Wheel = 0.5, 0.5
	},
	Convertible: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.1, 0.93, 1.2
		b.Spec.Mass, b.Spec.EngineForce = 1180, 9800
		b.Spec.MaxSpeed = 50
		b.Spec.GripFront, b.Spec.GripRear = 97000, 102000
		b.RoofFrac, b.RoofRise = 0.36, 0.3
		b.Ride = 0.22
	},
	CityCar: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 1.6, 0.78, 1.34
		b.Spec.Mass, b.Spec.EngineForce = 780, 6400
		b.Spec.MaxSpeed = 36
		b.RoofFrac, b.RoofRise = 0.6, 0.46
		b.Wheel = 0.26
	},
	IceCreamVan: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.75, 1.05, 2.3
		b.Spec.Mass, b.Spec.EngineForce = 2300, 6600
		b.Spec.MaxSpeed, b.Spec.BrakeForce = 24, 13000
		b.RoofFrac, b.RoofRise, b.Boxy = 0.72, 1.0, true
		b.Livery = 2 // white, with its trimmings
	},
	Camper: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.3, 0.92, 2.0
		b.Spec.Mass, b.Spec.EngineForce = 1500, 5600
		b.Spec.MaxSpeed, b.Spec.BrakeForce = 28, 14000
		b.RoofFrac, b.RoofRise, b.Boxy = 0.94, 0.95, true
	},
	FourByFour: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.3, 0.96, 1.98
		b.Spec.Mass, b.Spec.EngineForce = 1950, 7400
		b.Spec.MaxSpeed = 34
		b.Spec.GripFront, b.Spec.GripRear = 84000, 90000
		b.RoofFrac, b.RoofRise, b.Boxy = 0.62, 0.74, true
		b.Ride, b.Wheel = 0.44, 0.41
	},
	SUV: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.45, 1.0, 1.78
		b.Spec.Mass, b.Spec.EngineForce = 2150, 9600
		b.Spec.MaxSpeed = 46
		b.RoofFrac, b.RoofRise = 0.64, 0.6
		b.Ride, b.Wheel = 0.36, 0.4
	},
	Minibus: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.95, 1.02, 2.35
		b.Spec.Mass, b.Spec.EngineForce = 2500, 7600
		b.Spec.MaxSpeed, b.Spec.BrakeForce = 34, 14000
		b.Spec.GripFront, b.Spec.GripRear = 80000, 86000
		b.RoofFrac, b.RoofRise, b.Boxy = 0.86, 1.05, true
		b.Ride, b.Wheel = 0.32, 0.36
	},
	GrandTourer: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.5, 0.98, 1.3
		b.Spec.Mass, b.Spec.EngineForce = 1750, 12400
		b.Spec.MaxSpeed = 62
		b.Spec.GripFront, b.Spec.GripRear = 98000, 104000
		b.RoofFrac, b.RoofRise = 0.38, 0.36
		b.Ride = 0.22
	},
	Gritter: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 3.9, 1.25, 3.0
		b.Spec.FrontAxle, b.Spec.RearAxle = 2.4, 2.0
		b.Spec.haul(6800, 15000)
		b.Spec.MaxSpeed = 18 // it is working, not going anywhere
		b.RoofFrac, b.RoofRise, b.Boxy = 0.24, 1.85, true
		b.Ride, b.Wheel = 0.5, 0.5
		b.Livery = 4 // amber
	},
	Tractor: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.0, 1.0, 2.7
		b.Spec.FrontAxle, b.Spec.RearAxle = 1.3, 1.1
		b.Spec.haul(3200, 7000)
		b.Spec.MaxSpeed = 9 // twenty miles an hour, flat out
		b.RoofFrac, b.RoofRise, b.Boxy = 0.42, 1.35, true
		b.Ride, b.Wheel = 0.62, 0.46
		b.Livery = 5 // green
	},
	Hatchback: func(b *Body) {
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.18, 0.92, 1.46
		b.Spec.Mass = 1320
	},
}

// BodyOf returns the description of a shape.
func BodyOf(k BodyKind) Body {
	if k < 0 || k >= numBodies {
		k = Hatchback
	}
	b := Body{
		Kind: k, Spec: TrafficSpec(), RoofFrac: 0.52, RoofRise: 0.44, Livery: -1,
		Ride: carRide, Wheel: carWheel,
	}
	shapes[k](&b)
	return b
}

// haul turns a car's profile into a heavy vehicle's. Everything that holds a
// car on the road is scaled with the weight, so it corners and stops like a
// heavier car rather than like one with bald tyres; the engine is given
// outright, and is what makes it slow away from the lights.
func (s *Spec) haul(mass, engine float32) {
	weight := mass / s.Mass
	// A longer body takes more turning as well as a heavier one.
	reach := (s.HalfLength * s.HalfLength) / (2.25 * 2.25)
	s.Mass = mass
	s.Inertia *= weight * reach
	s.BrakeForce *= weight * 0.8
	s.GripFront *= weight
	s.GripRear *= weight
	s.MaxLateral *= weight
	s.EngineForce = engine
}

// mix is the share of each shape among a map's traffic, out of 100.
type mix [numBodies]int

// mixes is what is on the road in each place. A map with no mix of its own
// has the city's.
var mixes = map[city.Map]mix{
	city.MapCity: {
		Hatchback: 31,
		Saloon:    22,
		Estate:    13,
		Van:       14,
		Taxi:      8,
		HotHatch:  6,
		Bus:       3,
		Lorry:     3,
	},
	// The seaside is on holiday: fewer vans and lorries, more small cars and
	// open ones, campers heading for the front and an ice cream van working
	// along it.
	city.MapSeaside: {
		Hatchback:   28,
		Saloon:      16,
		Estate:      14,
		Van:         8,
		Taxi:        6,
		HotHatch:    6,
		Bus:         3,
		Lorry:       1,
		Convertible: 6,
		CityCar:     6,
		IceCreamVan: 2,
		Camper:      4,
	},
	// The country runs on estates and four-wheel drive, and now and then
	// everybody is stuck behind a tractor.
	city.MapVillage: {
		Hatchback:  26,
		Saloon:     14,
		Estate:     16,
		Van:        10,
		Taxi:       2,
		HotHatch:   4,
		Bus:        2,
		Lorry:      2,
		CityCar:    3,
		FourByFour: 10,
		SUV:        6,
		Tractor:    5,
	},
	// The motorway town is freight and commuters, with four-wheel drive out
	// for the snow and a gritter working the main roads.
	city.MapMotorway: {
		Hatchback:  24,
		Saloon:     20,
		Estate:     12,
		Van:        12,
		Taxi:       2,
		HotHatch:   4,
		Bus:        3,
		Lorry:      9,
		FourByFour: 5,
		SUV:        5,
		Minibus:    2,
		Gritter:    2,
	},
}

// Mix is the share of each shape among a map's traffic. A custom map may
// bring its own; see [Config.Mix].
type Mix = mix

// MixFor returns the traffic mix of a map.
func MixFor(m city.Map) Mix { return mixFor(m) }

// mixFor returns the traffic mix of a map.
func mixFor(m city.Map) mix {
	if w, ok := mixes[m]; ok {
		return w
	}
	return mixes[city.MapCity]
}

// RandomBody draws a shape from the mix that fills a map's streets.
func RandomBody(rng *rand.Rand, m city.Map) Body { return randomBody(rng, mixFor(m)) }

// randomBody draws a shape from a mix. One with nothing in it is the city's.
func randomBody(rng *rand.Rand, weights mix) Body {
	total := 0
	for _, w := range weights {
		total += max(w, 0)
	}
	if total == 0 {
		weights = mixes[city.MapCity]
		for _, w := range weights {
			total += w
		}
	}
	pick := rng.IntN(total)
	for k, w := range weights {
		if pick -= max(w, 0); pick < 0 {
			return BodyOf(BodyKind(k))
		}
	}
	return BodyOf(Hatchback)
}

// PlayerBody returns the car the player drives unless they choose another:
// quick, small and eager, since it is the one being chased.
func PlayerBody() Body { return PlayerBodyOf(HotHatch) }

// playerBodies are the shapes the garage offers, in the order it lists them.
var playerBodies = []BodyKind{
	HotHatch, Hatchback, Saloon, Estate, Taxi, Van,
	Convertible, CityCar, FourByFour, SUV, Minibus, GrandTourer,
	// The packs' cars. Each pack owns its own shape in its body_ file.
	BlackCab, CourierVan, Ambulance, FireEngine, ResponseCar,
	RaceCoupe, RaceRoadster, RaceProto,
}

// PlayerBodies returns the shapes the player may choose from.
func PlayerBodies() []BodyKind { return slices.Clone(playerBodies) }

// IsPlayerBody reports whether the player may drive a shape.
func IsPlayerBody(k BodyKind) bool { return slices.Contains(playerBodies, k) }

// PlayerBodyOf returns the player's car in the given shape.
//
// It is the player's car first and the shape second. The player's car is a
// good deal livelier than traffic, and every shape keeps that, but each one
// gives and takes against the hot hatch the way it does on the road: a van is
// heavier, slower and less sure in a corner. The axles and the steering are
// left alone, because the autopilot is calibrated to them, and the brakes keep
// pace with the weight, so the stopping distances it plans around still hold.
func PlayerBodyOf(k BodyKind) Body {
	if !IsPlayerBody(k) {
		k = HotHatch
	}
	ref, shape := BodyOf(HotHatch).Spec, BodyOf(k)
	b := shape
	b.Livery = -1 // the player picks the paint
	b.Spec = CarSpec()
	b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.18, 0.94, 1.4
	if k == HotHatch {
		return b
	}
	src := shape.Spec
	weight := src.Mass / ref.Mass
	b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = src.HalfLength, src.HalfWidth, src.Height
	b.Spec.Mass *= weight
	b.Spec.Inertia *= weight
	b.Spec.BrakeForce *= weight
	b.Spec.EngineForce *= src.EngineForce / ref.EngineForce
	b.Spec.MaxSpeed *= src.MaxSpeed / ref.MaxSpeed
	b.Spec.GripFront *= src.GripFront / ref.GripFront
	b.Spec.GripRear *= src.GripRear / ref.GripRear
	b.Spec.MaxLateral *= weight
	return b
}

// parkedShapes are the cars people leave at the kerb.
var parkedShapes = []BodyKind{Hatchback, Hatchback, Saloon, Saloon, Estate, HotHatch, Van, CityCar}

// ParkedBody returns the car standing in a parking space. The city says only
// that a car is there and gives a draw to choose it with.
func ParkedBody(p city.Parked) Body {
	return BodyOf(parkedShapes[int(p.Shape)%len(parkedShapes)])
}

// ParkedBox is the ground a parked car covers.
func ParkedBox(p city.Parked) mathx.OBB {
	s := ParkedBody(p).Spec
	return mathx.OBB{Center: p.Pos, HalfL: s.HalfLength, HalfW: s.HalfWidth, Heading: p.Heading}
}
