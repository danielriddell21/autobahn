// Package game wires the simulation, the renderer and the two driving modes
// together into a playable window.
//
// Part one is manual driving. Part two hands the same car to the autopilot,
// which sees the world only through the annotated camera feed rendered into an
// off-screen texture and scanned back a pixel at a time.
package game

import (
	"fmt"
	"image"
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"

	crucihud "github.com/danielriddell21/crucible/hud"
	"github.com/danielriddell21/crucible/record"
	"github.com/danielriddell21/crucible/status"
	"github.com/danielriddell21/crucible/telemetry"

	"github.com/danielriddell21/autobahn/internal/annotate"
	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/vision"
)

// CameraMode selects the player's viewpoint.
type CameraMode int

// The available viewpoints.
const (
	CameraChase CameraMode = iota
	CameraBonnet
	CameraHigh
	numCameraModes
)

// Options configures a game session.
type Options struct {
	Seed        uint64
	Traffic     int
	Width       int
	Height      int
	CamWidth    int // AI camera width in pixels
	CamHeight   int
	Autopilot   bool // start with the AI driving
	ShowPanel   bool // show the AI camera panel
	Frames      int  // when > 0, run this many frames then exit
	Screenshot  string
	Stats       bool    // print a session summary on exit
	Record      string  // capture the drive to this .gif or .mp4 path
	RecordScale int     // downscale factor for the capture
	DebugVision bool    // print camera range estimates against ground truth
	PerceptionH float32 // perception updates per second
}

// DefaultOptions returns the standard session settings.
func DefaultOptions() Options {
	return Options{
		Seed: 7, Traffic: 70, Width: 1280, Height: 720,
		CamWidth: 420, CamHeight: 236, ShowPanel: true, PerceptionH: 20,
	}
}

// Game holds every piece of live state for a session.
type Game struct {
	opts  Options
	world *sim.World

	shader    rl.Shader
	hasShader bool

	camMode  CameraMode
	cam      rl.Camera3D
	camPos   mathx.Vec
	aiCam    rl.Camera3D
	aiTarget rl.RenderTexture2D

	scanner   *vision.Scanner
	visionCam vision.Camera
	driver    *autopilot.Driver
	dets      []vision.Detection

	auto       bool
	showPanel  bool
	showLabels bool
	showHelp   bool
	paused     bool

	recorder  *record.Recorder
	notices   *crucihud.Overlay
	lastCmd   autopilot.Command
	perceived float32
	blink     bool
	blinkT    float32
	frame     int

	speedSum    float32
	detSum      int
	detFrames   int
	scanNanos   int64
	minLaneDist float32
}

// Summary reports how a driving session went. It is what the -stats flag
// prints, and it is the fair comparison between a human lap and an AI lap:
// both are produced by the same judge watching the same car.
type Summary struct {
	Mode          string
	Seconds       float32
	Distance      float32 // metres
	AvgSpeed      float32 // mph
	Points        int
	Faults        map[string]int
	AvgDetections float32
	Frames        int
	NearestLane   float32 // furthest lane marker the camera resolved, metres
	TotalFaults   int
}

// Summary returns the session statistics accumulated so far.
func (g *Game) Summary() Summary {
	s := Summary{
		Mode: "manual", Seconds: g.world.Time, Frames: g.frame,
		Distance: g.world.Judge.Distance, Points: g.world.Judge.Points,
		Faults: map[string]int{}, NearestLane: g.minLaneDist,
	}
	if g.auto {
		s.Mode = "autopilot"
	}
	if g.frame > 0 {
		s.AvgSpeed = mathx.ToMPH(g.speedSum / float32(g.frame))
	}
	if g.detFrames > 0 {
		s.AvgDetections = float32(g.detSum) / float32(g.detFrames)
	}
	for _, in := range g.world.Judge.Log() {
		s.Faults[in.Kind.String()]++
	}
	s.TotalFaults = g.world.Judge.Faults
	return s
}

// New creates a game with a freshly generated city. The window must already be
// open, because the AI camera allocates a render texture.
func New(opts Options) *Game {
	g := &Game{
		opts:  opts,
		world: sim.NewWorld(sim.Config{Seed: opts.Seed, Traffic: opts.Traffic}),
		auto:  opts.Autopilot, showPanel: opts.ShowPanel, showLabels: true,
		scanner:   vision.NewScanner(),
		visionCam: vision.DefaultCamera(opts.CamWidth, opts.CamHeight),
	}
	if opts.Record != "" {
		// crucible's recorder takes plain image.Image frames, so it works
		// behind raylib just as well as behind Ebiten.
		frames := opts.Frames
		if frames <= 0 || frames > 1800 {
			frames = 1800
		}
		g.recorder = record.NewRecorder(30, max(opts.RecordScale, 1), frames,
			record.WithFrameDiff())
	}
	g.notices = crucihud.New()
	g.watchInfractions()
	g.driver = autopilot.New(g.visionCam)
	g.aiTarget = rl.LoadRenderTexture(int32(opts.CamWidth), int32(opts.CamHeight))

	g.shader = rl.LoadShaderFromMemory(lightingVS, lightingFS)
	g.hasShader = g.shader.ID != 0

	g.cam = rl.Camera3D{Up: vec3(0, 1, 0), Fovy: 62, Projection: rl.CameraPerspective}
	g.aiCam = rl.Camera3D{Up: vec3(0, 1, 0),
		Fovy: g.visionCam.FovY, Projection: rl.CameraPerspective}
	g.camPos = g.world.Player.Pos
	g.updateCameras(0)
	return g
}

// watchInfractions turns judge events into HUD lines. The judge publishes on a
// crucible telemetry bus, a status source decides the wording, and the line is
// posted to the overlay: the game supplies the vocabulary, crucible the plumbing.
func (g *Game) watchInfractions() {
	const noticeFrames = 150

	source := status.Func[sim.Infraction](func(in sim.Infraction, emit func(status.Line)) {
		text := in.Kind.String()
		if in.Detail != "" {
			text += " - " + in.Detail
		}
		emit(status.Line{
			Text:    fmt.Sprintf("%s  (+%d)", text, in.Kind.Penalty()),
			Channel: crucihud.Notice,
			Frames:  noticeFrames,
		})
	})
	post := status.Emit(g.notices)
	g.world.Judge.Events.Subscribe(telemetry.SubscriberFunc[sim.Infraction](
		func(in sim.Infraction) {
			source.Request(in, post)
			if g.opts.DebugVision {
				fmt.Printf("FAULT t=%5.1fs %-16s %s | speed %.1f mph, autopilot %v: %s (target %.1f mph)\n",
					in.At, in.Kind, in.Detail, mathx.ToMPH(g.world.Player.Speed()),
					g.driver.State, g.driver.Reason, mathx.ToMPH(g.driver.TargetSpeed))
			}
		},
	))
}

// Close releases the GPU resources the game owns.
func (g *Game) Close() {
	rl.UnloadRenderTexture(g.aiTarget)
	if g.hasShader {
		rl.UnloadShader(g.shader)
	}
}

// World exposes the simulation, for tests and tools.
func (g *Game) World() *sim.World { return g.world }

// Detections returns the most recent frame of camera detections.
func (g *Game) Detections() []vision.Detection { return g.dets }

// Autopilot reports whether the AI is currently driving.
func (g *Game) Autopilot() bool { return g.auto }

// SetAutopilot switches between manual and AI driving.
func (g *Game) SetAutopilot(on bool) {
	if on == g.auto {
		return
	}
	g.auto = on
	g.driver.Reset()
	g.world.Judge.Reset()
}

// Step advances the game by one frame: perception, control, simulation, then
// rendering. dt is the frame duration in seconds.
func (g *Game) Step(dt float32) {
	g.frame++
	g.speedSum += g.world.Player.Speed()
	g.handleInput()

	if g.blinkT += dt; g.blinkT > 0.36 {
		g.blink, g.blinkT = !g.blink, 0
	}
	g.notices.Tick()

	// Perception runs on its own clock, as a real sensor stack would. The
	// camera image must be produced before the controller reads it.
	wantVision := g.auto || g.showPanel
	if wantVision {
		g.perceived += dt
		interval := 1 / max(g.opts.PerceptionH, 1)
		if g.perceived >= interval || g.dets == nil {
			g.perceived = 0
			g.renderAICamera()
			g.scanAICamera()
			g.detSum += len(g.dets)
			g.detFrames++
		}
	}

	controls := g.manualControls()
	if g.auto {
		cmd := g.driver.Drive(g.dets, g.world.Player.Speed(), dt)
		g.lastCmd = cmd
		controls = sim.Controls{
			Throttle: cmd.Throttle, Brake: cmd.Brake, Steer: cmd.Steer,
		}
	}

	if !g.paused {
		g.world.Update(controls, dt)
	}
	g.updateCameras(dt)

	rl.BeginDrawing()
	rl.ClearBackground(colSky)
	g.drawWorld(g.cam, g.camMode != CameraBonnet)
	g.drawHUD()
	rl.EndDrawing()
	g.capture()
}

// capture feeds the finished frame to the recorder, when one is running.
func (g *Game) capture() {
	if g.recorder == nil || g.recorder.Done() {
		return
	}
	shot := rl.LoadImageFromScreen()
	if shot == nil {
		return
	}
	px := rl.LoadImageColors(shot)
	img := image.NewRGBA(image.Rect(0, 0, int(shot.Width), int(shot.Height)))
	for i, c := range px[:min(len(px), int(shot.Width)*int(shot.Height))] {
		img.Set(i%int(shot.Width), i/int(shot.Width),
			color.RGBA{R: c.R, G: c.G, B: c.B, A: 255})
	}
	rl.UnloadImageColors(px)
	rl.UnloadImage(shot)
	g.recorder.Add(img)
}

// SaveRecording writes the captured drive, if recording was enabled.
func (g *Game) SaveRecording(path string) error {
	if g.recorder == nil || g.recorder.Len() == 0 {
		return nil
	}
	return g.recorder.Save(path)
}

func (g *Game) handleInput() {
	switch {
	case rl.IsKeyPressed(rl.KeyTab):
		g.SetAutopilot(!g.auto)
	case rl.IsKeyPressed(rl.KeyC):
		g.camMode = (g.camMode + 1) % numCameraModes
	case rl.IsKeyPressed(rl.KeyV):
		g.showPanel = !g.showPanel
	case rl.IsKeyPressed(rl.KeyL):
		g.showLabels = !g.showLabels
	case rl.IsKeyPressed(rl.KeyR):
		g.world.Respawn()
	case rl.IsKeyPressed(rl.KeyN):
		g.world.PlaceOnNearestLane()
	case rl.IsKeyPressed(rl.KeyH):
		g.showHelp = !g.showHelp
	case rl.IsKeyPressed(rl.KeyP):
		g.paused = !g.paused
	}
}

func (g *Game) manualControls() sim.Controls {
	var c sim.Controls
	if rl.IsKeyDown(rl.KeyW) || rl.IsKeyDown(rl.KeyUp) {
		c.Throttle = 1
	}
	if rl.IsKeyDown(rl.KeyA) || rl.IsKeyDown(rl.KeyLeft) {
		c.Steer = -1
	}
	if rl.IsKeyDown(rl.KeyD) || rl.IsKeyDown(rl.KeyRight) {
		c.Steer = 1
	}
	if rl.IsKeyDown(rl.KeyS) || rl.IsKeyDown(rl.KeyDown) {
		// Brake first; once stopped, the same key selects reverse.
		if g.world.Player.ForwardSpeed() < 0.6 {
			c.Reverse, c.Throttle = true, 1
		} else {
			c.Brake = 1
		}
	}
	c.Handbrake = rl.IsKeyDown(rl.KeySpace)
	return c
}

func (g *Game) updateCameras(dt float32) {
	p := g.world.Player
	fwd := p.Forward()

	// The AI camera is rigidly mounted, matching the calibration the autopilot
	// was built with. It never smooths, because a real sensor does not.
	mount := p.Pos.Add(fwd.Mul(1.7))
	eyeY := g.visionCam.Mount
	g.aiCam.Position = vec3(mount.X, eyeY, mount.Z)
	look := mount.Add(fwd.Mul(24))
	g.aiCam.Position.Y = eyeY
	g.aiCam.Target = vec3(look.X, eyeY-24*mathx.Tan(g.visionCam.Pitch), look.Z)

	switch g.camMode {
	case CameraBonnet:
		g.cam.Position = vec3(mount.X, 1.45, mount.Z)
		g.cam.Target = vec3(look.X, 1.25, look.Z)
	case CameraHigh:
		g.cam.Position = vec3(p.Pos.X, 34, p.Pos.Z+0.1)
		g.cam.Target = vec3(p.Pos.X, 0, p.Pos.Z)
	default:
		// Chase camera: trails the car, and swings wider as speed rises.
		back := 9.2 + min(p.Speed()*0.14, 3.6)
		want := p.Pos.Sub(fwd.Mul(back))
		rate := float32(7)
		if dt <= 0 {
			g.camPos = want
		} else {
			g.camPos = mathx.V(
				mathx.Approach(g.camPos.X, want.X, rate, dt),
				mathx.Approach(g.camPos.Z, want.Z, rate, dt),
			)
		}
		g.cam.Position = vec3(g.camPos.X, 4.3, g.camPos.Z)
		ahead := p.Pos.Add(fwd.Mul(9))
		g.cam.Target = vec3(ahead.X, 1.0, ahead.Z)
	}
}

// renderAICamera draws the scene from the bonnet camera into the off-screen
// target and paints the detection boxes over it.
func (g *Game) renderAICamera() {
	rl.BeginTextureMode(g.aiTarget)
	rl.ClearBackground(colSky)
	g.drawWorld(g.aiCam, false)
	// The boxes are drawn outside the lighting shader so their colours land in
	// the framebuffer exactly as specified. The scanner matches them exactly.
	annotate.Annotate(g.world, g.aiCam,
		int32(g.opts.CamWidth), int32(g.opts.CamHeight), g.showLabels)
	rl.EndTextureMode()
}

// scanAICamera reads the camera image back and recovers the detections. This
// is the only path by which world state reaches the autopilot.
func (g *Game) scanAICamera() {
	img := rl.LoadImageFromTexture(g.aiTarget.Texture)
	if img == nil {
		return
	}
	// A render target is stored bottom-up, so flip it to match what the camera
	// actually saw before reading any pixel coordinates from it.
	rl.ImageFlipVertical(img)
	px := rl.LoadImageColors(img)
	g.dets = g.scanner.Scan(px, g.opts.CamWidth, g.opts.CamHeight)
	if g.opts.DebugVision {
		g.checkPerception()
	}
	rl.UnloadImageColors(px)
	rl.UnloadImage(img)

	// Track the furthest lane marker resolved, which is a direct measure of how
	// far ahead the perception stack can actually see.
	for _, d := range g.dets {
		if !vision.IsLaneMarker(d.Class) {
			continue
		}
		if dist := g.visionCam.GroundDistance(d.MaxY); dist > g.minLaneDist && dist < 400 {
			g.minLaneDist = dist
		}
	}
}

// Run opens a window and drives the game loop until the user quits.
func Run(opts Options) error {
	rl.SetTraceLogLevel(rl.LogWarning)
	rl.SetConfigFlags(rl.FlagMsaa4xHint)
	rl.InitWindow(int32(opts.Width), int32(opts.Height), "Autobahn")
	defer rl.CloseWindow()
	rl.SetTargetFPS(60)

	g := New(opts)
	defer g.Close()

	for !rl.WindowShouldClose() {
		dt := rl.GetFrameTime()
		// Clamp the step so a stall cannot tunnel the car through a wall.
		g.Step(mathx.Clamp(dt, 0.001, 1.0/20))

		if opts.Frames > 0 && g.frame >= opts.Frames {
			break
		}
	}

	if opts.Screenshot != "" {
		rl.TakeScreenshot(opts.Screenshot)
		fmt.Println("wrote", opts.Screenshot)
	}
	if opts.Record != "" {
		if err := g.SaveRecording(opts.Record); err != nil {
			return fmt.Errorf("saving the recording: %w", err)
		}
		fmt.Println("wrote", opts.Record)
	}
	if opts.Stats {
		printSummary(g.Summary())
	}
	return nil
}

func printSummary(s Summary) {
	fmt.Printf("\n--- %s session ---\n", s.Mode)
	fmt.Printf("  drove      %.0f m in %.0f s (avg %.0f mph over %d frames)\n",
		s.Distance, s.Seconds, s.AvgSpeed, s.Frames)
	fmt.Printf("  perception %.1f detections/frame, saw lane markers to %.0f m\n",
		s.AvgDetections, s.NearestLane)
	fmt.Printf("  penalty    %d points from %d faults\n", s.Points, s.TotalFaults)
	for k, n := range s.Faults {
		fmt.Printf("               %-16s x%d\n", k, n)
	}
}

// checkPerception compares the range the camera model infers for the nearest
// vehicle against the simulation's true distance. It exists to validate the
// camera calibration during development; the autopilot never sees either
// number, and the check runs only under the -debugvision flag.
func (g *Game) checkPerception() {
	if g.frame%60 != 0 {
		return
	}
	p := g.world.Player
	eye := p.Pos.Add(p.Forward().Mul(1.7))
	fwd := p.Forward()

	truth := float32(-1)
	for _, a := range g.world.Agents {
		rel := a.V.Pos.Sub(eye)
		along := rel.Dot(fwd)
		if along <= 0 || mathx.Abs(rel.Dot(fwd.Right())) > 3 {
			continue
		}
		if truth < 0 || along < truth {
			truth = along
		}
	}

	est := float32(-1)
	for _, d := range g.dets {
		if d.Class != vision.ClassVehicle {
			continue
		}
		if v := g.visionCam.GroundDistance(d.MaxY); v > 0 && (est < 0 || v < est) {
			est = v
		}
	}
	fmt.Printf("perception t=%5.1fs  nearest vehicle: estimated %6.1fm  actual %6.1fm\n",
		g.world.Time, est, truth)
}
