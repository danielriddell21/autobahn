package city_test

import (
	"encoding/json"
	"math/rand/v2"
	"os"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
)

// The clamp table is held equal to testdata/ranges.json, so that widening a
// range cannot be done without editing a file a reviewer is shown.
func TestRangesMatchTheTable(t *testing.T) {
	raw, err := os.ReadFile("testdata/ranges.json")
	if err != nil {
		t.Fatal(err)
	}
	var want []city.Range
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	got := city.Ranges()
	if len(got) != len(want) {
		t.Fatalf("%d ranges in the code, %d in ranges.json", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("range %d: code has %+v, ranges.json has %+v", i, got[i], want[i])
		}
	}
}

// Every preset lies inside the ranges, and each range is as wide as the
// presets make it: nothing is supported that no preset has been gated at.
func TestRangesAreThePresetsExtremes(t *testing.T) {
	lo, hi := map[string]float64{}, map[string]float64{}
	note := func(name string, v float64) {
		if l, ok := lo[name]; !ok || v < l {
			lo[name] = v
		}
		if h, ok := hi[name]; !ok || v > h {
			hi[name] = v
		}
	}
	for _, m := range city.Maps() {
		p := city.ParamsFor(m, 1)
		got, outside := city.Clamp(p, false)
		if outside || got != p {
			t.Errorf("%s is not inside its own ranges: %+v became %+v", m.Name(), p, got)
		}
		note("minSpan", float64(p.MinSpan))
		note("maxSpan", float64(p.MaxSpan))
		note("roundabouts", float64(p.Roundabouts))
		note("parks", float64(p.Parks))
		note("rise", float64(p.Rise))
		note("floor", float64(p.Floor))
		note("storeys", float64(p.Storeys))
		note("mainEvery", float64(p.MainEvery))
		note("wind", float64(p.Wind))
		note("hills", float64(p.Hills))
		note("bluff", float64(p.Bluff))
		note("limit", float64(mathx.ToMPH(p.Limits[2])))
		if p.Hills > 0 {
			note("hillSpan", float64(p.HillSpan))
		}
	}
	for _, r := range city.Ranges() {
		const eps = 1e-4
		if l, h := lo[r.Name], hi[r.Name]; l-r.Lo > eps || r.Lo-l > eps || h-r.Hi > eps || r.Hi-h > eps {
			t.Errorf("%s: ranges say %v..%v, the presets span %v..%v", r.Name, r.Lo, r.Hi, l, h)
		}
	}
}

func TestClampPullsValuesIn(t *testing.T) {
	p := city.ParamsFor(city.MapCity, 3)
	p.Roundabouts, p.Wind, p.Hills, p.HillSpan = 0.9, 300, 30, 100
	p.MainEvery, p.MinSpan, p.MaxSpan = 1, 10, 20
	p.Limits = [3]float32{mathx.MPH(70), 0, mathx.MPH(5)}
	got, outside := city.Clamp(p, false)
	if outside {
		t.Error("a clamped result claims to be outside the ranges")
	}
	if got.Roundabouts != 0.28 || got.Wind != 40 || got.MainEvery != 2 || got.MinSpan != 70 {
		t.Errorf("not pulled in: %+v", got)
	}
	if got.MaxSpan < got.MinSpan+50 {
		t.Errorf("spans cross: %v..%v", got.MinSpan, got.MaxSpan)
	}
	if got.Limits[0] != mathx.MPH(50) || got.Limits[1] != 0 || got.Limits[2] != mathx.MPH(5) {
		t.Errorf("limits %v", got.Limits)
	}
	if got.Hills != 11 || float64(got.Hills)/float64(got.HillSpan) > 8.0/850*(1+1e-5) {
		t.Errorf("hills %v over %v are steeper than the village's", got.Hills, got.HillSpan)
	}
	if again, _ := city.Clamp(got, false); again != got {
		t.Error("clamping is not idempotent")
	}
}

func TestHillSlopeRule(t *testing.T) {
	p := city.ParamsFor(city.MapMotorway, 1)
	p.Hills, p.HillSpan = 11, 700 // both inside their ranges, but steeper than any preset
	if _, outside := city.Clamp(p, true); !outside {
		t.Error("a slope steeper than the village's was not flagged")
	}
	got, outside := city.Clamp(p, false)
	if outside || got.HillSpan < 1168 {
		t.Errorf("slope not eased: span %v outside %v", got.HillSpan, outside)
	}
	v := city.ParamsFor(city.MapVillage, 1)
	if _, outside := city.Clamp(v, true); outside {
		t.Error("the village's own hills were flagged")
	}
}

func TestClampFlagsWhatIsOutsideButKeepsTheGeneratorSafe(t *testing.T) {
	p := city.ParamsFor(city.MapCity, 1)
	p.Wind = 60
	got, outside := city.Clamp(p, true)
	if !outside || got.Wind != 60 {
		t.Errorf("wind 60: kept %v, flagged %v", got.Wind, outside)
	}
	p.Wind = 5000
	if got, _ := city.Clamp(p, true); got.Wind > 80 {
		t.Errorf("wind %v is past what the generator is safe on", got.Wind)
	}
	if _, outside := city.Clamp(city.ParamsFor(city.MapSeaside, 1), true); outside {
		t.Error("a preset was flagged unsupported")
	}
}

// randomCustom is a clamped set of Params drawn from every range.
func randomCustom(r *rand.Rand, seed uint64) city.Params {
	base := city.ParamsFor(city.Maps()[r.IntN(len(city.Maps()))], seed)
	in := func(name string) float64 {
		rg := city.RangeOf(name)
		return rg.Lo + r.Float64()*(rg.Hi-rg.Lo)
	}
	base.MinSpan, base.MaxSpan = float32(in("minSpan")), float32(in("maxSpan"))
	base.Roundabouts, base.Parks, base.Rise = float32(in("roundabouts")), float32(in("parks")), float32(in("rise"))
	base.Floor, base.Storeys = float32(in("floor")), float32(in("storeys"))
	base.MainEvery = 2 + r.IntN(4)
	base.Wind, base.Hills, base.HillSpan = float32(in("wind")), float32(in("hills")), float32(in("hillSpan"))
	if r.IntN(3) == 0 {
		base.Limits = [3]float32{mathx.MPH(30), mathx.MPH(40), mathx.MPH(50)}
	}
	if r.IntN(3) == 0 {
		base.Coast, base.CoastRow, base.Bluff = true, 3, 20
	}
	got, _ := city.Clamp(base, false)
	return got
}

// A custom map is held to the growth rule like any other: reached in pieces it
// comes out as it does built at once.
func TestRandomCustomGrowsTheSame(t *testing.T) {
	const span = 600
	r := rand.New(rand.NewPCG(17, 3))
	for i := range 20 {
		p := randomCustom(r, uint64(100+i))
		whole := city.NewGrowing(p)
		whole.EnsureAround(mathx.V(0, 0), span)
		grown := city.NewGrowing(p)
		for _, at := range []mathx.Vec{
			mathx.V(-300, -300), mathx.V(300, -300), mathx.V(-300, 300), mathx.V(300, 300), mathx.V(0, 0),
		} {
			grown.EnsureAround(at, span/2)
		}
		grown.EnsureAround(mathx.V(0, 0), span)

		reach := float32(1e9)
		if whole.Laid() {
			reach = span * 0.55
		}
		want := fingerprintWithin(whole, reach)
		if len(want) < 100 {
			t.Fatalf("params %d (%+v): only %d things to compare", i, p, len(want))
		}
		diff(t, want, fingerprintWithin(grown, reach))
		if t.Failed() {
			t.Fatalf("params %d differ: %+v", i, p)
		}
	}
}
