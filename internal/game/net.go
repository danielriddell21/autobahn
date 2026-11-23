package game

// The wire vocabulary for a two-player game. crucible's netplay package
// carries these but knows nothing about them: what a pose is, and which flags
// a remote viewer cannot infer from one, is autobahn's business.
//
// Only moving things go over the wire. The city is generated from a seed, so
// both ends build exactly the same streets, buildings and signals from the
// same number and never speak of them again. What is left is a few dozen
// poses, which is small enough to send whole twenty times a second rather
// than bothering with deltas.
//
// The same host-authoritative session carries every mode. The host simulates
// and the joiner sends controls and draws what comes back; Snapshot.Mode says
// what the two of them are doing, and the joiner learns it from the first
// snapshot, which is also how it learns which world to build.

// SnapshotRate is how many world updates the host sends per second. Well below
// the frame rate: a chase does not need sixty updates a second to read
// correctly, and the gap is smoothed on arrival.
const SnapshotRate = 20

// Proto is the version of this vocabulary. The joiner compares it with the
// host's before it builds anything, so two builds that disagree about what a
// snapshot means fail in the lobby, saying so, rather than drawing nonsense.
// Change it whenever a field changes meaning.
const Proto uint16 = 3

// NetMode is what two people on one session are doing together.
type NetMode uint8

// The modes the host chooses between.
const (
	// ModeChase is the original: a runner against a police car.
	ModeChase NetMode = iota
	// ModeCoopCars is two cars on one team, each driven by its own player.
	ModeCoopCars
	// ModeCoopCar is two players in one car: one steers, the other works the
	// pedals.
	ModeCoopCar
	numNetModes
)

// String names the mode, as the menus show it.
func (m NetMode) String() string {
	switch m {
	case ModeCoopCars:
		return "Co-op: two cars"
	case ModeCoopCar:
		return "Co-op: one car"
	default:
		return "Chase"
	}
}

// Coop reports whether the two players are on the same side.
func (m NetMode) Coop() bool { return m == ModeCoopCars || m == ModeCoopCar }

// SharedCar reports whether the two players are in one car.
func (m NetMode) SharedCar() bool { return m == ModeCoopCar }

// ParseNetMode reads a mode as -play names it: chase, coop or coop-car.
func ParseNetMode(name string) (NetMode, bool) {
	switch name {
	case "chase":
		return ModeChase, true
	case "coop":
		return ModeCoopCars, true
	case "coop-car":
		return ModeCoopCar, true
	}
	return ModeChase, false
}

// Pose is one vehicle as the wire sees it: where it is, which way it points,
// and the few flags the renderer needs.
type Pose struct {
	X, Z  float32
	Yaw   float32
	Speed float32
	// Flags carries the states a remote viewer cannot infer from a pose.
	Flags uint8
}

// Pose flags.
const (
	// FlagPursuing marks a police unit running to a call, so its lights flash.
	FlagPursuing uint8 = 1 << iota
	// FlagBraking lights the brake lamps.
	FlagBraking
	// FlagBoosting marks a car burning nitrous.
	FlagBoosting
	// FlagIndicateLeft and FlagIndicateRight show a turn signal a person set.
	FlagIndicateLeft
	FlagIndicateRight
)

// Snapshot is the whole moving world at one instant, as the host sees it.
type Snapshot struct {
	// Proto is the host's Proto, checked before anything else is believed.
	Proto uint16
	// Mode is what the two players are doing.
	Mode NetMode
	// Seed, Map, Traffic and Police are what the host built its world from, so
	// the joiner can build the same one whatever its own settings say.
	Seed    uint64
	Map     int
	Traffic int
	Police  int
	// Car and Paint are the shape and colour of the host's car, which the
	// joiner draws too when they share it.
	Car, Paint int
	// Custom is the share code of the custom map the host built its world from,
	// empty for a preset, so the joiner builds the same place.
	Custom string

	// Clock is the host's simulation time, which drives the signals. Sending
	// it keeps every traffic light on both machines showing the same aspect.
	Clock float32
	// Hour is the host's time of day, so both players drive in the same light.
	Hour float32
	// Weather is the host's weather setting, which with Clock is all the weather
	// there is: the guest works the sky out from the same seed.
	Weather uint8
	// Runner is the car being chased, or the host's car in co-op.
	Runner Pose
	// Agents are the traffic and police, in the order the seeded world creates
	// them, so the client can match them to its own.
	Agents []Pose
	// Chaser is the index in Agents of the unit the joining player drives.
	Chaser int
	// Partner is the index in Agents of the car the joining player drives in
	// two-car co-op, and -1 in every other mode.
	Partner int

	WantedLevel int
	WantedState int
	Elapsed     float32
	Over        bool
	Winner      string

	// The rest of the pursuit, so that each player's own machine can announce
	// it and bank it. Both players bank the whole of what the team earned.
	Bounty, Banked, Stops, Escapes int
	Busting, Cooldown              float32
	// Assisted is set while either player's autopilot has had a hand in the
	// pursuit, which stops it counting toward the packs at both ends.
	Assisted bool
}

// Input is one frame of a joining player's controls.
type Input struct {
	Throttle  float32
	Brake     float32
	Steer     float32
	Handbrake bool
	Reverse   bool
	Boost     bool
	// Indicator is the turn signal the player has set: 0 off, 1 left, 2 right.
	Indicator int8
	// Auto says the controls came from this player's autopilot rather than their
	// hands, which is what keeps a pursuit from counting toward the packs.
	Auto bool
}
