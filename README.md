# autobahn

> *autobahn* — a quarter mile at a time. The autopilot manages eleven miles an hour.

[![CI](https://github.com/danielriddell21/autobahn/actions/workflows/ci.yaml/badge.svg)](https://github.com/danielriddell21/autobahn/actions/workflows/ci.yaml)
[![codecov](https://codecov.io/gh/danielriddell21/autobahn/graph/badge.svg)](https://codecov.io/gh/danielriddell21/autobahn)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=danielriddell21_autobahn&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=danielriddell21_autobahn)
[![Go 1.26](https://img.shields.io/badge/go-1.26-blue)](https://go.dev)
[![MIT License](https://img.shields.io/badge/licence-MIT-green)](LICENSE)

A 3D driving game in Go. Part one is you, a car, and a procedurally generated
British city with traffic, lights, signs, road rules and police. Part two hands
the same car to an autopilot that sees the world **only through a camera**, as
colour-coded detection boxes it has to read back out of the pixels.

Both drivers are scored by the same judge, so the comparison is honest.

Two people can play: one hosts a chase and drives the runner, the other joins
over the network and gives chase in a police car. Set it up from the title
screen, or with `-host` and `-join`.

![The autopilot driving the city](docs/media/drive.gif)

*The autopilot at the wheel. The panel in the corner is the image it is reading.*

## Install

### Homebrew (macOS)
```sh
brew install --cask danielriddell21/tap/autobahn
```

### From source
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

The city has no edge. It is built outward as you drive into it, a junction at a
time, and everything about a place is decided from where that place is rather
than from when you got there — so the town behind you is still the town you
left, and there is always more road ahead.

The engine underneath is [crucible](https://github.com/danielriddell21/crucible),
shared with the rest of the family: the city's street plan comes from its
world generator, the menus from its menu model, the two-player session from
its netplay package, and the recording flags are the same `-record` every
other app answers to.

## Documentation

Full documentation lives in the [autobahn wiki](https://github.com/danielriddell21/autobahn/wiki) —
the city and its rules, the camera-only autopilot, pursuit mode, the synthesised
sound, and how the media above is generated.
