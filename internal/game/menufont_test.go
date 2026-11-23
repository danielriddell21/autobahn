package game

import (
	"encoding/binary"
	"runtime"
	"strings"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// fontRunes reads the characters a TrueType file has glyphs for, from its cmap
// (formats 4 and 12, which is every Unicode table Overpass carries).
func fontRunes(t *testing.T, data []byte) map[rune]bool {
	t.Helper()
	be := binary.BigEndian
	cmap := -1
	for i := range int(be.Uint16(data[4:])) {
		rec := data[12+16*i:]
		if string(rec[:4]) == "cmap" {
			cmap = int(be.Uint32(rec[8:]))
		}
	}
	if cmap < 0 {
		t.Fatal("no cmap table")
	}
	have := map[rune]bool{}
	for i := range int(be.Uint16(data[cmap+2:])) {
		sub := cmap + int(be.Uint32(data[cmap+4+8*i+4:]))
		switch be.Uint16(data[sub:]) {
		case 4:
			n := int(be.Uint16(data[sub+6:])) / 2
			ends, starts := sub+14, sub+16+2*n
			for s := range n {
				lo, hi := be.Uint16(data[starts+2*s:]), be.Uint16(data[ends+2*s:])
				for r := int(lo); r <= int(hi) && hi != 0xFFFF; r++ {
					have[rune(r)] = true
				}
			}
		case 12:
			for g := range int(be.Uint32(data[sub+12:])) {
				rec := data[sub+16+12*g:]
				for r := be.Uint32(rec); r <= be.Uint32(rec[4:]); r++ {
					have[rune(r)] = true
				}
			}
		}
	}
	return have
}

// TestMenuGlyphsAreInEveryWeight holds the character set to what the font
// files carry: a rune in menuGlyphs that a weight lacks would draw as a box.
func TestMenuGlyphsAreInEveryWeight(t *testing.T) {
	for _, file := range menuFontFile {
		data, err := menuFontFiles.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		have := fontRunes(t, data)
		for _, r := range menuGlyphs {
			if !have[r] {
				t.Errorf("%s has no glyph for %q (U+%04X)", file, r, r)
			}
		}
	}
	for _, r := range "£·‹›×−•…←↑→↓" {
		if !strings.ContainsRune(menuGlyphs, r) {
			t.Errorf("menuGlyphs lacks %q", r)
		}
	}
}

// TestMenuFontFallsBackWithoutAWindow is the headless case: no GL context, so
// no bundled font and no built-in one either, so widths are estimates.
func TestMenuFontFallsBackWithoutAWindow(t *testing.T) {
	if rl.IsWindowReady() {
		t.Skip("a window is open")
	}
	for _, f := range []typeFace{faceTitle, faceRow, faceBody, faceSmall, faceChip} {
		if _, ok := menuFont(f); ok {
			t.Errorf("%v: a font loaded with no window", f)
		}
	}
	// Unloading with nothing loaded is harmless, and measuring and drawing
	// with no window neither crash nor report nothing.
	unloadMenuFonts()
	if got := faceRow.width("Garage"); got <= 0 || got > 6*faceRow.size {
		t.Errorf("headless width of Garage is %d", got)
	}
	faceRow.draw("Garage", 0, 0, signInk)
}

// TestMenuFontsLoad opens a window and checks that all three weights load,
// that heavier weights measure wider, and that the fallback can be forced.
func TestMenuFontsLoad(t *testing.T) {
	requireDisplay(t)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	rl.SetTraceLogLevel(rl.LogError)
	rl.InitWindow(320, 180, "autobahn font test")
	defer rl.CloseWindow()
	defer unloadMenuFonts()

	const s = "Hot hatch - Rosso, banks £1,500 …"
	var widths [3]int32
	for w := weightRegular; w <= weightHeavy; w++ {
		f := typeFace{20, w}
		if _, ok := menuFont(f); !ok {
			t.Fatalf("weight %d did not load", w)
		}
		widths[w] = f.width(s)
	}
	if widths[weightRegular] >= widths[weightBold] || widths[weightBold] >= widths[weightHeavy] {
		t.Errorf("widths regular %d, bold %d, extra bold %d: want each wider than the last", widths[0], widths[1], widths[2])
	}
	if got := rl.MeasureText(s, 20); widths[weightBold] == got {
		t.Errorf("bundled width equals the built-in face's (%d): the font is not in use", got)
	}
	// Every character the menus use measures as a glyph, not as nothing.
	for _, r := range menuGlyphs {
		if r != ' ' && faceBody.width(string(r)) <= 0 {
			t.Errorf("%q measures as nothing", r)
		}
	}
	// Drawing with every face must not fail.
	rl.BeginDrawing()
	for _, f := range []typeFace{faceTitle, faceRow, faceBody, faceSmall, faceChip} {
		f.draw(s, 4, 4, signInk)
	}
	rl.EndDrawing()

	menuFontOff = true
	defer func() { menuFontOff = false }()
	if got, want := faceBody.width(s), rl.MeasureText(s, faceBody.size); got != want {
		t.Errorf("forced fallback width %d, built-in %d", got, want)
	}
}

// TestMenuFontsFollowTheWindow: fonts made for one window are not used in the
// next. The tests open several; a stale texture would draw garbage.
func TestMenuFontsFollowTheWindow(t *testing.T) {
	requireDisplay(t)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	rl.SetTraceLogLevel(rl.LogError)
	for range 2 {
		rl.InitWindow(320, 180, "autobahn font test")
		if _, ok := menuFont(faceRow); !ok {
			rl.CloseWindow()
			t.Fatal("the font did not load")
		}
		rl.CloseWindow()
	}
	if _, ok := menuFont(faceRow); ok {
		t.Error("a font was offered with no window open")
	}
}
