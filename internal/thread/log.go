package thread

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Log is an append-only JSONL event log backed by a file. Safe for
// concurrent Append calls within a single process (it serializes them
// through an internal mutex); cross-process concurrent writes rely on
// POSIX O_APPEND atomicity for writes smaller than PIPE_BUF (~4KiB).
// Events whose marshaled size may exceed 4KiB should not be appended
// concurrently across processes.
type Log struct {
	Path  string
	Now   func() time.Time
	NewID func() string

	mu sync.Mutex
}

// Open returns a Log pointing at path. The file and its parent directory
// are NOT created by Open; call EnsureFile or Append (which creates the
// file if missing).
func Open(path string) *Log {
	return &Log{
		Path:  path,
		Now:   time.Now,
		NewID: NewID,
	}
}

// EnsureFile creates the parent directory (0o700) and the empty log file
// (0o600) if they do not exist. Idempotent.
func (l *Log) EnsureFile() error {
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o700); err != nil {
		return fmt.Errorf("mkdir %q: %w", filepath.Dir(l.Path), err)
	}
	f, err := os.OpenFile(l.Path, os.O_CREATE|os.O_RDONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create %q: %w", l.Path, err)
	}
	return f.Close()
}

// Append validates e, fills in ID/TS/Mentions if zero, and writes it
// as a single line to the log file. The call is atomic within a process;
// see Log doc for cross-process caveats.
func (l *Log) Append(e Event) (Event, error) {
	if e.ID == "" {
		e.ID = l.NewID()
	}
	if e.TS.IsZero() {
		e.TS = l.Now()
	}
	if (e.Type == EventMessage || e.Type == EventHuman) && e.Mentions == nil && e.Body != "" {
		e.Mentions = ParseMentions(e.Body)
	}
	if err := e.Validate(); err != nil {
		return Event{}, fmt.Errorf("validate: %w", err)
	}

	buf, err := json.Marshal(e)
	if err != nil {
		return Event{}, fmt.Errorf("marshal: %w", err)
	}
	buf = append(buf, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(l.Path), 0o700); err != nil {
		return Event{}, fmt.Errorf("mkdir %q: %w", filepath.Dir(l.Path), err)
	}
	f, err := os.OpenFile(l.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return Event{}, fmt.Errorf("open %q: %w", l.Path, err)
	}
	defer f.Close()
	if _, err := f.Write(buf); err != nil {
		return Event{}, fmt.Errorf("write: %w", err)
	}
	return e, nil
}

// Load reads the entire log. Any malformed line returns an error with
// line number so corruption is visible rather than silently dropped.
// An empty or missing file returns an empty slice (no error for missing).
func (l *Log) Load() ([]Event, error) {
	f, err := os.Open(l.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %q: %w", l.Path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Allow long events (up to 4 MiB).
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	var events []Event
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := bytes.TrimRight(scanner.Bytes(), "\r")
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("line %d: unmarshal: %w", lineNum, err)
		}
		events = append(events, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return events, nil
}

// Truncate empties the log file, discarding all events.
func (l *Log) Truncate() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return os.Truncate(l.Path, 0)
}
