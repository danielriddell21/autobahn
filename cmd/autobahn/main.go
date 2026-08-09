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
	width := flag.Int("width", opts.Width, "window width in pixels")
	height := flag.Int("height", opts.Height, "window height in pixels")
	camW := flag.Int("camwidth", opts.CamWidth, "AI camera width in pixels")
	camH := flag.Int("camheight", opts.CamHeight, "AI camera height in pixels")
	auto := flag.Bool("autopilot", false, "start with the AI driving")
	panel := flag.Bool("panel", true, "show the AI camera panel")
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
	opts.Width, opts.Height = *width, *height
	opts.CamWidth, opts.CamHeight = *camW, *camH
	opts.Autopilot = *auto
	opts.ShowPanel = *panel
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
