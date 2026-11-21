package annotate

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/vision"
)

// A sign is boxed in the class of the limit posted on it, so the autopilot
// reads back the limit the road actually has.
func TestASpeedSignIsBoxedAsItsOwnLimit(t *testing.T) {
	for _, mph := range []float32{20, 30, 40, 50} {
		cl := speedClass(mathx.MPH(mph))
		got, ok := cl.SpeedValue()
		if !ok || mathx.Abs(mathx.ToMPH(got)-mph) > 0.1 {
			t.Errorf("a %.0f mph sign is boxed as %v, which reads %.0f", mph, cl, mathx.ToMPH(got))
		}
	}
	if speedClass(mathx.MPH(70)) != vision.ClassSpeed50 {
		t.Error("a limit above any sign there is should be boxed as the fastest")
	}
}
