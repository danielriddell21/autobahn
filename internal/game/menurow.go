package game

import (
	"github.com/danielriddell21/crucible/menu"
)

// The menu model the Signpost screens are built on (docs/menus.md, "Building
// it", stage 1). A screen is a list of rows; each row has a stable key, says
// what it is and what it is set to separately, and knows what ←→ and enter do
// to it. Movement is crucible's menu.Menu, so the keys are the family's.
//
// Nothing here draws. A list hands out [rowView]s, which say everything a row
// shows, and signpost.go turns those into pixels; tests read the views and
// need no window.

// rowKind is what a row is, and so what the keys do to it.
type rowKind int

const (
	// rowAction does something on enter: opens a screen, starts a drive. ←→
	// do nothing.
	rowAction rowKind = iota
	// rowSwitch is on or off. → turns it on, ← off, and enter flips it.
	rowSwitch
	// rowOptions is one of a few named choices. ←→ step through them,
	// wrapping at both ends, and enter steps forward.
	rowOptions
	// rowBar is a level from 0 to 1, drawn as a bar with its words beside it.
	// ←→ step it; enter does nothing.
	rowBar
	// rowStepper is a number with − and + either side. ←→ step it; enter does
	// nothing.
	rowStepper
)

// tone is what a colour means. A row, a chip or a card is given a tone by
// the model and the theme decides the colour, so every screen uses the same
// colours for the same things (docs/menus.md, "Principles").
type tone int

const (
	// tonePlain is ordinary white type.
	tonePlain tone = iota
	// toneYours is green: owned, held, done.
	toneYours
	// toneOffer is amber: for sale, next, or a warning.
	toneOffer
	// toneLocked is grey: not held, and cannot be used.
	toneLocked
	// toneDanger is red: quitting, abandoning.
	toneDanger
)

// menuRow is one row of a screen. Build one with [actionRow], [switchRow],
// [optionsRow], [barRow] or [stepperRow] and refine it with the methods below;
// the hooks are read every frame, so a row always shows the state it stands for.
type menuRow struct {
	// Key names the row for good. Selection is kept by key when a screen
	// rebuilds its rows, and tests find rows by it.
	Key string
	// Label is what the row is: "Volume", never "Volume: 80%".
	Label string
	// Desc is an optional line saying what the row does.
	Desc string
	Kind rowKind

	// Note is the words for what the row is set to: a bar's "40 cars", a
	// stepper's number, or what an action says beside it ("Saloon · red").
	Note func() string
	// On is a switch's state.
	On func() bool
	// Options are an options row's choices and Choice the one set.
	Options []string
	Choice  func() int
	// Level is a bar's fill, from 0 to 1.
	Level func() float64
	// Warn says the value is past what has been measured, which is amber.
	Warn func() bool
	// Tone is the row's own meaning, if it has one: a red Quit, a green
	// "yours". A disabled row is grey whatever this says.
	Tone func() tone
	// Disabled says why the row cannot be used now, or "" when it can.
	Disabled func() string

	// Adjust runs on ← (dir -1) and → (dir +1); nil means the row takes no
	// adjusting. Activate runs on enter; nil means enter does nothing.
	Adjust   func(dir int)
	Activate func()
}

// actionRow is a row that does something on enter.
func actionRow(key, label string, do func()) menuRow {
	return menuRow{Key: key, Label: label, Kind: rowAction, Activate: do}
}

// switchRow is an on/off row over a getter and a setter, so a setting stored
// inverted (Sound over Muted) needs no special case.
func switchRow(key, label string, get func() bool, set func(bool)) menuRow {
	return menuRow{
		Key: key, Label: label, Kind: rowSwitch, On: get,
		Adjust:   func(dir int) { set(dir > 0) },
		Activate: func() { set(!get()) },
	}
}

// optionsRow is a choice among opts, get and set being its index.
func optionsRow(key, label string, opts []string, get func() int, set func(int)) menuRow {
	step := func(dir int) {
		if n := len(opts); n > 0 {
			set(((get()+dir)%n + n) % n)
		}
	}
	return menuRow{
		Key: key, Label: label, Kind: rowOptions, Options: opts, Choice: get,
		Adjust:   step,
		Activate: func() { step(1) },
	}
}

// barRow is a level with its words: level reports the fill from 0 to 1,
// words what it means ("80%", "40 cars"), and step moves it one press.
func barRow(key, label string, level func() float64, words func() string, step func(dir int)) menuRow {
	return menuRow{Key: key, Label: label, Kind: rowBar, Level: level, Note: words, Adjust: step}
}

// stepperRow is a number with − and +: value words it and step moves it.
func stepperRow(key, label string, value func() string, step func(dir int)) menuRow {
	return menuRow{Key: key, Label: label, Kind: rowStepper, Note: value, Adjust: step}
}

// describe gives the row a line saying what it does.
func (r menuRow) describe(desc string) menuRow { r.Desc = desc; return r }

// disabledBy makes the row unusable while why returns a reason.
func (r menuRow) disabledBy(why func() string) menuRow { r.Disabled = why; return r }

// toned gives the row a meaning, and so a colour.
func (r menuRow) toned(t func() tone) menuRow { r.Tone = t; return r }

// noted gives the row words for what it is set to.
func (r menuRow) noted(note func() string) menuRow { r.Note = note; return r }

// warned turns the row's value amber while warn reports true.
func (r menuRow) warned(warn func() bool) menuRow { r.Warn = warn; return r }

// why is the reason the row cannot be used, or "".
func (r menuRow) why() string {
	if r.Disabled == nil {
		return ""
	}
	return r.Disabled()
}

// rowView is everything a row shows, worked out from its hooks: what the
// drawing reads and what the tests check.
type rowView struct {
	Key, Label, Desc string
	Kind             rowKind
	// Value is the words for what the row is set to: a switch's "on" or
	// "off", the option chosen, a bar's or stepper's words, an action's note.
	Value   string
	On      bool
	Options []string
	Choice  int
	Level   float64
	Warn    bool
	Tone    tone
	// Selected is whether the row has the selection, and Disabled why it
	// cannot be used, or "".
	Selected bool
	Disabled string
}

// view works the row out as it stands.
func (r menuRow) view(selected bool) rowView {
	v := rowView{
		Key: r.Key, Label: r.Label, Desc: r.Desc, Kind: r.Kind,
		Options: r.Options, Selected: selected, Disabled: r.why(),
	}
	if r.Note != nil {
		v.Value = r.Note()
	}
	if r.Kind == rowSwitch && r.On != nil {
		v.On = r.On()
		if r.Note == nil {
			v.Value = onOff(v.On)
		}
	}
	if r.Kind == rowOptions && r.Choice != nil && len(r.Options) > 0 {
		v.Choice = min(max(r.Choice(), 0), len(r.Options)-1)
		if r.Note == nil {
			v.Value = r.Options[v.Choice]
		}
	}
	if r.Level != nil {
		v.Level = min(max(r.Level(), 0), 1)
	}
	if r.Warn != nil {
		v.Warn = r.Warn()
	}
	if r.Tone != nil {
		v.Tone = r.Tone()
	}
	if v.Disabled != "" {
		v.Tone = toneLocked
	}
	return v
}

// rowList is a screen's rows and which one is selected.
//
// Disabled rows are not skipped: ↑↓ land on them, so the player can see a
// locked thing and be told why it is locked (docs/menus.md: "Locked is shown,
// never hidden ... and say why when asked"). ←→ and enter on a disabled row
// change nothing and leave the reason in Refused, which the screen shows.
type rowList struct {
	rows []menuRow
	m    menu.Menu
	// Across lays the rows out side by side, as a dialog's buttons are: ←→
	// then move the selection as ↑↓ do, and adjust nothing.
	Across bool
	// Refused is why the last ←→ or enter did nothing, the selected row being
	// disabled. Moving clears it.
	Refused string
}

// newRowList makes a list with the first row selected.
func newRowList(rows ...menuRow) *rowList {
	l := &rowList{}
	l.SetRows(rows)
	return l
}

// SetRows replaces the rows, keeping the selection on the row with the same
// key if there still is one, and as near its old place as there is if not.
func (l *rowList) SetRows(rows []menuRow) {
	key := l.SelectedKey()
	l.rows = rows
	l.m.Items = make([]menu.Item, len(rows))
	for i, r := range rows {
		l.m.Items[i] = menu.Item{
			Label:  func() string { return r.Label },
			Action: r.Activate,
			Adjust: r.Adjust,
		}
	}
	if !l.Select(key) {
		l.m.Sel = min(max(l.m.Sel, 0), max(len(rows)-1, 0))
	}
}

// Len is how many rows there are.
func (l *rowList) Len() int { return len(l.rows) }

// Sel is the index of the selected row.
func (l *rowList) Sel() int { return l.m.Sel }

// Selected is the selected row, if there are any rows.
func (l *rowList) Selected() (menuRow, bool) {
	if len(l.rows) == 0 {
		return menuRow{}, false
	}
	return l.rows[l.m.Sel], true
}

// SelectedKey is the selected row's key, or "" with no rows.
func (l *rowList) SelectedKey() string {
	r, _ := l.Selected()
	return r.Key
}

// Row finds a row by its key.
func (l *rowList) Row(key string) (menuRow, bool) {
	for _, r := range l.rows {
		if r.Key == key {
			return r, true
		}
	}
	return menuRow{}, false
}

// Select puts the selection on the row with key, reporting whether there is
// one.
func (l *rowList) Select(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range l.rows {
		if r.Key == key {
			l.m.Sel = i
			return true
		}
	}
	return false
}

// Move moves the selection by dir rows, wrapping at both ends.
func (l *rowList) Move(dir int) {
	if len(l.rows) == 0 {
		return
	}
	l.Refused = ""
	l.m.Move(dir)
}

// Update applies a frame's keys and says which sound they call for.
func (l *rowList) Update(in menu.Input) menu.Sound {
	if len(l.rows) == 0 {
		return menu.SoundNone
	}
	if l.Across {
		// Side by side, ←→ are the movement and there is nothing to adjust.
		in.Up, in.Down = in.Up || in.Left, in.Down || in.Right
		in.Left, in.Right = false, false
	}
	if in.Up || in.Down {
		l.Refused = ""
		return l.m.Update(menu.Input{Up: in.Up, Down: in.Down})
	}
	r := l.rows[l.m.Sel]
	asked := in.Select || (r.Adjust != nil && (in.Left || in.Right))
	if why := r.why(); why != "" && asked {
		l.Refused = why
		return menu.SoundNone
	}
	return l.m.Update(in)
}

// Views works every row out as it stands, the selected one marked.
func (l *rowList) Views() []rowView {
	out := make([]rowView, len(l.rows))
	for i, r := range l.rows {
		out[i] = r.view(i == l.m.Sel)
	}
	return out
}

// Reason is what to tell the player about the selected row: why the last
// press did nothing, or else why the row is disabled, or "".
func (l *rowList) Reason() string {
	if l.Refused != "" {
		return l.Refused
	}
	r, _ := l.Selected()
	return r.why()
}

// rowWindow is which rows to show when only fit of n will: first up to but
// not including last, the selection kept in view and as near the middle as
// the ends allow.
func rowWindow(n, sel, fit int) (first, last int) {
	if fit <= 0 || n <= 0 {
		return 0, 0
	}
	if n <= fit {
		return 0, n
	}
	first = min(max(sel-fit/2, 0), n-fit)
	return first, first + fit
}
