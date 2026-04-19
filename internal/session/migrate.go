package session

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// MigrateLegacyRoot renames `<cwd>/poddies` → `<cwd>/.poddies` when
// the legacy visible-directory layout is detected and the new hidden
// location isn't already present.
//
// Returns (migrated, error). migrated=true means a move happened and
// the caller should tell the user. error is non-nil on a failed move
// or on an ambiguous state (both old and new exist — we refuse to
// auto-resolve).
func MigrateLegacyRoot(cwd string) (migrated bool, err error) {
	legacy := cwd + "/poddies"
	target := cwd + "/.poddies"

	legacyInfo, legacyErr := os.Stat(legacy)
	legacyExists := legacyErr == nil && legacyInfo.IsDir()

	_, targetErr := os.Stat(target)
	targetExists := targetErr == nil

	if !legacyExists {
		return false, nil
	}
	if targetExists {
		return false, fmt.Errorf("both %q and %q exist; please merge manually — refusing to clobber", legacy, target)
	}
	if errors.Is(legacyErr, fs.ErrNotExist) {
		return false, nil
	}
	if err := os.Rename(legacy, target); err != nil {
		return false, fmt.Errorf("rename %q → %q: %w", legacy, target, err)
	}
	return true, nil
}
