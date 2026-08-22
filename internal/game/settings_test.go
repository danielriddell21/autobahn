package game

import (
	"testing"
)

func TestDefaultSettingsAreInRange(t *testing.T) {
	d := DefaultSettings()
	got := d
	got.clamp()
	if got != d {
		t.Errorf("clamp changed the defaults:\n got %+v\nwant %+v", got, d)
	}
}

func TestClampRepairsAnEditedFile(t *testing.T) {
	// The settings file is plain JSON a player can open, so nothing read from
	// it is trusted to be sensible.
	s := Settings{Volume: 4, Traffic: -20, Police: 9000, Camera: 42}
	s.clamp()

	if s.Volume != 1 {
		t.Errorf("Volume = %v, want 1", s.Volume)
	}
	if s.Traffic != trafficMin {
		t.Errorf("Traffic = %d, want %d", s.Traffic, trafficMin)
	}
	if s.Police != policeMax {
		t.Errorf("Police = %d, want %d", s.Police, policeMax)
	}
	if s.Camera != CameraChase {
		t.Errorf("Camera = %v, want the default viewpoint", s.Camera)
	}
}

func TestClampLeavesAValidDocumentAlone(t *testing.T) {
	s := Settings{Volume: 0.35, Muted: true, Traffic: 40, Police: 3, Camera: CameraBonnet}
	want := s
	s.clamp()
	if s != want {
		t.Errorf("clamp altered valid settings:\n got %+v\nwant %+v", s, want)
	}
}

func TestApplyToCarriesThePreferences(t *testing.T) {
	s := Settings{Traffic: 30, Police: 2, ShowPanel: false, Camera: CameraHigh}
	got := s.applyTo(DefaultOptions())

	if got.Traffic != 30 || got.Police != 2 {
		t.Errorf("world size = %d traffic, %d police; want 30 and 2", got.Traffic, got.Police)
	}
	if got.ShowPanel {
		t.Error("ShowPanel should follow the setting")
	}
	if got.Camera != CameraHigh {
		t.Errorf("Camera = %v, want the overhead view", got.Camera)
	}
}

func TestApplyToLeavesTheCommandLineAlone(t *testing.T) {
	// Asking for two hundred cars on the command line must not be quietly
	// replaced by a preference set weeks ago.
	s := Settings{Traffic: 30, Police: 2, ShowPanel: false, Camera: CameraHigh}
	o := DefaultOptions()
	o.Traffic, o.Police, o.ShowPanel, o.Camera = 200, 12, true, CameraBonnet
	o.Given = Given{"traffic": true, "police": true, "panel": true, "camera": true}

	got := s.applyTo(o)
	if got.Traffic != 200 || got.Police != 12 {
		t.Errorf("world size = %d traffic, %d police; want the 200 and 12 asked for",
			got.Traffic, got.Police)
	}
	if !got.ShowPanel {
		t.Error("-panel was overridden by the stored setting")
	}
	if got.Camera != CameraBonnet {
		t.Errorf("Camera = %v, want the bonnet view asked for", got.Camera)
	}
}

func TestAChaseIgnoresTheStoredWorldSize(t *testing.T) {
	// The snapshot names cars by index, so two ends that disagree about how
	// many there are point every pose at the wrong vehicle. Neither player can
	// see the other's settings screen, so a chase does not read them.
	s := Settings{Traffic: 30, Police: 2, ShowPanel: false, Camera: CameraHigh}
	base := DefaultOptions()

	got := s.applyViewTo(base)
	if got.Traffic != base.Traffic || got.Police != base.Police {
		t.Errorf("world size = %d traffic, %d police; want the shared %d and %d",
			got.Traffic, got.Police, base.Traffic, base.Police)
	}
	// What is drawn over the world is still the player's own business.
	if got.ShowPanel || got.Camera != CameraHigh {
		t.Errorf("the view settings did not carry: panel %v, camera %v", got.ShowPanel, got.Camera)
	}
}

func TestApplyToDoesNotUnmuteACommandLineMute(t *testing.T) {
	// Asking for silence explicitly must not be overridden by a preference
	// that happens to have sound on.
	o := DefaultOptions()
	o.Mute = true
	if got := (Settings{}).applyTo(o); !got.Mute {
		t.Error("a -mute run was unmuted by the stored settings")
	}
}

func TestCameraModeNames(t *testing.T) {
	for mode, want := range map[CameraMode]string{
		CameraChase:  "chase",
		CameraBonnet: "bonnet",
		CameraHigh:   "overhead",
	} {
		if got := mode.String(); got != want {
			t.Errorf("CameraMode(%d) = %q, want %q", mode, got, want)
		}
	}
}

func TestCycleCameraWraps(t *testing.T) {
	if got := cycleCamera(CameraChase, -1); got != numCameraModes-1 {
		t.Errorf("stepping back from the first = %v, want the last", got)
	}
	if got := cycleCamera(numCameraModes-1, 1); got != CameraChase {
		t.Errorf("stepping past the last = %v, want the first", got)
	}
}

func TestMasterVolume(t *testing.T) {
	if got := masterVolume(Settings{Volume: 0.5}); got != 0.5 {
		t.Errorf("volume = %v, want 0.5", got)
	}
	// Muting silences without forgetting the level.
	if got := masterVolume(Settings{Volume: 0.5, Muted: true}); got != 0 {
		t.Errorf("muted volume = %v, want 0", got)
	}
}

func TestHeadlessRunsSkipTheMenus(t *testing.T) {
	// A run that was told what to do has nobody to press a key.
	base := DefaultOptions()
	if base.headless() {
		t.Error("an ordinary run should open on the title screen")
	}
	for name, mutate := range map[string]func(*Options){
		"frames":     func(o *Options) { o.Frames = 100 },
		"record":     func(o *Options) { o.Rec.Path = "out.gif" },
		"screenshot": func(o *Options) { o.Screenshot = "shot.png" },
		"host":       func(o *Options) { o.Host = ":7777" },
		"join":       func(o *Options) { o.Join = "127.0.0.1" },
		"stats":      func(o *Options) { o.Stats = true },
		"chase":      func(o *Options) { o.Chase = true },
	} {
		o := DefaultOptions()
		mutate(&o)
		if !o.headless() {
			t.Errorf("a run with %s set should skip the menus", name)
		}
	}
}

func TestAChaseAlwaysHasAPoliceCar(t *testing.T) {
	// The settings screen allows a world with no police, which is fine for
	// driving and useless for a chase: the joining player would have nothing
	// to control.
	o := DefaultOptions()
	o.Police = 0
	if got := o.chaseReady().Police; got < 1 {
		t.Errorf("Police = %d, want at least one unit for a chase", got)
	}
	// A world that already has units is left alone.
	o.Police = 6
	if got := o.chaseReady().Police; got != 6 {
		t.Errorf("Police = %d, want the requested 6", got)
	}
}
