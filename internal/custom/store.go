package custom

import (
	"fmt"
	"sort"
	"strings"

	"github.com/danielriddell21/crucible/store"
)

// PresetsFile is the document a player's named maps are kept in, beside the
// settings.
const PresetsFile = "customs.json"

// Presets are a player's named custom maps, as name to share code. A map is
// kept as its code, which is all there is of it, so the file can be read and
// shared by hand.
type Presets map[string]string

// Names returns the names in order.
func (p Presets) Names() []string {
	out := make([]string, 0, len(p))
	for n := range p {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Get returns a preset's map.
func (p Presets) Get(name string) (Spec, error) {
	code, ok := p[name]
	if !ok {
		return Spec{}, fmt.Errorf("custom: no map called %q", name)
	}
	return Decode(code)
}

// Put keeps a map under a name, replacing any of that name.
func (p Presets) Put(name string, s Spec) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("custom: a map needs a name")
	}
	p[name] = s.Encode()
	return nil
}

// LoadPresets reads the presets at path. Anything unreadable is dropped rather
// than refused: a first run, a corrupt file and an entry somebody edited into
// nonsense all leave a player with the presets that are fine.
func LoadPresets(path string) Presets {
	raw := store.Load(path, Presets{})
	out := Presets{}
	for name, code := range raw {
		if _, err := Decode(code); err == nil && strings.TrimSpace(name) != "" {
			out[name] = code
		}
	}
	return out
}

// SavePresets writes the presets to path.
func SavePresets(path string, p Presets) error {
	if err := store.Save(path, p); err != nil {
		return fmt.Errorf("saving the custom maps: %w", err)
	}
	return nil
}
