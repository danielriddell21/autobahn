package game

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/crucible/geom"
)

// The Settings page (docs/menus.md, "Settings"): sections down the left, the
// chosen section's settings on the right. The page holds one list of every
// setting in section order, so up and down walk across sections and the
// section lit on the left follows the selection. Weather, start time and
// autopilot style are chosen on the Play page and are not here, though they
// stay in [Settings].

var _ = registerPage(pageSettings, newSettingsPage, hintsAdjust...)

// settingsSections are the sections, in order.
var settingsSections = []string{"Menus", "Sound", "View", "World", "Autopilot"}

// Section indices, matching settingsSections.
const (
	secMenus = iota
	secSound
	secView
	secWorld
	secAutopilot
)

// settingsKeySection says which section each setting's row key belongs to.
var settingsKeySection = map[string]int{
	"menudrive": secMenus,
	"volume":    secSound, "sound": secSound,
	"viewpoint": secView, "aicamera": secView, "boxlabels": secView,
	"traffic": secWorld, "police": secWorld, "daylength": secWorld,
	"jobsauto": secAutopilot,
}

// settingsBackKey is the Back row, which belongs to no section.
const settingsBackKey = "back"

// settingsRows builds every setting's row over the shell's settings.
func settingsRows(s *Shell, leave func()) []menuRow {
	set := &s.settings
	pct := func() string { return fmt.Sprintf("%d%%", int(set.Volume*100+0.5)) }
	span := func(v, lo, hi int) float64 { return float64(v-lo) / float64(hi-lo) }
	var cams, days []string
	for m := CameraMode(0); m < numCameraModes; m++ {
		cams = append(cams, m.String())
	}
	for d := DayLength(0); d < numDayLengths; d++ {
		days = append(days, d.String())
	}
	return []menuRow{
		switchRow("menudrive", "Menu drive", set.MenuDrive, set.SetMenuDrive).
			describe("The autopilot drives behind the menus. Off shows a still backdrop."),
		barRow("volume", "Volume",
			func() float64 { return set.Volume }, pct,
			func(d int) { set.Volume = geom.Clamp(set.Volume+float64(d)*volumeStep, 0, 1) }).
			describe("How loud everything is."),
		switchRow("sound", "Sound",
			func() bool { return !set.Muted }, func(on bool) { set.Muted = !on }).
			describe("Off silences everything and remembers the volume."),
		optionsRow("viewpoint", "Viewpoint", cams,
			func() int { return int(set.Camera) }, func(i int) { set.Camera = CameraMode(i) }).
			describe("Where the camera sits at the start of a drive."),
		switchRow("aicamera", "AI camera",
			func() bool { return set.ShowPanel }, func(on bool) { set.ShowPanel = on }).
			describe("Shows what the autopilot sees while it drives."),
		switchRow("boxlabels", "Box labels",
			func() bool { return set.ShowLabels }, func(on bool) { set.ShowLabels = on }).
			describe("Names each detection beside its box."),
		barRow("traffic", "Traffic",
			func() float64 { return span(set.Traffic, trafficMin, trafficMax) },
			func() string { return fmt.Sprintf("%d cars", set.Traffic) },
			func(d int) { set.Traffic = geom.Clamp(set.Traffic+d*trafficStep, trafficMin, trafficMax) }).
			describe("Cars on the road. Takes effect next drive."),
		barRow("police", "Police",
			func() float64 { return span(set.Police, policeMin, policeMax) },
			func() string { return fmt.Sprintf("%d units", set.Police) },
			func(d int) { set.Police = geom.Clamp(set.Police+d*policeStep, policeMin, policeMax) }).
			describe("Police cars on patrol. Takes effect next drive."),
		optionsRow("daylength", "Day length", days,
			func() int { return int(set.DayLength) }, func(i int) { set.DayLength = DayLength(i) }).
			describe("How fast the day goes round."),
		switchRow("jobsauto", "Jobs on auto",
			func() bool { return set.AutopilotJobs }, func(on bool) { set.AutopilotJobs = on }).
			describe("A job carries on, unpaid, while the autopilot drives. Takes effect next drive."),
		actionRow(settingsBackKey, "Back", leave),
	}
}

// leaveSettingsPage saves the settings, pushes the live ones to the game and
// goes back.
func (s *Shell) leaveSettingsPage() {
	if err := s.settings.Save(); err != nil {
		s.notice = err.Error()
	}
	s.applyLive()
	s.backPage()
}

// settingsSectionOf is the section the row with key sits in, or -1 for Back.
func settingsSectionOf(key string) int {
	if i, ok := settingsKeySection[key]; ok {
		return i
	}
	return -1
}

func newSettingsPage(s *Shell) *menuPage {
	list := newRowList(settingsRows(s, s.leaveSettingsPage)...)
	pg := &menuPage{Title: "SETTINGS", List: list, Back: s.leaveSettingsPage}
	pg.Draw = func(float32) { drawSettingsPage(pg, int32(s.opts.Width)) }
	return pg
}

// settingsSection is the section the page's selection is in; Back keeps the
// last one lit.
func settingsSection(l *rowList) int {
	if i := settingsSectionOf(l.SelectedKey()); i >= 0 {
		return i
	}
	return secAutopilot
}

func drawSettingsPage(pg *menuPage, w int32) {
	drawPageTitle(pg.Title, "")
	l := pg.List
	cur := settingsSection(l)
	onBack := l.SelectedKey() == settingsBackKey
	const colW = 200
	y := int32(pageListTop)
	for i, name := range settingsSections {
		r := rect(pageMargin, y, colW, pageRowH-4)
		if i == cur && !onBack {
			rounded(r, 6, signLift)
			rl.DrawRectangle(pageMargin, y+6, 3, pageRowH-16, signBlue)
		}
		ink := signDim
		if i == cur {
			ink = signInk
		}
		faceRow.draw(name, pageMargin+16, y+(pageRowH-4-faceRow.height())/2, ink)
		y += pageRowH
	}
	y += 12
	drawButton(rect(pageMargin, y, colW, pageRowH-4), "Back", onBack, false, tonePlain)

	// The section's rows, drawn with the page's own selection.
	var rows []rowView
	for _, v := range l.Views() {
		if settingsSectionOf(v.Key) == cur {
			rows = append(rows, v)
		}
	}
	x := int32(pageMargin + colW + 48)
	rw := min(pageListW, w-x-pageMargin)
	rowH := listRowHeight(rows)
	y = pageListTop
	// A panel under the rows, so the words stand on something steady whatever
	// the menu drive shows through the fade.
	rounded(rect(x-10, y-8, rw+20, int32(len(rows))*rowH+12), 10, rl.ColorAlpha(signCard, 0.92))
	for _, v := range rows {
		drawRowView(rect(x, y, rw, rowH-4), v)
		y += rowH
	}
}
