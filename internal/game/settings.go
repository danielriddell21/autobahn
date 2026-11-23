package game

import (
	"fmt"

	"github.com/danielriddell21/autobahn/internal/autopilot"
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/weather"

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
	// Time is the time of day a session starts at, and DayLength how fast
	// the day then goes round.
	Time      TimeOfDay `json:"time"`
	DayLength DayLength `json:"dayLength"`
	// Mode is how the autopilot drives.
	Mode autopilot.Mode `json:"mode"`
	// Car is the shape of the player's car, and Paint its colour, as chosen
	// in the garage.
	Car   sim.BodyKind `json:"car"`
	Paint int          `json:"paint"`
	// Map is the place a new session is driven in.
	Map city.Map `json:"map"`
	// UnlockAll offers every pack without it being earned. There is no row
	// for it on the settings screen: it is for whoever opens the file.
	UnlockAll bool `json:"unlockAll"`
	// AutopilotJobs lets a job continue, unpaid, while the autopilot drives.
	AutopilotJobs bool `json:"autopilotJobs"`
	// Weather is the sky a new session is driven under: drifting, or held at one
	// kind.
	Weather weather.Setting `json:"weather"`
	// NoMenuDrive turns off the autopilot driving behind the menus. It is
	// stored inverted so a settings file written before the setting existed
	// reads as the default, which is on; see [Settings.MenuDrive].
	NoMenuDrive bool `json:"noMenuDrive,omitempty"`
	// Custom is the share code of the custom map last edited, or empty. The
	// named ones are kept apart, in customs.json.
	Custom string `json:"custom,omitempty"`
	// HostMode is what the host row on the title screen starts: a chase or one
	// of the co-op games.
	HostMode NetMode `json:"hostMode,omitempty"`
}

// MenuDrive reports whether the autopilot drives behind the menus. On unless
// the player has turned it off.
func (s Settings) MenuDrive() bool { return !s.NoMenuDrive }

// SetMenuDrive turns the menu drive on or off.
func (s *Settings) SetMenuDrive(on bool) { s.NoMenuDrive = !on }

// DefaultSettings returns the settings a first run starts with.
func DefaultSettings() Settings {
	return Settings{
		Volume: 0.7, ShowPanel: true, ShowLabels: true,
		Traffic: 70, Police: 7, Camera: CameraChase, Car: sim.HotHatch,
		DayLength: DayMedium, Weather: weather.Drift,
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
	if s.Mode < 0 || s.Mode >= autopilot.NumModes {
		s.Mode = autopilot.ModeNormal
	}
	if s.Time < 0 || s.Time >= NumTimes {
		s.Time = TimeDay
	}
	if s.DayLength < 0 || s.DayLength >= numDayLengths {
		s.DayLength = DayMedium
	}
	if s.Weather > weather.Drift {
		s.Weather = weather.Drift
	}
	if !sim.IsPlayerBody(s.Car) {
		s.Car = sim.HotHatch
	}
	if s.Paint < 0 || s.Paint >= len(playerPaints) {
		s.Paint = 0
	}
	if !s.Map.Valid() {
		s.Map = city.MapCity
	}
	s.clampStored()
}

// clampStored brings the settings that are stored as codes back into range.
func (s *Settings) clampStored() {
	if s.HostMode >= numNetModes {
		s.HostMode = ModeChase
	}
	if _, err := custom.Decode(s.Custom); err != nil {
		s.Custom = ""
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
	// The car's shape changes its size, which a chase against another person
	// has to agree on, so it is only ever taken here.
	if !o.Given["car"] {
		o.Car = s.Car
	}
	// The map is what the world is made of, so like its size it is never
	// taken from a preference in a chase against another person.
	if !o.Given["map"] {
		o.Map = s.Map
	}
	// The weather is the grip of the road, so like the map it is the host's
	// to say in a chase; see Snapshot.Weather.
	if !o.Given["weather"] {
		o.Weather = s.Weather
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
	if !o.Given["mode"] {
		o.Mode = s.Mode
	}
	if !o.Given["time"] {
		o.Time = s.Time
	}
	if !o.Given["daylength"] {
		o.DayLength = s.DayLength.Minutes()
	}
	if !o.Given["autopilot-jobs"] {
		o.AutopilotJobs = s.AutopilotJobs
	}
	if !o.Given["paint"] {
		o.Paint = s.Paint
	}
	// Muting is the exception: -mute silences a session whatever the stored
	// setting says, and the stored setting silences one the flag left alone.
	o.Mute = o.Mute || s.Muted
	return o
}
