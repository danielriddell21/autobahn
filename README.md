# autobahn

> *autobahn* — a quarter mile at a time. The autopilot manages fifteen miles an hour, and never gets a ticket.

[![CI](https://github.com/danielriddell21/autobahn/actions/workflows/ci.yaml/badge.svg)](https://github.com/danielriddell21/autobahn/actions/workflows/ci.yaml)
[![codecov](https://codecov.io/gh/danielriddell21/autobahn/graph/badge.svg)](https://codecov.io/gh/danielriddell21/autobahn)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=danielriddell21_autobahn3&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=danielriddell21_autobahn3)
[![Go 1.26](https://img.shields.io/badge/go-1.26-blue)](https://go.dev)
[![MIT License](https://img.shields.io/badge/licence-MIT-green)](LICENSE)

A 3D driving game in Go. Part one is you, a car, and a procedurally generated
British city with traffic, lights, signs, road rules and police. Part two hands
the same car to an autopilot that sees the world **only through a camera**, as
colour-coded detection boxes it has to read back out of the pixels.

| Key | |
| --- | --- |
| `W` `A` `S` `D` | drive |
| `Space` | handbrake |
| `Shift` | nitrous, which a slide earns back |
| `Tab` | hand the car to the autopilot, or take it back |
| `C` | camera |
| `T` | time of day: day, dusk, night |
| `H` | the rest |

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
just ci               # lint + test + build + the boundary check + the autopilot gate
```

Without `just`, Linux needs `-tags x11`: `go run -tags x11 ./cmd/autobahn`.

Everything is generated in code, with no asset files, and the city has no edge:
it is built outward as you drive. The engine underneath is
[crucible](https://github.com/danielriddell21/crucible).

## Documentation

Full documentation lives in the [autobahn wiki](https://github.com/danielriddell21/autobahn/wiki) —
the city and its rules, the camera-only autopilot, pursuit mode, the synthesised
sound, and how the media above is generated.
