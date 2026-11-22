package game

import (
	"fmt"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/menu"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/weather"

	"github.com/danielriddell21/crucible/store"
)

// The custom map screen's state and rules, with no drawing in it: the menu that
// shows it asks for its rows and sends it a press. It works on a [custom.Spec]
// and keeps what the player has named in customs.json beside the settings.

// customRow is one line of the screen.
type customRow struct {
	Label string
	Value string
}

// customScreen is the editor.
type customScreen struct {
	spec    custom.Spec
	presets custom.Presets
	access  pack.Access
	// Which row is selected; rows are laid out by rows.
	row int
}

// The rows after the sliders: the preset map and seed before them are the first
// two, and these follow.
const (
	customRowBase = iota
	customRowSeed
	customRowFirstSlider
	customRowCoast       = customRowFirstSlider + custom.NumSliders
	customRowWeather     = customRowCoast + 1
	customRowUnsupported = customRowWeather + 1
	customNumRows        = customRowUnsupported + 1
)

// customPresetsPath is where the named custom maps live.
func customPresetsPath() (string, error) {
	p, err := store.Path(appName, custom.PresetsFile)
	if err != nil {
		return "", fmt.Errorf("locating the custom maps: %w", err)
	}
	return p, nil
}

// newCustomScreen opens the editor on the map in the settings, or on the
// preset map the settings name when no custom map has been made yet.
func newCustomScreen(set Settings, a pack.Access) *customScreen {
	c := &customScreen{access: a, presets: custom.Presets{}}
	if path, err := customPresetsPath(); err == nil {
		c.presets = custom.LoadPresets(path)
	}
	c.spec = custom.FromMap(set.Map, 1)
	c.spec.Weather = set.Weather
	if s, err := custom.Decode(set.Custom); err == nil {
		c.spec = s
	}
	return c
}

// choice is the words for the sliders that are choices rather than numbers.
func (c *customScreen) choice(i, step int) string {
	switch i {
	case custom.Season:
		if step == 0 {
			return "the map's own"
		}
		return city.Seasons()[step-1].Name()
	case custom.Traffic:
		if step == 0 {
			return "the map's own"
		}
		return "as in the " + city.Maps()[step-1].Name()
	case custom.Limits:
		return [...]string{"urban", "fast"}[min(max(step, 0), 1)]
	}
	return ""
}

func (c *customScreen) sliderText(i int) string {
	sl, step := custom.Sliders[i], c.spec.Step[i]
	switch i {
	case custom.Season, custom.Traffic, custom.Limits:
		return c.choice(i, step)
	case custom.Buildings:
		return fmt.Sprintf("%d%%", step*100/sl.Max)
	case custom.Roundabouts, custom.Parks, custom.Grip:
		return fmt.Sprintf("%.0f%%", sl.Value(step)*100)
	}
	return fmt.Sprintf("%g%s", sl.Value(step), sl.Unit)
}

// Rows returns the screen's lines as they are drawn.
func (c *customScreen) Rows() []customRow {
	rows := make([]customRow, 0, customNumRows)
	rows = append(rows,
		customRow{"Starts from", c.spec.Base.Name()},
		customRow{"Seed", fmt.Sprint(c.spec.Seed)})
	for i, sl := range custom.Sliders {
		rows = append(rows, customRow{sl.Name, c.sliderText(i)})
	}
	rows = append(rows,
		customRow{"Coast", onOff(c.spec.Coast)},
		customRow{"Weather", c.spec.Weather.String()},
		customRow{"Allow unsupported", onOff(c.spec.Unsupported)})
	return rows
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// Adjust moves the selected row one press either way, dir being 1 or -1.
func (c *customScreen) Adjust(dir int) {
	switch r := c.row; {
	case r == customRowBase:
		c.spec.Base = c.nextMap(dir)
	case r == customRowSeed:
		c.spec.Seed = uint64(max(int64(c.spec.Seed)+int64(dir), 0))
	case r < customRowCoast:
		i := r - customRowFirstSlider
		lo, hi := 0, custom.Sliders[i].Max
		if c.spec.Unsupported && i != custom.Season && i != custom.Traffic && i != custom.Limits {
			lo, hi = -custom.MaxPad, hi+custom.MaxPad
		}
		c.spec.Step[i] = min(max(c.spec.Step[i]+dir, lo), hi)
	case r == customRowCoast:
		c.spec.Coast = !c.spec.Coast
	case r == customRowWeather:
		n := int(weather.Drift) + 1
		c.spec.Weather = weather.Setting((int(c.spec.Weather) + dir + n) % n)
	case r == customRowUnsupported:
		c.spec.Unsupported = !c.spec.Unsupported
	}
	c.spec = c.spec.Normalize()
}

// nextMap is the next base map that the player has unlocked.
func (c *customScreen) nextMap(dir int) city.Map {
	ms := city.Maps()
	at := int(c.spec.Base)
	for range ms {
		at = (at + dir + len(ms)) % len(ms)
		if c.access.HasMap(ms[at]) {
			return ms[at]
		}
	}
	return c.spec.Base
}

// Move selects another row.
func (c *customScreen) Move(dir int) {
	c.row = (c.row + dir + customNumRows) % customNumRows
}

// Flagged is whether the map as it stands is unsupported, which the HUD tags.
func (c *customScreen) Flagged() bool { return c.spec.Flagged() }

// Code is the map's share code.
func (c *customScreen) Code() string { return c.spec.Encode() }

// Paste replaces the map with one from a share code.
func (c *customScreen) Paste(code string) error {
	s, err := custom.Decode(code)
	if err != nil {
		return err
	}
	c.spec = s
	return nil
}

// Playable reports why the map cannot be started, or nil.
func (c *customScreen) Playable() error { return c.spec.Locked(c.access) }

// Save keeps the map under a name.
func (c *customScreen) Save(name string) error {
	if err := c.presets.Put(name, c.spec); err != nil {
		return err
	}
	path, err := customPresetsPath()
	if err != nil {
		return err
	}
	return custom.SavePresets(path, c.presets)
}

// Remember writes the map into the settings as the last one used.
func (c *customScreen) Remember(set *Settings) { set.Custom = c.Code() }

// customOf is the custom map a session is to be played on, or nil: the map the
// settings last held, once the player has chosen to play it. Nothing in a game
// played on it is banked; see [custom.Banks].
func customOf(code string) *custom.Spec {
	s, err := custom.Decode(code)
	if err != nil {
		return nil
	}
	return &s
}

// nextPresetName is the first "custom N" not yet taken among the named maps.
func nextPresetName(p custom.Presets) string {
	for n := 1; ; n++ {
		if name := fmt.Sprintf("custom %d", n); p[name] == "" {
			return name
		}
	}
}

// customStatus words whether the map as it stands is one the game vouches for.
func (c *customScreen) customStatus() string {
	switch {
	case c.Flagged():
		return "UNSUPPORTED: outside what has been measured; nothing is banked"
	default:
		return "nothing is banked on a custom map"
	}
}

// showCustom opens the custom map editor, a menu with a row for each of the
// editor's lines and the actions below them.
func (s *Shell) showCustom() {
	s.screen, s.customNote = screenCustom, ""
	cs := newCustomScreen(s.settings, s.access())
	s.customEd = cs
	m := &menu.Menu{
		Title:    "CUSTOM MAP",
		Subtitle: []string{"left and right adjust a row, enter on Play drives it"},
	}
	for i := range customNumRows {
		move := func() { cs.row = i }
		m.Items = append(m.Items, menu.Item{
			Label: func() string {
				r := cs.Rows()[i]
				return fmt.Sprintf("%-20s %s", r.Label, r.Value)
			},
			Adjust: func(d int) { move(); cs.Adjust(d) },
			Action: func() { move(); cs.Adjust(1) },
		})
	}
	note := func(err error) {
		s.customNote = ""
		if err != nil {
			s.customNote = err.Error()
		}
	}
	m.Items = append(m.Items,
		menu.Item{Label: func() string { return "Share code  " + cs.Code() }, Action: func() {}},
		menu.Item{Label: text("Copy the code"), Action: func() { rl.SetClipboardText(cs.Code()); note(nil) }},
		menu.Item{Label: text("Paste a code"), Action: func() { note(cs.Paste(strings.TrimSpace(rl.GetClipboardText()))) }},
		menu.Item{Label: text("Save it by name"), Action: func() { note(cs.Save(nextPresetName(cs.presets))) }},
		menu.Item{Label: func() string {
			if s.customNote != "" {
				return s.customNote
			}
			return cs.customStatus()
		}, Action: func() {}},
		menu.Item{Label: text("Play"), Action: func() {
			if err := cs.Playable(); err != nil {
				note(err)
				return
			}
			cs.Remember(&s.settings)
			s.customPlay = true
			s.startGame(false)
		}},
		menu.Item{Label: text("Back"), Action: func() {
			cs.Remember(&s.settings)
			if err := s.settings.Save(); err != nil {
				s.notice = err.Error()
			}
			s.showTitle()
		}},
	)
	s.menu = m
}
