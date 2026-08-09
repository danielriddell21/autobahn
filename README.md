# autobahn

*n.* a road built for driving on. Also: a British city that only exists to be driven through.

A 3D driving game in Go. Part one is you, a car, and a procedurally generated
British city with traffic, lights, signs and road rules. Part two hands the same
car to an autopilot that sees the world **only through a camera**, as
colour-coded detection boxes it has to read back out of the pixels.

Both drivers are scored by the same judge, so the comparison is honest.

![Go 1.26](https://img.shields.io/badge/go-1.26-blue)

## Running it

```sh
just run              # drive it yourself
just drive            # hand the car to the autopilot
just ci               # lint + test + build + the boundary check
```

Or without `just`:

```sh
go run -tags x11 ./cmd/autobahn            # the x11 tag is Linux-only
go run -tags x11 ./cmd/autobahn -autopilot
```

raylib needs a windowing backend. On Linux, `-tags x11` builds against X11
alone, which avoids needing the Wayland development headers; on macOS and
Windows no tag is required. The `justfile` picks the right one for you.

Everything is generated in code — geometry, layout, materials. There are no
asset files, nothing is downloaded at runtime, and the whole thing runs offline.

## Controls

| Key | |
|---|---|
| `W` / `S` | throttle, brake (and reverse once stopped) |
| `A` `D` | steer |
| `Space` | handbrake — it will break traction and slide |
| `Tab` | **hand the car to the autopilot, and take it back** |
| `C` | chase / bonnet / overhead camera |
| `V` | show or hide the AI camera panel |
| `L` | class labels on the detection boxes |
| `N` | drop back onto the nearest lane |
| `R` | return to the start |
| `P` `H` | pause, help |

## Part one: driving

The city is a seeded grid with varied block spans, arterials every third line,
and buildings that grow taller toward the middle. Reroll it with `-seed`.

The car is a bicycle model with slip angles, so it has weight transfer, it
understeers when you ask too much of the front, and the handbrake kills rear
grip and rotates it. Traffic runs the Intelligent Driver Model along the lane
graph: it queues, follows, indicates, gives way, and stops at red lights.

It is British, so:

- traffic keeps **left**, and it is the **right** turn that crosses oncoming traffic
- limits are posted in **mph** — 20 on residential streets, 30 built-up, 40 on the main roads
- signals run **red → red and amber → green → amber**, and red-and-amber is *not* permission to go
- **give way** is the common priority marking, with stop signs the rarer case
- centre lines are **white**; the yellow paint is double yellows against the kerb
- zebra crossings, pavements and kerbs, because you will end up on one

### The rules it judges you on

A judge watches your car and logs what you break: running a red, failing to stop,
speeding, driving on the wrong side, leaving the carriageway, and collisions
(with severity from the impact speed). Each fault carries penalty points.

The judge does not care whether a human or the autopilot is driving.

## Part two: the AI drives

Press `Tab`. The car keeps driving; nothing else changes.

Every frame the game renders a second view from a bonnet-mounted camera into an
off-screen texture, and paints colour-coded boxes over everything relevant — one
colour per class, in the style of a security camera running object detection.
That image is then **read back a pixel at a time** and the boxes are recovered by
flood-filling runs of each exact class colour. The result is a list of
detections: a class and a rectangle. That is all the autopilot ever receives.

The panel in the corner is not a visualisation of the AI's state. It is the
actual image being scanned. What you see is what it gets.

### The classes

| | | | |
|---|---|---|---|
| 🟥 vehicle | 🟪 light: red | 🌸 light: red+amber | 🟧 light: amber |
| 🟩 light: green | 🟨 stop | 🩵 give way | 🟦 lane |
| 🟢 lane (alt) | 🔵 limit 20 | 🩵 limit 30 | 🟣 limit 40 |
| 🟩 stop line | | | |

Every class colour is a fully saturated combination of 0, 128 and 255. Nothing
in the world's palette uses one, so an exact pixel match cannot produce a false
positive — a fact the tests check.

Consecutive lane markers **alternate** between two colours. Once they are far
enough away to be a few pixels across their boxes touch, and two runs of one
colour would flood-fill into a single meaningless blob.

### What the autopilot actually knows

Being precise about this, because it is the whole point:

- **From pixels**: every vehicle, signal, sign, stop line and lane marker, as a
  class and a screen rectangle. Range comes from where a box's bottom edge
  falls, intersected with the road plane — or, for a signal, with its known
  mounting height. Bearing comes from the column.
- **From odometry**: its own road speed. Every real car has this.
- **From navigation**: which way to turn at the next junction. That is a satnav
  decision, not a perception one. It reaches the autopilot *through the camera*
  as the lane markers it must still visually track.
- **Nothing else.** It cannot read the simulation.

That last claim is enforced by the compiler, not by good intentions. Package
`autopilot` imports `vision` and nothing else from this project, and `vision` is
pure — it depends only on the standard library and `mathx`. The annotator, which
does know about the world, lives in a separate package that the autopilot cannot
reach:

```sh
just boundary
# github.com/danielriddell21/autobahn/internal/mathx
# github.com/danielriddell21/autobahn/internal/vision
# github.com/danielriddell21/autobahn/internal/autopilot
# OK: autopilot sees only vision and mathx
```

### How it drives

Pure pursuit on the lane markers for steering; a target-speed governor feeding
throttle and brake for everything else. It obeys the tightest constraint among
the speed limit it last read, the curvature ahead, the car in front, and any
junction it can see.

Three behaviours exist because the naive version got caught out, and each is a
fix for a failure that actually happened during testing:

- **Signals are ranged directly** off their mounting height, because the stop
  line painted at their feet is routinely hidden by the car waiting on it.
- **A stop line is dead-reckoned** on odometry once it leaves view, so it
  survives occlusion and the final metres where it drops below the lens.
- **Junctions are approached at a speed the car can stop from** unless green is
  actually confirmed. Without this it would arrive at a green light too fast to
  stop when it changed — the dilemma zone — and run the red.

Amber means stop unless stopping would be unsafe, which is the rule as written.

### Seeing how it did

```sh
just soak 1800     # drive headlessly, then print a summary
```

```
--- autopilot session ---
  drove      342 m in 36 s (avg 21 mph over 2000 frames)
  perception 30.3 detections/frame, saw lane markers to 72 m
  penalty    0 points from 0 faults
```

`-record drive.gif` captures the drive. `-debugvision` prints the camera's range
estimates against the simulation's ground truth, which is how the camera model
was validated: **15.0 m estimated against 16.2 m actual**, from pixels alone.

## Built on crucible

This game is a raylib app, so the Ebiten-facing half of
[crucible](https://github.com/danielriddell21/crucible) (`window`, `menu`,
`camera`) does not apply. Everything display-free does, and is used:

| Package | Used for |
|---|---|
| `rng` | Every random draw. Layout, traffic, signal offsets and routing each take a named stream, so adding one system never disturbs another's sequence. |
| `ring` | The infraction history, bounded so a long session cannot grow it without end. |
| `telemetry` | The judge publishes each infraction on a bus, generic over this game's own event type. |
| `hud` + `status` | Those events become the on-screen fault toast, wired judge → bus → status source → overlay. |
| `keymap` | The controls panel, wrapped to the available width. |
| `paint` | Colour dimming for façades, roofs and brake lights. |
| `record` | `-record drive.gif` — its `Add(image.Image)` is renderer-agnostic, so it works behind raylib unchanged. |

### What crucible would need to cover more

Answering the question directly — all of these are additive and
backwards-compatible:

1. **`geom` cannot serve this game's vector math.** `Vec2` is `float64` and
   models an XY plane; a driving sim wants `float32` on the XZ ground plane with
   a heading-relative perpendicular. Hence `internal/mathx`. Adding a `Perp`
   (or `Right`) method, `WrapPi`, and an angle-lerp would cover most of the gap;
   making `Vec2` generic over the float type via a generic type alias would
   close it entirely without touching existing call sites.
2. **`geom` has no oriented-box overlap.** The separating-axis test with a
   penetration axis and depth — what any collision response needs — is in
   `mathx.OBB`. It is small, general, and would sit naturally in `geom`.
3. **`menu` is the one real blocker.** Its model (items, selection, settings) is
   renderer-agnostic, but it imports Ebiten for input polling, so a raylib app
   cannot use any of it. Splitting the state machine from the input adapter
   would make the model reusable by any front-end, and existing callers would
   keep the same API.
4. **`worldgen` is dungeon-shaped.** BSP rooms and corridors do not describe a
   road grid. A weighted-choice and flood-fill helper is all this game would
   have borrowed, and both are already there — the layout code is genuinely
   app-specific and should stay here.

## Layout

```
cmd/autobahn        entrypoint and flags
internal/mathx      ground-plane vectors, oriented boxes, units
internal/city       procedural layout: roads, lanes, junctions, signs, buildings
internal/sim        vehicle dynamics, traffic, signals, routing, the judge
internal/vision     detection classes, camera model, the pixel scanner  (pure)
internal/annotate   draws the boxes  (knows the world; the autopilot cannot see it)
internal/autopilot  the camera-driven driver  (imports vision, nothing else)
internal/game       rendering, HUD, input, the two driving modes
```

`sim` does not import raylib, so the simulation steps headlessly under test.

## Testing

```sh
just test
just boundary   # prove the autopilot cannot reach simulation state
```

The scanner and the autopilot are pure Go and test without a GPU. The autopilot
tests build synthetic detections at known ranges and assert on behaviour: it
stops for red, and for red-and-amber; it does not stop for green; it stops for a
red whose stop line is occluded; it stops for an amber it can stop for and
commits to one it cannot; it slows for the car in front but ignores the next
lane; it steers toward the lane and reads limits from signs; and it crawls when
it cannot see the road at all.

## Known limitations

- A vehicle box drawn over a stop line splits it into several detections, since
  the outline is no longer connected. The autopilot takes the nearest fragment,
  which is the safe reading. It is genuine occlusion and behaves like it.
- Traffic is recycled outside a radius of the player rather than simulated
  city-wide, so the far side of the map is empty until you drive there.
- Roads are axis-aligned. No roundabouts, which for a British city is a notable
  omission and the most obvious thing to build next.
- Pedestrians exist only as the crossings they would use.

## Licence

MIT.
