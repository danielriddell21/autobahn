package game

import (
	"math"
	"strconv"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/danielriddell21/autobahn/internal/mathx"
)

// The day goes round on its own. A clock runs through the hours, the light
// is blended between the times of day as it passes them, and the sun (or, at
// night, the moon) crosses the sky. T skips ahead to the next time of day,
// gliding there rather than cutting, and the clock carries on from wherever
// it lands.

// hour is the time on the clock each time of day is set at.
func (t TimeOfDay) hour() float32 {
	switch t {
	case TimeDawn:
		return 6.5
	case TimeDusk:
		return 19
	case TimeNight:
		return 0
	default:
		return 12
	}
}

// DayLength is how many real minutes a whole day lasts. Fixed holds the clock
// still.
type DayLength int

// The day lengths the settings page offers.
const (
	DayFixed DayLength = iota
	DayShort
	DayMedium
	DayLong
	numDayLengths
)

// Minutes returns how long a day lasts in real minutes, or 0 when it is held.
func (d DayLength) Minutes() float32 {
	switch d {
	case DayShort:
		return 6
	case DayMedium:
		return 12
	case DayLong:
		return 24
	default:
		return 0
	}
}

// String returns the length's name, for a settings row to show.
func (d DayLength) String() string {
	if d.Minutes() == 0 {
		return "fixed"
	}
	return strconv.Itoa(int(d.Minutes())) + " min"
}

// skipSeconds is how long T takes to glide to the next time of day.
const skipSeconds = 2.5

// dayClock is the hour the city is lit for, and how fast it moves on.
type dayClock struct {
	hour    float32 // 0 to 24
	minutes float32 // real minutes to a day; 0 holds the clock
	// glide is how many hours a skip still has to cover, and glideRate how
	// fast it covers them, in hours a second.
	glide, glideRate float32
	// day counts the midnights the clock has passed, so evening events can tell
	// one night from the next.
	day int
}

// abs is the time in hours counted from the start of the first day, which is
// what night events keep their nights by.
func (c dayClock) abs() float32 { return float32(c.day)*24 + c.hour }

// set puts the clock at a time of day, at once.
func (c *dayClock) set(t TimeOfDay) {
	c.hour, c.glide = t.hour(), 0
}

// step moves the clock on by dt seconds of real time.
func (c *dayClock) step(dt float32) {
	adv := float32(0)
	if c.minutes > 0 {
		adv = 24 / (c.minutes * 60) * dt
	}
	if c.glide > 0 {
		g := min(c.glideRate*dt, c.glide)
		c.glide -= g
		adv += g
	}
	if next := c.hour + adv; next >= 24 {
		c.day += int(next / 24)
	}
	c.hour = wrapHour(c.hour + adv)
}

// skip starts a glide to the next time of day after the clock's.
func (c *dayClock) skip() {
	at := wrapHour(c.hour + c.glide)
	best := float32(24)
	for t := range NumTimes {
		ahead := wrapHour(t.hour() - at)
		if ahead > 0.01 && ahead < best {
			best = ahead
		}
	}
	c.glide += best
	c.glideRate = c.glide / skipSeconds
}

// stage is the time of day the clock is nearest.
func (c dayClock) stage() TimeOfDay {
	best, gap := TimeDay, float32(24)
	for t := range NumTimes {
		d := wrapHour(c.hour - t.hour())
		d = min(d, 24-d)
		if d < gap {
			best, gap = t, d
		}
	}
	return best
}

func wrapHour(h float32) float32 {
	h = float32(math.Mod(float64(h), 24))
	if h < 0 {
		h += 24
	}
	return h
}

// dayKeys are the hours at which the light is exactly one time of day's.
// Between them it is blended, and the light holds through the middle of the
// day and the night.
var dayKeys = []struct {
	hour float32
	t    TimeOfDay
}{
	{4.5, TimeNight}, {6.5, TimeDawn}, {9, TimeDay}, {16.5, TimeDay}, {19, TimeDusk}, {21, TimeNight},
}

// lightingAt returns the light for an hour on the clock.
func lightingAt(h float32) lighting {
	h = wrapHour(h)
	n := len(dayKeys)
	from, to := dayKeys[n-1], dayKeys[0]
	a, b := from.hour-24, to.hour
	for i := range n - 1 {
		if h >= dayKeys[i].hour && h < dayKeys[i+1].hour {
			from, to = dayKeys[i], dayKeys[i+1]
			a, b = from.hour, to.hour
		}
	}
	if h >= dayKeys[n-1].hour {
		a, b = from.hour, to.hour+24
	}
	f := smooth((h - a) / (b - a))
	l := mixLighting(lightingFor(from.t), lightingFor(to.t), f)
	l.sunDir = sunAt(h)
	return l
}

// sunAt is the direction toward the light in the sky at an hour: the sun from
// six in the morning to eight at night, rising in the east and setting in the
// west, and the moon the rest of the time. Where one hands over to the other
// the two are blended, so the shadows swing rather than jump.
func sunAt(h float32) rl.Vector3 {
	h = wrapHour(h)
	sun := arc((h - 6) / 14)
	// The moon is up from eight at night: before noon it is this morning's
	// moon setting, after noon tonight's yet to rise.
	m := h - 20
	if m < -8 {
		m += 24
	}
	moon := arc(m / 10)
	up := smooth((h-5.4)/1.2) * (1 - smooth((h-19.4)/1.2))
	return norm3(
		mathx.Lerp(moon.X, sun.X, up),
		mathx.Lerp(moon.Y, sun.Y, up),
		mathx.Lerp(moon.Z, sun.Z, up),
	)
}

// arc is a light a fraction f of the way across the sky, from rising in the
// east to setting in the west. It never quite reaches the horizon, so there is
// always something to see the city by.
func arc(f float32) rl.Vector3 {
	a := float64(mathx.Clamp(f, 0, 1)) * math.Pi
	return vec3(float32(math.Cos(a)), 0.18+0.7*float32(math.Sin(a)), 0.4)
}

// smooth eases a fraction from 0 to 1, flat at both ends.
func smooth(f float32) float32 {
	f = mathx.Clamp(f, 0, 1)
	return f * f * (3 - 2*f)
}

func mixLighting(a, b lighting, f float32) lighting {
	v := func(p, q rl.Vector3) rl.Vector3 {
		return vec3(mathx.Lerp(p.X, q.X, f), mathx.Lerp(p.Y, q.Y, f), mathx.Lerp(p.Z, q.Z, f))
	}
	c := func(p, q rl.Color) rl.Color {
		ch := func(x, y uint8) uint8 { return uint8(mathx.Lerp(float32(x), float32(y), f) + 0.5) }
		return rl.NewColor(ch(p.R, q.R), ch(p.G, q.G), ch(p.B, q.B), 255)
	}
	return lighting{
		sunDir: v(a.sunDir, b.sunDir), sun: v(a.sun, b.sun),
		sky: v(a.sky, b.sky), ground: v(a.ground, b.ground), fog: v(a.fog, b.fog),
		fogRange: mathx.Lerp(a.fogRange, b.fogRange, f),
		skyTop:   c(a.skyTop, b.skyTop), skyLow: c(a.skyLow, b.skyLow),
		night: mathx.Lerp(a.night, b.night, f),
	}
}
