# autobahn — Claude Code instructions

autobahn is a 3D driving game in Go: a procedurally generated British city with
traffic, signals, road rules and police, and an autopilot that sees the world
only through a camera. It is built on raylib, not Ebiten — the only game in the
family that is — and on [crucible](https://github.com/danielriddell21/crucible)
for everything that is engine rather than game.

## Before every commit

* Run `just ci` autonomously (lint + test + build + the autopilot boundary
  check + `eval`). All must pass.
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

## eval is a gate, not a report

`just eval` drives many cities headlessly and scores the result. It runs in
`just ci`, so a change that makes the autopilot worse fails the build.

Its numbers are noisy: judge a change over at least twenty seeds, and remember
that changing city generation changes which cities the seeds produce, so a
before-and-after on the same seed is not a comparison. Compare distributions.

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
