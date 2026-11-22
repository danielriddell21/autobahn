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
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/danielriddell21/autobahn/internal/annotate"
	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/scenario"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/vision"
	"github.com/danielriddell21/autobahn/internal/weather"
	"github.com/danielriddell21/autobahn/internal/works"
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
	// sites is how many stretches of roadworks the car came up to and went
	// past, which is how a gate with roadworks in it is known to have met any.
	sites int
}

func main() {
	var (
		seeds    = flag.Int("seeds", 12, "how many cities to drive")
		from     = flag.Uint64("from", 1, "first seed")
		seconds  = flag.Float64("seconds", 90, "simulated seconds per city")
		traffic  = flag.Int("traffic", 70, "ambient traffic cars")
		police   = flag.Int("police", 7, "patrolling police units")
		hz       = flag.Float64("perception", 20, "perception updates per second")
		width    = flag.Int("camwidth", 420, "camera width in pixels")
		height   = flag.Int("camheight", 236, "camera height in pixels")
		workers  = flag.Int("workers", runtime.NumCPU(), "cities to drive in parallel")
		modeName = flag.String("mode", "normal", "autopilot style: cautious, normal, brisk, aggressive or getaway")
		verbose  = flag.Bool("verbose", false, "report the circumstances of every fault")
		failOver = flag.Int("max-points", -1, "exit non-zero if mean penalty exceeds this")
		mapName  = flag.String("map", "city",
			"the map to drive: a name, all for every one in turn, custom:edge for every custom range at its "+
				"hardest bound, or a comma-separated list of them")
		customArg  = flag.String("custom", "", "drive the custom map with this share code, from its own seed")
		worksArg   = flag.String("works", "normal", "how much of the town is dug up: off, normal or dense")
		weatherArg = flag.String("weather", "clear",
			"the weather to drive in: clear, rain, fog, storm or drift, or a comma-separated list of them")
		ghostArg = flag.String("ghost", "",
			"drive every challenge with the autopilot and write its scores to this file (the checked-in ghosts.json), then exit")
		minSites = flag.Int("min-sites", -1,
			"exit non-zero if fewer roadworks than this were passed over the whole run; "+
				"with -works dense and -max-points set it is 24 unless given")
	)
	ambulances := flag.Int("ambulances", 0, "ambulances going about the town, now and then out on a call")
	flag.Parse()

	if *ghostArg != "" {
		if err := writeGhosts(*ghostArg, ghostsFor(scenario.All(), max(*workers, 1))); err != nil {
			fmt.Fprintln(os.Stderr, "eval:", err)
			os.Exit(1)
		}
		return
	}

	maps, err := targetsNamed(*mapName, *customArg, *from)
	if err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(2)
	}

	mode, ok := autopilot.ParseMode(*modeName)
	if !ok {
		fmt.Fprintf(os.Stderr, "eval: unknown mode %q\n", *modeName)
		os.Exit(2)
	}
	level, ok := works.ParseLevel(*worksArg)
	if !ok {
		fmt.Fprintf(os.Stderr, "eval: unknown works level %q\n", *worksArg)
		os.Exit(2)
	}
	if *minSites < 0 && level == works.Dense && *failOver >= 0 {
		*minSites = defaultMinSites
	}
	skies, err := weathersNamed(*weatherArg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(2)
	}
	cfg := runConfig{
		mode: mode, seconds: float32(*seconds), traffic: *traffic, police: *police, ambulances: *ambulances,
		perception: float32(*hz), width: *width, height: *height,
		verbose: *verbose, works: level,
	}

	if !scoreIn(skies, maps, cfg, *seeds, *from, max(*workers, 1), *failOver, *minSites) {
		os.Exit(1)
	}
}

// weathersNamed reads -weather: one setting or a comma-separated list.
func weathersNamed(arg string) ([]weather.Setting, error) {
	var out []weather.Setting
	for _, name := range strings.Split(arg, ",") {
		set, ok := weather.ParseSetting(name)
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("unknown weather %q", name)
		}
		out = append(out, set)
	}
	return out, nil
}

// scoreIn drives every map in each of the weathers, and reports whether every
// one passed. Every weather is driven before any verdict.
func scoreIn(skies []weather.Setting, maps []target, cfg runConfig, seeds int, from uint64, workers, failOver, minSites int) bool {
	ok := true
	for _, set := range skies {
		cfg.weather = set
		if len(skies) > 1 {
			fmt.Printf("\n######## weather: %s ########\n", set)
		}
		if !score(maps, cfg, seeds, from, workers, failOver, minSites) {
			ok = false
		}
	}
	return ok
}

// score drives every map and reports whether the run passed its marks.
//
// Every map is held to the same mark, and every one is driven before any
// verdict, so a run that fails says everything that failed.
func score(maps []target, cfg runConfig, seeds int, from uint64, workers, failOver, minSites int) bool {
	ok := true
	passed := 0
	for _, m := range maps {
		cfg.world, cfg.spec, cfg.seedShift = m.world, m.spec, m.shift
		if len(maps) > 1 {
			fmt.Printf("\n== %s, in %s ==\n", m.name, m.season.Name())
		}
		results := drive(seeds, from, cfg, workers)
		for _, r := range results {
			passed += r.sites
		}
		mean := report(results)
		if failOver >= 0 && mean > float64(failOver) {
			fmt.Fprintf(os.Stderr,
				"\neval: %s: mean penalty %.1f exceeds the %d allowed\n", m.name, mean, failOver)
			ok = false
		}
	}
	if cfg.works != works.Off {
		fmt.Printf("\nroadworks passed: %d sites\n", passed)
	}
	if minSites >= 0 && passed < minSites {
		fmt.Fprintf(os.Stderr,
			"\neval: only %d roadworks sites were passed, and the gate wants %d: it is not testing them\n",
			passed, minSites)
		ok = false
	}
	return ok
}

// target is one thing to drive: a preset map, or a custom one.
type target struct {
	name   string
	season city.Season
	world  city.Map
	spec   *custom.Spec
	// shift is added to a run's seed to make the custom map's own.
	shift uint64
}

// edgeName is the profile the gate drives to hold the custom ranges to what the
// autopilot copes with: every slider at its hardest bound.
const edgeName = "custom:edge"

// targetsNamed reads -map and -custom. -map is a comma-separated list of preset
// maps by name, all of them, or custom:edge; -custom is a share code, driven
// from the code's own seed for as many seeds as -seeds asks for.
func targetsNamed(mapArg, code string, from uint64) ([]target, error) {
	var out []target
	if code != "" {
		spec, err := custom.Decode(code)
		if err != nil {
			return nil, err
		}
		out = append(out, target{
			name: "custom " + code, season: spec.Season(), spec: &spec, shift: spec.Seed - from,
		})
		if mapArg == "city" {
			return out, nil
		}
	}
	for _, name := range strings.Split(mapArg, ",") {
		name = strings.TrimSpace(name)
		if name == edgeName {
			spec := custom.Edge(city.MapCity, 0)
			out = append(out, target{name: edgeName, season: spec.Season(), spec: &spec})
			continue
		}
		ms, err := mapsNamed(name)
		if err != nil {
			return nil, err
		}
		for _, m := range ms {
			out = append(out, target{name: m.Name(), season: m.Season(), world: m})
		}
	}
	return out, nil
}

// mapsNamed reads a preset map by name, or all of them.
func mapsNamed(name string) ([]city.Map, error) {
	if name == "all" {
		return city.Maps(), nil
	}
	m, ok := city.ParseMap(name)
	if !ok {
		return nil, fmt.Errorf("unknown map %q", name)
	}
	return []city.Map{m}, nil
}

// runConfig is what every city in a run shares.
type runConfig struct {
	world city.Map
	// spec, if set, is a custom map driven in place of world. Each city is the
	// spec with its seed taken from the run's, plus seedShift.
	spec          *custom.Spec
	seedShift     uint64
	mode          autopilot.Mode
	seconds       float32
	traffic       int
	police        int
	ambulances    int
	perception    float32
	width, height int
	verbose       bool
	works         works.Level
	weather       weather.Setting
}

// defaultMinSites is how many stretches of roadworks a dense gate has to have
// driven past, over every map, to count as having tested them at all.
const defaultMinSites = 24

// Roadworks counted as passed: the car came within nearSite of a cone or a
// barrier that was standing, and went on until it was farSite further off than
// it had been at its nearest.
const (
	nearSite float32 = 30
	farSite  float32 = 25
)

// siteCounter counts the roadworks a car passes.
type siteCounter struct {
	nearest map[works.Edge]float32
	passed  map[works.Edge]bool
}

func newSiteCounter() *siteCounter {
	return &siteCounter{nearest: map[works.Edge]float32{}, passed: map[works.Edge]bool{}}
}

// look takes note of where the player is against everything standing.
func (c *siteCounter) look(w *sim.World) {
	at := w.Player.Pos
	for _, cl := range w.Works.Active() {
		if cl.Phase != works.PhaseSolid || c.passed[cl.Edge] {
			continue
		}
		nearest := float32(1e9)
		for _, o := range cl.Cones {
			nearest = min(nearest, o.Center.DistTo(at))
		}
		for _, o := range cl.Barriers {
			nearest = min(nearest, o.Center.DistTo(at))
		}
		best, seen := c.nearest[cl.Edge]
		switch {
		case nearest < nearSite && (!seen || nearest < best):
			c.nearest[cl.Edge] = nearest
		case seen && nearest > best+farSite:
			c.passed[cl.Edge] = true
		}
	}
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

// session is one car being driven by the autopilot through the whole
// perception loop, a fixed step at a time.
type session struct {
	w         *sim.World
	driver    *autopilot.Driver
	cam       vision.Camera
	scanner   *vision.Scanner
	mirrors   [vision.NumMirrors]vision.Camera
	eyes      [vision.NumMirrors]*annotate.MirrorEye
	frame     *image.RGBA
	px        []color.RGBA
	dets      []vision.Detection
	perceived float32
	interval  float32
	// looks counts perception frames, and seen the detections over them.
	looks, seen int
}

func newSession(w *sim.World, cfg runConfig) *session {
	cam := vision.Camera{
		Width: cfg.width, Height: cfg.height,
		FovY: 62, Mount: 1.35, Pitch: 0.10,
	}
	s := &session{
		w: w, driver: autopilot.New(cam), cam: cam, scanner: vision.NewScanner(),
		// One frame buffer for the whole run: the rasteriser clears and reuses it.
		frame:    image.NewRGBA(image.Rect(0, 0, cfg.width, cfg.height)),
		px:       make([]color.RGBA, cfg.width*cfg.height),
		interval: 1 / max(cfg.perception, 1),
	}
	s.driver.Mode = cfg.mode
	s.mirrors = s.driver.Mirrors()
	for m := range s.eyes {
		s.eyes[m] = annotate.NewMirrorEye(s.mirrors[m].Width, s.mirrors[m].Height)
	}
	return s
}

// step runs one fixed step: a look through the cameras if one is due, the
// autopilot's decision, and the world moving on.
func (s *session) step() {
	w := s.w
	// Perception on its own clock, as in the game.
	if s.perceived += fixedStep; s.perceived >= s.interval || s.dets == nil {
		s.perceived = 0
		car := w.City.StandAt(w.Player.Pos, w.Player.Yaw)
		annotate.RasteriseInto(s.frame, annotate.LayoutIn(w, annotate.BonnetView(s.cam, car), w.Weather()))
		s.dets = s.scanner.Scan(annotate.Pixels(s.frame, s.px), s.cam.Width, s.cam.Height)
		for m, eye := range s.eyes {
			s.driver.Watch(vision.Mirror(m), eye.Look(w, s.mirrors[m], car, w.Weather()))
		}
		s.looks++
		s.seen += len(s.dets)
	}
	cmd := s.driver.Drive(s.dets, w.Player.Speed(), fixedStep)
	w.Update(sim.Controls{Throttle: cmd.Throttle, Brake: cmd.Brake, Steer: cmd.Steer}, fixedStep)
}

func driveOne(seed uint64, cfg runConfig) result {
	wc := sim.Config{
		Seed: seed, Map: cfg.world, Traffic: cfg.traffic, Police: cfg.police, Ambulances: cfg.ambulances, Works: cfg.works, Weather: cfg.weather,
	}
	if cfg.spec != nil {
		// A custom map brings its own sky, as the player's does.
		spec := *cfg.spec
		spec.Seed = seed + cfg.seedShift
		wc = spec.Apply(wc)
	}
	w := sim.NewWorld(wc)
	s := newSession(w, cfg)
	sites := newSiteCounter()
	driver := s.driver

	if cfg.verbose {
		// The judge already publishes every fault; listening tells us what the
		// controller believed at the moment it broke a rule, which is the only
		// way to tell a perception failure from a control one.
		w.Judge.Events.Subscribe(faultWatcher(func(in sim.Infraction) {
			fmt.Printf("seed %-3d t=%6.1fs  %-16s %-34s speed %4.1f target %4.1f  %v: %s\n",
				seed, in.At, in.Kind, in.Detail,
				mathx.ToMPH(w.Player.Speed()), mathx.ToMPH(driver.TargetSpeed),
				driver.State, driver.Reason)
		}))
	}

	var (
		speedSum   float32
		blindCount int
		steps      int
	)
	for t := float32(0); t < cfg.seconds; t += fixedStep {
		steps++
		speedSum += w.Player.Speed()
		s.step()
		if steps%10 == 0 {
			sites.look(w)
		}
		if driver.State == autopilot.StateSearching {
			blindCount++
		}
	}

	r := result{
		seed: seed, seconds: w.Time, distance: w.Judge.Distance,
		points: w.Judge.Points, faults: w.Judge.Faults,
		byKind: map[string]int{}, sites: len(sites.passed),
	}
	if steps > 0 {
		r.avgSpeed = mathx.ToMPH(speedSum / float32(steps))
		r.blind = float32(blindCount) / float32(steps)
	}
	if s.looks > 0 {
		r.detections = float32(s.seen) / float32(s.looks)
	}
	for _, in := range w.Judge.Log() {
		r.byKind[in.Kind.String()]++
	}
	return r
}

// faultWatcher adapts a function to the telemetry subscriber interface.
type faultWatcher func(sim.Infraction)

// OnEvent implements the telemetry subscriber interface.
func (f faultWatcher) OnEvent(in sim.Infraction) { f(in) }

func report(results []result) float64 {
	sort.Slice(results, func(i, j int) bool { return results[i].seed < results[j].seed })

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "seed\tdistance\tavg mph\tdet/frame\tno lane\tpoints\tfaults\tsites passed")
	var totalPoints, totalFaults int
	var sumSpeed, sumDist, sumBlind float64
	clean := 0

	for _, r := range results {
		_, _ = fmt.Fprintf(tw, "%d\t%.0f m\t%.0f\t%.1f\t%.0f%%\t%d\t%d\t%d\n",
			r.seed, r.distance, r.avgSpeed, r.detections, r.blind*100, r.points, r.faults, r.sites)
		totalPoints += r.points
		totalFaults += r.faults
		sumSpeed += float64(r.avgSpeed)
		sumDist += float64(r.distance)
		sumBlind += float64(r.blind)
		if r.faults == 0 {
			clean++
		}
	}
	_ = tw.Flush()

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
