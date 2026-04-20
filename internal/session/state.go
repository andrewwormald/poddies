package session

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// StateFileName is the TOML file under the root that records per-pod
// last-active session IDs. Allows poddies to re-open where the user
// left off without any explicit /resume.
const StateFileName = "state.toml"

// state is the TOML structure written to state.toml.
// The map key is the pod name; the value is the last session ID.
type state struct {
	LastSession map[string]string `toml:"last_session,omitempty"`
}

// StatePath returns the full path to state.toml under root.
func StatePath(root string) string {
	return filepath.Join(root, StateFileName)
}

// LoadLastSession returns the most-recently-saved session ID for pod
// within root. Returns ("", nil) when the state file is absent or the
// pod has no recorded entry.
func LoadLastSession(root, pod string) (string, error) {
	data, err := os.ReadFile(StatePath(root))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read state: %w", err)
	}
	var s state
	dec := toml.NewDecoder(byteReader(data))
	if err := dec.Decode(&s); err != nil {
		return "", fmt.Errorf("decode state: %w", err)
	}
	return s.LastSession[pod], nil
}

// SaveLastSession records sessionID as the last-active session for pod
// within root. Writes atomically via tmp + rename. Creates the state
// file when absent; updates the pod entry when present (preserving all
// other pod entries).
func SaveLastSession(root, pod, sessionID string) error {
	// Load existing state, or start fresh.
	data, err := os.ReadFile(StatePath(root))
	var s state
	if err == nil {
		dec := toml.NewDecoder(byteReader(data))
		if decErr := dec.Decode(&s); decErr != nil {
			// Corrupted state file; overwrite rather than propagating.
			s = state{}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read state: %w", err)
	}

	if s.LastSession == nil {
		s.LastSession = map[string]string{}
	}
	s.LastSession[pod] = sessionID

	var buf bufferWriter
	enc := toml.NewEncoder(&buf)
	enc.SetIndentTables(true)
	if err := enc.Encode(s); err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("mkdir root: %w", err)
	}
	path := StatePath(root)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write state tmp: %w", err)
	}
	return os.Rename(tmp, path)
}

