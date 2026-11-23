package game

import (
	"testing"

	"github.com/danielriddell21/autobahn/internal/city"
	"github.com/danielriddell21/autobahn/internal/custom"
	"github.com/danielriddell21/autobahn/internal/pack"
)

func TestCustomScreenAdjust(t *testing.T) {
	c := &customScreen{spec: custom.FromMap(city.MapCity, 3), access: pack.Access{}, presets: custom.Presets{}}
	c.row = customRowFirstSlider + custom.Wind
	before := c.spec.Step[custom.Wind]
	c.Adjust(1)
	if c.spec.Step[custom.Wind] != before+1 {
		t.Error("the winding did not move")
	}
	for range 50 {
		c.Adjust(1)
	}
	if c.spec.Step[custom.Wind] != custom.Sliders[custom.Wind].Max || c.Flagged() {
		t.Error("a supported map went past its range, or was flagged")
	}
	c.row = customRowUnsupported
	c.Adjust(1)
	c.row = customRowFirstSlider + custom.Wind
	c.Adjust(1)
	if !c.Flagged() {
		t.Error("winding past the range on an unsupported map was not flagged")
	}
}

func TestNewCustomScreenStartsOnTheSettingsMap(t *testing.T) {
	set := DefaultSettings()
	set.Map = city.MapVillage
	c := newCustomScreen(set, pack.Access{Bypass: true})
	if c.spec.Base != city.MapVillage {
		t.Errorf("started on %s", c.spec.Base.Name())
	}
	set.Custom = custom.Edge(city.MapSeaside, 4).Encode()
	if c := newCustomScreen(set, pack.Access{Bypass: true}); c.spec.Base != city.MapSeaside || c.spec.Seed != 4 {
		t.Error("did not open on the last custom map")
	}
}

func TestCustomScreenBaseSkipsLockedMaps(t *testing.T) {
	c := &customScreen{spec: custom.FromMap(city.MapCity, 1), access: pack.Access{}}
	c.Adjust(1) // row 0: the base map
	if c.spec.Base != city.MapCity {
		t.Errorf("moved to %s, which is locked", c.spec.Base.Name())
	}
	c.access.Bypass = true
	c.Adjust(1)
	if c.spec.Base == city.MapCity {
		t.Error("did not move with everything unlocked")
	}
}

func TestCustomScreenPasteAndSettings(t *testing.T) {
	c := &customScreen{spec: custom.FromMap(city.MapCity, 1), access: pack.Access{Bypass: true}}
	edge := custom.Edge(city.MapVillage, 9)
	if err := c.Paste(edge.Encode()); err != nil || c.spec != edge {
		t.Errorf("paste: %v", err)
	}
	if err := c.Paste("AB1-nope"); err == nil {
		t.Error("a bad code was pasted")
	}
	s := DefaultSettings()
	c.Remember(&s)
	s.clamp()
	if got := customOf(s.Custom); got == nil || *got != edge {
		t.Error("the remembered map did not survive the settings")
	}
	s.Custom = "garbage"
	s.clamp()
	if s.Custom != "" {
		t.Error("a bad code stayed in the settings")
	}
}
