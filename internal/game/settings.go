package game

import (
	"fmt"

	"github.com/danielriddell21/crucible/geom"
	"github.com/danielriddell21/crucible/store"
)

// settingsFile is the document crucible's store keeps under the user's config
// directory, following the family's one-file-per-document convention.
const settingsFile = "settings.json"

// appName is the directory the settings live in.
const appName = "autobahn"

// Settings is what the settings screen adjusts and the store remembers between
// sessions. It is deliberately small: the things a player changes once and
// expects to stay changed, not every flag the command line offers.
type Settings struct {
	// Volume is the master output level, 0 to 1.
	Volume float64 `json:"volume"`
	// Muted silences everything without forgetting the volume.
	Muted bool `json:"muted"`
	// ShowPanel draws the AI camera feed, which appears only while the
	// autopilot is driving — there is nothing looking through it otherwise.
	// ShowLabels writes each detection's class beside its box.
	ShowPanel  bool `json:"showPanel"`
	ShowLabels bool `json:"showLabels"`
	// Traffic and Police size the world a new session starts with.
	Traffic int `json:"traffic"`
	Police  int `json:"police"`
	// Camera is the viewpoint a new session opens on.
	Camera CameraMode `json:"camera"`
}

// DefaultSettings returns the settings a first run starts with.
func DefaultSettings() Settings {
	return Settings{
		Volume: 0.7, ShowPanel: true, ShowLabels: true,
		Traffic: 70, Police: 5, Camera: CameraChase,
	}
}

// The ranges the settings screen adjusts within, and how far one press moves.
const (
	trafficStep, trafficMin, trafficMax = 10, 0, 200
	policeStep, policeMin, policeMax    = 1, 0, 40
	volumeStep                          = 0.05
)

// clamp brings a loaded document back into range. A settings file is a plain
// JSON document a player may have edited, so nothing read from it is trusted
// to be sensible.
func (s *Settings) clamp() {
	s.Volume = geom.Clamp(s.Volume, 0, 1)
	s.Traffic = geom.Clamp(s.Traffic, trafficMin, trafficMax)
	s.Police = geom.Clamp(s.Police, policeMin, policeMax)
	if s.Camera < 0 || s.Camera >= numCameraModes {
		s.Camera = CameraChase
	}
}

// settingsPath resolves where the document lives, or reports why it cannot.
func settingsPath() (string, error) {
	p, err := store.Path(appName, settingsFile)
	if err != nil {
		return "", fmt.Errorf("locating the settings file: %w", err)
	}
	return p, nil
}

// LoadSettings reads the player's settings, falling back to the defaults for
// anything missing or unreadable. A first run, a corrupt file and a deleted
// config directory all land on the same defaults, so the game always starts.
func LoadSettings() Settings {
	path, err := settingsPath()
	if err != nil {
		return DefaultSettings()
	}
	s := store.Load(path, DefaultSettings())
	s.clamp()
	return s
}

// Save writes the settings back. Unlike loading, this reports failures: a
// player who changes a setting should be told when it will not stick.
func (s Settings) Save() error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	if err := store.Save(path, s); err != nil {
		return fmt.Errorf("saving the settings: %w", err)
	}
	return nil
}

// applyTo folds the settings into the session options a new game is built
// from. The command line wins wherever it named a value, because asking for
// something explicitly should not be quietly overridden by a preference set
// weeks ago.
func (s Settings) applyTo(o Options) Options {
	if !o.Given["traffic"] {
		o.Traffic = s.Traffic
	}
	if !o.Given["police"] {
		o.Police = s.Police
	}
	return s.applyViewTo(o)
}

// applyViewTo folds in only the settings that do not change what the world is
// made of: what is drawn on top of it and how loud it is.
//
// It is what a two-player chase uses. Both machines have to build the same
// city with the same traffic in it — the snapshot names cars by their index,
// so an extra car at one end points every pose at the wrong vehicle — and
// neither player can see the other's settings screen. The world size therefore
// comes from the shared defaults or from a command line the two players agreed
// on, never from a preference one of them set alone.
func (s Settings) applyViewTo(o Options) Options {
	if !o.Given["panel"] {
		o.ShowPanel = s.ShowPanel
	}
	if !o.Given["camera"] {
		o.Camera = s.Camera
	}
	// Muting is the exception: -mute silences a session whatever the stored
	// setting says, and the stored setting silences one the flag left alone.
	o.Mute = o.Mute || s.Muted
	return o
}
