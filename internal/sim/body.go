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
	numBodies
)

// Name returns the shape's name.
func (b BodyKind) Name() string {
	switch b {
	case Saloon:
		return "saloon"
	case Estate:
		return "estate"
	case Van:
		return "van"
	case Taxi:
		return "taxi"
	case HotHatch:
		return "hot hatch"
	case Bus:
		return "bus"
	case Lorry:
		return "lorry"
	case Convertible:
		return "convertible"
	case CityCar:
		return "city car"
	case IceCreamVan:
		return "ice cream van"
	case Camper:
		return "camper"
	default:
		return "hatchback"
	}
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

// BodyOf returns the description of a shape.
func BodyOf(k BodyKind) Body {
	base := TrafficSpec()
	b := Body{
		Kind: k, Spec: base, RoofFrac: 0.52, RoofRise: 0.44, Livery: -1,
		Ride: carRide, Wheel: carWheel,
	}

	switch k {
	case Saloon:
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.42, 0.95, 1.44
		b.Spec.Mass, b.Spec.EngineForce = 1480, 7600
		b.RoofFrac, b.RoofRise = 0.46, 0.42
	case Estate:
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.55, 0.96, 1.52
		b.Spec.Mass, b.Spec.EngineForce = 1580, 7400
		b.RoofFrac, b.RoofRise = 0.62, 0.46
	case Van:
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.75, 1.05, 2.05
		b.Spec.Mass, b.Spec.EngineForce = 2100, 7000
		b.Spec.MaxSpeed, b.Spec.BrakeForce = 32, 13000
		b.Spec.GripFront, b.Spec.GripRear = 78000, 84000
		b.RoofFrac, b.RoofRise, b.Boxy = 0.7, 0.72, true
	case Taxi:
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.35, 0.98, 1.72
		b.Spec.Mass, b.Spec.EngineForce = 1700, 7200
		b.Spec.MaxSpeed = 34
		b.RoofFrac, b.RoofRise, b.Boxy = 0.6, 0.62, true
		b.Livery = 3 // the black cab
	case HotHatch:
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.12, 0.93, 1.38
		b.Spec.Mass, b.Spec.EngineForce = 1250, 9200
		b.Spec.MaxSpeed = 46
		b.Spec.GripFront, b.Spec.GripRear = 94000, 99000
		b.RoofFrac, b.RoofRise = 0.48, 0.38
	case Bus:
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 5.2, 1.25, 4.3
		b.Spec.FrontAxle, b.Spec.RearAxle = 2.9, 2.6
		b.Spec.haul(5600, 15000)
		b.Spec.MaxSpeed = 22
		b.RoofFrac, b.RoofRise, b.Boxy = 1, 3.2, true
		b.Ride, b.Wheel = 0.36, 0.5
		b.Livery = 0 // the red bus
	case Lorry:
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 4.3, 1.25, 3.6
		b.Spec.FrontAxle, b.Spec.RearAxle = 2.6, 2.2
		b.Spec.haul(6400, 16000)
		b.Spec.MaxSpeed = 25
		b.RoofFrac, b.RoofRise, b.Boxy = 0.22, 2.45, true
		b.Ride, b.Wheel = 0.5, 0.5
	case Convertible:
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.1, 0.93, 1.2
		b.Spec.Mass, b.Spec.EngineForce = 1180, 9800
		b.Spec.MaxSpeed = 50
		b.Spec.GripFront, b.Spec.GripRear = 97000, 102000
		b.RoofFrac, b.RoofRise = 0.36, 0.3
		b.Ride = 0.22
	case CityCar:
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 1.6, 0.78, 1.34
		b.Spec.Mass, b.Spec.EngineForce = 780, 6400
		b.Spec.MaxSpeed = 36
		b.RoofFrac, b.RoofRise = 0.6, 0.46
		b.Wheel = 0.26
	case IceCreamVan:
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.75, 1.05, 2.3
		b.Spec.Mass, b.Spec.EngineForce = 2300, 6600
		b.Spec.MaxSpeed, b.Spec.BrakeForce = 24, 13000
		b.RoofFrac, b.RoofRise, b.Boxy = 0.72, 1.0, true
		b.Livery = 2 // white, with its trimmings
	case Camper:
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.3, 0.92, 2.0
		b.Spec.Mass, b.Spec.EngineForce = 1500, 5600
		b.Spec.MaxSpeed, b.Spec.BrakeForce = 28, 14000
		b.RoofFrac, b.RoofRise, b.Boxy = 0.94, 0.95, true
	default:
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.18, 0.92, 1.46
		b.Spec.Mass = 1320
	}
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
}

// mixFor returns the traffic mix of a map.
func mixFor(m city.Map) mix {
	if w, ok := mixes[m]; ok {
		return w
	}
	return mixes[city.MapCity]
}

// RandomBody draws a shape from the mix that fills a map's streets.
func RandomBody(rng *rand.Rand, m city.Map) Body {
	weights := mixFor(m)
	total := 0
	for _, w := range weights {
		total += w
	}
	pick := rng.IntN(total)
	for k, w := range weights {
		if pick -= w; pick < 0 {
			return BodyOf(BodyKind(k))
		}
	}
	return BodyOf(Hatchback)
}

// PlayerBody returns the car the player drives unless they choose another:
// quick, small and eager, since it is the one being chased.
func PlayerBody() Body { return PlayerBodyOf(HotHatch) }

// playerBodies are the shapes the garage offers, in the order it lists them.
var playerBodies = []BodyKind{HotHatch, Hatchback, Saloon, Estate, Taxi, Van, Convertible, CityCar}

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
