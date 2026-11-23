package game

import (
	"slices"
	"strings"
	"testing"

	"github.com/danielriddell21/crucible/menu"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/pack"
	"github.com/danielriddell21/autobahn/internal/weather"
)

// customTestRig is the page on a shell with a fresh config, everything
// unlocked unless the test says otherwise, and the clipboard and the drive
// replaced so nothing needs a window. The record says what the drive was told.
type customTestRig struct {
	*customPage
	driven []bool
	clipb  string
}

func newCustomRig(t *testing.T, bypass bool) *customTestRig {
	t.Helper()
	sh := testShell(t)
	sh.settings.UnlockAll = bypass
	r := &customTestRig{customPage: makeCustomPage(sh)}
	r.start = func(auto bool) { r.driven = append(r.driven, auto) }
	r.clip = func() string { return r.clipb }
	r.setClip = func(s string) { r.clipb = s }
	return r
}

// press sends a key to the page as the shell would, and lets the page see it.
func (r *customTestRig) press(in menu.Input) {
	r.step(navInput{Input: in})
	r.list.Update(in)
}

// selectKey puts the selection on a row.
func (r *customTestRig) selectKey(t *testing.T, key string) {
	t.Helper()
	if !r.list.Select(key) {
		t.Fatalf("no row %q", key)
	}
}

func viewOf(t *testing.T, r *customTestRig, key string) rowView {
	t.Helper()
	for _, v := range r.list.Views() {
		if v.Key == key {
			return v
		}
	}
	t.Fatalf("no row %q", key)
	return rowView{}
}

func TestCustomPageIsRegistered(t *testing.T) {
	if _, stub := pageFor(pageCustom); stub {
		t.Fatal("the custom page is still a stub")
	}
	sh := testShell(t)
	sh.openPage(pageCustom)
	if !sh.onPage() || sh.screen != pageCustom || sh.pager.cur.Title != "CUSTOM MAP" {
		t.Fatalf("opened %v on screen %v", sh.pager.cur, sh.screen)
	}
	if sh.pager.cur.Draw == nil || sh.pager.cur.List == nil || sh.pager.cur.Step == nil {
		t.Error("the page has no list, step or drawing of its own")
	}
	if !slices.Equal(hintsFor(pageCustom, sh.pager.cur), hintsCustom) {
		t.Error("the page does not use its own hints")
	}
}

func TestCustomSectionsHoldEveryRowOnceInOrder(t *testing.T) {
	r := newCustomRig(t, true)
	var want []string
	for _, sec := range customSections {
		want = append(want, sec.Keys...)
	}
	var got []string
	for _, row := range r.list.rows {
		got = append(got, row.Key)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("rows %v, want %v", got, want)
	}
	names := make([]string, len(customSections))
	for i, sec := range customSections {
		names[i] = sec.Name
	}
	if !slices.Equal(names, []string{"Start", "Layout", "Land", "Rules", "Share"}) {
		t.Errorf("sections %v", names)
	}
	// Every slider of the editor has a row, and none twice.
	seen := map[int]bool{}
	for _, i := range sliderKeys {
		if seen[i] {
			t.Errorf("slider %d has two rows", i)
		}
		seen[i] = true
	}
	if len(seen) != custom.NumSliders {
		t.Errorf("%d of the %d sliders have a row", len(seen), custom.NumSliders)
	}
}

func TestCustomSectionContents(t *testing.T) {
	want := map[string][]string{
		"Start":  {"Starts from", "Seed", "Coast", "Weather"},
		"Layout": {"Shortest block", "Longest block", "Main road every", "Roundabouts", "Open ground", "Building height"},
		"Land":   {"Winding", "Hills", "Hill length"},
		"Rules":  {"Speed limits", "Season", "Traffic mix", "Road grip"},
		"Share":  {"Copy the code", "Paste a code", "Save it by name", "Allow unsupported", "Drive this map"},
	}
	r := newCustomRig(t, true)
	for i, sec := range customSections {
		var labels []string
		for _, at := range r.sectionRows(i) {
			labels = append(labels, r.list.rows[at].Label)
		}
		if !slices.Equal(labels, want[sec.Name]) {
			t.Errorf("%s: %v, want %v", sec.Name, labels, want[sec.Name])
		}
	}
	for _, row := range r.list.rows {
		for _, c := range row.Label {
			if c > 126 {
				t.Errorf("%q is not ASCII", row.Label)
			}
		}
	}
}

func TestCustomRowKinds(t *testing.T) {
	r := newCustomRig(t, true)
	kinds := map[string]rowKind{
		ckBase: rowOptions, ckSeed: rowStepper, ckCoast: rowSwitch, ckWeather: rowOptions,
		ckMinSpan: rowBar, ckMaxSpan: rowBar, ckMainEvery: rowBar, ckRoundabout: rowBar, ckParks: rowBar, ckBuildings: rowBar,
		ckWind: rowBar, ckHills: rowBar, ckHillSpan: rowBar,
		ckLimits: rowOptions, ckSeason: rowOptions, ckTraffic: rowOptions, ckGrip: rowBar,
		ckCopy: rowAction, ckPaste: rowAction, ckSave: rowAction, ckUnsup: rowSwitch, ckDrive: rowAction,
	}
	for key, kind := range kinds {
		if got := viewOf(t, r, key).Kind; got != kind {
			t.Errorf("%s is kind %d, want %d", key, got, kind)
		}
	}
	// A choice row offers each step of its slider.
	for key, i := range map[string]int{ckLimits: custom.Limits, ckSeason: custom.Season, ckTraffic: custom.Traffic} {
		if n := len(viewOf(t, r, key).Options); n != custom.Sliders[i].Max+1 {
			t.Errorf("%s offers %d choices for %d steps", key, n, custom.Sliders[i].Max+1)
		}
	}
	if n := len(viewOf(t, r, ckWeather).Options); n != int(weather.Drift)+1 {
		t.Errorf("weather offers %d choices", n)
	}
}

func TestCustomRowsMoveTheSpecThroughTheEditor(t *testing.T) {
	r := newCustomRig(t, true)
	wind := custom.Wind
	before := r.cs.spec.Step[wind]
	row, _ := r.list.Row(ckWind)
	row.Adjust(1)
	if r.cs.spec.Step[wind] != before+1 {
		t.Error("+ on winding did not move it")
	}
	if v := viewOf(t, r, ckWind); v.Level <= 0 || v.Value == "" {
		t.Errorf("winding reads %q at level %v", v.Value, v.Level)
	}
	row.Adjust(-1)
	if r.cs.spec.Step[wind] != before {
		t.Error("- did not undo it")
	}

	seed, _ := r.list.Row(ckSeed)
	s0 := r.cs.spec.Seed
	seed.Adjust(1)
	if r.cs.spec.Seed != s0+1 || viewOf(t, r, ckSeed).Value == "" {
		t.Error("the seed did not step")
	}

	coast, _ := r.list.Row(ckCoast)
	coast.Adjust(1)
	coast.Adjust(1)
	if !r.cs.spec.Coast || !viewOf(t, r, ckCoast).On {
		t.Error("coast is not on after two ons")
	}
	coast.Activate()
	if r.cs.spec.Coast {
		t.Error("enter did not flip the coast off")
	}

	season, _ := r.list.Row(ckSeason)
	season.Adjust(1)
	if r.cs.spec.Step[custom.Season] != 1 || viewOf(t, r, ckSeason).Value != "autumn" {
		t.Errorf("season step %d reads %q", r.cs.spec.Step[custom.Season], viewOf(t, r, ckSeason).Value)
	}
	season.Adjust(-1)
	season.Adjust(-1)
	if r.cs.spec.Step[custom.Season] != custom.Sliders[custom.Season].Max {
		t.Error("the season row does not wrap")
	}

	w, _ := r.list.Row(ckWeather)
	w.Adjust(1)
	if r.cs.spec.Weather == weather.Drift && r.cs.spec.Weather == custom.FromMap(city.MapCity, 1).Weather {
		t.Error("the weather did not change")
	}
	if got := viewOf(t, r, ckWeather).Value; got != r.cs.spec.Weather.String() {
		t.Errorf("weather reads %q for %v", got, r.cs.spec.Weather)
	}
}

func TestCustomStartsFromHeldMapsOnly(t *testing.T) {
	r := newCustomRig(t, false)
	if got := viewOf(t, r, ckBase).Options; !slices.Equal(got, []string{"city"}) {
		t.Fatalf("with nothing unlocked the map row offers %v", got)
	}
	base, _ := r.list.Row(ckBase)
	base.Adjust(1)
	if r.cs.spec.Base != city.MapCity {
		t.Errorf("stepped onto %s, which is not held", r.cs.spec.Base.Name())
	}

	r = newCustomRig(t, true)
	if n := len(viewOf(t, r, ckBase).Options); n != len(city.Maps()) {
		t.Fatalf("with everything unlocked the map row offers %d", n)
	}
	base, _ = r.list.Row(ckBase)
	base.Adjust(1)
	if r.cs.spec.Base == city.MapCity || viewOf(t, r, ckBase).Value != r.cs.spec.Base.Name() {
		t.Errorf("did not move to another map: %s", r.cs.spec.Base.Name())
	}
	base.Adjust(-1)
	if r.cs.spec.Base != city.MapCity {
		t.Error("did not come back")
	}
}

func TestCustomShowsAMapItCannotDrive(t *testing.T) {
	// A pasted code can name a map the player has not unlocked: the row says so
	// and Drive refuses, rather than showing some other map.
	r := newCustomRig(t, false)
	r.clipb = custom.FromMap(city.MapMotorway, 3).Encode()
	paste, _ := r.list.Row(ckPaste)
	paste.Activate()
	if r.cs.spec.Base != city.MapMotorway {
		t.Fatalf("pasted onto %s", r.cs.spec.Base.Name())
	}
	v := viewOf(t, r, ckBase)
	if v.Value != "roadtown" || v.Tone != toneLocked {
		t.Errorf("the map row reads %q in tone %d", v.Value, v.Tone)
	}
	if viewOf(t, r, ckDrive).Disabled == "" {
		t.Error("Drive is open on a locked map")
	}
	r.selectKey(t, ckDrive)
	r.press(menu.Input{Select: true})
	if len(r.driven) != 0 || r.s.customPlay {
		t.Error("a locked map was driven")
	}
	if !strings.Contains(r.list.Reason(), "locked") {
		t.Errorf("the reason is %q", r.list.Reason())
	}
}

func TestCustomWarnsOnlyPastTheMeasuredRange(t *testing.T) {
	r := newCustomRig(t, true)
	sliders := []string{ckMinSpan, ckMaxSpan, ckMainEvery, ckRoundabout, ckParks, ckBuildings, ckWind, ckHills, ckHillSpan, ckLimits, ckSeason, ckTraffic, ckGrip}
	warned := func() []string {
		var out []string
		for _, v := range r.list.Views() {
			if v.Warn {
				out = append(out, v.Key)
			}
		}
		return out
	}
	// Held in range, nothing is warned however hard the bars are pushed.
	for _, k := range sliders {
		row, _ := r.list.Row(k)
		for range 40 {
			row.Adjust(1)
		}
	}
	if w := warned(); len(w) != 0 || r.cs.Flagged() {
		t.Fatalf("a supported map warns %v, flagged %v", w, r.cs.Flagged())
	}
	if line, _ := r.status(); strings.Contains(line, "UNSUPPORTED") {
		t.Error("tagged unsupported")
	}

	un, _ := r.list.Row(ckUnsup)
	un.Adjust(1)
	if w := warned(); !slices.Equal(w, []string{ckUnsup}) {
		t.Errorf("only the switch is amber at first, got %v", w)
	}
	wind, _ := r.list.Row(ckWind)
	wind.Adjust(1)
	if v := viewOf(t, r, ckWind); !v.Warn {
		t.Errorf("winding past its range is not amber (%q)", v.Value)
	}
	if !r.cs.Flagged() {
		t.Error("the map is not tagged unsupported")
	}
	if line, _ := r.status(); !strings.Contains(line, "UNSUPPORTED") {
		t.Errorf("the status says %q", line)
	}
	if viewOf(t, r, ckMinSpan).Warn {
		t.Error("a bar in range is amber")
	}
	// And back in range, it is not.
	wind.Adjust(-1)
	if viewOf(t, r, ckWind).Warn || r.cs.Flagged() {
		t.Error("still warned back in range")
	}
	un.Adjust(-1)
	if r.cs.spec.Unsupported {
		t.Error("the switch did not turn off")
	}
}

func TestCustomWarnsOnTheHillSlopeRule(t *testing.T) {
	r := newCustomRig(t, true)
	r.cs.spec.Unsupported = true
	r.cs.spec.Step[custom.Hills] = custom.Sliders[custom.Hills].Max
	r.cs.spec.Step[custom.HillSpan] = 0
	if !viewOf(t, r, ckHills).Warn || !viewOf(t, r, ckHillSpan).Warn || !r.cs.Flagged() {
		t.Error("the steepest hills on the shortest swell are not flagged")
	}
	// Held to range, the same pair is quietly lengthened and the row says so.
	r.cs.spec.Unsupported = false
	if viewOf(t, r, ckHillSpan).Warn || r.cs.Flagged() {
		t.Error("a supported map warns")
	}
	if v := viewOf(t, r, ckHillSpan).Value; !strings.Contains(v, "(") {
		t.Errorf("hill length %q does not say it was held back", v)
	}
	r.cs.spec.Step[custom.Hills] = 0
	if viewOf(t, r, ckHillSpan).Value != "no hills" {
		t.Errorf("hill length with no hills reads %q", viewOf(t, r, ckHillSpan).Value)
	}
}

func TestCustomWarnsOnGripBelowWinters(t *testing.T) {
	r := newCustomRig(t, true)
	// In winter the road already has winter's grip, so any less is held back.
	r.cs.spec.Step[custom.Season] = int(city.Winter) + 1
	r.cs.spec.Step[custom.Grip] = 0
	if viewOf(t, r, ckGrip).Warn {
		t.Error("a supported map warns on grip")
	}
	if v := viewOf(t, r, ckGrip).Value; !strings.Contains(v, "(") {
		t.Errorf("grip %q does not say it was held to winter's", v)
	}
	r.cs.spec.Unsupported = true
	if !viewOf(t, r, ckGrip).Warn || !r.cs.Flagged() {
		t.Error("grip below winter's on an unsupported map is not flagged")
	}
}

func TestCustomCopyAndPaste(t *testing.T) {
	r := newCustomRig(t, true)
	cp, _ := r.list.Row(ckCopy)
	cp.Activate()
	if r.clipb != r.cs.Code() || r.note == "" {
		t.Errorf("copied %q, note %q", r.clipb, r.note)
	}

	edge := custom.Edge(city.MapSeaside, 41)
	r.clipb = "  " + edge.Encode() + "\n"
	paste, _ := r.list.Row(ckPaste)
	paste.Activate()
	if r.cs.spec.Seed != 41 || r.cs.spec.Base != city.MapSeaside || r.cs.Code() != edge.Encode() {
		t.Errorf("pasted %v", r.cs.spec)
	}
	if got := viewOf(t, r, ckBase).Value; got != "seatown" {
		t.Errorf("the map row reads %q after a paste", got)
	}

	before := r.cs.Code()
	r.clipb = "not a code"
	paste.Activate()
	if r.cs.Code() != before || r.note == "" || r.note == "code pasted" {
		t.Errorf("a bad code changed the map or said %q", r.note)
	}
}

func TestCustomSaveByName(t *testing.T) {
	r := newCustomRig(t, true)
	save, _ := r.list.Row(ckSave)
	save.Activate()
	if r.note != "saved as custom 1" {
		t.Fatalf("note %q", r.note)
	}
	save.Activate()
	if r.note != "saved as custom 2" || len(r.cs.presets) != 2 {
		t.Errorf("note %q with %d saved", r.note, len(r.cs.presets))
	}
	if r.cs.presets["custom 1"] != r.cs.Code() {
		t.Error("the saved code is not the map's")
	}
}

func TestCustomDriveStartsThePlayerDrive(t *testing.T) {
	r := newCustomRig(t, true)
	r.cs.spec.Seed = 77
	r.selectKey(t, ckDrive)
	r.press(menu.Input{Select: true})
	if !slices.Equal(r.driven, []bool{false}) {
		t.Fatalf("the drive was started as %v", r.driven)
	}
	if !r.s.customPlay {
		t.Error("the shell was not told the next game is on the custom map")
	}
	if customOf(r.s.settings.Custom) == nil || r.s.settings.Custom != r.cs.Code() {
		t.Error("the map was not remembered in the settings before starting")
	}
	if got := customOf(r.s.settings.Custom); got.Seed != 77 {
		t.Errorf("remembered seed %d", got.Seed)
	}
}

func TestCustomDriveIsRefusedOnALockedMap(t *testing.T) {
	r := newCustomRig(t, false)
	r.cs.spec = custom.FromMap(city.MapCity, 1)
	r.cs.spec.Step[custom.Traffic] = int(city.MapMotorway) + 1 // the roadtown's traffic, not held
	if viewOf(t, r, ckDrive).Disabled == "" {
		t.Fatal("Drive is open with traffic that is locked")
	}
	r.selectKey(t, ckDrive)
	r.press(menu.Input{Select: true})
	if len(r.driven) != 0 || r.s.customPlay || r.s.settings.Custom == r.cs.Code() {
		t.Error("a map the player cannot drive was started or remembered")
	}
}

func TestCustomDoneRemembersAndGoesBack(t *testing.T) {
	sh := testShell(t)
	sh.settings.UnlockAll = true
	sh.openPage(pageCustom)
	pg := sh.pager.cur
	if sh.screen != pageCustom {
		t.Fatalf("on screen %v", sh.screen)
	}
	row, _ := pg.List.Row(ckSeed)
	row.Adjust(5)
	sh.stepPage(navInput{Back: true})
	if sh.screen == pageCustom {
		t.Fatal("esc did not leave the page")
	}
	got := customOf(sh.settings.Custom)
	if got == nil || got.Seed != custom.FromMap(sh.settings.Map, 1).Seed+5 {
		t.Errorf("the settings hold %v", got)
	}
	// It is on disk too, so the next run finds it.
	if saved := LoadSettings(); saved.Custom != sh.settings.Custom {
		t.Error("the map was kept in memory only")
	}
}

func TestCustomPageKeepsItsPlanWhileNothingChanges(t *testing.T) {
	r := newCustomRig(t, true)
	r.press(menu.Input{})
	first := r.plan
	if first == nil {
		t.Fatal("no plan after a frame")
	}
	for range 100 {
		r.press(menu.Input{})
	}
	if r.plan != first || r.cache.builds != 1 {
		t.Errorf("plan rebuilt: %d builds", r.cache.builds)
	}
	// Moving the selection, or changing what is not drawn, builds nothing.
	r.press(menu.Input{Down: true})
	row, _ := r.list.Row(ckWeather)
	row.Adjust(1)
	r.press(menu.Input{})
	if r.cache.builds != 1 {
		t.Errorf("%d builds after the selection moved and the weather changed", r.cache.builds)
	}
	// A slider that is drawn builds one once it has been still, and only then.
	wind, _ := r.list.Row(ckWind)
	wind.Adjust(1)
	r.press(menu.Input{})
	if r.cache.builds != 1 || !r.cache.Stale(r.cs.spec) {
		t.Errorf("a plan was built the frame the slider moved: %d builds", r.cache.builds)
	}
	for range planSettle {
		r.press(menu.Input{})
	}
	if r.cache.builds != 2 || r.cache.Stale(r.cs.spec) || !r.plan.Winding {
		t.Errorf("the settled spec was not built: %d builds, winding %v", r.cache.builds, r.plan.Winding)
	}
}

func TestCustomSectionFollowsTheSelection(t *testing.T) {
	r := newCustomRig(t, true)
	last := -1
	seen := map[int]bool{}
	for range r.list.Len() {
		sec := r.section()
		if want := customSectionOf(r.list.SelectedKey()); sec != want {
			t.Fatalf("section %d for %s", sec, r.list.SelectedKey())
		}
		if sec < last {
			t.Fatalf("walking down went from section %d to %d", last, sec)
		}
		inSection := false
		for _, at := range r.sectionRows(sec) {
			inSection = inSection || at == r.list.Sel()
		}
		if !inSection {
			t.Fatalf("%s is not among its own section's rows", r.list.SelectedKey())
		}
		last = sec
		seen[sec] = true
		r.press(menu.Input{Down: true})
	}
	if len(seen) != len(customSections) {
		t.Errorf("walked through %d of %d sections", len(seen), len(customSections))
	}
	// One more press wraps to the start.
	if r.list.SelectedKey() != ckBase {
		t.Errorf("wrapped to %s", r.list.SelectedKey())
	}
	r.press(menu.Input{Up: true})
	if r.list.SelectedKey() != ckDrive || r.section() != len(customSections)-1 {
		t.Errorf("up from the top is %s", r.list.SelectedKey())
	}
}

func TestCustomFrameFitsTheWindow(t *testing.T) {
	for _, size := range [][2]int32{{1280, 720}, {1024, 600}, {1920, 1080}} {
		w, h := size[0], size[1]
		f := customFrameFor(w, h)
		for name, r := range map[string]struct{ x, y, w, h float32 }{
			"tabs": {f.Tabs.X, f.Tabs.Y, f.Tabs.Width, f.Tabs.Height},
			"rows": {f.Rows.X, f.Rows.Y, f.Rows.Width, f.Rows.Height},
			"plan": {f.Plan.X, f.Plan.Y, f.Plan.Width, f.Plan.Height},
			"card": {f.Card.X, f.Card.Y, f.Card.Width, f.Card.Height},
		} {
			if r.x < 0 || r.y < 0 || r.x+r.w > float32(w) || r.y+r.h > float32(h) || r.w <= 0 || r.h <= 0 {
				t.Errorf("%dx%d: %s at %+v is not in the window", w, h, name, r)
			}
		}
		if f.Plan.X < f.Rows.X+f.Rows.Width || f.Plan.Y+f.Plan.Height > f.Card.Y {
			t.Errorf("%dx%d: the plan overlaps the rows or the card", w, h)
		}
		if f.Drive.X < f.Card.X || f.Drive.X+f.Drive.Width > f.Card.X+f.Card.Width {
			t.Errorf("%dx%d: the button is not on the card", w, h)
		}
		if f.Rows.Y+f.Rows.Height > float32(h-pageFoot) {
			t.Errorf("%dx%d: the rows run into the hint bar", w, h)
		}
	}
}

func TestCustomPageNeverOffersWhatItsAccessForbids(t *testing.T) {
	// The page offers exactly the maps the editor steps through.
	r := newCustomRig(t, false)
	r.s.career.Banked = 0
	got := mapNames(r.heldMaps())
	for _, m := range city.Maps() {
		held := (pack.Access{}).HasMap(m)
		if held != slices.Contains(got, m.Name()) && m != r.cs.spec.Base {
			t.Errorf("%s: held %v, offered %v", m.Name(), held, slices.Contains(got, m.Name()))
		}
	}
}
