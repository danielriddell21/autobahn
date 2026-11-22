package game

import (
	"image/color"
	"testing"

	"github.com/danielriddell21/autobahn/internal/vision"
)

// The vision system reads the camera by colour, so nothing roadworks put in the
// scene may be one of the colours it has reserved for its boxes, and none may
// have a channel at full, as every reserved colour has.
func TestNoWorksColourIsReserved(t *testing.T) {
	reserved := map[color.RGBA]bool{}
	for _, c := range BoxColours() {
		reserved[c] = true
	}
	for _, c := range worksColours() {
		if reserved[toRGBAColor(c)] {
			t.Errorf("roadworks draw in %v, which a detection box is drawn in", c)
		}
		if c.R == 255 || c.G == 255 || c.B == 255 {
			t.Errorf("roadworks draw in %v, with a channel at full", c)
		}
		if c.A != 255 {
			t.Errorf("roadworks colour %v names a material or is see-through (alpha %d)", c, c.A)
		}
	}
}

// Every class the vision system has is in the palette a recording is made with,
// which is what makes BoxColours the one place to ask.
func TestBoxColoursCoverEveryClass(t *testing.T) {
	if got, want := len(BoxColours()), vision.NumClasses-1; got != want {
		t.Errorf("%d box colours for %d classes", got, want)
	}
}
