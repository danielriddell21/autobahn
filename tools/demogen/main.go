// Command demogen builds the project's documentation media: the GIFs and
// contact sheets in the README.
//
// It drives real sessions rather than staging anything. The clips are recorded
// from the same game loop the player runs, with the autopilot at the wheel, so
// what the media shows is what the code does.
//
// crucible's demo package supplies the machinery — [demo.Clip] drives a run and
// captures frames, [demo.Montage] tiles stills, [demo.Ramp] builds the GIF
// palette — while what each clip shows stays here.
//
// A GPU context is required because the scene is drawn by raylib, so on a
// headless machine run it under xvfb:
//
//	xvfb-run -a go run -tags x11 ./tools/demogen
package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/danielriddell21/crucible/demo"
	"github.com/danielriddell21/crucible/record"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/game"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// config is what the flags collect.
type config struct {
	out     string
	seed    uint64
	width   int
	height  int
	traffic int
	police  int
	scale   int
	fps     int
	only    []string
	list    bool
}

// The simulation runs at a fixed step so a clip is reproducible from its seed
// rather than depending on how fast the machine renders it.
const fixedStep float32 = 1.0 / 60

// item is one piece of media demogen knows how to build.
type item struct {
	name string
	file string
	desc string
	make func(cfg config) error
}

func items() []item {
	return []item{
		{"drive", "drive.gif", "the autopilot driving the city, chase camera", clipDrive},
		{"vision", "vision.gif", "the annotated camera feed the autopilot reads", clipVision},
		{"junction", "junction.gif", "the autopilot stopping at a red light", clipJunction},
		{"police", "police.gif", "a pursuit, earned by driving badly", clipPolice},
		{"city", "city.png", "four seeds from above, showing the layout variety", sheetCity},
		{"cameras", "cameras.png", "the three viewpoints on one scene", sheetCameras},
	}
}

func main() {
	cfg := config{}
	var only string

	flag.StringVar(&cfg.out, "out", filepath.Join("docs", "media"), "directory to write media into")
	flag.Uint64Var(&cfg.seed, "seed", 21, "city seed")
	flag.IntVar(&cfg.width, "width", 1280, "render width in pixels")
	flag.IntVar(&cfg.height, "height", 720, "render height in pixels")
	flag.IntVar(&cfg.traffic, "traffic", 70, "number of ambient traffic cars")
	flag.IntVar(&cfg.police, "police", 10, "number of patrolling police units")
	flag.IntVar(&cfg.scale, "scale", 3, "downscale factor for the captures")
	flag.IntVar(&cfg.fps, "fps", 20, "frames per second in the output")
	flag.StringVar(&only, "only", "", "comma-separated names to build; default builds all")
	flag.BoolVar(&cfg.list, "list", false, "list what can be built and exit")
	flag.Parse()

	if only != "" {
		cfg.only = strings.Split(only, ",")
	}
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "demogen:", err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	all := items()
	if cfg.list {
		for _, it := range all {
			fmt.Printf("  %-10s %-14s %s\n", it.name, it.file, it.desc)
		}
		return nil
	}
	if err := os.MkdirAll(cfg.out, 0o755); err != nil {
		return fmt.Errorf("creating the output directory: %w", err)
	}

	var built int
	for _, it := range all {
		if len(cfg.only) > 0 && !slices.Contains(cfg.only, it.name) {
			continue
		}
		fmt.Printf("building %-10s -> %s\n", it.name, filepath.Join(cfg.out, it.file))
		if err := it.make(cfg); err != nil {
			return fmt.Errorf("%s: %w", it.name, err)
		}
		built++
	}
	if built == 0 {
		return errors.New("nothing matched; try -list")
	}
	fmt.Printf("wrote %d file(s) to %s\n", built, cfg.out)
	return nil
}

func (c config) opts(seed uint64) game.Options {
	// Builds the session settings the clips share.
	o := game.DefaultOptions()
	o.Seed = seed
	o.Traffic = c.traffic
	o.Police = c.police
	o.Width, o.Height = c.width, c.height
	o.Autopilot = true
	o.ShowPanel = true
	o.Mute = true
	return o
}

func (c config) path(file string) string { return filepath.Join(c.out, file) }

func (c config) recorder(frames, scale int) *record.Recorder {
	// Builds a recorder with a palette ramped from the game's own colours,
	// which keeps both the city and the saturated detection boxes clean in a GIF.
	return record.NewRecorder(c.fps, scale, frames,
		record.WithPalette(demo.Ramp(game.PaletteBases(), 5)),
		record.WithFrameDiff(),
		record.WithFinalHold(120),
	)
}

func rolling(g *game.Game) bool {
	// Reports whether the car is properly under way, so a clip opens on
	// driving rather than on the standing start.
	return mathx.ToMPH(g.World().Player.Speed()) > 6
}

func clipDrive(cfg config) error {
	return game.WithWindow(cfg.opts(cfg.seed), func(g *game.Game) error {
		g.SetCameraMode(game.CameraChase)
		rec := cfg.recorder(150, cfg.scale)

		clip := demo.Clip{
			Frames:   150,
			Every:    3, // the sim runs at 60/s; capture 20/s
			MaxSteps: 6000,
			Step:     func(int) error { g.Step(fixedStep); return nil },
			Ready:    func(int) bool { return rolling(g) },
			Frame:    func(int) image.Image { return g.Snapshot() },
		}
		return save(clip, rec, cfg.path("drive.gif"))
	})
}

func clipVision(cfg config) error {
	return game.WithWindow(cfg.opts(cfg.seed), func(g *game.Game) error {
		g.SetCameraMode(game.CameraChase)
		// The feed is already small, so it is captured at full resolution.
		rec := cfg.recorder(140, 1)

		clip := demo.Clip{
			Frames:   140,
			Every:    3,
			MaxSteps: 6000,
			Step:     func(int) error { g.Step(fixedStep); return nil },
			Ready:    func(int) bool { return rolling(g) },
			Frame:    func(int) image.Image { return g.CameraImage() },
		}
		return save(clip, rec, cfg.path("vision.gif"))
	})
}

func clipJunction(cfg config) error {
	// Waits for the autopilot to actually be stopping for something
	// before it starts recording, which is what demo.Clip's Ready gate is for.
	return game.WithWindow(cfg.opts(cfg.seed), func(g *game.Game) error {
		g.SetCameraMode(game.CameraChase)
		rec := cfg.recorder(130, cfg.scale)

		var held int
		clip := demo.Clip{
			Frames:   130,
			Every:    3,
			MaxSteps: 12000,
			Step:     func(int) error { g.Step(fixedStep); return nil },
			Ready: func(int) bool {
				// Open once the car is slowing for a signal, not for traffic.
				d := g.Driver()
				return rolling(g) && d.State == autopilot.StateSlowing &&
					strings.Contains(d.Reason, "light")
			},
			Frame: func(int) image.Image { return g.Snapshot() },
			Stop: func(int) bool {
				// Carry on a little past the stop so the clip shows it holding.
				if g.Driver().State == autopilot.StateHalted {
					held++
				}
				return held > 40
			},
		}
		return save(clip, rec, cfg.path("junction.gif"))
	})
}

func clipPolice(cfg config) error {
	// Records a pursuit. Nothing about it is staged: the car is driven
	// by the reckless controller, which follows the road and ignores every rule, so
	// the offences are judged and the response summoned through the ordinary path.
	// Recording opens only once units are actually chasing.
	o := cfg.opts(cfg.seed)
	o.Autopilot = false
	o.Reckless = true
	o.Police = max(cfg.police, 10)
	o.Camera = game.CameraHigh

	return game.WithWindow(o, func(g *game.Game) error {
		// High enough to take in the units converging on the car.
		g.SetOverheadHeight(120)
		rec := cfg.recorder(140, cfg.scale)

		clip := demo.Clip{
			Frames:   140,
			Every:    3,
			MaxSteps: 9000,
			Step:     func(int) error { g.Step(fixedStep); return nil },
			Ready: func(int) bool {
				return g.World().Wanted.State == sim.PursuitActive
			},
			Frame: func(int) image.Image { return g.Snapshot() },
		}
		return save(clip, rec, cfg.path("police.gif"))
	})
}

func sheetCity(cfg config) error {
	// Renders the same overhead view of four different seeds, which shows
	// how much the generator varies between them.
	seeds := []uint64{cfg.seed, cfg.seed + 1, cfg.seed + 2, cfg.seed + 3}

	return game.WithWindow(cfg.opts(seeds[0]), func(first *game.Game) error {
		cells := make([]image.Image, 0, len(seeds))
		for i, seed := range seeds {
			g := first
			if i > 0 {
				// The window is already open, so further sessions just need a
				// game built in it.
				g = game.New(cfg.opts(seed))
				defer g.Close()
			}
			g.SetCameraMode(game.CameraHigh)
			g.ShowHUD(false)
			// High enough to take in a district rather than a single street.
			g.SetOverheadHeight(200)
			settle(g, 150)
			cells = append(cells, downscale(g.Snapshot(), cfg.scale))
		}
		sheet := demo.Montage(cells, 2, 12, color.RGBA{R: 16, G: 18, B: 22, A: 255})
		return writePNG(cfg.path("city.png"), sheet)
	})
}

func sheetCameras(cfg config) error {
	// Shows one scene from each viewpoint.
	return game.WithWindow(cfg.opts(cfg.seed), func(g *game.Game) error {
		settle(g, 220)

		modes := []game.CameraMode{game.CameraChase, game.CameraBonnet, game.CameraHigh}
		cells := make([]image.Image, 0, len(modes))
		for _, m := range modes {
			g.SetCameraMode(m)
			// One more step so the camera move is reflected in the frame.
			g.Step(fixedStep)
			cells = append(cells, downscale(g.Snapshot(), cfg.scale))
		}
		sheet := demo.Montage(cells, 3, 12, color.RGBA{R: 16, G: 18, B: 22, A: 255})
		return writePNG(cfg.path("cameras.png"), sheet)
	})
}

func settle(g *game.Game, steps int) {
	// Runs the simulation on for a while so traffic disperses and the car is
	// somewhere more interesting than the start line.
	for range steps {
		g.Step(fixedStep)
	}
}

func save(clip demo.Clip, rec *record.Recorder, path string) error {
	n, err := clip.Record(rec)
	if err != nil {
		return fmt.Errorf("recording: %w", err)
	}
	if n == 0 {
		return errors.New("captured no frames; the clip's Ready gate never opened")
	}
	if err := rec.Save(path); err != nil {
		return fmt.Errorf("saving: %w", err)
	}
	fmt.Printf("  %d frames\n", n)
	return nil
}

func writePNG(path string, img image.Image) error {
	if err := record.SavePNG(path, img); err != nil {
		return fmt.Errorf("writing the sheet: %w", err)
	}
	return nil
}

func downscale(src image.Image, factor int) image.Image {
	// Shrinks a frame by an integer factor with a box filter, so contact
	// sheet cells are a sensible size without depending on the render resolution.
	if src == nil {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	if factor < 2 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx()/factor, b.Dy()/factor
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			var r, g, bl, n uint32
			for dy := range factor {
				for dx := range factor {
					cr, cg, cb, _ := src.At(b.Min.X+x*factor+dx, b.Min.Y+y*factor+dy).RGBA()
					r, g, bl, n = r+cr>>8, g+cg>>8, bl+cb>>8, n+1
				}
			}
			dst.Set(x, y, color.RGBA{
				R: uint8(r / n), G: uint8(g / n), B: uint8(bl / n), A: 255,
			})
		}
	}
	return dst
}
