package annotate_test

import (
	"slices"
	"testing"

	"github.com/danielriddell21/autobahn/internal/annotate"
	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/mathx"
	"github.com/danielriddell21/autobahn/internal/sim"
	"github.com/danielriddell21/autobahn/internal/vision"
	"github.com/danielriddell21/autobahn/internal/weather"
)

func inWeather(c weather.Cond) weather.State {
	return weather.At(1, city.Autumn, weather.Held(c), 0)
}

func count(boxes []annotate.Box, cl vision.Class) int {
	n := 0
	for _, b := range boxes {
		if b.Class == cl {
			n++
		}
	}
	return n
}

func TestClearWeatherLaysOutExactlyAsBefore(t *testing.T) {
	w := world(t)
	_, view := bonnet()
	for i := range 5 {
		w.Time = float32(i) * 0.37
		if !slices.Equal(annotate.LayoutIn(w, view(w), inWeather(weather.Clear)), annotate.Layout(w, view(w))) {
			t.Fatal("clear weather changed the boxes")
		}
		if !slices.Equal(annotate.LayoutMirrorIn(w, view(w), inWeather(weather.Clear)), annotate.LayoutMirror(w, view(w))) {
			t.Fatal("clear weather changed the mirror's boxes")
		}
	}
}

func TestFogHidesWhatIsPastSight(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 3})
	w.Agents = w.Agents[:3]
	for _, a := range w.Agents {
		a.V.Place(mathx.V(1e4, 1e4), 0)
	}
	_, view := bonnet()
	fog := inWeather(weather.Fog)

	put(w, w.Agents[0], 70, 0)
	if count(annotate.Layout(w, view(w)), vision.ClassVehicle) == 0 {
		t.Skip("a car 70 m ahead is not seen even in clear weather")
	}
	for i := range 20 {
		w.Time = float32(i) * 0.05
		if count(annotate.LayoutIn(w, view(w), fog), vision.ClassVehicle) != 0 {
			t.Fatal("a car 70 m off was seen in fog with 45 m of sight")
		}
	}
	put(w, w.Agents[0], 15, 0)
	for i := range 20 {
		w.Time = float32(i) * 0.05
		if count(annotate.LayoutIn(w, view(w), fog), vision.ClassVehicle) == 0 {
			t.Fatal("a car 15 m off was lost in fog")
		}
	}
	// And in the mirror, which is held to the same sight.
	put(w, w.Agents[0], -50, 3.6)
	short := inWeather(weather.Fog)
	short.Sight = 30
	mirror := annotate.MirrorView(vision.DefaultCamera(420, 236), w.City.StandAt(w.Player.Pos, w.Player.Yaw))
	if count(annotate.LayoutMirrorIn(w, mirror, short), vision.ClassVehicle) != 0 {
		t.Error("a car 50 m behind was seen in the mirror with 30 m of sight")
	}
}

func TestHazeIsTheSameForTheSameMomentAndOwnLaneIsKept(t *testing.T) {
	w := world(t)
	_, view := bonnet()
	rain := inWeather(weather.Rain)
	w.Time = 3.21
	a, b := annotate.LayoutIn(w, view(w), rain), annotate.LayoutIn(w, view(w), rain)
	if !slices.Equal(a, b) {
		t.Error("the same moment in the same weather laid out differently")
	}
	cam := vision.DefaultCamera(420, 236)
	for _, bx := range a {
		if bx.MinX < 0 || bx.MinY < 0 || bx.MaxX > float32(cam.Width) || bx.MaxY > float32(cam.Height) || bx.Width() < 2 || bx.Height() < 2 {
			t.Fatalf("a shaken box is off the image or degenerate: %+v", bx)
		}
	}

	// Within 30 m the car's own lane is never lost to haze: with a sight that
	// starts hazing well inside that, the count of its markers holds still.
	haze := inWeather(weather.Fog)
	haze.Sight = 30.5
	want := -1
	for i := range 60 {
		w.Time = float32(i) * 0.05
		n := count(annotate.LayoutIn(w, view(w), haze), vision.ClassLaneMarker) +
			count(annotate.LayoutIn(w, view(w), haze), vision.ClassLaneMarkerAlt)
		if want < 0 {
			want = n
		}
		if n != want {
			t.Fatalf("own-lane markers within 30 m came and went: %d then %d", want, n)
		}
	}
	if want == 0 {
		t.Error("no lane markers within 30 m")
	}
}

func TestFogTakesAwayRoadworksPastSightToo(t *testing.T) {
	w := sim.NewWorld(sim.Config{Seed: 7, Traffic: 0, Police: 0})
	_, view := bonnet()
	fog := inWeather(weather.Fog)
	for i := range 20 {
		w.Time = float32(i) * 3
		for _, b := range annotate.LayoutIn(w, view(w), fog) {
			if b.Class == vision.ClassCone || b.Class == vision.ClassBarrier {
				t.Fatalf("%v marked in a fog that sees %.0f m", b.Class, fog.Sight)
			}
		}
	}
}
