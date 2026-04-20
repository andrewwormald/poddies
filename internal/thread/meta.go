package thread

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// MetaSuffix is appended to a thread log's path to name its sidecar
// metadata file. `threads/default.jsonl` pairs with
// `threads/default.jsonl.meta.toml`.
const MetaSuffix = ".meta.toml"

// Meta is the per-thread sidecar capturing adapter-side state that
// survives between poddies runs: which session each member was in
// last, and how many tokens we've burned in total.
//
// Meta is optional — threads work fine without it. It exists so
// resume-capable adapters (claude --resume) can pick up where they
// left off, and so the TUI can show a running token counter.
type Meta struct {
	// LastSessionIDs maps member (or chief-of-staff) name → the
	// adapter-side session identifier from their most recent response.
	// Used to pass `--resume <id>` on subsequent invocations.
	LastSessionIDs map[string]string `toml:"last_session_ids,omitempty"`

	// LastEventIdx maps member name → the exclusive-end index into the
	// thread event slice at the time that member's response was last
	// appended. On a subsequent invocation with a PriorSessionID set,
	// only events from this index onwards are passed to the adapter
	// (A2 delta-resume). Zero means "no prior invocation; send full
	// thread." Only meaningful when a matching LastSessionIDs entry exists.
	LastEventIdx map[string]int `toml:"last_event_idx,omitempty"`

	// Cumulative token + cost counters for the thread. Updated after
	// each successful adapter invocation.
	InputTokens  int     `toml:"input_tokens,omitempty"`
	OutputTokens int     `toml:"output_tokens,omitempty"`
	CostUSD      float64 `toml:"cost_usd,omitempty"`
	DurationMs  int      `toml:"duration_ms,omitempty"`

	// TurnCount tracks how many adapter invocations have been recorded.
	// Lets the TUI show "N turns · X tokens · $Y.YY".
	TurnCount int `toml:"turn_count,omitempty"`
}

// TotalTokens returns InputTokens + OutputTokens.
func (m *Meta) TotalTokens() int { return m.InputTokens + m.OutputTokens }

// MetaPath returns the sidecar path for a given thread log path.
func MetaPath(logPath string) string { return logPath + MetaSuffix }

// LoadMeta reads the metadata sidecar for a thread log. Returns an
// empty, zero-value Meta (not an error) when the sidecar doesn't yet
// exist — threads without it are valid.
func LoadMeta(logPath string) (*Meta, error) {
	path := MetaPath(logPath)
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Meta{
				LastSessionIDs: map[string]string{},
				LastEventIdx:   map[string]int{},
			}, nil
		}
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	var m Meta
	dec := toml.NewDecoder(newByteReader(b))
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("decode %q: %w", path, err)
	}
	if m.LastSessionIDs == nil {
		m.LastSessionIDs = map[string]string{}
	}
	if m.LastEventIdx == nil {
		m.LastEventIdx = map[string]int{}
	}
	return &m, nil
}

// SaveMeta writes m atomically next to the log. Creates the file with
// 0o600 permissions. The log's parent directory is expected to exist
// (thread.Log.EnsureFile handles that at append time).
func SaveMeta(logPath string, m *Meta) error {
	if m == nil {
		return fmt.Errorf("SaveMeta: nil meta")
	}
	var buf bufferWriter
	enc := toml.NewEncoder(&buf)
	enc.SetIndentTables(true)
	if err := enc.Encode(m); err != nil {
		return fmt.Errorf("encode meta: %w", err)
	}
	path := MetaPath(logPath)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write %q: %w", tmp, err)
	}
	return os.Rename(tmp, path)
}

// RecordTurn mutates m to reflect one adapter invocation. memberName
// identifies the speaker; sessionID may be empty when the adapter
// doesn't report one (e.g. gemini's plain-stdout path). inputTokens,
// outputTokens, costUSD, durationMs come from the adapter's Usage.
func (m *Meta) RecordTurn(memberName, sessionID string, inputTokens, outputTokens int, costUSD float64, durationMs int) {
	if m.LastSessionIDs == nil {
		m.LastSessionIDs = map[string]string{}
	}
	if sessionID != "" && memberName != "" {
		m.LastSessionIDs[memberName] = sessionID
	}
	m.InputTokens += inputTokens
	m.OutputTokens += outputTokens
	m.CostUSD += costUSD
	m.DurationMs += durationMs
	m.TurnCount++
}

// --- tiny io adapters to avoid pulling bytes.Reader/bytes.Buffer into
//     this file when they're the only callers.

type byteReader struct {
	b []byte
	i int
}

func newByteReader(b []byte) *byteReader { return &byteReader{b: b} }

func (r *byteReader) Read(p []byte) (int, error) {
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
