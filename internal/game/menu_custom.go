package game

import (
	"fmt"
	"math"
	"slices"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/keymap"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/weather"
)

// The custom map page (docs/menus.md, "Custom map"): a new face on
// [customScreen], which keeps all the rules. Every press here goes through
// customScreen.Adjust and its siblings, so what the old screen does and what
// this one does cannot differ; this file decides which rows there are, how
// they are grouped and how they read, and menu_custom_plan.go and
// menu_custom_draw.go the plan beside them.

var _ = registerPage(pageCustom, newCustomPage, hintsCustom...)

// hintsCustom is the bar for the page: esc here keeps the map and goes back.
var hintsCustom = []keymap.Binding{
	{Key: "up/down", Action: "choose"},
	{Key: "left/right", Action: "change"},
	{Key: "enter", Action: "select"},
	{Key: "esc", Action: "done"},
}

// The keys of the page's rows.
const (
	ckBase       = "base"
	ckSeed       = "seed"
	ckCoast      = "coast"
	ckWeather    = "weather"
	ckMinSpan    = "minspan"
	ckMaxSpan    = "maxspan"
	ckMainEvery  = "mainevery"
	ckRoundabout = "roundabouts"
	ckParks      = "parks"
	ckBuildings  = "buildings"
	ckWind       = "wind"
	ckHills      = "hills"
	ckHillSpan   = "hillspan"
	ckLimits     = "limits"
	ckSeason     = "season"
	ckTraffic    = "traffic"
	ckGrip       = "grip"
	ckCopy       = "copy"
	ckPaste      = "paste"
	ckSave       = "save"
	ckUnsup      = "unsupported"
	ckDrive      = "drive"
)

// customSection is a group of rows, shown together under its name.
type customSection struct {
	Name string
	Keys []string
}

// customSections are the sections along the top, in the order the rows are
// walked. Drive this map sits last in Share, so the way down through every row
// ends at it, and the way up from the first row starts there.
var customSections = []customSection{
	{"Start", []string{ckBase, ckSeed, ckCoast, ckWeather}},
	{"Layout", []string{ckMinSpan, ckMaxSpan, ckMainEvery, ckRoundabout, ckParks, ckBuildings}},
	{"Land", []string{ckWind, ckHills, ckHillSpan}},
	{"Rules", []string{ckLimits, ckSeason, ckTraffic, ckGrip}},
	{"Share", []string{ckCopy, ckPaste, ckSave, ckUnsup, ckDrive}},
}

// sliderKeys is which slider each bar or choice row moves.
var sliderKeys = map[string]int{
	ckMinSpan: custom.MinSpan, ckMaxSpan: custom.MaxSpan, ckMainEvery: custom.MainEvery,
	ckRoundabout: custom.Roundabouts, ckParks: custom.Parks, ckBuildings: custom.Buildings,
	ckWind: custom.Wind, ckHills: custom.Hills, ckHillSpan: custom.HillSpan,
	ckLimits: custom.Limits, ckSeason: custom.Season, ckTraffic: custom.Traffic, ckGrip: custom.Grip,
}

// customPage is the page's state: the editor it is a face of, the rows, and
// the plan shown beside them.
type customPage struct {
	s    *Shell
	cs   *customScreen
	list *rowList
	// note is the last thing a press said: a code copied, a name saved, why
	// something failed.
	note string
	// cache is the plan, built for the spec as it stands and not rebuilt while
	// nothing it is drawn from has changed.
	cache planCache
	plan  *customPlan

	// start begins the drive; it is s.startGame, and a test's own.
	start func(autopilot bool)
	// clip reads the clipboard and setClip writes it.
	clip    func() string
	setClip func(string)
}

// newCustomPage builds the page on the map in the settings.
func newCustomPage(s *Shell) *menuPage {
	p := makeCustomPage(s)
	return &menuPage{
		Title: "CUSTOM MAP",
		List:  p.list,
		Step:  p.step,
		Draw:  p.draw,
		Back:  p.done,
	}
}

// makeCustomPage is the page's state, apart from the menuPage that carries it.
func makeCustomPage(s *Shell) *customPage {
	p := &customPage{
		s:       s,
		cs:      newCustomScreen(s.settings, s.access()),
		start:   s.startGame,
		clip:    rl.GetClipboardText,
		setClip: rl.SetClipboardText,
	}
	p.list = newRowList(p.rows()...)
	return p
}

// step runs every frame, before the keys: it keeps the plan up to date and
// clears a note once the player moves on.
func (p *customPage) step(in navInput) bool {
	if in.Up || in.Down || in.Left || in.Right {
		p.note = ""
	}
	p.plan = p.cache.Ask(p.cs.spec)
	return false
}

// done keeps the map in the settings and goes back.
func (p *customPage) done() {
	p.cs.Remember(&p.s.settings)
	if err := p.s.settings.Save(); err != nil {
		p.s.notice = err.Error()
	}
	p.s.backPage()
}

// drive starts a drive on the map as it stands, by the way the old Play row
// did, or says why it cannot.
func (p *customPage) drive() {
	if err := p.cs.Playable(); err != nil {
		p.note = err.Error()
		return
	}
	p.cs.Remember(&p.s.settings)
	p.s.customPlay = true
	p.start(false)
}

// section is the index of the section the selected row is in.
func (p *customPage) section() int {
	return customSectionOf(p.list.SelectedKey())
}

// customSectionOf is the index of the section a row's key is in, or 0.
func customSectionOf(key string) int {
	for i, sec := range customSections {
		for _, k := range sec.Keys {
			if k == key {
				return i
			}
		}
	}
	return 0
}

// sectionRows is the rows of a section, in order, as indexes into the list.
func (p *customPage) sectionRows(i int) []int {
	var out []int
	for _, k := range customSections[i].Keys {
		for at, r := range p.list.rows {
			if r.Key == k {
				out = append(out, at)
			}
		}
	}
	return out
}

// rebuild lays the rows out again, which only the list of held maps needs
// when the base changes.
func (p *customPage) rebuild() { p.list.SetRows(p.rows()) }

// moveTo is a press on the editor's own row r, whichever row is selected here.
func (p *customPage) moveTo(r, dir int) {
	p.cs.row = r
	p.cs.Adjust(dir)
}

// sliderRow is the editor row of a slider.
func sliderRow(i int) int { return customRowFirstSlider + i }

// outside reports whether a slider is past what the gate has measured, and
// so is amber and tags the map UNSUPPORTED. Only a map that has asked to be
// unsupported can be: otherwise every value is held in range. It asks
// city.Clamp what it would do to the map's parameters, so the ranges stay
// where they are written.
func (p *customPage) outside(i int) bool {
	sp := p.cs.spec.Normalize()
	if !sp.Unsupported {
		return false
	}
	prm, _ := sp.Params()
	held, _ := city.Clamp(prm, false)
	switch i {
	case custom.MinSpan:
		return prm.MinSpan != held.MinSpan
	case custom.MaxSpan:
		return prm.MaxSpan != held.MaxSpan
	case custom.Roundabouts:
		return prm.Roundabouts != held.Roundabouts
	case custom.Parks:
		return prm.Parks != held.Parks
	case custom.Buildings:
		return prm.Rise != held.Rise || prm.Floor != held.Floor || prm.Storeys != held.Storeys
	case custom.MainEvery:
		return prm.MainEvery != held.MainEvery
	case custom.Wind:
		return prm.Wind != held.Wind
	case custom.Hills, custom.HillSpan:
		// The slope rule joins the two: either is past the range if the pair is.
		return prm.Hills != held.Hills || prm.HillSpan != held.HillSpan
	case custom.Grip:
		return p.gripBelowFloor()
	}
	return false
}

// gripBelowFloor says the road's grip is set below what winter gives, which
// only an unsupported map keeps. The unsupported spec is asked what a
// supported one would do.
func (p *customPage) gripBelowFloor() bool {
	sp := p.cs.spec.Normalize()
	if !sp.Unsupported {
		return false
	}
	return gripOf(sp) < gripOf(withoutUnsupported(sp))-1e-6
}

func withoutUnsupported(sp custom.Spec) custom.Spec {
	sp.Unsupported = false
	return sp.Normalize()
}

// gripOf is the grip a spec drives with.
func gripOf(sp custom.Spec) float32 { return sp.Apply(sim.Config{}).GripScale }

// words is how a slider reads, with what the map will really use after it
// where the spec asks for something that is held back: a longest block
// shorter than the shortest, hills with no length, grip below winter's.
func (p *customPage) words(i int) string {
	w := p.cs.sliderText(i)
	sp := p.cs.spec.Normalize()
	prm, _ := sp.Params()
	switch i {
	case custom.MaxSpan:
		if math.Abs(float64(prm.MaxSpan)-custom.Sliders[i].Value(sp.Step[i])) > 0.5 {
			w += fmt.Sprintf(" (%gm)", prm.MaxSpan)
		}
	case custom.HillSpan:
		if prm.Hills <= 0 {
			return "no hills"
		}
		if math.Abs(float64(prm.HillSpan)-custom.Sliders[i].Value(sp.Step[i])) > 0.5 {
			w += fmt.Sprintf(" (%.0fm)", prm.HillSpan)
		}
	case custom.Grip:
		if g := gripOf(sp); math.Abs(float64(g)-custom.Sliders[i].Value(sp.Step[i])) > 0.004 {
			w += fmt.Sprintf(" (%.0f%%)", g*100)
		}
	}
	return w
}

// level is how full a slider's bar is.
func (p *customPage) level(i int) float64 {
	return float64(p.cs.spec.Step[i]) / float64(custom.Sliders[i].Max)
}

// bar is the row of a numeric slider.
func (p *customPage) bar(key string) menuRow {
	i := sliderKeys[key]
	label := custom.Sliders[i].Name
	return barRow(key, label,
		func() float64 { return p.level(i) },
		func() string { return p.words(i) },
		func(dir int) { p.moveTo(sliderRow(i), dir) },
	).warned(func() bool { return p.outside(i) })
}

// choice is the row of a slider that is a choice.
func (p *customPage) choice(key string, opts []string) menuRow {
	i := sliderKeys[key]
	return optionsRow(key, custom.Sliders[i].Name, opts,
		func() int { return p.cs.spec.Step[i] },
		func(at int) { p.cs.spec.Step[i] = at; p.cs.spec = p.cs.spec.Normalize() },
	)
}

// heldMaps are the maps the player may start from, and the map the spec is on
// if that is not one of them (a pasted code can name any), so the row always
// shows the truth and Drive says why it cannot.
func (p *customPage) heldMaps() []city.Map {
	var out []city.Map
	for _, m := range city.Maps() {
		if p.cs.access.HasMap(m) || m == p.cs.spec.Base {
			out = append(out, m)
		}
	}
	return out
}

// mapNames are the names of the maps, as the editor writes them.
func mapNames(ms []city.Map) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Name()
	}
	return out
}

// rows builds every row of the page, grouped as customSections says.
func (p *customPage) rows() []menuRow {
	byKey := map[string]menuRow{}
	for _, group := range [][]menuRow{p.startRows(), p.ruleRows(), p.shareRows()} {
		for _, r := range group {
			byKey[r.Key] = r
		}
	}
	for key := range sliderKeys {
		if _, set := byKey[key]; !set {
			byKey[key] = p.bar(key)
		}
	}
	var rows []menuRow
	for _, sec := range customSections {
		for _, k := range sec.Keys {
			rows = append(rows, byKey[k])
		}
	}
	return rows
}

// startRows are the rows that are not sliders: the map, seed, coast and sky.
func (p *customPage) startRows() []menuRow {
	cs := p.cs
	held := p.heldMaps()
	base := optionsRow(ckBase, "Starts from", mapNames(held),
		func() int {
			if at := slices.Index(held, cs.spec.Base); at >= 0 {
				return at
			}
			return 0
		},
		func(at int) {
			cs.spec.Base = held[at]
			cs.spec = cs.spec.Normalize()
			p.rebuild()
		}).toned(func() tone {
		if !cs.access.HasMap(cs.spec.Base) {
			return toneLocked
		}
		return tonePlain
	})
	return []menuRow{
		base,
		stepperRow(ckSeed, "Seed",
			func() string { return fmt.Sprint(cs.spec.Seed) },
			func(dir int) { p.moveTo(customRowSeed, dir) }),
		switchRow(ckCoast, "Coast",
			func() bool { return cs.spec.Coast },
			func(on bool) {
				if on != cs.spec.Coast {
					p.moveTo(customRowCoast, 1)
				}
			}),
		optionsRow(ckWeather, "Weather", weatherNames(),
			func() int { return int(cs.spec.Weather) },
			func(at int) { cs.spec.Weather = weather.Setting(at); cs.spec = cs.spec.Normalize() }),
	}
}

// ruleRows are the rows of the sliders that are choices.
func (p *customPage) ruleRows() []menuRow {
	limits := []string{p.cs.choice(custom.Limits, 0), p.cs.choice(custom.Limits, 1)}
	return []menuRow{
		p.choice(ckLimits, limits),
		p.choice(ckSeason, seasonNames()),
		p.choice(ckTraffic, trafficNames()),
	}
}

// shareRows are the rows that copy, paste and save the map, and drive it.
func (p *customPage) shareRows() []menuRow {
	cs := p.cs
	return []menuRow{
		actionRow(ckCopy, "Copy the code", func() {
			p.setClip(cs.Code())
			p.note = "code copied"
		}),
		actionRow(ckPaste, "Paste a code", func() {
			if err := cs.Paste(strings.TrimSpace(p.clip())); err != nil {
				p.note = err.Error()
				return
			}
			p.note = "code pasted"
			p.rebuild()
		}),
		actionRow(ckSave, "Save it by name", func() {
			name := nextPresetName(cs.presets)
			if err := cs.Save(name); err != nil {
				p.note = err.Error()
				return
			}
			p.note = "saved as " + name
		}),
		switchRow(ckUnsup, "Allow unsupported",
			func() bool { return cs.spec.Unsupported },
			func(on bool) {
				if on != cs.spec.Unsupported {
					p.moveTo(customRowUnsupported, 1)
				}
			}).warned(func() bool { return cs.spec.Unsupported }),
		actionRow(ckDrive, "Drive this map", p.drive).
			disabledBy(func() string {
				if err := cs.Playable(); err != nil {
					return err.Error()
				}
				return ""
			}),
	}
}

// weatherNames are the skies a map can be driven under, in the order the
// editor steps through them.
func weatherNames() []string {
	out := make([]string, 0, int(weather.Drift)+1)
	for s := weather.Setting(0); s <= weather.Drift; s++ {
		out = append(out, s.String())
	}
	return out
}

// seasonNames are the choices for the season: the map's own, then each.
func seasonNames() []string {
	out := make([]string, 1, 1+len(city.Seasons()))
	out[0] = "own"
	for _, s := range city.Seasons() {
		out = append(out, s.Name())
	}
	return out
}

// trafficNames are the choices for the traffic mix: the map's own, then the
// mix of each place.
func trafficNames() []string {
	out := make([]string, 1, 1+len(city.Maps()))
	out[0] = "own"
	for _, m := range city.Maps() {
		out = append(out, m.Name())
	}
	return out
}

// status is the line under the plan: what the map is, or what went wrong.
func (p *customPage) status() (string, tone) {
	switch {
	case p.note != "":
		return p.note, tonePlain
	case p.list.Reason() != "":
		return p.list.Reason(), toneOffer
	case p.cs.Flagged():
		return "UNSUPPORTED: outside what has been measured", toneOffer
	}
	return "", tonePlain
}
