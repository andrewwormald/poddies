package tui

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Prefs holds user display preferences that persist across sessions.
type Prefs struct {
	VizOpen    *bool       `toml:"viz_open,omitempty"`
	AvatarSize *AvatarSize `toml:"avatar_size,omitempty"`
	Debug      *bool       `toml:"debug,omitempty"`
}

const prefsFileName = "prefs.toml"

// LoadPrefs reads prefs from <root>/prefs.toml. Returns zero Prefs
// (all nil) when the file is missing — callers apply size-based defaults.
func LoadPrefs(root string) Prefs {
	data, err := os.ReadFile(filepath.Join(root, prefsFileName))
	if err != nil {
		return Prefs{}
	}
	var p Prefs
	if err := toml.Unmarshal(data, &p); err != nil {
		return Prefs{}
	}
	return p
}

// SavePrefs writes prefs to <root>/prefs.toml atomically.
func SavePrefs(root string, p Prefs) error {
	if root == "" {
		return nil
	}
	data, err := toml.Marshal(p)
	if err != nil {
		return err
	}
	path := filepath.Join(root, prefsFileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // root deleted (debug-restart)
		}
		return err
	}
	return os.Rename(tmp, path)
}

// boolPtr is a helper for creating *bool values.
func boolPtr(v bool) *bool { return &v }

// avatarSizePtr is a helper for creating *AvatarSize values.
func avatarSizePtr(v AvatarSize) *AvatarSize { return &v }
