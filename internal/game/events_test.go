package game

import (
	"image/color"
	"testing"

	"github.com/danielriddell21/autobahn/internal/pack"
)

// The vision system reads the camera by colour, so nothing an event puts in the
// scene may be one of the colours it has reserved for its boxes, and none may
// have a channel at full, as every reserved colour has.
func TestNoEventColourIsReserved(t *testing.T) {
	reserved := map[color.RGBA]bool{}
	for _, c := range BoxColours() {
		reserved[c] = true
	}
	for _, c := range eventColours() {
		if reserved[toRGBAColor(c)] {
			t.Errorf("events draw in %v, which a detection box is drawn in", c)
		}
		if c.R == 255 || c.G == 255 || c.B == 255 {
			t.Errorf("events draw in %v, with a channel at full", c)
		}
		if c.A != 255 {
			t.Errorf("event colour %v names a material or is see-through (alpha %d)", c, c.A)
		}
	}
}

// A firework's sparks fade as they age, and never get brighter than they were
// defined, so a pale colour scaled past full cannot wrap round.
func TestSparksOnlyDim(t *testing.T) {
	for serial := -5; serial < 12; serial++ {
		base := eventSparks[((serial%len(eventSparks))+len(eventSparks))%len(eventSparks)]
		prev := base
		for _, life := range []float32{0, 0.25, 0.5, 0.9, 1, 2, -1} {
			c := sparkColour(serial, life)
			if c.R > base.R || c.G > base.G || c.B > base.B {
				t.Fatalf("spark %v brighter than its definition %v", c, base)
			}
			if life >= 0 && life <= 1 && (c.R > prev.R || c.G > prev.G || c.B > prev.B) && life > 0 {
				t.Fatalf("spark brightens with age at %v: %v after %v", life, c, prev)
			}
			if life > 0 && life <= 1 {
				prev = c
			}
		}
	}
}

func TestNightPackIsTheOneThatHoldsEvents(t *testing.T) {
	var found bool
	for _, c := range registered {
		if c.ID == pack.Night {
			found = true
			if c.Dressing == nil || c.Audio == nil {
				t.Error("the night pack has no dressing or audio")
			}
		}
	}
	if !found {
		t.Fatal("the night pack is not registered")
	}
}

func TestDayClockCountsMidnights(t *testing.T) {
	c := dayClock{hour: 23, minutes: 6}
	start := c.abs()
	for range 60 * 60 {
		c.step(1.0 / 60)
	}
	// A six minute day: a minute is four hours.
	if got := c.abs() - start; got < 3.9 || got > 4.1 {
		t.Errorf("an hour of real minutes moved the clock %.2f hours", got)
	}
	if c.day != 1 || c.hour > 4 {
		t.Errorf("the clock reads day %d hour %.2f after passing midnight", c.day, c.hour)
	}
}
