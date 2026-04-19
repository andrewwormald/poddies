// Package session manages per-launch conversation sessions. Each
// `poddies` invocation opens a fresh session by default; /resume lets
// the user re-attach to a prior one. Sessions live under
// <root>/sessions/<id>/ and are indexed in <root>/sessions.toml.
//
// Session IDs are YYYY-MM-DD-HHMMSS-<6-hex> — sortable, roughly
// readable, and unique enough that collisions between two rapid
// launches in the same second are negligible.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// SessionsDirName is the subdirectory under the root that holds all
// per-session directories.
const SessionsDirName = "sessions"

// IndexFileName is the TOML file under the root that indexes all
// sessions — live metadata consumed by the /resume picker.
const IndexFileName = "sessions.toml"

// ThreadFileName is the JSONL log inside each session directory.
const ThreadFileName = "thread.jsonl"

// Session is one row of the index. The thread log and meta sidecar
// live inside <root>/sessions/<ID>/.
type Session struct {
	ID           string    `toml:"id"`
	Pod          string    `toml:"pod"`
	CreatedAt    time.Time `toml:"created_at"`
	LastEditedAt time.Time `toml:"last_edited_at"`
	TurnCount    int       `toml:"turn_count,omitempty"`
	LastSpeaker  string    `toml:"last_speaker,omitempty"`
}

// Index is the full session list (header + entries). Stored as TOML
// under <root>/sessions.toml.
type Index struct {
	Sessions []Session `toml:"session"`
}

// Dir returns the directory path for a session.
func Dir(root, id string) string {
	return filepath.Join(root, SessionsDirName, id)
}

// ThreadPath returns the JSONL thread log path for a session.
func ThreadPath(root, id string) string {
	return filepath.Join(Dir(root, id), ThreadFileName)
}

// IndexPath returns the full path to sessions.toml under root.
func IndexPath(root string) string {
	return filepath.Join(root, IndexFileName)
}

// NewID returns a freshly-minted session ID. Safe to call many times
// per second (the hex suffix absorbs collisions).
func NewID() string {
	now := time.Now().UTC()
	var b [3]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%04d-%02d-%02d-%02d%02d%02d-%s",
		now.Year(), now.Month(), now.Day(),
		now.Hour(), now.Minute(), now.Second(),
		hex.EncodeToString(b[:]))
}

// LoadIndex reads the sessions index from root. Missing file → empty
// Index, not error; a fresh root is legitimate.
func LoadIndex(root string) (*Index, error) {
	path := IndexPath(root)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Index{}, nil
		}
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	var idx Index
	dec := toml.NewDecoder(byteReader(data))
	if err := dec.Decode(&idx); err != nil {
		return nil, fmt.Errorf("decode %q: %w", path, err)
	}
	return &idx, nil
}

// SaveIndex writes idx atomically via tmp + rename. Creates the root
// if missing (with 0o700).
func SaveIndex(root string, idx *Index) error {
	if idx == nil {
		return fmt.Errorf("SaveIndex: nil index")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("mkdir %q: %w", root, err)
	}
	var buf bufferWriter
	enc := toml.NewEncoder(&buf)
	enc.SetIndentTables(true)
	if err := enc.Encode(idx); err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	path := IndexPath(root)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	return os.Rename(tmp, path)
}

// Create allocates a new session under root, bound to podName. Creates
// the session directory and the thread log (empty). Appends to the
// index and persists it. Returns the newly-minted Session.
func Create(root, podName string) (Session, error) {
	now := time.Now().UTC()
	s := Session{
		ID:           NewID(),
		Pod:          podName,
		CreatedAt:    now,
		LastEditedAt: now,
	}
	dir := Dir(root, s.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Session{}, fmt.Errorf("mkdir %q: %w", dir, err)
	}
	// Touch the thread log so subsequent callers don't race on its
	// creation and so ListRecent / resume can stat it reliably.
	f, err := os.OpenFile(ThreadPath(root, s.ID), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return Session{}, fmt.Errorf("touch thread: %w", err)
	}
	_ = f.Close()

	idx, err := LoadIndex(root)
	if err != nil {
		return Session{}, err
	}
	idx.Sessions = append(idx.Sessions, s)
	if err := SaveIndex(root, idx); err != nil {
		return Session{}, err
	}
	return s, nil
}

// Touch updates a session's LastEditedAt / TurnCount / LastSpeaker
// fields in the index. Called by the orchestrator after each turn so
// /resume ranks accurately and cleanup skips live sessions.
func Touch(root, id string, turnCount int, lastSpeaker string) error {
	idx, err := LoadIndex(root)
	if err != nil {
		return err
	}
	for i := range idx.Sessions {
		if idx.Sessions[i].ID == id {
			idx.Sessions[i].LastEditedAt = time.Now().UTC()
			if turnCount > 0 {
				idx.Sessions[i].TurnCount = turnCount
			}
			if lastSpeaker != "" {
				idx.Sessions[i].LastSpeaker = lastSpeaker
			}
			return SaveIndex(root, idx)
		}
	}
	return fmt.Errorf("session %q not found in index", id)
}

// ListRecent returns sessions sorted by LastEditedAt desc (newest
// first). Never returns nil — empty index returns empty slice.
func ListRecent(root string) ([]Session, error) {
	idx, err := LoadIndex(root)
	if err != nil {
		return nil, err
	}
	out := make([]Session, len(idx.Sessions))
	copy(out, idx.Sessions)
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastEditedAt.After(out[j].LastEditedAt)
	})
	return out, nil
}

// Find returns the Session with the given ID, or an error.
func Find(root, id string) (Session, error) {
	idx, err := LoadIndex(root)
	if err != nil {
		return Session{}, err
	}
	for _, s := range idx.Sessions {
		if s.ID == id {
			return s, nil
		}
	}
	return Session{}, fmt.Errorf("session %q not found", id)
}

// --- tiny io glue; kept local to avoid leaking bytes.Buffer / Reader
//     to the rest of the package.

func byteReader(b []byte) io.Reader { return &br{b: b} }

type br struct {
	b []byte
	i int
}

func (r *br) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}

type bufferWriter struct{ b []byte }

func (w *bufferWriter) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}

func (w *bufferWriter) Bytes() []byte { return w.b }
