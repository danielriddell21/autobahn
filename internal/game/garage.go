package game

import (
	"fmt"
	"image"
	"math"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
)

// The garage is where the player chooses their car: its shape, which changes
// how it drives as well as how it looks, and its paint.

// carPaint is one colour the player's car can be sprayed.
type carPaint struct {
	name string
	col  rl.Color
}

// playerPaints are the colours the garage offers. The first is the car's
// usual red.
var playerPaints = []carPaint{
	{"rosso", rl.NewColor(186, 62, 58, 255)},
	{"racing blue", rl.NewColor(58, 92, 152, 255)},
	{"arctic white", rl.NewColor(212, 208, 198, 255)},
	{"jet black", rl.NewColor(40, 42, 48, 255)},
	{"sunburst yellow", rl.NewColor(214, 176, 52, 255)},
	{"british racing green", rl.NewColor(36, 84, 58, 255)},
	{"silver", rl.NewColor(156, 160, 166, 255)},
	{"plum", rl.NewColor(116, 64, 124, 255)},
	{"tangerine", rl.NewColor(214, 112, 46, 255)},
	{"lime", rl.NewColor(132, 176, 58, 255)},
	{"teal", rl.NewColor(38, 128, 132, 255)},
	{"gunmetal", rl.NewColor(78, 84, 92, 255)},
}

// paintOf returns the paint with the given index, or the usual red for one
// that is out of range.
func paintOf(i int) carPaint {
	if i < 0 || i >= len(playerPaints) {
		return playerPaints[0]
	}
	return playerPaints[i]
}

// cyclePaint steps through the paints, wrapping at both ends.
func cyclePaint(i, dir int) int {
	n := len(playerPaints)
	return ((i+dir)%n + n) % n
}

// cycleBody steps through the shapes the player may drive, wrapping at both
// ends.
func cycleBody(k sim.BodyKind, dir int) sim.BodyKind {
	all := sim.PlayerBodies()
	at := 0
	for i, b := range all {
		if b == k {
			at = i
		}
	}
	n := len(all)
	return all[((at+dir)%n+n)%n]
}

// SetCar repaints and reshapes the player's car in a running game. A chase
// against another person keeps the car it started with, because both machines
// have to agree on its size.
func (g *Game) SetCar(k sim.BodyKind, paintIdx int) {
	g.opts.Paint = paintIdx
	if g.Networked() {
		return
	}
	g.opts.Car = k
	g.world.SetPlayerBody(sim.PlayerBodyOf(k))
}

// playerPaint is the colour the player's car is drawn in.
func (g *Game) playerPaint() rl.Color { return paintOf(g.opts.Paint).col }

// Garage preview layout.
const (
	previewW, previewH = 640, 300
	// previewSpin is how fast the car on the turntable goes round, rad/s.
	previewSpin = 0.55
)

// showroom draws a car on a turntable: the preview the garage shows.
type showroom struct {
	scene  sceneShader
	target rl.RenderTexture2D
	ready  bool
	angle  float32
}

// load prepares the showroom. It needs a window, so it is done on first use
// rather than when the shell is built.
func (r *showroom) load() {
	if r.ready {
		return
	}
	r.scene = loadSceneShader()
	r.target = rl.LoadRenderTexture(previewW, previewH)
	r.ready = true
}

// unload releases what load took.
func (r *showroom) unload() {
	if !r.ready {
		return
	}
	if r.scene.ok {
		rl.UnloadShader(r.scene.shader)
	}
	rl.UnloadRenderTexture(r.target)
	r.ready = false
}

// render draws the car in the given shape and paint into the preview,
// turning it a little further round.
func (r *showroom) render(k sim.BodyKind, p carPaint, dt float32) rl.Texture2D {
	r.load()
	r.angle = float32(math.Mod(float64(r.angle+previewSpin*dt), 2*math.Pi))
	body := sim.PlayerBodyOf(k)
	v := sim.NewVehicle(body.Spec, mathx.V(0, 0), r.angle)

	light := lightingFor(TimeDay)
	eye := vec3(5.8, 2.3, 3.8)
	cam := rl.Camera3D{
		Position: eye, Target: vec3(0, 0.45, 0), Up: vec3(0, 1, 0),
		Fovy: 33, Projection: rl.CameraPerspective,
	}

	rl.BeginTextureMode(r.target)
	rl.ClearBackground(rl.NewColor(20, 23, 28, 255))
	rl.DrawRectangleGradientV(0, 0, previewW, previewH, rl.NewColor(30, 34, 42, 255), rl.NewColor(14, 16, 20, 255))
	r.scene.use(light, eye, 1)
	rl.BeginMode3D(cam)
	rl.DisableBackfaceCulling()
	if r.scene.ok {
		rl.BeginShaderMode(r.scene.shader)
	}
	// The turntable, then the car on it.
	turntable(3.3, 48, rl.NewColor(64, 70, 80, 255))
	drawCar(v, body, carLook{paint: p.col, wheels: true, player: true})
	if r.scene.ok {
		rl.EndShaderMode()
	}
	rl.EnableBackfaceCulling()
	rl.EndMode3D()
	rl.EndTextureMode()
	return r.target.Texture
}

// turntable lays the flat disc the car stands on, lit like the ground.
func turntable(radius float32, sides int, col rl.Color) {
	rl.Begin(rl.Triangles)
	tint(col)
	rl.Normal3f(0, 1, 0)
	for i := range sides {
		a0 := float32(i) / float32(sides) * 2 * math.Pi
		a1 := float32(i+1) / float32(sides) * 2 * math.Pi
		p, q := mathx.FromAngle(a0).Mul(radius), mathx.FromAngle(a1).Mul(radius)
		rl.Vertex3f(0, 0.005, 0)
		rl.Vertex3f(q.X, 0.005, q.Z)
		rl.Vertex3f(p.X, 0.005, p.Z)
	}
	rl.End()
}

// carBlurb is the line under the preview saying how a shape drives.
func carBlurb(k sim.BodyKind) string {
	switch k {
	case sim.Hatchback:
		return "light and easy, a little down on power"
	case sim.Saloon:
		return "longer and steadier, slower off the line"
	case sim.Estate:
		return "heavy and long, room for the dog"
	case sim.Taxi:
		return "tall and heavy, and it knows every street"
	case sim.Van:
		return "slow and tall, and it leans on every corner"
	default:
		return "quick, small and eager: the one being chased"
	}
}

// squash is a name as typed on a command line: lower case, no spaces.
func squash(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), " ", "")
}

// CarNames returns the shapes the player may drive, as -car names them.
func CarNames() []string {
	all := sim.PlayerBodies()
	out := make([]string, 0, len(all))
	for _, k := range all {
		out = append(out, squash(k.Name()))
	}
	return out
}

// ParseCar reads a car shape and a paint as the command line names them.
func ParseCar(car, paintName string) (sim.BodyKind, int, error) {
	kind, found := sim.HotHatch, false
	for _, k := range sim.PlayerBodies() {
		if squash(k.Name()) == squash(car) {
			kind, found = k, true
		}
	}
	if !found {
		return kind, 0, fmt.Errorf("unknown car %q; try %s", car, strings.Join(CarNames(), ", "))
	}
	for i, p := range playerPaints {
		if squash(p.name) == squash(paintName) {
			return kind, i, nil
		}
	}
	return kind, 0, fmt.Errorf("unknown paint %q", paintName)
}

// ShowroomShot renders one car on the garage turntable, turned to the given
// angle, and returns the picture. It needs a window, as everything drawn does;
// the media tool uses it to show what the garage offers.
func ShowroomShot(k sim.BodyKind, paintIdx int, angle float32) image.Image {
	var r showroom
	defer r.unload()
	r.angle = angle
	tex := r.render(k, paintOf(paintIdx), 0)
	shot := rl.LoadImageFromTexture(tex)
	if shot == nil {
		return nil
	}
	defer rl.UnloadImage(shot)
	rl.ImageFlipVertical(shot) // render targets are stored upside down
	return toRGBA(shot)
}

// PaintCount is how many paints the garage offers.
func PaintCount() int { return len(playerPaints) }
