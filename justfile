# Autobahn — a British driving game with a camera-driven autopilot.
#
# raylib needs a windowing backend on Linux. The x11 tag builds against X11
# alone, which avoids requiring the Wayland development headers.

tags := if os() == "linux" { "x11" } else { "" }
go := "go"

# list available recipes
default:
    @just --list

# build the game
[group('build')]
build:
    {{go}} build -tags "{{tags}}" ./...

# run the game
[group('build')]
run *ARGS:
    {{go}} run -tags "{{tags}}" ./cmd/autobahn {{ARGS}}

# hand the car to the autopilot
[group('build')]
drive *ARGS:
    {{go}} run -tags "{{tags}}" ./cmd/autobahn -autopilot {{ARGS}}

# run the tests
[group('test')]
test:
    {{go}} test -tags "{{tags}}" ./...

# run the tests with the race detector
[group('test')]
race:
    {{go}} test -race -tags "{{tags}}" ./...

# run the benchmarks
[group('test')]
bench:
    {{go}} test -bench=. -benchmem -tags "{{tags}}" ./...

# score the autopilot across many cities, with no display needed
[group('test')]
eval *ARGS:
    {{go}} run ./tools/eval {{ARGS}}

# prove the autopilot cannot read simulation state
[group('test')]
boundary:
    #!/usr/bin/env bash
    set -euo pipefail
    deps=$({{go}} list -tags "{{tags}}" -deps ./internal/autopilot | grep autobahn || true)
    echo "$deps"
    if echo "$deps" | grep -qE '/internal/(sim|city|game|annotate)$'; then
        echo "FAIL: autopilot reaches simulation state" >&2
        exit 1
    fi
    echo "OK: autopilot sees only vision and mathx"

# run the linter
[group('dev')]
lint:
    golangci-lint run --build-tags "{{tags}}"

# format the code
[group('dev')]
fmt:
    golangci-lint fmt

# tidy module dependencies
[group('dev')]
tidy:
    {{go}} mod tidy

# build the documentation media into docs/media
[group('dev')]
demo *ARGS:
    {{go}} run -tags "{{tags}}" ./tools/demogen {{ARGS}}

# list the media demogen can build
[group('dev')]
demo-list:
    {{go}} run -tags "{{tags}}" ./tools/demogen -list

# drive headlessly and print a session summary
[group('dev')]
soak frames="1800":
    {{go}} run -tags "{{tags}}" ./cmd/autobahn -autopilot -frames {{frames}} -stats

# full gate: lint + test + build + boundary. all must pass before committing
[group('dev')]
ci: lint test build boundary eval
