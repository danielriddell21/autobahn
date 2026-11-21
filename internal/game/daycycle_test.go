package game

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

func TestEachTimeOfDayIsLitAsItself(t *testing.T) {
	for tod := range NumTimes {
		got, want := lightingAt(tod.hour()), lightingFor(tod)
		if mathx.Abs(got.night-want.night) > 0.001 || got.skyTop != want.skyTop {
			t.Errorf("%v at %.1fh is not lit as %v", tod, tod.hour(), tod)
		}
	}
}

// The light must never jump: a small step on the clock is a small change.
func TestTheLightChangesSmoothly(t *testing.T) {
	const step = 1.0 / 60
	prev := lightingAt(0)
	for h := float32(step); h <= 24.001; h += step {
		l := lightingAt(h)
		if d := mathx.Abs(l.night - prev.night); d > 0.02 {
			t.Fatalf("night jumps %.3f at %.2fh", d, h)
		}
		ds := l.sunDir
		dp := prev.sunDir
		if dot := ds.X*dp.X + ds.Y*dp.Y + ds.Z*dp.Z; dot < 0.99 {
			t.Fatalf("the light in the sky jumps at %.2fh (dot %.3f)", h, dot)
		}
		if ds.Y <= 0 {
			t.Fatalf("the light comes from below the horizon at %.2fh", h)
		}
		prev = l
	}
}

func TestTheSunCrossesTheSky(t *testing.T) {
	morning, noon, evening := sunAt(8), sunAt(13), sunAt(18)
	if morning.X <= 0 || evening.X >= 0 {
		t.Errorf("the sun should rise in the east (+x) and set in the west: %.2f then %.2f", morning.X, evening.X)
	}
	if noon.Y <= morning.Y || noon.Y <= evening.Y {
		t.Error("the sun should be highest in the middle of the day")
	}
}

func TestTheClockRunsAtTheDayLength(t *testing.T) {
	c := dayClock{minutes: 12}
	c.set(TimeDay)
	for range 60 * 60 {
		c.step(1.0 / 60)
	}
	// A minute of a twelve-minute day is two hours.
	if mathx.Abs(c.hour-14) > 0.01 {
		t.Errorf("after a minute the clock reads %.2f, want 14", c.hour)
	}
	held := dayClock{}
	held.set(TimeDusk)
	held.step(5)
	if held.hour != TimeDusk.hour() {
		t.Error("a fixed day should not move")
	}
}

func TestSkipGlidesToTheNextTimeOfDay(t *testing.T) {
	want := []TimeOfDay{TimeDusk, TimeNight, TimeDawn, TimeDay}
	c := dayClock{}
	c.set(TimeDay)
	for _, w := range want {
		c.skip()
		c.step(skipSeconds / 2)
		if c.stage() == w && mathx.Abs(c.hour-w.hour()) < 0.01 {
			t.Fatalf("the skip to %v cut straight there", w)
		}
		for range 200 {
			c.step(skipSeconds / 100)
		}
		if d := wrapHour(c.hour - w.hour() + 12); c.stage() != w || mathx.Abs(d-12) > 0.01 {
			t.Fatalf("after a skip the clock reads %.2f (%v), want %v", c.hour, c.stage(), w)
		}
	}
}

func TestDayLengthsHaveNamesAndWrap(t *testing.T) {
	if DayFixed.Minutes() != 0 || DayFixed.String() != "fixed" || DayMedium.String() != "12 min" {
		t.Error("day lengths are misnamed")
	}
	if cycleDayLength(DayFixed, -1) != DayLong || cycleDayLength(DayLong, 1) != DayFixed {
		t.Error("day lengths should wrap")
	}
}
