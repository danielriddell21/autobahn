// Command eval scores the autopilot over many cities without a display.
//
// It runs the complete perception and control loop: the annotator projects the
// world into detection boxes, those boxes are rasterised into an image, the
// scanner reads that image back a pixel at a time, and the autopilot drives on
// what it recovers. Nothing is short-circuited — the autopilot still receives
// only pixels.
//
// The scanner matches exact class colours and never looks at the scene behind
// the boxes, so rasterising the boxes alone yields exactly the detections a
// rendered frame would. That is what makes this run with no GPU, which in turn
// is what lets it run in CI and across many seeds at once.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"os"
	"runtime"
	"sort"
	"sync"
	"text/tabwriter"

	"github.com/danielriddell21/autobahn/internal/annotate"
	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/vision"
)

// The simulation runs at a fixed step, so a run is reproducible from its seed.
const fixedStep float32 = 1.0 / 60

// result is one city's worth of driving.
type result struct {
	seed       uint64
	seconds    float32
	distance   float32
	avgSpeed   float32 // mph
	points     int
	faults     int
	byKind     map[string]int
	detections float32
	blind      float32 // share of frames with no lane in view
}

func main() {
	var (
		seeds    = flag.Int("seeds", 12, "how many cities to drive")
		from     = flag.Uint64("from", 1, "first seed")
		seconds  = flag.Float64("seconds", 90, "simulated seconds per city")
		traffic  = flag.Int("traffic", 70, "ambient traffic cars")
		police   = flag.Int("police", 5, "patrolling police units")
		hz       = flag.Float64("perception", 20, "perception updates per second")
		width    = flag.Int("camwidth", 420, "camera width in pixels")
		height   = flag.Int("camheight", 236, "camera height in pixels")
		workers  = flag.Int("workers", runtime.NumCPU(), "cities to drive in parallel")
		failOver = flag.Int("max-points", -1, "exit non-zero if mean penalty exceeds this")
	)
	flag.Parse()

	cfg := runConfig{
		seconds: float32(*seconds), traffic: *traffic, police: *police,
		perception: float32(*hz), width: *width, height: *height,
	}

	results := drive(*seeds, *from, cfg, max(*workers, 1))
	mean := report(results)

	if *failOver >= 0 && mean > float64(*failOver) {
		fmt.Fprintf(os.Stderr,
			"\neval: mean penalty %.1f exceeds the %d allowed\n", mean, *failOver)
		os.Exit(1)
	}
}

// runConfig is what every city in a run shares.
type runConfig struct {
	seconds       float32
	traffic       int
	police        int
	perception    float32
	width, height int
}

func drive(seeds int, from uint64, cfg runConfig, workers int) []result {
	out := make([]result, seeds)
	var wg sync.WaitGroup
	work := make(chan int)

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				out[i] = driveOne(from+uint64(i), cfg)
			}
		}()
	}
	for i := range seeds {
		work <- i
	}
	close(work)
	wg.Wait()
	return out
}

func driveOne(seed uint64, cfg runConfig) result {
	w := sim.NewWorld(sim.Config{Seed: seed, Traffic: cfg.traffic, Police: cfg.police})
	cam := vision.Camera{
		Width: cfg.width, Height: cfg.height,
		FovY: 62, Mount: 1.35, Pitch: 0.10,
	}
	driver := autopilot.New(cam)
	scanner := vision.NewScanner()

	// One frame buffer for the whole run: the rasteriser clears and reuses it.
	frame := image.NewRGBA(image.Rect(0, 0, cfg.width, cfg.height))
	px := make([]color.RGBA, cfg.width*cfg.height)

	var (
		dets       []vision.Detection
		perceived  float32
		detSum     int
		detFrames  int
		speedSum   float32
		blindCount int
		steps      int
	)
	interval := 1 / max(cfg.perception, 1)

	for t := float32(0); t < cfg.seconds; t += fixedStep {
		steps++
		speedSum += w.Player.Speed()

		// Perception on its own clock, as in the game.
		if perceived += fixedStep; perceived >= interval || dets == nil {
			perceived = 0
			annotate.RasteriseInto(frame, annotate.Layout(w, annotate.BonnetView(cam, w.Player.Pos, w.Player.Yaw)))
			dets = scanner.Scan(asColors(frame, px), cfg.width, cfg.height)
			detSum += len(dets)
			detFrames++
		}

		cmd := driver.Drive(dets, w.Player.Speed(), fixedStep)
		if driver.State == autopilot.StateSearching {
			blindCount++
		}
		w.Update(sim.Controls{
			Throttle: cmd.Throttle, Brake: cmd.Brake, Steer: cmd.Steer,
		}, fixedStep)
	}

	r := result{
		seed: seed, seconds: w.Time, distance: w.Judge.Distance,
		points: w.Judge.Points, faults: w.Judge.Faults,
		byKind: map[string]int{},
	}
	if steps > 0 {
		r.avgSpeed = mathx.ToMPH(speedSum / float32(steps))
		r.blind = float32(blindCount) / float32(steps)
	}
	if detFrames > 0 {
		r.detections = float32(detSum) / float32(detFrames)
	}
	for _, in := range w.Judge.Log() {
		r.byKind[in.Kind.String()]++
	}
	return r
}

func asColors(img *image.RGBA, into []color.RGBA) []color.RGBA {
	// Reinterprets an RGBA image as the pixel slice the scanner reads,
	// filling a buffer the caller owns so a long run does not allocate per frame.
	for i := range into {
		o := i * 4
		into[i] = color.RGBA{
			R: img.Pix[o], G: img.Pix[o+1], B: img.Pix[o+2], A: img.Pix[o+3],
		}
	}
	return into
}

func report(results []result) float64 {
	sort.Slice(results, func(i, j int) bool { return results[i].seed < results[j].seed })

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "seed\tdistance\tavg mph\tdet/frame\tno lane\tpoints\tfaults")
	var totalPoints, totalFaults int
	var sumSpeed, sumDist, sumBlind float64
	clean := 0

	for _, r := range results {
		fmt.Fprintf(tw, "%d\t%.0f m\t%.0f\t%.1f\t%.0f%%\t%d\t%d\n",
			r.seed, r.distance, r.avgSpeed, r.detections, r.blind*100, r.points, r.faults)
		totalPoints += r.points
		totalFaults += r.faults
		sumSpeed += float64(r.avgSpeed)
		sumDist += float64(r.distance)
		sumBlind += float64(r.blind)
		if r.faults == 0 {
			clean++
		}
	}
	tw.Flush()

	n := float64(len(results))
	if n == 0 {
		return 0
	}
	meanPoints := float64(totalPoints) / n

	kinds := map[string]int{}
	for _, r := range results {
		for k, v := range r.byKind {
			kinds[k] += v
		}
	}
	names := make([]string, 0, len(kinds))
	for k := range kinds {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool { return kinds[names[i]] > kinds[names[j]] })

	fmt.Printf("\n%d cities, %.0f%% clean (%d of %d)\n",
		len(results), float64(clean)/n*100, clean, len(results))
	fmt.Printf("mean %.0f m at %.0f mph, %.1f penalty points, %.2f faults\n",
		sumDist/n, sumSpeed/n, meanPoints, float64(totalFaults)/n)
	fmt.Printf("lane lost on %.1f%% of frames\n", sumBlind/n*100)
	for _, k := range names {
		fmt.Printf("  %-16s %d\n", k, kinds[k])
	}
	if len(names) == 0 {
		fmt.Println("  no faults of any kind")
	}
	return meanPoints
}
