// Command autobahn is a 3D driving game set in a procedurally generated city.
//
// Drive it yourself, or press Tab to hand the car to an autopilot that sees the
// world only through a camera feed annotated with colour-coded detection boxes.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/danielriddell21/autobahn/internal/game"
)

func main() {
	opts := game.DefaultOptions()

	seed := flag.Uint64("seed", opts.Seed, "city generation seed")
	traffic := flag.Int("traffic", opts.Traffic, "number of ambient traffic cars")
	police := flag.Int("police", opts.Police, "number of patrolling police units; 0 disables them")
	width := flag.Int("width", opts.Width, "window width in pixels")
	height := flag.Int("height", opts.Height, "window height in pixels")
	camW := flag.Int("camwidth", opts.CamWidth, "AI camera width in pixels")
	camH := flag.Int("camheight", opts.CamHeight, "AI camera height in pixels")
	auto := flag.Bool("autopilot", false, "start with the AI driving")
	reckless := flag.Bool("reckless", false, "drive the route at full throttle ignoring every rule, to exercise the police")
	panel := flag.Bool("panel", true, "show the AI camera panel")
	mute := flag.Bool("mute", false, "synthesise no sound")
	host := flag.String("host", "", "host a two-player chase on this address, e.g. :7777")
	join := flag.String("join", "", "join a chase hosted at this address")
	camera := flag.String("camera", "chase", "viewpoint: chase, bonnet or high")
	hz := flag.Float64("perception", float64(opts.PerceptionH), "perception updates per second")
	frames := flag.Int("frames", 0, "run this many frames then exit; 0 runs until closed")
	shot := flag.String("screenshot", "", "write a screenshot to this path before exiting")
	stats := flag.Bool("stats", false, "print a session summary on exit")
	dbg := flag.Bool("debugvision", false, "print camera range estimates against ground truth")
	rec := flag.String("record", "", "capture the drive to this .gif or .mp4 path")
	recScale := flag.Int("recordscale", 3, "downscale factor for the capture")
	flag.Parse()

	opts.Seed = *seed
	opts.Traffic = *traffic
	opts.Police = *police
	opts.Width, opts.Height = *width, *height
	opts.CamWidth, opts.CamHeight = *camW, *camH
	opts.Autopilot = *auto
	opts.Reckless = *reckless
	opts.ShowPanel = *panel
	opts.Mute = *mute
	opts.Host, opts.Join = *host, *join
	if opts.Host != "" && opts.Join != "" {
		fmt.Fprintln(os.Stderr, "autobahn: give -host or -join, not both")
		os.Exit(2)
	}
	switch *camera {
	case "bonnet":
		opts.Camera = game.CameraBonnet
	case "high":
		opts.Camera = game.CameraHigh
	case "chase":
		opts.Camera = game.CameraChase
	default:
		fmt.Fprintf(os.Stderr, "autobahn: unknown camera %q\n", *camera)
		os.Exit(2)
	}
	opts.PerceptionH = float32(*hz)
	opts.Frames = *frames
	opts.Screenshot = *shot
	opts.Stats = *stats
	opts.DebugVision = *dbg
	opts.Record = *rec
	opts.RecordScale = *recScale

	if err := game.Run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "autobahn:", err)
		os.Exit(1)
	}
}
