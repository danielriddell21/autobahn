package game

// The wire vocabulary for a two-player chase. crucible's netplay package
// carries these but knows nothing about them: what a pose is, and which flags
// a remote viewer cannot infer from one, is autobahn's business.
//
// Only moving things go over the wire. The city is generated from a seed, so
// both ends build exactly the same streets, buildings and signals from the
// same number and never speak of them again. What is left is a few dozen
// poses, which is small enough to send whole twenty times a second rather
// than bothering with deltas.

// SnapshotRate is how many world updates the host sends per second. Well below
// the frame rate: a chase does not need sixty updates a second to read
// correctly, and the gap is smoothed on arrival.
const SnapshotRate = 20

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
)

// Snapshot is the whole moving world at one instant, as the host sees it.
type Snapshot struct {
	// Clock is the host's simulation time, which drives the signals. Sending
	// it keeps every traffic light on both machines showing the same aspect.
	Clock float32
	// Runner is the car being chased.
	Runner Pose
	// Agents are the traffic and police, in the order the seeded world creates
	// them, so the client can match them to its own.
	Agents []Pose
	// Chaser is the index in Agents of the unit the joining player drives.
	Chaser int

	WantedLevel int
	WantedState int
	Elapsed     float32
	Over        bool
	Winner      string
}

// Input is one frame of a joining player's controls.
type Input struct {
	Throttle  float32
	Brake     float32
	Steer     float32
	Handbrake bool
	Reverse   bool
}
