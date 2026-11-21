package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// TimeOfDay selects the light the city is seen in.
type TimeOfDay int

// The times of day.
const (
	TimeDay TimeOfDay = iota
	TimeDusk
	TimeNight
	// TimeDawn comes last so a stored setting keeps meaning what it did.
	TimeDawn
	// NumTimes is how many times of day there are.
	NumTimes
)

// String returns the time's name, for a settings row to show.
func (t TimeOfDay) String() string {
	switch t {
	case TimeDusk:
		return "dusk"
	case TimeNight:
		return "night"
	case TimeDawn:
		return "dawn"
	default:
		return "day"
	}
}

// lighting is everything about a time of day the scene is drawn with.
type lighting struct {
	sunDir         rl.Vector3 // toward the sun, or the moon
	sun            rl.Vector3 // direct light
	sky, ground    rl.Vector3 // ambient from above and from below
	fog            rl.Vector3
	fogRange       float32 // metres at which the fog has taken most of a colour
	skyTop, skyLow rl.Color
	// night runs from 0 in daylight to 1 in the dark, and drives everything
	// that only shows once the light goes: lit windows, lamps, headlights.
	night float32
}

// lightingFor returns the light for a time of day.
func lightingFor(t TimeOfDay) lighting {
	switch t {
	case TimeDusk:
		return lighting{
			sunDir: norm3(-0.78, 0.26, 0.46),
			sun:    vec3(0.80, 0.44, 0.26), sky: vec3(0.46, 0.42, 0.55), ground: vec3(0.27, 0.22, 0.23),
			fog: vec3(0.83, 0.55, 0.45), fogRange: 420,
			skyTop: rl.NewColor(66, 74, 132, 255), skyLow: rl.NewColor(236, 150, 108, 255),
			night: 0.6,
		}
	case TimeDawn:
		return lighting{
			sunDir: norm3(0.82, 0.24, 0.4),
			sun:    vec3(0.70, 0.52, 0.44), sky: vec3(0.48, 0.47, 0.58), ground: vec3(0.28, 0.25, 0.26),
			fog: vec3(0.80, 0.68, 0.66), fogRange: 400,
			skyTop: rl.NewColor(88, 106, 162, 255), skyLow: rl.NewColor(238, 180, 156, 255),
			night: 0.45,
		}
	case TimeNight:
		return lighting{
			sunDir: norm3(0.34, 0.78, -0.42),
			sun:    vec3(0.17, 0.21, 0.33), sky: vec3(0.17, 0.20, 0.31), ground: vec3(0.10, 0.10, 0.13),
			fog: vec3(0.05, 0.06, 0.115), fogRange: 330,
			skyTop: rl.NewColor(6, 8, 20, 255), skyLow: rl.NewColor(22, 26, 48, 255),
			night: 1,
		}
	default:
		return lighting{
			sunDir: norm3(-0.45, 0.82, 0.35),
			sun:    vec3(0.52, 0.50, 0.46), sky: vec3(0.66, 0.70, 0.76), ground: vec3(0.44, 0.43, 0.41),
			fog: vec3(0.67, 0.77, 0.87), fogRange: 560,
			skyTop: rl.NewColor(96, 142, 198, 255), skyLow: rl.NewColor(176, 202, 226, 255),
		}
	}
}

func norm3(x, y, z float32) rl.Vector3 {
	l := float32(1) / sqrt32(x*x+y*y+z*z)
	return vec3(x*l, y*l, z*l)
}

// The scene's materials. Everything is drawn untextured in flat colours, so
// there is nowhere to say what a surface is made of except the colour itself:
// the top few alpha values are taken to name a material, and anything below
// them is ordinary transparency.
const (
	matPlain  uint8 = 255 // lit, and nothing else
	matFacade uint8 = 254 // a building wall, which grows windows
	matRoad   uint8 = 253 // asphalt, which is given some grain
	matGlow   uint8 = 252 // unlit: a lamp, a bulb, a light bar
	matPaint  uint8 = 251 // car paint, which picks up the sky
	matFloor  uint8 = 249 // the lowest value that names a material
)

// as returns a colour marked as the given material.
func as(c rl.Color, mat uint8) rl.Color { return rl.NewColor(c.R, c.G, c.B, mat) }

// lightingVS is the vertex shader used for the whole scene. It matches
// raylib's default attribute names so it works with immediate-mode drawing.
//
// Immediate mode applies the matrix stack on the CPU as each vertex is
// submitted, so positions and normals reach the shader already in world space
// and are passed straight through. The model matrix raylib offers is
// deliberately not used: it holds whatever the stack was when the batch
// happened to be flushed, which is the wrong thing to apply a second time.
const lightingVS = `#version 330
in vec3 vertexPosition;
in vec2 vertexTexCoord;
in vec4 vertexColor;
in vec3 vertexNormal;
uniform mat4 mvp;
out vec3 fragPos;
out vec4 fragColor;
out vec3 fragNormal;
void main() {
    fragPos = vertexPosition;
    fragColor = vertexColor;
    fragNormal = vertexNormal;
    gl_Position = mvp * vec4(vertexPosition, 1.0);
}`

// lightingFS lights the scene with a sun and a sky, fades it into the distance,
// and paints the two surfaces that would otherwise need textures: building
// walls get windows, and asphalt gets grain.
//
// The result is held just short of full brightness in every channel. Each of
// the vision system's reserved colours has at least one channel at 255, so no
// combination of light, fog and glow can ever land on one of them, and the
// scanner's exact match stays exact whatever time of day it is.
const lightingFS = `#version 330
in vec3 fragPos;
in vec4 fragColor;
in vec3 fragNormal;
uniform vec3 viewPos;
uniform vec3 sunDir;
uniform vec3 sunCol;
uniform vec3 skyCol;
uniform vec3 groundCol;
uniform vec3 fogCol;
uniform float fogRange;
uniform float night;
out vec4 finalColor;

float hash(vec2 p) { return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }

void main() {
    float mat = floor(fragColor.a * 255.0 + 0.5);
    vec3 base = fragColor.rgb;
    vec3 n = normalize(fragNormal + vec3(0.0, 1e-6, 0.0));
    vec3 glow = vec3(0.0);

    if (mat == 254.0 && abs(n.y) < 0.5) {
        float along = abs(n.x) > abs(n.z) ? fragPos.z : fragPos.x;
        vec2 face = floor(n.xz * 3.0) * 17.0;
        if (fragPos.y > 3.9) {
            // Windows: a grid across the wall, some of them lit once it is dark.
            vec2 at = vec2(along / 2.7, (fragPos.y - 3.9) / 3.2);
            vec2 cell = floor(at);
            vec2 f = fract(at);
            float pane = step(0.22, f.x) * step(f.x, 0.78) * step(0.26, f.y) * step(f.y, 0.76);
            float r = hash(cell + face);
            vec3 glass = mix(vec3(0.20, 0.25, 0.32), vec3(0.40, 0.48, 0.58), r) * (1.0 - 0.55 * night);
            base = mix(base, glass, pane);
            float lit = step(0.70, hash(cell * 1.7 + face + 4.0)) * pane;
            vec3 lamp = mix(vec3(0.96, 0.74, 0.42), vec3(0.62, 0.76, 0.94), step(0.88, r));
            glow = lamp * lit * night * (0.45 + 0.5 * r);
        } else if (fragPos.y > 0.5 && fragPos.y < 3.1) {
            // Shop fronts along the street: wide bays of glass, and after dark
            // some of them have left the lights on.
            vec2 at = vec2(along / 3.9, 0.0);
            float bay = floor(at.x);
            float f = fract(at.x);
            float pane = step(0.07, f) * step(f, 0.93);
            float r = hash(vec2(bay, 3.0) + face);
            vec3 glass = mix(vec3(0.17, 0.21, 0.27), vec3(0.30, 0.37, 0.45), r) * (1.0 - 0.5 * night);
            base = mix(base * 0.8, glass, pane);
            float lit = step(0.55, r) * pane;
            vec3 lamp = mix(vec3(0.94, 0.76, 0.48), vec3(0.72, 0.86, 0.92), step(0.9, r));
            glow = lamp * lit * night * 0.7;
        } else {
            base *= 0.8;
        }
    }
    if (mat == 253.0) {
        // Grain in the asphalt, close to. It is dropped with distance, where it
        // would only shimmer.
        float near = clamp(1.0 - length(fragPos - viewPos) / 45.0, 0.0, 1.0);
        float g = hash(floor(fragPos.xz * 9.0));
        base *= 1.0 + (g - 0.5) * 0.09 * near;
    }

    // A surface with no normal is not a surface but a pool of light on one:
    // it is drawn as it is, added to what lies under it, and thins with
    // distance instead of taking on the colour of the fog.
    float d = length(fragPos - viewPos);
    float fog = clamp(1.0 - exp(-pow(d / fogRange, 1.7)), 0.0, 1.0);
    if (dot(fragNormal, fragNormal) < 0.01) {
        finalColor = vec4(min(base, vec3(0.975)), fragColor.a * (1.0 - fog));
        return;
    }

    vec3 col;
    if (mat == 252.0) {
        col = base;
    } else {
        float lambert = max(dot(n, sunDir), 0.0);
        vec3 ambient = mix(groundCol, skyCol, n.y * 0.5 + 0.5);
        col = base * (ambient + sunCol * lambert) + glow;
        if (mat == 251.0) {
            // A little of the sky in the paintwork, strongest at a glancing angle.
            vec3 v = normalize(viewPos - fragPos);
            float rim = pow(1.0 - max(dot(n, v), 0.0), 3.0);
            col += skyCol * rim * 0.35 + sunCol * pow(max(dot(reflect(-sunDir, n), v), 0.0), 24.0) * 0.5;
        }
    }

    col = mix(col, fogCol, fog);

    float alpha = mat >= 249.0 ? 1.0 : fragColor.a;
    finalColor = vec4(min(col, vec3(0.975)), alpha);
}`

// sceneShader is the lighting shader and where its uniforms live.
type sceneShader struct {
	shader rl.Shader
	ok     bool
	// material is what a patch of ground is drawn with: the same shader.
	material rl.Material

	viewPos, sunDir, sunCol, skyCol, groundCol, fogCol, fogRange, night int32
}

func loadSceneShader() sceneShader {
	s := sceneShader{shader: rl.LoadShaderFromMemory(lightingVS, lightingFS)}
	s.ok = s.shader.ID != 0
	s.material = rl.LoadMaterialDefault()
	if !s.ok {
		return s
	}
	s.material.Shader = s.shader
	loc := func(name string) int32 { return rl.GetShaderLocation(s.shader, name) }
	s.viewPos, s.sunDir, s.sunCol = loc("viewPos"), loc("sunDir"), loc("sunCol")
	s.skyCol, s.groundCol, s.fogCol = loc("skyCol"), loc("groundCol"), loc("fogCol")
	s.fogRange, s.night = loc("fogRange"), loc("night")
	return s
}

// use hands the shader the light and the eye for the frame about to be drawn.
// reach scales the fog, so a view from high above is not looking at the city
// through a wall of it.
func (s sceneShader) use(l lighting, eye rl.Vector3, reach float32) {
	if !s.ok {
		return
	}
	v3 := func(loc int32, v rl.Vector3) {
		rl.SetShaderValue(s.shader, loc, []float32{v.X, v.Y, v.Z}, rl.ShaderUniformVec3)
	}
	v3(s.viewPos, eye)
	v3(s.sunDir, l.sunDir)
	v3(s.sunCol, l.sun)
	v3(s.skyCol, l.sky)
	v3(s.groundCol, l.ground)
	v3(s.fogCol, l.fog)
	rl.SetShaderValue(s.shader, s.fogRange, []float32{l.fogRange * reach}, rl.ShaderUniformFloat)
	rl.SetShaderValue(s.shader, s.night, []float32{l.night}, rl.ShaderUniformFloat)
}

// drawSky fills the frame with the sky before anything is drawn over it: a
// plain gradient from overhead down to the horizon colour the fog fades to.
func drawSky(l lighting, w, h int32) {
	rl.ClearBackground(l.skyLow)
	rl.DrawRectangleGradientV(0, 0, w, h*6/10, l.skyTop, l.skyLow)
}
