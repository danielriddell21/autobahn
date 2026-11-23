package game

import (
	"errors"
	"os"
	"reflect"
	"runtime"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// TestMenuDriveSettles holds the rebuild to once per settled choice: stepping
// through the maps builds nothing, stopping on one builds it once, and coming
// back to the map already driven builds nothing at all.
func TestMenuDriveSettles(t *testing.T) {
	const dt = float32(1.0 / 60)
	var d menuDrive
	builds := 0
	frame := func(k driveKey) {
		if d.settle(k, dt) {
			builds++
			d.adopt(&Game{})
		}
	}
	city1, sea, vil, road := driveKey{m: city.MapCity}, driveKey{m: city.MapSeaside},
		driveKey{m: city.MapVillage}, driveKey{m: city.MapMotorway}

	frame(city1)
	if builds != 1 || d.key != city1 {
		t.Fatalf("the first frame built %d drives, of %v", builds, d.key)
	}
	// Through three cards a tenth of a second each, then a rest on the last.
	for _, k := range []driveKey{sea, vil, road} {
		for range 6 {
			frame(k)
		}
	}
	if builds != 1 {
		t.Fatalf("stepping through the maps built %d drives, want none past the first", builds-1)
	}
	for range 60 {
		frame(road)
	}
	if builds != 2 || d.key != road {
		t.Fatalf("resting on the roadtown: %d builds, driving %v", builds, d.key)
	}
	// Away and back again inside the settling time: the drive is already of it.
	for range 12 {
		frame(vil)
	}
	for range 120 {
		frame(road)
	}
	if builds != 2 {
		t.Errorf("a glance at another map rebuilt the drive (%d builds)", builds)
	}
	// A custom map is a choice of its own, even on the same preset.
	code := custom.FromMap(city.MapMotorway, 1).Encode()
	for range 60 {
		frame(driveKey{m: city.MapMotorway, code: code})
	}
	if builds != 3 || d.key.code != code {
		t.Errorf("a custom map: %d builds, driving %+v", builds, d.key)
	}
}

// TestMenuDriveWedgeNet holds the safety net: ninety seconds within a metre
// is a wedge, time at a red is not counted, and moving clears it.
func TestMenuDriveWedgeNet(t *testing.T) {
	const dt = float32(0.5)
	never := func() bool { return false }
	red := func() bool { return true }
	var d menuDrive
	d.adopt(&Game{})
	at := mathx.V(10, 10)

	run := func(n int, pos mathx.Vec, held func() bool) bool {
		for range n {
			if d.stuck(pos, held, dt) {
				return true
			}
		}
		return false
	}
	if run(int(driveStuckFor/dt)+40, at, red) {
		t.Fatal("a car held at a red was called wedged")
	}
	if run(int(driveStuckFor/dt)-1, at, never) {
		t.Fatal("wedged before ninety seconds")
	}
	// A creep of under a metre is not a way out.
	if !run(2, at.Add(mathx.V(0.6, 0)), never) {
		t.Fatal("not wedged after ninety seconds")
	}
	// Moving off starts the count again.
	d.adopt(&Game{})
	run(int(driveStuckFor/dt)-2, at, never)
	if run(4, at.Add(mathx.V(3, 0)), never) {
		t.Error("a car that moved off was called wedged")
	}
}

// TestMenuDriveOptions holds the attract mode to what docs/menus.md says it
// is, and the custom map to its share code.
func TestMenuDriveOptions(t *testing.T) {
	inTempConfig(t)
	s := &Shell{opts: DefaultOptions(), settings: DefaultSettings()}
	s.settings.Map = city.MapVillage
	o := s.menuDriveOptions(s.menuDriveKey(), 0)
	switch {
	case !o.Autopilot, o.ShowPanel, !o.Mute, o.Police != 0, o.Traffic != driveTraffic:
		t.Errorf("not the attract mode: %+v", o)
	case o.PerceptionH != drivePerception, o.DayLength != 0, o.Time != TimeDay:
		t.Errorf("perception %v, day %v at %v", o.PerceptionH, o.DayLength, o.Time)
	case o.Challenge != nil, o.Custom != nil, o.Chase, o.Networked, o.headless():
		t.Errorf("more than a drive: %+v", o)
	case o.Map != city.MapVillage:
		t.Errorf("driving %v, want the map under Play", o.Map)
	}
	if again := s.menuDriveOptions(s.menuDriveKey(), 2); again.Seed != o.Seed+2 {
		t.Errorf("a rescue's seed %d, want %d", again.Seed, o.Seed+2)
	}

	// The custom map in the settings is driven only when it is what Play starts.
	sp := custom.FromMap(city.MapSeaside, 9)
	s.settings.Custom = sp.Encode()
	if k := s.menuDriveKey(); k.code != "" {
		t.Errorf("a custom map driven when Play is on a preset: %+v", k)
	}
	s.customPlay = true
	o = s.menuDriveOptions(s.menuDriveKey(), 1)
	if o.Custom == nil || o.Custom.Base != city.MapSeaside || o.Custom.Seed != 10 {
		t.Errorf("the custom map's drive: %+v", o.Custom)
	}
}

// menuDriveShell opens a window and a shell on a page that wants the menu
// drive, with the career and settings in a scratch directory.
func menuDriveShell(t *testing.T) *Shell {
	t.Helper()
	requireDisplay(t)
	inTempConfig(t)
	// GL is current on the thread that opened the window, and a test runs on
	// a goroutine the scheduler may move; building a city gives it every
	// chance to.
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	rl.SetTraceLogLevel(rl.LogError)
	rl.InitWindow(480, 270, "autobahn test")
	t.Cleanup(rl.CloseWindow)

	o := DefaultOptions()
	o.Width, o.Height, o.Mute = 480, 270, true
	sh := NewShell(o)
	t.Cleanup(sh.Close)
	sh.openPage(pageTitle)
	return sh
}

// TestMenuDriveBanksNothing drives the menu drive for a stretch and holds the
// career to exactly what it was: no bounty, earnings, achievements or
// challenge bests, and no career file written.
func TestMenuDriveBanksNothing(t *testing.T) {
	sh := menuDriveShell(t)
	before := sh.career
	before.Cars = append([]int(nil), before.Cars...)
	before.Paints = append([]int(nil), before.Paints...)

	const frames = 900 // fifteen seconds
	for range frames {
		sh.Step(1.0 / 60)
		if sh.game != nil {
			t.Fatal("the menu drive is the shell's game")
		}
	}
	g := sh.drive.game
	if g == nil {
		t.Fatal("no menu drive behind the title page")
	}
	if g.world.Time < 14 || g.world.Judge.Distance < 5 {
		t.Fatalf("the drive went %.0f m in %.1f s; it was not driving", g.world.Judge.Distance, g.world.Time)
	}
	if !g.auto || g.showHUD || g.showPanel || len(g.world.Police()) != 0 {
		t.Errorf("not the attract mode: auto %v, HUD %v, panel %v, %d police",
			g.auto, g.showHUD, g.showPanel, len(g.world.Police()))
	}
	if !reflect.DeepEqual(sh.career, before) {
		t.Errorf("the career moved:\n got %+v\nwant %+v", sh.career, before)
	}
	if path, err := careerPath(); err != nil {
		t.Fatal(err)
	} else if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a career file was written (%v)", err)
	}
}

// TestMenuDriveLifecycle walks the drive through its life: built on the first
// page, rebuilt once for a settled map, held still behind a plain page, closed
// with the setting and by a real drive, and rebuilt on the next seed by the
// wedge net.
func TestMenuDriveLifecycle(t *testing.T) {
	sh := menuDriveShell(t)
	step := func(n int) {
		for range n {
			sh.Step(1.0 / 60)
		}
	}
	d := &sh.drive

	step(2)
	if d.built != 1 || d.game == nil || !sh.menuDriveOn() {
		t.Fatalf("after the first page: %d built, drawn %v", d.built, sh.menuDriveOn())
	}

	// Through the maps a few frames each, then a rest on the village.
	for _, m := range []city.Map{city.MapSeaside, city.MapMotorway, city.MapVillage} {
		sh.settings.Map = m
		step(5)
	}
	if d.built != 1 {
		t.Fatalf("stepping through the maps built %d drives", d.built-1)
	}
	step(40)
	if d.built != 2 || d.game.opts.Map != city.MapVillage {
		t.Fatalf("after resting on the village: %d built, driving %v", d.built, d.game.opts.Map)
	}

	// The garage stands on its own scene: the drive waits, kept.
	held := d.game
	sh.openPage(pageGarage)
	sh.pager.cur.Backdrop = backdropPlain
	at := held.world.Time
	step(10)
	if d.game != held || held.world.Time != at || sh.menuDriveOn() {
		t.Fatalf("behind the garage the drive moved on (%v to %v) or was drawn", at, held.world.Time)
	}
	sh.backPage()
	step(1)
	if d.game != held || held.world.Time == at {
		t.Fatal("back from the garage the drive did not carry on")
	}

	// The setting off closes it; on again builds it.
	sh.settings.SetMenuDrive(false)
	step(1)
	if d.game != nil || sh.menuDriveOn() {
		t.Fatal("the setting is off and the drive is still there")
	}
	sh.settings.SetMenuDrive(true)
	step(1)
	if d.game == nil || d.built != 3 {
		t.Fatalf("the setting back on: %d built", d.built)
	}

	// The wedge net rebuilds on the next seed.
	d.anchor, d.anchored, d.still = d.game.world.Player.Pos, true, driveStuckFor
	tries := d.tries
	sh.stepMenuDrive(1.0 / 60)
	if d.built != 4 || d.tries != tries+1 || d.game.opts.Seed != sh.opts.Seed+d.tries {
		t.Fatalf("the wedge net: tries %d, seed %d", d.tries, d.game.opts.Seed)
	}

	// A real drive closes it; the paused game's pages never build one. (The
	// map tried above need not be held, and a drive is only started on one that is.)
	sh.settings.Map = city.MapCity
	sh.startGame(false)
	if d.game != nil || sh.game == nil {
		t.Fatal("a drive started and the menu drive was not closed")
	}
	sh.openPage(pageSettings)
	step(3)
	if d.game != nil {
		t.Fatal("a menu drive was built while a game is paused")
	}
	sh.pager = pager{}
	sh.endGame()
	sh.openPage(pageTitle)
	step(1)
	if d.game == nil {
		t.Fatal("no menu drive back on the title page")
	}
}

// TestMenuDriveDraws draws a page over the drive and over the plain backdrop,
// and checks the right of the screen shows the scene in one and not the other.
func TestMenuDriveDraws(t *testing.T) {
	sh := menuDriveShell(t)
	for range 3 {
		sh.Step(1.0 / 60)
	}
	right := func() rl.Color {
		img := rl.LoadImageFromScreen()
		defer rl.UnloadImage(img)
		return rl.GetImageColor(*img, int32(sh.opts.Width)*9/10, int32(sh.opts.Height)/3)
	}
	if c := right(); c == menuBackdrop {
		t.Errorf("the drive is on and the right of the screen is the plain backdrop")
	}
	sh.settings.SetMenuDrive(false)
	sh.Step(1.0 / 60)
	if c := right(); c != menuBackdrop {
		t.Errorf("the drive is off and the right of the screen is %v", c)
	}
}

// TestDrivePan holds the menu drive's camera to a turn: it looks to the car's
// left by drivePanBy and is not moved, so it cannot be put inside a wall.
func TestDrivePan(t *testing.T) {
	cam := rl.Camera3D{Position: vec3(1, 5, 2), Target: vec3(1, 1, 22), Up: vec3(0, 1, 0)}
	fwd := mathx.V(0, 1)
	got := drivePan(cam, fwd)
	if got.Position != cam.Position || got.Target.Y != cam.Target.Y {
		t.Fatalf("the camera moved: %+v", got)
	}
	moved := mathx.V(got.Target.X-cam.Target.X, got.Target.Z-cam.Target.Z)
	if mathx.Abs(moved.Len()-drivePanBy) > 1e-4 || moved.Dot(fwd.Right()) >= 0 {
		t.Errorf("the target moved by %+v, want %v m to the car's left", moved, float32(drivePanBy))
	}
}
