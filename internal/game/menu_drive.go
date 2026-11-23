package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// The menu drive (docs/menus.md, "Behind the menus: the menu drive"): the
// autopilot drives the map chosen under Play behind the menu pages.
//
// It is a real Game in an attract mode, built from options of its own: the
// autopilot at the wheel, no HUD, no AI camera panel, no police, light
// traffic, no sound and a held midday. Three things keep it apart from a drive
// the player is scored on:
//
//   - It lives in Shell.drive, never in Shell.game. Shell.collect, the feats and
//     the job pay only ever read s.game, so nothing this drive does can reach
//     the career, and there is no flag to forget to check.
//   - It is stepped by stepAttract, not Game.Step: no keys are read (the menus
//     have them), and no jobs, challenge, pursuit banner or bounty meter run.
//   - It exists only while no game does. A real drive closes it before it is
//     built, and while one is paused its pages stand on the frozen game or
//     the plain backdrop.
//
// The shell owns its life. It is built lazily the first frame a page wants it
// with the setting on, stepped at the real frame time while such a page shows,
// left as it is (but kept) behind a page that does not, rebuilt once the map
// under Play has stayed changed for driveSettle, rebuilt on a new seed if it
// wedges, and closed when the setting goes off or a drive starts.

// pageBackdrop is what a page stands on when it is not drawn over a game.
type pageBackdrop int

const (
	// backdropDrive is the menu drive while it is on, and the plain backdrop
	// while it is off. It is the zero value, so every page has it unless it
	// says otherwise.
	backdropDrive pageBackdrop = iota
	// backdropPlain is always the plain backdrop, for a page that draws a whole
	// scene of its own (the garage) and would only hide the drive.
	backdropPlain
)

// The menu drive's world and its safety net. The perception rate and traffic
// are the cost spike's (#50): perception is most of the drive's cost, and at
// five looks a second it is about two milliseconds a frame.
const (
	driveTraffic    = 20
	drivePerception = 5
	// driveSettle is how long the map under Play has to stay chosen before the
	// drive is rebuilt on it, so stepping through the cards builds nothing.
	driveSettle float32 = 0.5
	// driveMaxStep caps a frame, as the shell does, so a slow build cannot hand
	// the sim one long step.
	driveMaxStep float32 = 1.0 / 20
	// The autopilot has no way out of a wedge. A car that has stood within
	// driveStuckDist of one spot for driveStuckFor seconds of drive time, not
	// counting time held at a red, is rebuilt on the next seed.
	driveStuckFor  float32 = 90
	driveStuckDist float32 = 1
)

// driveKey is what a menu drive is of: a preset map, or a custom map's share
// code (with the preset it starts from).
type driveKey struct {
	m    city.Map
	code string
}

// menuDrive is the shell's end of the menu drive.
type menuDrive struct {
	// game is the drive, nil when none is built, and key what it is of.
	game *Game
	key  driveKey
	// want is what the menus last asked for, and settled how long they have
	// asked for it without a change.
	want    driveKey
	settled float32
	// tries is added to the seed: how many times the wedge net has rebuilt.
	tries uint64
	// anchor is where the car stood when it last moved a metre, and still how
	// long it has stood within a metre of it since, red lights aside. anchored
	// is false until the first frame of a drive sets the anchor.
	anchor   mathx.Vec
	anchored bool
	still    float32
	// built counts the drives built, which is what the tests hold.
	built int
}

// settle notes what the menus want this frame and reports whether the drive
// should be built now: there is none, or they have wanted another map for
// driveSettle.
func (d *menuDrive) settle(want driveKey, dt float32) bool {
	if want != d.want {
		d.want, d.settled = want, 0
	} else {
		d.settled += dt
	}
	if d.game == nil {
		return true
	}
	return d.key != d.want && d.settled >= driveSettle
}

// adopt makes g the drive, of what the menus want.
func (d *menuDrive) adopt(g *Game) {
	d.game, d.key = g, d.want
	d.anchored, d.still = false, 0
	d.built++
}

// stuck feeds the wedge net a frame: where the car is, whether it is held at
// a red (asked only when it has not moved), and how long the frame was. It
// reports whether the car has stood long enough to be called wedged.
func (d *menuDrive) stuck(pos mathx.Vec, held func() bool, dt float32) bool {
	if !d.anchored || pos.DistTo(d.anchor) > driveStuckDist {
		d.anchor, d.anchored, d.still = pos, true, 0
		return false
	}
	if !held() {
		d.still += dt
	}
	return d.still >= driveStuckFor
}

// heldAtRed reports whether the player's car is on a lane whose light says
// stop: a red can hold a queue for a while, and that is not a wedge. Near a
// junction the lane found is only a guess, which at worst slows the net down.
func heldAtRed(w *sim.World) bool {
	p := w.Player
	pr := w.City.ProjectAlong(p.Pos, p.Forward())
	return pr.Valid && w.Signals.LaneState(pr.Lane).MustStop()
}

// menuDriveAllowed reports whether there may be a menu drive at all: the
// setting is on and no game is in progress, paused or not.
func (s *Shell) menuDriveAllowed() bool {
	return s.settings.MenuDrive() && s.game == nil
}

// menuDriveBehind reports whether the page showing wants the drive behind it.
// A page over a game never does.
func (s *Shell) menuDriveBehind() bool {
	if !s.onPage() {
		return false
	}
	pg := s.pager.cur
	return pg.Backdrop == backdropDrive && !pg.OverGame
}

// menuDriveOn reports whether the menu drive is being drawn behind the page
// showing, for a page that lays itself out differently over it (the title
// card sits small at the bottom right).
func (s *Shell) menuDriveOn() bool {
	return s.drive.game != nil && s.menuDriveAllowed() && s.menuDriveBehind()
}

// menuDriveKey is what the drive should be of now: the map under Play, or the
// custom map when the next drive is to be on it.
func (s *Shell) menuDriveKey() driveKey {
	k := driveKey{m: s.settings.Map}
	if s.customPlay && customOf(s.settings.Custom) != nil {
		k.code = s.settings.Custom
	}
	return k
}

// menuDriveOptions are the attract mode's options: the autopilot, on its own,
// quietly, at midday, in the player's car.
func (s *Shell) menuDriveOptions(k driveKey, tries uint64) Options {
	o := DefaultOptions()
	o.Width, o.Height = s.opts.Width, s.opts.Height
	o.Seed, o.Map = s.opts.Seed+tries, k.m
	o.Traffic, o.Police = driveTraffic, 0
	o.Autopilot, o.ShowPanel, o.Mute = true, false, true
	o.PerceptionH = drivePerception
	o.Time, o.DayLength = TimeDay, 0
	o.Camera = CameraChase
	o.Car, o.Paint = s.settings.Car, s.settings.Paint
	if sp := customOf(k.code); sp != nil {
		sp.Seed += tries
		o.Custom = sp
	}
	return o
}

// stepMenuDrive runs the menu drive for a frame of the menus, building,
// rebuilding or closing it as the frame calls for. It runs before the frame is
// drawn, because the autopilot's camera renders to its own target.
func (s *Shell) stepMenuDrive(dt float32) {
	if !s.menuDriveAllowed() {
		s.closeMenuDrive()
		return
	}
	if !s.menuDriveBehind() {
		return // kept as it is until a page wants it again
	}
	d := &s.drive
	if d.settle(s.menuDriveKey(), dt) {
		s.buildMenuDrive()
	}
	g := d.game
	if g.opts.Car != s.settings.Car || g.opts.Paint != s.settings.Paint {
		g.SetCar(s.settings.Car, s.settings.Paint) // the garage changed it
	}
	dt = min(dt, driveMaxStep)
	g.stepAttract(dt)
	if d.stuck(g.world.Player.Pos, func() bool { return heldAtRed(g.world) }, dt) {
		s.rescueMenuDrive()
	}
}

// rescueMenuDrive is the wedge net: the drive is rebuilt on the next seed.
// Putting the car back on a lane would not clear a jam, and the rebuild is
// rare enough that its frame of building does not matter.
func (s *Shell) rescueMenuDrive() {
	s.drive.tries++
	s.buildMenuDrive()
}

// buildMenuDrive replaces the drive with a new one of what the menus want.
func (s *Shell) buildMenuDrive() {
	s.closeMenuDrive()
	d := &s.drive
	g := New(s.menuDriveOptions(d.want, d.tries))
	g.ShowHUD(false)
	d.adopt(g)
}

// closeMenuDrive closes the drive, if there is one, freeing what it holds on
// the GPU. What the menus last wanted is kept, so the next is built at once.
func (s *Shell) closeMenuDrive() {
	if s.drive.game != nil {
		s.drive.game.Close()
		s.drive.game = nil
	}
}

// stepAttract is Game.Step for the menu drive: perception, the autopilot and
// the world, and nothing else. It reads no keys, draws nothing (the shell draws
// the scene behind its page) and sounds nothing, and none of what a drive is
// scored by runs: no jobs, no challenge, no pursuit banner, no bounty meter.
func (g *Game) stepAttract(dt float32) {
	g.frame++
	g.lastDT = dt
	if g.blinkT += dt; g.blinkT > 0.36 {
		g.blink, g.blinkT = !g.blink, 0
	}
	g.advanceDay(dt)
	g.perceive(dt)
	cmd := g.driver.Drive(g.dets, g.world.Player.Speed(), dt)
	g.lastCmd = cmd
	g.world.Update(sim.Controls{Throttle: cmd.Throttle, Brake: cmd.Brake, Steer: cmd.Steer}, dt)
	g.updateEffects(dt)
	g.updateCameras(dt)
}

// drawMenuDrive draws the menu drive behind the page, if the page wants it and
// there is one, and reports whether it did. It goes through the same scene
// path as any drive (the sky, then drawWorld and its shader), so the vision
// colour rules hold; the page is drawn over it in 2D.
func (s *Shell) drawMenuDrive() bool {
	if !s.menuDriveOn() {
		return false
	}
	g := s.drive.game
	w, h := int32(s.opts.Width), int32(s.opts.Height)
	g.sky(int(w), int(h))
	g.drawWorld(drivePan(g.cam, g.world.Player.Forward()), true)
	drawDriveShade(w, h)
	return true
}

// drivePanBy is how far the menu drive's camera looks to the left of the
// point the chase camera looks at, in metres, which puts the car right of the
// middle, out from behind the list and into the clear.
const drivePanBy = 4

// drivePan turns the chase camera to look past the car's left, without
// moving it, so it cannot be put inside a wall.
func drivePan(cam rl.Camera3D, fwd mathx.Vec) rl.Camera3D {
	left := fwd.Right().Mul(-drivePanBy)
	cam.Target.X += left.X
	cam.Target.Z += left.Z
	return cam
}

// drawDriveShade darkens the left of the screen, where the list is, so the
// words stand on something steady whatever drives past. It holds to half way
// across and fades to clear by three quarters; a band along the foot does the
// same for the hint bar.
func drawDriveShade(w, h int32) {
	half, fade := w/2, w*3/4
	dark := rl.ColorAlpha(menuBackdrop, 0.86)
	mid := rl.ColorAlpha(menuBackdrop, 0.72)
	none := rl.ColorAlpha(menuBackdrop, 0)
	rl.DrawRectangleGradientH(0, 0, half, h, dark, mid)
	rl.DrawRectangleGradientH(half, 0, fade-half, h, mid, none)
	rl.DrawRectangleGradientV(0, h-pageFoot, w, pageFoot, none, rl.ColorAlpha(menuBackdrop, 0.6))
}
