package sim

import "math/rand/v2"

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
}

// BodyOf returns the description of a shape.
func BodyOf(k BodyKind) Body {
	base := TrafficSpec()
	b := Body{Kind: k, Spec: base, RoofFrac: 0.52, RoofRise: 0.44, Livery: -1}

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
	default:
		b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.18, 0.92, 1.46
		b.Spec.Mass = 1320
	}
	return b
}

// bodyWeights is the mix on the road, out of 100.
var bodyWeights = [numBodies]int{
	Hatchback: 34,
	Saloon:    24,
	Estate:    14,
	Van:       14,
	Taxi:      8,
	HotHatch:  6,
}

// RandomBody draws a shape from the mix that fills an ordinary street.
func RandomBody(rng *rand.Rand) Body {
	total := 0
	for _, w := range bodyWeights {
		total += w
	}
	pick := rng.IntN(total)
	for k, w := range bodyWeights {
		if pick -= w; pick < 0 {
			return BodyOf(BodyKind(k))
		}
	}
	return BodyOf(Hatchback)
}

// PlayerBody returns the car the player drives: quick, small and eager, since
// it is the one being chased.
func PlayerBody() Body {
	b := BodyOf(HotHatch)
	b.Spec = CarSpec()
	b.Spec.HalfLength, b.Spec.HalfWidth, b.Spec.Height = 2.18, 0.94, 1.4
	return b
}
