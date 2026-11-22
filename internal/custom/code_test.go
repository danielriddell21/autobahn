package custom_test

import (
	"errors"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/weather"
)

func TestCodeRoundTrip(t *testing.T) {
	for _, m := range city.Maps() {
		s := custom.FromMap(m, 0xDEADBEEF12345678)
		s.Coast, s.Weather = true, weather.Held(weather.Fog)
		got, err := custom.Decode(s.Encode())
		if err != nil || got != s {
			t.Errorf("%s: %+v became %+v (%v)", m.Name(), s, got, err)
		}
	}
	if code := custom.Edge(city.MapCity, 1).Encode(); !strings.HasPrefix(code, "AB1-") {
		t.Errorf("code %q has no prefix", code)
	}
}

func TestFuzzedCodeRoundTrip(t *testing.T) {
	r := rand.New(rand.NewPCG(9, 9))
	for range 2000 {
		s := randomSpec(r)
		code := s.Encode()
		got, err := custom.Decode(code)
		if err != nil || got != s {
			t.Fatalf("%+v -> %s -> %+v (%v)", s, code, got, err)
		}
		// Forgiving of how a person copies it.
		loose := strings.ToLower(strings.ReplaceAll(code, "-", " "))
		if got, err := custom.Decode(loose); err != nil || got != s {
			t.Fatalf("%q did not decode: %v", loose, err)
		}
	}
}

func TestBadCodesAreRefused(t *testing.T) {
	good := custom.FromMap(city.MapVillage, 5).Encode()
	// Change one character in each place in turn: the checksum catches it.
	for i := len("AB1-"); i < len(good); i++ {
		alt := byte('0')
		if good[i] == '0' {
			alt = '1'
		}
		bad := good[:i] + string(alt) + good[i+1:]
		if _, err := custom.Decode(bad); err == nil {
			t.Errorf("a mistyped character at %d was accepted: %s", i, bad)
		}
	}
	for _, bad := range []string{"", "AB1-", "AB2-" + good[4:], good[4:], "AB1-U!U", good[:len(good)-3]} {
		if _, err := custom.Decode(bad); !errors.Is(err, custom.ErrBadCode) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

// goldenCode is a code that has been shared, and has to go on decoding to the
// same map for ever. If this fails the format was changed instead of extended.
const goldenCode = "AB1-041CP17VE4FG20004CMJ89H24CGJE99848GJ80G1J0"

func goldenSpec() custom.Spec {
	return custom.Spec{
		Base: city.MapVillage, Seed: 1234567890123,
		Step:    [custom.NumSliders]int{3, 9, 4, 6, 2, 3, 1, 7, 5, 8, 2, 1, 4},
		Coast:   true,
		Weather: weather.Held(weather.Rain),
	}
}

func TestGoldenCodeKeepsDecoding(t *testing.T) {
	want := goldenSpec()
	if got := want.Encode(); got != goldenCode {
		t.Errorf("version 1 now encodes the golden map as\n%s\nnot\n%s", got, goldenCode)
	}
	got, err := custom.Decode(goldenCode)
	if err != nil || got != want {
		t.Errorf("the golden code decoded to %+v (%v), want %+v", got, err, want)
	}
}
