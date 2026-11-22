package audio

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/weather"
)

func TestThunderComesOnceAndOnlyAfterItsDelay(t *testing.T) {
	var s Sky
	f := weather.Flash{At: 20, Delay: 3, Strength: 1}
	if s.ready(f, true, 21) {
		t.Error("thunder before it could have arrived")
	}
	if s.ready(f, false, 30) {
		t.Error("thunder with no strike")
	}
	if !s.ready(f, true, 23.01) {
		t.Error("thunder did not come when due")
	}
	if s.ready(f, true, 24) {
		t.Error("the same thunder came twice")
	}
	if !s.ready(weather.Flash{At: 40, Delay: 1}, true, 41.5) {
		t.Error("the next strike's thunder did not come")
	}
}

func TestRainIsLouderTheHarderItFalls(t *testing.T) {
	if RainVolume(0) != 0 || RainVolume(1) <= RainVolume(0.4) || RainVolume(5) != RainVolume(1) {
		t.Error("rain volume does not follow rain")
	}
}

func TestASkyWithNoDeviceIsSilent(t *testing.T) {
	s := OpenSky(nil)
	s.Update(weather.State{Rain: 1}, weather.Flash{}, true, 100)
	s.Close()
}
