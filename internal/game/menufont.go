package game

import (
	"embed"
	"math"
	"unicode/utf8"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// The menus' type: Overpass (SIL OFL 1.1, fonts/OFL.txt), embedded and loaded
// on first use. The in-drive HUD keeps raylib's built-in face and never comes
// through here.
//
// A face is loaded once per weight and pixel size, rasterised at exactly that
// size so the text is crisp at one to one, which a single large atlas drawn
// down to 12 px is not. Loading needs a GL context, so it happens on the first
// draw or measure of that face and never at init. If a font will not load,
// typeFace falls back to the built-in face, as it always did. With no window at
// all (the headless tests) nothing can be drawn, and a width is an estimate
// (11/20 of the size a character) rather than a crash.

//go:embed fonts/Overpass-Regular.ttf fonts/Overpass-Bold.ttf fonts/Overpass-ExtraBold.ttf
var menuFontFiles embed.FS

// menuFontFile is the file for each weight: regular, bold, extra bold.
var menuFontFile = [...]string{
	weightRegular: "fonts/Overpass-Regular.ttf",
	weightBold:    "fonts/Overpass-Bold.ttf",
	weightHeavy:   "fonts/Overpass-ExtraBold.ttf",
}

// menuGlyphs is every character the menus may use beyond nothing: ASCII, the
// pound sign, the middle dot, single angle quotes, the multiplication and
// minus signs, bullet, ellipsis, en and em dashes, curly quotes, the degree
// sign and the four arrows. Overpass has all of them in all three weights
// (menufont_test.go holds it to that). A rune outside the set is drawn as the
// font's "?" box, so a page that wants another character adds it here.
const menuGlyphs = " !\"#$%&'()*+,-./0123456789:;<=>?@" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~" +
	"£·‹›×−•…–—‘’“”°←↑→↓"

// fontKey names one loaded face.
type fontKey struct {
	weight fontWeight
	size   int32
}

// loadedFonts is every menu face loaded so far, for the window that loaded
// them (gen is its windowGen: game.go). A key mapped to a font with no texture
// is one that failed to load.
type loadedFonts struct {
	gen   uint64
	faces map[fontKey]rl.Font
}

var menuFonts loadedFonts

// menuFontLoads counts the faces rasterised, which is what the tests hold the
// cache to.
var menuFontLoads int

// menuFontOff forces the built-in face. Tests use it to hold the fallback to
// its old behaviour.
var menuFontOff bool

// menuFont returns the font for f and whether there is one. It is false with
// no window, or if the font would not load.
func menuFont(f typeFace) (rl.Font, bool) {
	if menuFontOff || !rl.IsWindowReady() {
		menuFonts = loadedFonts{} // whatever was loaded went with its window
		return rl.Font{}, false
	}
	if menuFonts.gen != windowGen {
		// A different window than the one the fonts were made for (a test
		// opens several): their textures went with the old one.
		menuFonts = loadedFonts{gen: windowGen}
	}
	key := fontKey{f.weight, f.size}
	font, ok := menuFonts.faces[key]
	if !ok {
		font = loadMenuFont(f)
		menuFontLoads++
		if menuFonts.faces == nil {
			menuFonts.faces = map[fontKey]rl.Font{}
		}
		menuFonts.faces[key] = font
	}
	return font, font.Texture.ID != 0
}

// loadMenuFont rasterises one face. A zero Font means it failed.
func loadMenuFont(f typeFace) rl.Font {
	if f.weight < 0 || int(f.weight) >= len(menuFontFile) || f.size <= 0 {
		return rl.Font{}
	}
	data, err := menuFontFiles.ReadFile(menuFontFile[f.weight])
	if err != nil {
		return rl.Font{}
	}
	font := rl.LoadFontFromMemory(".ttf", data, f.size, []rune(menuGlyphs))
	if !rl.IsFontValid(font) {
		return rl.Font{}
	}
	rl.SetTextureFilter(font.Texture, rl.FilterBilinear)
	return font
}

// unloadMenuFonts frees the menu faces and forgets them. It is Shell.Close's
// and closeWindow's, while the window is still open; fonts made for a window
// that has gone are only forgotten.
func unloadMenuFonts() {
	if rl.IsWindowReady() && menuFonts.gen == windowGen {
		for _, font := range menuFonts.faces {
			if font.Texture.ID != 0 {
				rl.UnloadFont(font)
			}
		}
	}
	menuFonts = loadedFonts{}
}

// spacing is the gap raylib adds between glyphs: none, except that very small
// type reads better a little open.
func (f typeFace) spacing() float32 {
	switch {
	case f.size <= 12:
		return 0.6
	case f.size <= 14:
		return 0.3
	}
	return 0
}

// drawText puts s with its top-left corner at x, y, in the bundled font where
// there is one and in the built-in face where there is not.
func (f typeFace) drawText(s string, x, y int32, col rl.Color) {
	font, ok := menuFont(f)
	if !ok {
		if rl.IsWindowReady() {
			rl.DrawText(s, x, y, f.size, col)
		}
		return
	}
	rl.DrawTextEx(font, s, rl.NewVector2(float32(x), float32(y)), float32(f.size), f.spacing(), col)
}

// measureText is how wide s is, in whole pixels, rounded up so a box fitted to
// it never clips it.
func (f typeFace) measureText(s string) int32 {
	font, ok := menuFont(f)
	if !ok {
		if !rl.IsWindowReady() {
			// The built-in face is not there to ask without a window (and
			// asking after one has closed is a crash), so say roughly.
			return int32(utf8.RuneCountInString(s)) * f.size * 11 / 20
		}
		return rl.MeasureText(s, f.size)
	}
	return int32(math.Ceil(float64(rl.MeasureTextEx(font, s, float32(f.size), f.spacing()).X)))
}
