package main

import (
	"strings"
	"testing"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/game"
	"github.com/danielriddell21/autobahn/internal/sim"
)

func TestParseVersionAnswersWithoutAskingForAGame(t *testing.T) {
	was := version
	t.Cleanup(func() { version = was })
	version = "v1.2.3"

	var out strings.Builder
	_, done, err := parse("autobahn", []string{"-version"}, &out)
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if !done {
		t.Error("parse() done = false, want true: -version must not start a game")
	}
	if got, want := out.String(), "autobahn v1.2.3\n"; got != want {
		t.Errorf("parse() wrote %q, want %q", got, want)
	}
}

func TestParseAppliesFlagsAndRecordsWhichWereGiven(t *testing.T) {
	opts, done, err := parse("autobahn", []string{
		"-seed", "7", "-traffic", "12", "-police", "0",
		"-camera", "bonnet", "-autopilot", "-frames", "30",
	}, &strings.Builder{})
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if done {
		t.Fatal("parse() done = true, want false")
	}

	if opts.Seed != 7 {
		t.Errorf("Seed = %d, want 7", opts.Seed)
	}
	if opts.Traffic != 12 {
		t.Errorf("Traffic = %d, want 12", opts.Traffic)
	}
	if opts.Camera != game.CameraBonnet {
		t.Errorf("Camera = %v, want CameraBonnet", opts.Camera)
	}
	if !opts.Autopilot {
		t.Error("Autopilot = false, want true")
	}
	if opts.Frames != 30 {
		t.Errorf("Frames = %d, want 30", opts.Frames)
	}

	// Given is what lets stored settings know which flags the player chose.
	for _, name := range []string{"seed", "traffic", "police", "camera", "autopilot", "frames"} {
		if !opts.Given[name] {
			t.Errorf("Given[%q] = false, want true", name)
		}
	}
	if opts.Given["width"] {
		t.Error(`Given["width"] = true, want false: -width was not given`)
	}
}

func TestParseDefaultsToTheChaseCamera(t *testing.T) {
	opts, _, err := parse("autobahn", nil, &strings.Builder{})
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if opts.Camera != game.CameraChase {
		t.Errorf("Camera = %v, want CameraChase", opts.Camera)
	}
}

func TestParseRejectsCommandLinesTheGameCannotHonour(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"both ends of a chase", []string{"-host", ":7777", "-join", ":7777"}, "give -host or -join, not both"},
		{"a viewpoint that does not exist", []string{"-camera", "periscope"}, `unknown camera "periscope"`},
		{"an autopilot mode that does not exist", []string{"-mode", "reckless"}, `unknown mode "reckless"`},
		{"a time of day that does not exist", []string{"-time", "teatime"}, `unknown time "teatime"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parse("autobahn", tt.args, &strings.Builder{})
			if err == nil {
				t.Fatalf("parse(%q) error = nil, want %q", tt.args, tt.want)
			}
			if err.Error() != tt.want {
				t.Errorf("parse(%q) error = %q, want %q", tt.args, err, tt.want)
			}
		})
	}
}

func TestParseReadsTheTimeOfDay(t *testing.T) {
	for arg, want := range map[string]game.TimeOfDay{
		"day": game.TimeDay, "dusk": game.TimeDusk, "night": game.TimeNight,
	} {
		opts, _, err := parse("autobahn", []string{"-time", arg}, &strings.Builder{})
		if err != nil {
			t.Fatalf("parse(-time %s) error = %v", arg, err)
		}
		if opts.Time != want {
			t.Errorf("-time %s gave %v, want %v", arg, opts.Time, want)
		}
		if !opts.Given["time"] {
			t.Errorf("-time %s was not recorded as given", arg)
		}
	}
	opts, _, err := parse("autobahn", nil, &strings.Builder{})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Time != game.TimeDay {
		t.Errorf("with no -time the light is %v, want day", opts.Time)
	}
}

func TestParseReadsTheAutopilotMode(t *testing.T) {
	opts, _, err := parse("autobahn", []string{"-mode", "getaway"}, &strings.Builder{})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Mode != autopilot.ModeGetaway || !opts.Given["mode"] {
		t.Errorf("-mode getaway gave %v (given %v)", opts.Mode, opts.Given["mode"])
	}
}

func TestParseCarAndPaint(t *testing.T) {
	opts, _, err := parse("autobahn", []string{"-car", "estate", "-paint", "teal"}, &strings.Builder{})
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if opts.Car != sim.Estate || !opts.Given["car"] || !opts.Given["paint"] {
		t.Errorf("Car = %v, Given = %v", opts.Car, opts.Given)
	}
	if _, _, err := parse("autobahn", []string{"-car", "tank"}, &strings.Builder{}); err == nil {
		t.Error("an unknown car should be refused")
	}
}
