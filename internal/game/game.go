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
	"math"
	"os"
	"path/filepath"

	rl "github.com/gen2brain/raylib-go/raylib"

	crucihud "github.com/danielriddell21/crucible/hud"
	"github.com/danielriddell21/crucible/netplay"
	"github.com/danielriddell21/crucible/record"
	"github.com/danielriddell21/crucible/status"
	"github.com/danielriddell21/crucible/telemetry"

	"github.com/danielriddell21/autobahn/internal/annotate"
	"github.com/danielriddell21/autobahn/internal/audio"
	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/emergency"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/race"
	"github.com/danielriddell21/autobahn/internal/scenario"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/vision"
	"github.com/danielriddell21/autobahn/internal/weather"
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

// String returns the viewpoint's name, for a settings row to show.
func (m CameraMode) String() string {
	switch m {
	case CameraBonnet:
		return "bonnet"
	case CameraHigh:
		return "overhead"
	default:
		return "chase"
	}
}

// Options configures a game session.
type Options struct {
	Seed uint64
	// Map is which place to drive. The zero value is the city.
	Map city.Map
	// Custom, when set, is the custom map to drive: it replaces Map and Seed and
	// shapes the world (see custom.Spec.Apply). Nothing driven on it is banked.
	Custom *custom.Spec
	// Challenge, when set, is the scenario being driven against the autopilot's
	// ghost. It fixes the world and the rules; see challenge.go.
	Challenge *scenario.Scenario
	// UnlockAll offers every pack whatever the player has banked.
	UnlockAll bool
	// Access is what the player holds, which decides the jobs J offers. The zero
	// value holds only the base game; UnlockAll is added to it.
	Access    pack.Access
	Traffic   int
	Police    int // marked units patrolling; zero disables the police
	Width     int
	Height    int
	CamWidth  int // AI camera width in pixels
	CamHeight int
	Autopilot bool // start with the AI driving
	// Reckless drives the car along its route at full throttle, ignoring every
	// rule. It is a development aid for exercising the judge and the police.
	Reckless bool
	// AutopilotJobs lets a job go on while the autopilot drives. It then pays
	// nothing; without it, turning the autopilot on cancels the job.
	AutopilotJobs bool
	// Weather is how the sky is chosen. The zero value is clear; a player's
	// stored setting, which drifts by default, replaces it unless -weather
	// was given.
	Weather weather.Setting
	// ShowPanel draws the AI camera panel while the autopilot is driving. It
	// never appears in manual driving: it is the autopilot's view of the road,
	// and the camera is not even run when nothing is reading it.
	ShowPanel bool
	Mute      bool // synthesise no sound at all
	// Chase puts the player in a police car with the autopilot running from
	// them, on this machine alone. It needs no network: there is only one
	// human, and the other driver is the AI.
	Chase bool
	// Host makes this machine the authority for a two-player chase against
	// another person, listening on the given address. Join connects to one.
	Host string
	Join string
	// Networked says the game has another player in it, however the session was
	// started: the command line sets Host or Join, and the lobby sets this.
	Networked bool
	// Play is what the two players of a hosted game are doing: a chase, or one
	// of the co-op games. A joiner takes it from the host, so only the host's
	// is read. It means nothing without a host.
	Play   NetMode
	Camera CameraMode
	// Mode is how the autopilot drives.
	Mode autopilot.Mode
	// Time is the time of day a session starts at, and DayLength how many
	// real minutes a day then lasts; 0 holds it there.
	Time      TimeOfDay
	DayLength float32
	// Car is the shape of the player's car, and Paint the index of its colour
	// among the garage's paints.
	Car        sim.BodyKind
	Paint      int
	Frames     int // when > 0, run this many frames then exit
	Screenshot string
	Stats      bool // print a session summary on exit
	// Rec is where and how to capture the drive. The family's shared --record
	// flags fill it; see [record.Options.AddStdFlags].
	Rec         record.Options
	DebugVision bool    // print camera range estimates against ground truth
	PerceptionH float32 // perception updates per second
	// Given names the options the command line asked for by name, so a stored
	// preference can yield to them. Only the flag package can tell a value that
	// was given from one that merely defaulted, so whoever parses records it.
	Given Given
}

// Given is the set of option names a command line supplied. Build it from
// [flag.Visit], which reports only the flags that were actually set:
//
//	opts.Given = game.Given{}
//	flag.Visit(func(f *flag.Flag) { opts.Given[f.Name] = true })
type Given map[string]bool

// DefaultOptions returns the standard session settings.
func DefaultOptions() Options {
	return Options{
		Seed: 7, Traffic: 70, Police: 7, Width: 1280, Height: 720,
		CamWidth: 420, CamHeight: 236, ShowPanel: true, PerceptionH: 20,
		Car: sim.HotHatch,
		// A 1280x720 scene needs downscaling to make a sensible GIF, and half
		// a minute at sixty frames a second is long enough for any clip.
		Rec: record.Options{FPS: 30, Scale: 3, Frames: 1800},
	}
}

// Game holds every piece of live state for a session.
type Game struct {
	opts  Options
	world *sim.World

	// scene is the lighting shader, light what it is currently lighting the
	// city with, and clock the hour that light belongs to.
	scene sceneShader
	light lighting
	// look is how the map's season dresses the scene, and town how its
	// buildings are finished.
	look  seasonLook
	town  townLook
	clock dayClock
	// outlines caches each block's kerb line, and trees what has been planted;
	// treedBlocks and treedRoads say how much of the city that covers.
	outlines  [][]mathx.Vec
	blockMids []mathx.Vec
	// risen holds the rising ground of each block that has any, and rise is
	// that of the block being dressed, nil on the level.
	risen []*risen
	rise  *risen
	// baking is the patch a block's ground is being made in, while it is:
	// what is laid flat then goes into it rather than being drawn.
	baking *patchwork
	// land is how high the ground stands, nil in a level place; tiles are
	// the sheets of ground laid over it.
	land  land
	tiles map[tile]*patch
	// viewW and viewH are the size of whatever is being drawn to, as the sky
	// was last painted for it.
	viewW, viewH int
	trees        []tree
	treedBlocks  int
	treedRoads   int
	// furniture is what stands on the pavements, bays the parking marked on
	// them and parked the cars left there; the counts say how much of the
	// city each covers.
	furniture      []furnishing
	furnishedRoads int
	bays           []city.Bay
	bayOn          []mathx.Vec
	bayedRoads     int
	parked         []parkedCar
	// barriers are the crash barriers along the fast roads.
	barriers       []barrierStand
	barrieredRoads int
	fx             effects
	// fov is the chase camera's lens, which opens up with speed.
	fov float32
	// banner is the word across the screen when a pursuit ends, bannerFor how
	// much longer it stays up, and the seen counts what it has already
	// announced.
	banner, bannerNote string
	bannerCol          rl.Color
	bannerFor          float32
	seenStops          int
	seenEscapes        int
	seenBanked         int
	// career counts the bounty banked here that goes toward the packs.
	career bountyMeter
	// jobs is the jobs framework's end of the game: see jobs.go.
	jobs jobs
	// sites is the emergency pack's hospitals, stations and fire sites.
	sites *emergency.Index
	// taxi is the taxi pack's state for this game: see pack_taxi.go.
	taxi taxiState
	// jobMenu is the menu J opens, nil when it is shut: see jobmenu.go.
	jobMenu *jobMenu
	// chal is the challenge being driven, nil in an ordinary game, and
	// chalDone the outcome of one that has finished and not yet been collected.
	chal     *challenge
	chalDone *scenario.Outcome

	camMode CameraMode
	// overheadHeight is how much ground the overhead camera shows either side
	// of the car, in metres. The media tool raises it to take in a district.
	overheadHeight float32
	cam            rl.Camera3D
	camPos         mathx.Vec
	aiCam          rl.Camera3D
	aiView         annotate.View
	aiTarget       rl.RenderTexture2D

	scanner   *vision.Scanner
	visionCam vision.Camera
	driver    *autopilot.Driver
	dets      []vision.Detection
	// eyes look through the autopilot's mirrors, and mirrorBoxes is what each
	// showed last, for the panel.
	eyes        [vision.NumMirrors]*annotate.MirrorEye
	mirrorBoxes [vision.NumMirrors][]annotate.Box

	auto       bool
	showHUD    bool
	showPanel  bool
	showLabels bool
	showHelp   bool

	// net is the two-player session. It is always present and starts offline,
	// so nothing here has to check for nil before asking about it.
	net    *netplay.Session[Snapshot, Input]
	chase  Chase
	chaser *sim.Agent
	// mode is what the two players are doing; see coop.go.
	mode NetMode
	// indicator is the turn signal this car is showing when nobody's autopilot
	// is, and myIndicator the one a joining player has asked the host for.
	indicator, myIndicator sim.Indicator
	// remoteAuto is whether the other player's autopilot is driving, as the
	// host hears it, and assisted whether either is, as the joiner is told.
	remoteAuto, assisted bool

	sinceSnapshot float32
	lastDT        float32

	audio    *audio.Kit
	skySound *audio.Sky
	// nightSound is the night pack's crowd and fireworks, nil without the pack.
	nightSound *audio.Night
	recorder   *record.Recorder
	notices    *crucihud.Overlay
	lastCmd    autopilot.Command
	perceived  float32
	blink      bool
	blinkT     float32
	frame      int

	peakWanted  int
	speedSum    float32
	detSum      int
	detFrames   int
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
	Wanted        int // the wanted level reached at its worst
	Stops         int // how many times the police pulled the driver over
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
	s.Wanted, s.Stops = g.peakWanted, g.world.Wanted.Stops
	return s
}

// chaseReady makes sure a chase has a police car in it. The settings screen
// allows a world with no police at all, which is fine for driving and useless
// for a chase: nothing crashes, the joining player simply has nothing to
// control. Raising the count is friendlier than refusing the session.
func (o Options) chaseReady() Options {
	o.Police = max(o.Police, 1)
	return o
}

// New creates a game with a freshly generated city. The window must already be
// open, because the AI camera allocates a render texture.
func New(opts Options) *Game {
	if opts.Chase || opts.Host != "" || opts.Join != "" {
		opts = opts.chaseReady()
	}
	if opts.Play.SharedCar() {
		opts.Autopilot = false // nobody can take the wheel of a shared car
	}
	cfg := sim.Config{
		Seed: opts.Seed, Map: opts.Map, Traffic: opts.Traffic, Police: opts.Police,
		PlayerBody: sim.PlayerBodyOf(opts.Car), Weather: opts.Weather,
	}
	switch {
	case opts.Challenge != nil:
		// The scenario names the place and the rules; only the car is the player's.
		cfg = challengeConfig(*opts.Challenge)
		cfg.PlayerBody = sim.PlayerBodyOf(opts.Car)
		opts.Map, opts.Seed, opts.Custom = opts.Challenge.CityMap(), opts.Challenge.Seed, nil
		opts.Autopilot, opts.Chase = false, false
	case opts.Custom != nil:
		// The map is the preset the custom one starts from, so it is dressed as that
		// preset is; the rest of the world is the spec's.
		cfg = opts.Custom.Apply(cfg)
		opts.Map, opts.Seed = opts.Custom.Base, opts.Custom.Seed
	}
	g := &Game{
		opts: opts, mode: opts.Play,
		world: sim.NewWorld(cfg),
		// In a chase against the machine the runner is always the autopilot;
		// that is what makes it a chase against the machine.
		auto: opts.Autopilot || opts.Chase, showPanel: opts.ShowPanel,
		showHUD: true, showLabels: true, overheadHeight: 34,
		scanner:   vision.NewScanner(),
		net:       &netplay.Session[Snapshot, Input]{},
		visionCam: vision.DefaultCamera(opts.CamWidth, opts.CamHeight),
	}
	if opts.Rec.Recording() {
		// crucible's recorder takes plain image.Image frames, so it works
		// behind raylib just as well as behind Ebiten, and picks GIF or MP4
		// from the path's extension.
		g.recorder = record.New(opts.Rec, record.WithFrameDiff())
	}
	g.audio = audio.Open(opts.Mute)
	g.skySound = audio.OpenSky(g.audio)
	g.notices = crucihud.New()
	g.watchInfractions()
	g.watchJobs()
	g.driver = autopilot.New(g.visionCam)
	g.driver.Mode = opts.Mode
	g.aiTarget = rl.LoadRenderTexture(int32(opts.CamWidth), int32(opts.CamHeight))

	g.scene = loadSceneShader()
	if g.world.City.Hilly() {
		g.land = g.world.City.Height
	}
	g.look = lookFor(g.season())
	g.town = townFor(opts.Map)
	g.clock.minutes = opts.DayLength
	g.SetTime(opts.Time)
	g.fov = groundFov

	g.cam = rl.Camera3D{Up: vec3(0, 1, 0), Fovy: groundFov, Projection: rl.CameraPerspective}
	g.aiCam = rl.Camera3D{
		Up: vec3(0, 1, 0), Fovy: g.visionCam.FovY,
		Projection: rl.CameraPerspective,
	}
	g.camMode = opts.Camera % numCameraModes
	if opts.Challenge != nil {
		g.chal = newChallenge(*opts.Challenge)
	}
	if opts.Chase {
		g.chaser = g.world.AssignChaser()
	} else if opts.Host != "" || opts.Join != "" {
		// Both ends need the car the joining player drives, and it must be the
		// same one, which it is because the world came from one seed.
		g.seat()
	}
	g.camPos = g.world.Player.Pos
	g.updateCameras(0)
	applyPacks(g)
	return g
}

func (g *Game) watchInfractions() {
	// Turns judge events into HUD lines. The judge publishes on a
	// crucible telemetry bus, a status source decides the wording, and the line is
	// posted to the overlay: the game supplies the vocabulary, crucible the plumbing.
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
	g.skySound.Close()
	g.nightSound.Close()
	g.audio.Close()
	rl.UnloadRenderTexture(g.aiTarget)
	g.freeGround()
	if g.scene.ok {
		rl.UnloadShader(g.scene.shader)
	}
}

// season is the time of year the map is driven in: a custom map may choose its
// own, as a preset keeps the one it was given.
func (g *Game) season() city.Season {
	if g.opts.Custom != nil {
		return g.opts.Custom.Season()
	}
	return g.opts.Map.Season()
}

// Time returns the time of day the city is nearest to being shown at.
func (g *Game) Time() TimeOfDay { return g.clock.stage() }

// SetTime puts the clock at a time of day at once.
func (g *Game) SetTime(t TimeOfDay) {
	g.opts.Time = ((t % NumTimes) + NumTimes) % NumTimes
	g.clock.set(g.opts.Time)
	g.relight()
}

// SetDayLength changes how many real minutes a day lasts; 0 holds the clock.
func (g *Game) SetDayLength(minutes float32) {
	g.opts.DayLength = minutes
	g.clock.minutes = minutes
}

// advanceDay moves the clock on and relights the city for the new hour.
func (g *Game) advanceDay(dt float32) {
	g.clock.step(dt)
	g.relight()
}

// relight sets the light for the hour on the clock, in the map's season.
func (g *Game) relight() {
	g.light = relight(lightingAt(g.clock.hour), g.season())
	if g.world == nil {
		return
	}
	// The weather is asked of the clock each frame, as everything in it is.
	flash := float32(0)
	if f, ok := g.strike(); ok {
		flash = flashLevel(f, g.world.Time)
	}
	g.light = rewet(g.light, g.world.Weather(), flash)
}

// strike returns the lightning that has lately struck, if any.
func (g *Game) strike() (weather.Flash, bool) {
	c := &g.world.Climate
	return weather.Strike(c.Seed, c.Season, c.Setting, g.world.Time)
}

// sky clears the frame to the sky for the current time of day.
func (g *Game) sky(w, h int) {
	g.viewW, g.viewH = w, h
	drawSky(g.light, int32(w), int32(h))
}

// World exposes the simulation, for tests and tools.
func (g *Game) World() *sim.World { return g.world }

// Detections returns the most recent frame of camera detections.
func (g *Game) Detections() []vision.Detection { return g.dets }

// Autopilot reports whether the AI is currently driving.
func (g *Game) Autopilot() bool { return g.auto }

// SetAutopilot switches between manual and AI driving.
func (g *Game) SetAutopilot(on bool) {
	if on == g.auto || (on && g.autoBlocked()) {
		return
	}
	g.auto = on
	g.driver.Reset()
	g.world.Judge.Reset()
	// The camera does not run while a person is driving, so the detections in
	// hand are as old as the manual stint. Throw them away and look again on
	// the first frame rather than handing the driver a stale view of the road.
	g.dets, g.perceived = nil, 0
}

// Networked reports whether this session is one end of a chase against another
// person, rather than against the AI on this machine.
func (g *Game) Networked() bool { return g.net.Online() }

// hosting reports whether this machine owns the simulation. The other end of
// a two-player chase draws what it is told and simulates nothing.
func (g *Game) hosting() bool { return g.net.Role() == netplay.Hosting }

// Session exposes the network session, so a lobby screen can start, watch and
// end it.
func (g *Game) Session() *netplay.Session[Snapshot, Input] { return g.net }

// Chasing reports whether the player is driving a police car, whoever or
// whatever is running from them. Two players on a team are not.
func (g *Game) Chasing() bool { return g.opts.Chase || (g.Networked() && !g.mode.Coop()) }

// Step advances the game by one frame: perception, control, simulation, then
// rendering. dt is the frame duration in seconds.
func (g *Game) Step(dt float32) {
	g.frame++
	g.lastDT = dt
	if g.Coop() {
		g.stepCoop(dt)
		return
	}
	if g.Chasing() {
		g.stepChase(dt)
		return
	}
	g.speedSum += g.world.Player.Speed()
	g.peakWanted = max(g.peakWanted, g.world.Wanted.Level)
	g.handleInput()

	if g.blinkT += dt; g.blinkT > 0.36 {
		g.blink, g.blinkT = !g.blink, 0
	}
	g.notices.Tick()
	g.advanceDay(dt)

	g.perceive(dt)

	controls := g.manualControls()
	if g.opts.Reckless {
		controls = sim.RecklessControls(g.world)
	}
	if g.auto {
		cmd := g.driver.Drive(g.dets, g.world.Player.Speed(), dt)
		g.lastCmd = cmd
		controls = sim.Controls{
			Throttle: cmd.Throttle, Brake: cmd.Brake, Steer: cmd.Steer,
		}
	}

	g.world.Update(controls, dt)
	// A drift event scores the slip of the car, which job.Frame does not carry.
	p := g.world.Player
	race.Feed(g.jobs.board.Active(), race.Sample{Now: g.world.Time, Pos: p.Pos, Vel: p.Vel, Yaw: p.Yaw, Step: dt})
	g.stepJobs()
	g.taxiStep()
	g.stepChallenge(dt)
	g.updateEffects(dt)
	g.watchPursuit(dt)
	g.updateCameras(dt)
	g.updateAudio()

	rl.BeginDrawing()
	g.sky(g.opts.Width, g.opts.Height)
	g.drawWorld(g.cam, g.camMode != CameraBonnet)
	if g.showHUD {
		g.drawHUD()
	}
	rl.EndDrawing()
	g.capture()
}

func (g *Game) capture() {
	// Feeds the finished frame to the recorder, when one is running.
	if g.recorder == nil || g.recorder.Done() {
		return
	}
	if img := g.Snapshot(); img != nil {
		g.recorder.Add(img)
	}
}

// Snapshot returns the frame currently on screen, HUD and all. It is what the
// recorder captures and what the media tool tiles into contact sheets.
func (g *Game) Snapshot() image.Image {
	shot := rl.LoadImageFromScreen()
	if shot == nil {
		return nil
	}
	defer rl.UnloadImage(shot)
	return toRGBA(shot)
}

// CameraImage returns the AI camera's annotated view: the exact image the
// scanner reads, boxes included. It is only refreshed on a perception tick, so
// consecutive calls within one tick return the same picture.
func (g *Game) CameraImage() image.Image {
	shot := rl.LoadImageFromTexture(g.aiTarget.Texture)
	if shot == nil {
		return nil
	}
	defer rl.UnloadImage(shot)
	// A render target is stored bottom-up, so flip it to what the camera saw.
	rl.ImageFlipVertical(shot)
	return toRGBA(shot)
}

func toRGBA(src *rl.Image) *image.RGBA {
	w, h := int(src.Width), int(src.Height)
	px := rl.LoadImageColors(src)
	defer rl.UnloadImageColors(px)

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i, c := range px[:min(len(px), w*h)] {
		// Writing the pixel slice directly avoids Set's bounds check per pixel.
		o := i * 4
		img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = c.R, c.G, c.B, 255
	}
	return img
}

// CameraMode returns the viewpoint currently in use.
func (g *Game) CameraMode() CameraMode { return g.camMode }

// SetCameraMode selects the viewpoint.
func (g *Game) SetCameraMode(m CameraMode) { g.camMode = m % numCameraModes }

// ShowPanel turns the AI camera panel on or off.
func (g *Game) ShowPanel(on bool) { g.showPanel = on }

// ShowHUD turns the whole overlay on or off, for media that wants the world
// on its own.
func (g *Game) ShowHUD(on bool) { g.showHUD = on }

// Camera fields of view, in degrees.
const (
	// groundFov is the lens used from the car: wide, as a driver's view is.
	groundFov float32 = 62
	// planFov is the lens used from above. It is narrow so the view
	// approximates a plan, and the altitude is derived from it.
	planFov float32 = 20
)

// SetOverheadHeight sets how much ground the overhead camera shows either side
// of the car, in metres. The altitude follows from the plan lens.
func (g *Game) SetOverheadHeight(m float32) { g.overheadHeight = max(m, 8) }

// Driver exposes the autopilot controller, so a caller can gate on what it is
// currently doing.
func (g *Game) Driver() *autopilot.Driver { return g.driver }

// SaveRecording writes the captured drive, if recording was enabled.
func (g *Game) SaveRecording(path string) error {
	if g.recorder == nil || g.recorder.Len() == 0 {
		return nil
	}
	return g.recorder.Save(path)
}

func (g *Game) perceive(dt float32) {
	// Perception runs on its own clock, as a real sensor stack would. The
	// camera image must be produced before the controller reads it.
	//
	// Nothing reads it while a person is driving: the panel showing it is the
	// autopilot's view of the road, and there is no autopilot. Skipping it
	// saves rendering the scene a second time into an off-screen target.
	if !g.auto {
		return
	}
	g.perceived += dt
	interval := 1 / max(g.opts.PerceptionH, 1)
	if g.perceived < interval && g.dets != nil {
		return
	}
	g.perceived = 0
	g.renderAICamera()
	g.scanAICamera()
	g.watchMirrors()
	g.detSum += len(g.dets)
	g.detFrames++
}

func (g *Game) updateAudio() {
	// The engine note tracks the car, and the siren the nearest unit that is
	// actually running to a call.
	p := g.world.Player
	g.audio.Engine(p.Speed(), p.Throttle)
	g.audio.Skid(p.Slip, p.Speed())
	strike, struck := g.strike()
	g.skySound.Update(g.world.Weather(), strike, struck, g.world.Time)
	g.updateNightAudio()

	nearest, bearing := float32(1e9), float32(0)
	for _, u := range g.world.Police() {
		if !u.Pursuing() {
			continue
		}
		rel := u.V.Pos.Sub(p.Pos)
		if d := rel.Len(); d < nearest {
			fwd := p.Forward()
			nearest = d
			bearing = mathx.Atan2(rel.Dot(fwd.Right()), rel.Dot(fwd))
		}
	}
	for _, u := range g.world.Ambulances() {
		if !u.BlueLights() {
			continue
		}
		rel := u.V.Pos.Sub(p.Pos)
		if d := rel.Len(); d < nearest {
			fwd := p.Forward()
			nearest = d
			bearing = mathx.Atan2(rel.Dot(fwd.Right()), rel.Dot(fwd))
		}
	}
	g.audio.Siren(nearest, bearing)

	// And the nearest driver leaning on their horn.
	nearest, bearing = float32(1e9), 0
	for _, a := range g.world.Agents {
		if !a.Hooting() {
			continue
		}
		rel := a.V.Pos.Sub(p.Pos)
		if d := rel.Len(); d < nearest {
			fwd := p.Forward()
			nearest = d
			bearing = mathx.Atan2(rel.Dot(fwd.Right()), rel.Dot(fwd))
		}
	}
	g.audio.Horn(nearest, bearing)
}

func (g *Game) stepChase(dt float32) {
	// Runs one frame of a two-player chase. The host simulates and
	// publishes; the joining player sends controls and draws what comes back.
	g.handleInput()
	g.notices.Tick()
	if g.blinkT += dt; g.blinkT > 0.36 {
		g.blink, g.blinkT = !g.blink, 0
	}

	if g.net.Role() != netplay.Joining {
		g.advanceDay(dt) // the joining player takes the hour from the host
	}
	if g.net.Role() == netplay.Joining {
		g.joinChase(dt)
	} else {
		g.driveChase(g.runnerControls(dt), dt)
	}

	g.updateEffects(dt)
	g.chaseCamera(dt)
	g.updateAudio()

	rl.BeginDrawing()
	g.sky(g.opts.Width, g.opts.Height)
	g.drawWorld(g.cam, true)
	if g.showHUD {
		g.drawSpeedoFor(g.chaseSubject())
		g.drawChaseHUD()
		g.drawMinimap(int32(g.opts.Width)-156, int32(g.opts.Height)-156, 144)
	}
	rl.EndDrawing()
	g.capture()
}

func (g *Game) handleInput() {
	if g.jobMenu != nil {
		g.stepJobMenu()
		return
	}
	g.handleIndicatorKeys()
	switch {
	case rl.IsKeyPressed(rl.KeyTab):
		g.toggleAutopilot()
	case rl.IsKeyPressed(rl.KeyC):
		g.camMode = (g.camMode + 1) % numCameraModes
	case rl.IsKeyPressed(rl.KeyV):
		g.showPanel = !g.showPanel
	case rl.IsKeyPressed(rl.KeyL):
		g.showLabels = !g.showLabels
	case rl.IsKeyPressed(rl.KeyR):
		g.world.Respawn()
	case rl.IsKeyPressed(rl.KeyJ):
		g.offerJobUnlessCoop()
	case rl.IsKeyPressed(rl.KeyN):
		g.world.PlaceOnNearestLane()
	case rl.IsKeyPressed(rl.KeyH):
		g.showHelp = !g.showHelp
	case rl.IsKeyPressed(rl.KeyT):
		if g.net.Role() != netplay.Joining {
			g.clock.skip()
		}
	case rl.IsKeyPressed(rl.KeyM):
		g.driver.Mode = (g.driver.Mode + 1) % autopilot.NumModes
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
	c.Boost = rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)
	return c
}

func (g *Game) updateCameras(dt float32) {
	p := g.world.Player
	fwd := p.Forward()

	// The AI camera is rigidly mounted, matching the calibration the autopilot
	// was built with. It never smooths, because a real sensor does not. The pose
	// comes from the annotator so the rendered view and the projected boxes
	// cannot disagree.
	g.aiView = annotate.BonnetView(g.visionCam, g.world.City.StandAt(p.Pos, p.Yaw))
	g.aiCam.Position = vec3(g.aiView.Position.X, g.aiView.Position.Y, g.aiView.Position.Z)
	g.aiCam.Target = vec3(g.aiView.Target.X, g.aiView.Target.Y, g.aiView.Target.Z)
	g.aiCam.Up = vec3(g.aiView.Up.X, g.aiView.Up.Y, g.aiView.Up.Z)
	// Every other camera stands on the same ground, and looks at it.
	car := g.world.City.StandAt(p.Pos, p.Yaw)
	v3 := func(v mathx.Vec3) rl.Vector3 { return vec3(v.X, v.Y, v.Z) }
	g.cam.Up = vec3(0, 1, 0)

	if g.camMode != CameraHigh {
		g.cam.Fovy = g.speedLens(p, dt)
	}
	switch g.camMode {
	case CameraBonnet:
		// On the bonnet, and so tipped as the car is.
		g.cam.Position = v3(car.Point(1.7, 0, 1.45))
		g.cam.Target = v3(car.Point(25.7, 0, 1.25))
		g.cam.Up = v3(car.Up)
	case CameraHigh:
		// A plan view: a narrow lens a long way up, rather than a wide one
		// just overhead. With a 62 degree lens at rooftop height the towers
		// lean out across the streets they stand beside and the layout is
		// unreadable — which is what made the city contact sheet grey mush.
		// Pulling back and narrowing keeps each building over its own
		// footprint.
		g.cam.Fovy = planFov
		alt := g.overheadHeight / float32(math.Tan(float64(planFov)*0.5*math.Pi/180))
		g.cam.Position = vec3(p.Pos.X, car.At.Y+alt, p.Pos.Z+0.1)
		g.cam.Target = vec3(p.Pos.X, car.At.Y, p.Pos.Z)
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
		sx, sy := g.rattle()
		side := fwd.Right().Mul(sx)
		// Never lower over the ground it is over than it would be on the
		// level, so a hill behind the car cannot come between the two.
		at := mathx.V(g.camPos.X+side.X, g.camPos.Z+side.Z)
		g.cam.Position = vec3(at.X, max(g.land.at(at), car.At.Y)+4.3+sy, at.Z)
		ahead := p.Pos.Add(fwd.Mul(9))
		g.cam.Target = vec3(ahead.X, g.land.at(ahead)+1.0, ahead.Z)
	}
}

// Lens tuning for the view from the car.
const (
	// speedFov is how much the lens opens between a standstill and flat out,
	// and boostFov how much further it opens on nitrous. A wider lens is most
	// of what makes speed look like speed.
	speedFov float32 = 15
	boostFov float32 = 7
)

// speedLens returns the field of view for the view from a car, easing toward
// the lens its speed calls for.
func (g *Game) speedLens(v *sim.Vehicle, dt float32) float32 {
	want := groundFov + speedFov*mathx.Clamp(v.Speed()/v.Spec.MaxSpeed, 0, 1)
	if v.Boosting {
		want += boostFov
	}
	if dt <= 0 {
		g.fov = want
	} else {
		g.fov = mathx.Approach(g.fov, want, 3, dt)
	}
	return g.fov
}

func (g *Game) renderAICamera() {
	// Draws the scene from the bonnet camera into the off-screen
	// target and paints the detection boxes over it.
	rl.BeginTextureMode(g.aiTarget)
	g.sky(g.opts.CamWidth, g.opts.CamHeight)
	g.drawWorld(g.aiCam, false)
	boxes := annotate.LayoutIn(g.world, g.aiView, g.world.Weather())
	drawBoxes(boxes, g.showLabels)
	rl.EndTextureMode()
}

// watchMirrors takes a look through each of the autopilot's mirrors.
func (g *Game) watchMirrors() {
	p := g.world.Player
	cams := g.driver.Mirrors()
	for m := range g.eyes {
		if g.eyes[m] == nil {
			g.eyes[m] = annotate.NewMirrorEye(cams[m].Width, cams[m].Height)
		}
		g.driver.Watch(vision.Mirror(m), g.eyes[m].Look(g.world, cams[m], g.world.City.StandAt(p.Pos, p.Yaw), g.world.Weather()))
		g.mirrorBoxes[m] = g.eyes[m].Boxes
	}
}

func (g *Game) scanAICamera() {
	// Reads the camera image back and recovers the detections. This
	// is the only path by which world state reaches the autopilot.
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

// WithWindow opens a window, builds a game in it, and hands it to fn. The
// window and the game are torn down before it returns, whatever fn does.
//
// It exists so raylib's setup lives in one place: the game loop and the media
// tool both go through here rather than each initialising a window themselves.
// A window is still required even when nothing is watching, because the scene
// is drawn on the GPU; on a headless Linux box run under xvfb-run.
func WithWindow(opts Options, fn func(*Game) error) error {
	// A session asked for on the command line is opened before the window, so
	// a bad address fails immediately rather than after a window has appeared.
	// Starting one from the lobby instead goes through the same session.
	session := &netplay.Session[Snapshot, Input]{}
	switch {
	case opts.Host != "":
		if err := session.StartHosting(opts.Host); err != nil {
			return err
		}
		defer session.Leave()
		fmt.Println("hosting a chase on", session.Addr())
	case opts.Join != "":
		if err := session.StartJoining(opts.Join); err != nil {
			return err
		}
		defer session.Leave()
		fmt.Println("joined", opts.Join, "as the police")
	}

	rl.SetTraceLogLevel(rl.LogWarning)
	rl.SetConfigFlags(rl.FlagMsaa4xHint)
	rl.InitWindow(int32(opts.Width), int32(opts.Height), "Autobahn")
	defer rl.CloseWindow()

	g := New(opts)
	g.net = session
	if g.Networked() {
		g.seat()
	}
	defer g.Close()
	return fn(g)
}

// Run opens a window and plays until the player quits.
//
// An ordinary run opens on the title screen, where the drive, the two-player
// chase and the settings are chosen. A run that was told what to do on the
// command line — a fixed frame count, a recording, a screenshot, a session to
// host or join — goes straight to the wheel instead, because a menu waiting
// for a keypress is no use to a script or a media build.
func Run(opts Options) error {
	if opts.headless() {
		// There is no menu to keep a run like this to what the player holds,
		// so it is checked here.
		set := LoadSettings()
		c := loadCareerFor(set)
		held := pack.Access{Banked: c.Banked, Bypass: opts.UnlockAll || set.UnlockAll}
		opts.Access = held
		if err := opts.coopValid(); err != nil {
			return err
		}
		if err := opts.lockedFor(c.holdings(held)); err != nil {
			return err
		}
		return WithWindow(opts, run)
	}
	return WithWindow(opts, shell)
}

// headless reports whether the run was told what to do rather than being
// played, in which case the menus are skipped.
func (o Options) headless() bool {
	return o.Frames > 0 || o.Rec.Recording() || o.Screenshot != "" ||
		o.Host != "" || o.Join != "" || o.Chase || o.Stats
}

// shell plays through the menu layer.
func shell(g *Game) error {
	rl.SetTargetFPS(60)

	// The game the window was opened with is discarded: the shell builds its
	// own from the player's stored settings once they choose to drive. Only
	// the network session carries over, so a -host or -join on the command
	// line still lands in the lobby.
	sh := NewShell(g.opts)
	sh.net = g.net
	defer sh.Close()
	g.Close()

	for sh.Step(mathx.Clamp(rl.GetFrameTime(), 0.001, 1.0/20)) {
	}
	return nil
}

func run(g *Game) error {
	opts := g.opts
	rl.SetTargetFPS(60)

	for !rl.WindowShouldClose() {
		dt := rl.GetFrameTime()
		// Clamp the step so a stall cannot tunnel the car through a wall.
		g.Step(mathx.Clamp(dt, 0.001, 1.0/20))

		if opts.Frames > 0 && g.frame >= opts.Frames {
			break
		}
	}

	if opts.Screenshot != "" {
		if err := screenshot(opts.Screenshot); err != nil {
			return err
		}
		fmt.Println("wrote", opts.Screenshot)
	}
	if opts.Rec.Recording() {
		if err := g.SaveRecording(opts.Rec.Path); err != nil {
			return fmt.Errorf("saving the recording: %w", err)
		}
		fmt.Println("wrote", opts.Rec.Path)
	}
	if opts.Stats {
		printSummary(g.Summary())
	}
	return nil
}

// screenshot writes the current frame and confirms it landed.
//
// raylib resolves the path against the working directory and reports failure
// only to its own log, so an absolute path silently produces nothing while the
// caller cheerfully announces success. Checking afterwards is the only way to
// know.
func screenshot(path string) error {
	if filepath.IsAbs(path) {
		return fmt.Errorf("screenshot path %q must be relative to the working directory", path)
	}
	rl.TakeScreenshot(path)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("writing the screenshot to %q: %w", path, err)
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
	if s.Wanted > 0 || s.Stops > 0 {
		fmt.Printf("  police     peak wanted level %d, pulled over %d time(s)\n",
			s.Wanted, s.Stops)
	}
	for k, n := range s.Faults {
		fmt.Printf("               %-16s x%d\n", k, n)
	}
}

func (g *Game) checkPerception() {
	// Compares the range the camera model infers for the nearest
	// vehicle against the simulation's true distance. It exists to validate the
	// camera calibration during development; the autopilot never sees either
	// number, and the check runs only under the -debugvision flag.
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
