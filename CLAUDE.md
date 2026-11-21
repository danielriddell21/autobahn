# autobahn — Claude Code instructions

autobahn is a 3D driving game in Go: procedurally generated British places — a
city, a seaside town, a village and a motorway town — with traffic, signals,
road rules and police, and an autopilot that sees the world only through a
camera. It is built on raylib, not Ebiten — the only game in the
family that is — and on [crucible](https://github.com/danielriddell21/crucible)
for everything that is engine rather than game.

## Before every commit

* Run `just ci` autonomously (lint + test + build + the autopilot boundary
  check + `just gate`, which is `eval` with a pass mark). All must pass.
* When adding a function or package, write unit tests alongside the code in the
  same commit.
* Never commit until the user explicitly confirms. Propose changes as diffs,
  run `just ci` autonomously, then stop and wait before `git commit`.

## Build tags

raylib needs a windowing backend. On Linux `-tags x11` builds against X11
alone, avoiding the Wayland development headers; macOS and Windows need no tag.
The `justfile` picks the right one, so prefer `just build` / `just test` over
bare `go` commands. A bare `go test ./...` will fail to link on Linux.

Linking also needs the X11 and xkbcommon development headers
(`libxkbcommon-dev`, `libx11-dev`, `libgl1-mesa-dev` on Debian).

## The autopilot boundary

The autopilot must not be able to read simulation state. It depends on
`internal/vision` and `internal/mathx` and nothing else from this repo, so the
compiler enforces what would otherwise be an honour system: if it can reach
`internal/sim`, it is no longer driving on what it can see.

`just boundary` proves it, and `just ci` runs it. Never add an import to
`internal/autopilot` that reaches the world — pass the information through a
`vision.Detection` instead, or accept that the autopilot cannot know it.

`internal/annotate` is the other half: it draws what the autopilot is allowed
to perceive, in reserved colours no material in the scene uses. Both the live
window and the headless harness scan the same boxes, which is what lets `eval`
score the autopilot with no GPU.

## The city grows, and never changes its mind

`internal/city` is built outward as the car drives, and has no extent. The rule
that makes that work: **everything about a place is a function of where that
place is**, never of when it was reached or of a running random stream. A
junction arrived at from the north an hour into a drive has to come out
identical to the same junction arrived at from the south at the start.
`cellChance` and `Node.Chance` are how; `grow_test.go` holds it to the property
by comparing a city reached in four passes against the same ground built in one.

The corollary is that nothing may be revised once built. A junction moves
forward through reached → settled → wired and never back, so anything that
depends on a junction's surroundings waits for them to exist rather than
guessing and correcting later. The commentary at the top of `grow.go` sets it out.

Two things follow that are easy to trip over. A growing city always has a
frontier of unfinished junctions, so anything walking the whole city has to skip
those that are not `City.Complete`. And `EnsureAround` is called every frame, so
it must cost nothing when the car has not reached new ground.

## Maps are presets, never generators

There are four maps, and each is `internal/city` asked for something
different: `city.ParamsFor` is the only place their generation differs. A map
also has a season, a traffic mix (`mixes` in `internal/sim`) and a look
(`townFor` and `lookFor` in `internal/game`). Nothing about a map may be a
special case inside the generator; if a new place needs something the
generator cannot do, it becomes a parameter, as the coast, the speed limits
and the spacing of the main roads did.

That is what keeps the growth rule cheap to hold. `grow_test.go` runs its
comparison for every map, so a new one is held to it the moment it exists.

Two things about the existing maps look like mistakes and are not:

* `ParamsFor(MapCity)` must stay equal to `DefaultParams`. The city grades
  its grid lines lopsidedly, with no collectors below zero, and the gate's
  numbers are measured on that. The other maps set `Balanced`.
* The motorway town is nearly all traffic lights, and the village nearly all
  give ways. A roundabout is where the autopilot comes to grief: a motorway
  town built of them scored five to nine penalty points against one with
  lights, and the village with many put the gate's own seeds within a point
  of its limit.

## The plan and the ground

Every map but the city is laid on the ground differently from the way it was
planned. `internal/city` still builds a plan of straight streets on a grid,
exactly as it does for the city; then each junction is moved and turned by a
smooth field, and each street run between its two junctions as a curve that
leaves both square. `lay.go` is that mapping and `ground.go` the city that
comes of it. Three things about it are easy to get wrong:

* A place laid this way is two cities under the same numbers. `City.Plan()` is
  the plan, where every choice is made and every row of anything is straight;
  the city itself is the ground, which is what `sim` drives on and collides
  with. Lanes, junctions, buildings, signs and parked cars are laid for it.
  Anything that only has to be drawn is worked out on the plan by whoever
  draws it and put on the ground with `City.Lay`: stood, if it stands up, so
  it keeps its shape, or laid corner by corner if it lies flat. `laid.go` in
  `internal/game` has both.
* A lane is not a straight line. `Fwd` and `Heading` are the way it runs at
  its far end, which is what a junction wants. Anywhere else ask `DirAt`,
  `HeadingAt`, `Locate` or `Run`; `p.Sub(l.A).Dot(l.Fwd)` is wrong on a bend.
* Where a point lies is a function of where it is on the plan and nothing
  else, so the growth rule still holds: `grow_test.go` compares laid cities
  too, away from their frontier.

A junction is rigid, and so is everything within its arms. Near a street the
ground is laid exactly alongside it, which is what keeps a lane its width
round a bend; out in the middle of a block it is laid between the block's
corners, less exactly. The land does not wander by the sea.

Over 120 seeds the autopilot scores the same on the curved village as on the
straight one, about four penalty points. Twenty-four seeds are not enough to
tell the two apart, in either direction.

## Height

Every map but the city stands on hills. `city.Height` says how high the
ground is at a point, and it is a function of where the point is on the
ground and nothing else, so it costs a few sines and the growth rule holds
without anyone trying.

The hills are long and low on purpose, and `height_test.go` holds them to
it. The autopilot ranges things from how far down the picture they sit. Its
camera is fixed to the car and tips with it (`annotate.BonnetView` takes a
`city.Stand`), so that is right on any slope the car and the thing share;
what throws it is the road curving up or down between them. On the motorway,
where it has to judge sixty metres ahead, swells 1500 m long put collisions
up from five in 120 seeds to thirteen, and 2600 m put them back. The village
and the seaside, which are slower, scored no worse on hills than on the flat.

A car is still driven on the plane. The hill reaches it as `Vehicle.Grade`,
a pull along its length, and a brake that holds against it: without `held`
a stopped car creeps a millimetre a second down the hill and over its stop
line. Nothing is level, junctions included, and nothing has to be.

Drawing is where it costs. Anything standing is lifted by a matrix
(`standOn`, `pushUp`, `footingOf` for a car, which also tips it); anything
lying flat takes the land at each corner (`flat`, and the `ground…` shapes in
`prim.go`, which take a `land`). A building is stood level with the street at
its door and carried down on a plinth.

Ground that has a shape is not drawn corner by corner, which cost more than
the rest of the frame: it is made once and handed to the graphics card as a
`patch`. `terrain.go` tiles the road surface and the grass beyond;
`relief.go` makes each block a net of ground stitched to its kerb. A patch
keeps its list of triangles after it is uploaded. Without one the card draws
its corners three at a time as they come, and the result is shards.

On top of the land, open ground rises on its own account: a field, a green,
the middle of a block behind its houses. While a block is being dressed
`Game.rise` is its net, and `risen.at` is the height of the net as drawn,
not of the hill it hangs on, because on a bank the two are half a metre
apart. Paths and crop rows are baked into the block's patch.

The hills on the horizon are painted on the sky, outside the shader, so
`ridgeShade` keeps them off the vision system's colours by its own
arithmetic. They are the same from every street, which is right for
something a long way off in a place with no edge.

The camera's near plane is fitted to its height (`clipFor`). From a few
hundred metres up, starting at arm's length, a kerb and the road under it
are the same depth, and thin triangles flicker.

## Seasons

A map is driven in one season and keeps it. The season dresses the scene and,
in winter, takes grip away: `sim` fits every vehicle to the road it is on, and
traffic drives to it. The autopilot is told nothing, because it could not see
it. Whatever falls from the sky is stored nowhere: each piece is worked out
from its number and the clock.

Anything a season or a map draws must keep off the vision system's colours,
and must never be made brighter than it was defined. A pale colour scaled past
full wraps round, and came out cyan once; `crownShade` and the season tests
are there because of it.

## Dressing

Roofs, shop fronts, street furniture, park paths, fields and the beach are
worked out in `internal/game` from where they stand, the way the trees always
were, and none of it is annotated. Parked cars are the exception: they are
solid, so they live in `internal/city` and `sim` collides with them.

## Packs

`internal/pack` groups maps and garage cars into the base game and the packs
that add to it. A map or a car no pack lists belongs to the base game, so
nothing is locked by accident. A pack unlocks when the bounty banked over a
player's career reaches its price, and only a pursuit the player drove all of
counts: `bountyMeter` marks one the autopilot touched. The menus never offer
what is locked; `-unlock-all` and the tools bypass it.

## eval is a gate, not a report

`just eval` drives many cities headlessly and scores the result. `just gate`
is the same thing with a pass mark, on every map, and runs in `just ci` and in
the workflow, so a change that makes the autopilot worse anywhere fails the
build. Keep the two pass marks the same.

Its numbers are noisy: judge a change over at least twenty seeds, and remember
that changing city generation changes which cities the seeds produce, so a
before-and-after on the same seed is not a comparison. Compare distributions.

## Junction geometry is what makes the city drivable

A junction's box reaches past its carriageways by `city.KerbRadius`, lanes stop
at the edge of the box, and the kerbs are rounded to match. That radius is what
gives the turn toward the kerb an arc a car can follow; with square corners it
had a radius under two metres and nothing could make it. Lane 0 is the lane by
the centre line whichever side traffic keeps to, so on a two-lane approach the
turn toward the kerb is made from the last lane and the turn across the road
from lane 0.

Inside the box the nearest lane says nothing about where a car should be. The
route planner, the judge and the traffic all have to ask whether they are in a
junction before they trust `City.Project`, and the tests that hold them to it
are the ones about keeping a plan through a junction and not driving the wrong
way across one.

## Drawing

The scene is flat colours through one shader (`internal/game/light.go`). Two
conventions in it are load-bearing:

* The top few alpha values name a material rather than a transparency — a
  façade grows windows, asphalt gets grain, a glow is unlit. Mark a colour with
  `as(col, mat…)`; anything below `matFloor` is ordinary alpha.
* The shader holds every channel just short of full brightness. Each of the
  vision system's reserved colours has a channel at 255, so no light, fog or
  glow can land on one. Do not draw anything in the 3D scene outside the shader.

Immediate mode hands the shader world-space positions and normals. Do not use
the model matrix raylib offers: it is whatever the matrix stack held when the
batch flushed.

## What belongs in crucible

Engine concerns live in crucible and are imported, not reimplemented here. It
already supplies the street lattice under `internal/city`, the spatial index,
the menus, the two-player session, the HUD overlay, the recorder and the demo
toolkit, procedural audio, telemetry and the control-hint bar.

When something here starts to look general, the test is crucible's own: if two
repos in the family carry the same display-free logic, extract it; if it is
nearly the same but carries app-defined values, make the engine side generic
and let each app keep its vocabulary; otherwise leave it here and say why. A
lane graph, the road rules and the vehicle dynamics are this game's, and stay.

## Units and conventions

* The world is the XZ ground plane with +Y up, matching raylib's right-handed
  convention. `internal/mathx` is float32 throughout; crucible's `geom` is
  float64, so convert at the boundary rather than spreading casts.
* Speeds are metres per second internally. British limits are posted in miles
  per hour, so `mathx.MPH` and `ToMPH` bracket anything a player sees.
* Anything random takes an explicit seed and draws from a named crucible
  stream, so adding a draw in one subsystem never disturbs another.

## Media

`just demo` regenerates `docs/media` through `tools/demogen`, driving real
sessions rather than staging anything. It needs a GPU context, so on a headless
machine run it under `xvfb-run -a`.
