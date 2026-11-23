package game

import (
	"encoding/json"
	"testing"
)

func settingsPageOf(t *testing.T) (*Shell, *menuPage) {
	t.Helper()
	sh := testShell(t)
	sh.openPage(pageSettings)
	if !sh.onPage() || sh.screen != pageSettings {
		t.Fatal("pageSettings did not open")
	}
	return sh, sh.pager.cur
}

func settingsPress(t *testing.T, pg *menuPage, key string, dir int) {
	t.Helper()
	r, ok := pg.List.Row(key)
	if !ok {
		t.Fatalf("no row %q", key)
	}
	if dir == 0 {
		r.Activate()
	} else {
		r.Adjust(dir)
	}
}

func TestSettingsPageStoresEachSetting(t *testing.T) {
	sh, pg := settingsPageOf(t)
	set := &sh.settings
	settingsPress(t, pg, "menudrive", -1)
	if set.MenuDrive() {
		t.Error("left on Menu drive did not turn it off")
	}
	settingsPress(t, pg, "menudrive", 1)
	if !set.MenuDrive() {
		t.Error("right on Menu drive did not turn it on")
	}
	v := set.Volume
	settingsPress(t, pg, "volume", -1)
	if set.Volume >= v {
		t.Error("volume did not drop")
	}
	settingsPress(t, pg, "viewpoint", 1)
	if set.Camera != cycleCamera(CameraChase, 1) {
		t.Errorf("camera = %v", set.Camera)
	}
	settingsPress(t, pg, "aicamera", -1)
	settingsPress(t, pg, "boxlabels", -1)
	if set.ShowPanel || set.ShowLabels {
		t.Error("view switches did not turn off")
	}
	settingsPress(t, pg, "traffic", 1)
	if set.Traffic != 70+trafficStep {
		t.Errorf("traffic = %d", set.Traffic)
	}
	settingsPress(t, pg, "police", -1)
	if set.Police != 7-policeStep {
		t.Errorf("police = %d", set.Police)
	}
	settingsPress(t, pg, "daylength", 1)
	if set.DayLength != cycleDayLength(DayMedium, 1) {
		t.Errorf("day length = %v", set.DayLength)
	}
	settingsPress(t, pg, "jobsauto", 1)
	if !set.AutopilotJobs {
		t.Error("jobs on auto did not turn on")
	}
}

func TestSettingsBarsClamp(t *testing.T) {
	sh, pg := settingsPageOf(t)
	for range 100 {
		settingsPress(t, pg, "traffic", 1)
		settingsPress(t, pg, "police", -1)
		settingsPress(t, pg, "volume", 1)
	}
	if sh.settings.Traffic != trafficMax || sh.settings.Police != policeMin || sh.settings.Volume != 1 {
		t.Errorf("not clamped: %+v", sh.settings)
	}
}

func TestSettingsSoundIsInverseOfMuted(t *testing.T) {
	sh, pg := settingsPageOf(t)
	settingsPress(t, pg, "sound", -1)
	if !sh.settings.Muted {
		t.Error("Sound off did not mute")
	}
	r, _ := pg.List.Row("sound")
	if r.On() {
		t.Error("Sound shows on while muted")
	}
	settingsPress(t, pg, "sound", 0)
	if sh.settings.Muted {
		t.Error("enter on Sound did not unmute")
	}
}

func TestOldSettingsFileReadsMenuDriveOn(t *testing.T) {
	var s Settings
	if err := json.Unmarshal([]byte(`{"volume":0.5}`), &s); err != nil {
		t.Fatal(err)
	}
	s.clamp()
	if !s.MenuDrive() {
		t.Error("a file without the field must read Menu drive on")
	}
	if !DefaultSettings().MenuDrive() {
		t.Error("default is not on")
	}
}

func TestSettingsSaveKeepsEverything(t *testing.T) {
	sh, pg := settingsPageOf(t)
	sh.settings.Time = TimeNight
	sh.settings.Mode = 1
	sh.settings.UnlockAll = true
	settingsPress(t, pg, "menudrive", -1)
	settingsPress(t, pg, "sound", -1)
	want := sh.settings
	pg.Back()
	got := LoadSettings()
	if got != want {
		t.Errorf("saved %+v, want %+v", got, want)
	}
	if got.MenuDrive() || !got.Muted || got.Time != TimeNight || !got.UnlockAll {
		t.Error("a stored value was lost")
	}
	if sh.screen == pageSettings {
		t.Error("Back did not leave the page")
	}
}

func TestSettingsSectionFollowsSelection(t *testing.T) {
	_, pg := settingsPageOf(t)
	l := pg.List
	if settingsSection(l) != secMenus {
		t.Fatal("does not start on Menus")
	}
	seen := map[int]bool{}
	for range l.Len() {
		seen[settingsSection(l)] = true
		l.Move(1)
	}
	for i := range settingsSections {
		if !seen[i] {
			t.Errorf("walking never reached section %s", settingsSections[i])
		}
	}
	l.Select("police")
	if settingsSection(l) != secWorld {
		t.Error("Police is not in World")
	}
	if l.Len() != len(settingsKeySection)+1 {
		t.Error("Back is not the one extra row")
	}
}
