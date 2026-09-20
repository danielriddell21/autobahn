// Command autobahn is a 3D driving game set in a procedurally generated city.
//
// Drive it yourself, or press Tab to hand the car to an autopilot that sees the
// world only through a camera feed annotated with colour-coded detection boxes.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/danielriddell21/autobahn/internal/game"
)

// version is written by the linker at release time. The release tool verifies
// the symbol exists before injecting, so a binary that reports "dev" was built
// from a checkout rather than published.
var version = "dev"

func main() {
	opts, done, err := parse(os.Args[0], os.Args[1:], os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "autobahn:", err)
		os.Exit(2)
	}
	if done {
		return
	}

	if err := game.Run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "autobahn:", err)
		os.Exit(1)
	}
}

// parse turns a command line into the options the game runs with, writing
// anything the command line asked to be told to out. It reports done when the
// command line wanted an answer rather than a game, so that asking a headless
// machine which version it is never opens a window.
func parse(name string, args []string, out io.Writer) (opts game.Options, done bool, err error) {
	opts = game.DefaultOptions()

	fs := flag.NewFlagSet(name, flag.ExitOnError)
	seed := fs.Uint64("seed", opts.Seed, "city generation seed")
	traffic := fs.Int("traffic", opts.Traffic, "number of ambient traffic cars")
	police := fs.Int("police", opts.Police, "number of patrolling police units; 0 disables them")
	width := fs.Int("width", opts.Width, "window width in pixels")
	height := fs.Int("height", opts.Height, "window height in pixels")
	camW := fs.Int("camwidth", opts.CamWidth, "AI camera width in pixels")
	camH := fs.Int("camheight", opts.CamHeight, "AI camera height in pixels")
	auto := fs.Bool("autopilot", false, "start with the AI driving")
	reckless := fs.Bool("reckless", false, "drive the route at full throttle ignoring every rule, to exercise the police")
	panel := fs.Bool("panel", true, "show the AI camera panel while the autopilot drives")
	mute := fs.Bool("mute", false, "synthesise no sound")
	chase := fs.Bool("chase", false, "drive a police car and chase the autopilot")
	host := fs.String("host", "", "host a two-player chase on this address, e.g. :7777")
	join := fs.String("join", "", "join a chase hosted at this address")
	camera := fs.String("camera", "chase", "viewpoint: chase, bonnet or high")
	hz := fs.Float64("perception", float64(opts.PerceptionH), "perception updates per second")
	frames := fs.Int("frames", 0, "run this many frames then exit; 0 runs until closed")
	shot := fs.String("screenshot", "", "write a screenshot to this path before exiting")
	showVersion := fs.Bool("version", false, "print the version and exit")
	stats := fs.Bool("stats", false, "print a session summary on exit")
	dbg := fs.Bool("debugvision", false, "print camera range estimates against ground truth")
	// The family's shared recording flags, so -record means the same thing
	// here as it does in every other app in the family.
	opts.Rec.AddStdFlags(fs)
	if err := fs.Parse(args); err != nil {
		return opts, false, err
	}

	if *showVersion {
		fmt.Fprintln(out, "autobahn", version)
		return opts, true, nil
	}

	// Which flags were actually given, so the player's stored settings know
	// what they may not override.
	opts.Given = game.Given{}
	fs.Visit(func(f *flag.Flag) { opts.Given[f.Name] = true })

	opts.Seed = *seed
	opts.Traffic = *traffic
	opts.Police = *police
	opts.Width, opts.Height = *width, *height
	opts.CamWidth, opts.CamHeight = *camW, *camH
	opts.Autopilot = *auto
	opts.Reckless = *reckless
	opts.ShowPanel = *panel
	opts.Mute = *mute
	opts.Chase = *chase
	opts.Host, opts.Join = *host, *join
	if opts.Host != "" && opts.Join != "" {
		return opts, false, errors.New("give -host or -join, not both")
	}
	switch *camera {
	case "bonnet":
		opts.Camera = game.CameraBonnet
	case "high":
		opts.Camera = game.CameraHigh
	case "chase":
		opts.Camera = game.CameraChase
	default:
		return opts, false, fmt.Errorf("unknown camera %q", *camera)
	}
	opts.PerceptionH = float32(*hz)
	opts.Frames = *frames
	opts.Screenshot = *shot
	opts.Stats = *stats
	opts.DebugVision = *dbg

	return opts, false, nil
}
