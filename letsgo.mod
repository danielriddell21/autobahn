// letsgo.mod

// darwin only, as the GoReleaser config published.
//
// That restriction used to be forced: raylib needed cgo, and a Linux clang
// cannot cross-build a darwin binary, so the release ran on a macOS runner.
// It is no longer forced — raylib-go 0.60 goes through purego and embeds the
// shared library for each platform, so all five targets cross-compile with
// CGO_ENABLED=0 from any runner. Widening the published list is a decision
// about what this game supports, not part of moving release tools, so it is
// left alone here.
build (
	darwin/amd64
	darwin/arm64
)

// The shared GoReleaser workflow marked releases as pre-releases after
// publishing; letsgo does it while publishing, so promote.yaml still fires on
// manual promotion.
release prerelease=true
