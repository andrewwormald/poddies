package thread

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var updateGolden = flag.Bool("update", false, "regenerate golden files in testdata/")

// fixedLog returns a Log whose ID/TS generators are deterministic,
// so tests can produce reproducible JSONL output for golden comparison.
func fixedLog(t *testing.T) *Log {
	t.Helper()
	var counter int64
	base := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	return &Log{
		Path: filepath.Join(t.TempDir(), "thread.jsonl"),
		Now: func() time.Time {
			n := atomic.AddInt64(&counter, 1)
			return base.Add(time.Duration(n) * time.Second)
		},
		NewID: func() string {
			n := atomic.LoadInt64(&counter)
			return fmt.Sprintf("evt-%03d", n)
		},
	}
}

func TestOpen_DefaultsAreUsable(t *testing.T) {
	l := Open(filepath.Join(t.TempDir(), "a.jsonl"))
	if l.Now == nil || l.NewID == nil {
		t.Fatal("Open must set Now and NewID")
	}
	if got := l.NewID(); len(got) == 0 {
		t.Error("NewID returned empty")
	}
	if l.Now().IsZero() {
		t.Error("Now returned zero time")
	}
}

func TestEnsureFile_CreatesMissing(t *testing.T) {
	l := Open(filepath.Join(t.TempDir(), "sub", "thread.jsonl"))
	if err := l.EnsureFile(); err != nil {
		t.Fatalf("EnsureFile: %v", err)
	}
	info, err := os.Stat(l.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Errorf("want empty file, got %d bytes", info.Size())
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("want 0600, got %o", got)
	}
}

func TestEnsureFile_Idempotent(t *testing.T) {
	l := Open(filepath.Join(t.TempDir(), "a.jsonl"))
	if err := l.EnsureFile(); err != nil {
		t.Fatal(err)
	}
	if err := l.EnsureFile(); err != nil {
		t.Fatalf("second EnsureFile: %v", err)
	}
}

func TestAppend_AssignsIDAndTS(t *testing.T) {
	l := fixedLog(t)
	got, err := l.Append(Event{Type: EventMessage, From: "alice", Body: "hello"})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if got.ID == "" {
		t.Error("want ID assigned")
	}
	if got.TS.IsZero() {
		t.Error("want TS assigned")
	}
}

func TestAppend_PreservesProvidedIDAndTS(t *testing.T) {
	l := fixedLog(t)
	when := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	in := Event{ID: "fixed-id", TS: when, Type: EventMessage, From: "alice", Body: "hi"}
	got, err := l.Append(in)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "fixed-id" {
		t.Errorf("want fixed-id, got %s", got.ID)
	}
	if !got.TS.Equal(when) {
		t.Errorf("TS mismatch: %v", got.TS)
	}
}

func TestAppend_AutoParsesMentions(t *testing.T) {
	l := fixedLog(t)
	got, _ := l.Append(Event{Type: EventMessage, From: "alice", Body: "ping @bob"})
	if !eqStrSlice(got.Mentions, []string{"bob"}) {
		t.Errorf("want [bob], got %v", got.Mentions)
	}
}

func TestAppend_PreservesExplicitMentions(t *testing.T) {
	l := fixedLog(t)
	got, _ := l.Append(Event{Type: EventMessage, From: "alice", Body: "irrelevant", Mentions: []string{"bob"}})
	if !eqStrSlice(got.Mentions, []string{"bob"}) {
		t.Errorf("got %v", got.Mentions)
	}
}

func TestAppend_InvalidEvent_Errors(t *testing.T) {
	l := fixedLog(t)
	_, err := l.Append(Event{}) // empty type
	if err == nil {
		t.Error("want validate error")
	}
}

func TestLoad_MissingFile_ReturnsEmpty(t *testing.T) {
	l := Open(filepath.Join(t.TempDir(), "nope.jsonl"))
	got, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty, got %d", len(got))
	}
}

func TestLoad_EmptyFile_ReturnsEmpty(t *testing.T) {
	l := Open(filepath.Join(t.TempDir(), "a.jsonl"))
	_ = l.EnsureFile()
	got, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty, got %d", len(got))
	}
}

func TestAppendLoad_RoundTrip_SingleEvent(t *testing.T) {
	l := fixedLog(t)
	written, err := l.Append(Event{Type: EventMessage, From: "alice", Body: "hi @bob"})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := l.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("want 1 event, got %d", len(loaded))
	}
	if !reflect.DeepEqual(loaded[0], written) {
		t.Errorf("mismatch\nwant %#v\ngot  %#v", written, loaded[0])
	}
}

func TestAppendLoad_RoundTrip_MultipleEventTypes(t *testing.T) {
	l := fixedLog(t)
	events := []Event{
		{Type: EventSystem, Body: "pod started"},
		{Type: EventMessage, From: "alice", Body: "hi @bob"},
		{Type: EventMessage, From: "bob", Body: "@alice ack"},
		{Type: EventPermissionRequest, From: "alice", Action: "run_shell", Payload: json.RawMessage(`{"cmd":"ls"}`)},
		{Type: EventPermissionGrant, From: "human", RequestID: "req-1"},
		{Type: EventHuman, Body: "looks good, ship it"},
	}
	for _, e := range events {
		if _, err := l.Append(e); err != nil {
			t.Fatalf("append %v: %v", e.Type, err)
		}
	}
	loaded, err := l.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != len(events) {
		t.Fatalf("want %d, got %d", len(events), len(loaded))
	}
	for i, e := range loaded {
		if e.Type != events[i].Type {
			t.Errorf("event %d type: want %s, got %s", i, events[i].Type, e.Type)
		}
	}
}

func TestLoad_UnknownEventType_Preserved(t *testing.T) {
	l := Open(filepath.Join(t.TempDir(), "a.jsonl"))
	_ = l.EnsureFile()
	line := `{"id":"x","type":"future_type","ts":"2026-04-18T12:00:00Z","body":"hi"}` + "\n"
	if err := os.WriteFile(l.Path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("want 1, got %d", len(loaded))
	}
	if loaded[0].Type != "future_type" {
		t.Errorf("want future_type, got %s", loaded[0].Type)
	}
	if loaded[0].Body != "hi" {
		t.Errorf("want body=hi, got %q", loaded[0].Body)
	}
}

func TestLoad_MalformedLine_Errors(t *testing.T) {
	l := Open(filepath.Join(t.TempDir(), "a.jsonl"))
	_ = l.EnsureFile()
	content := `{"id":"x","type":"system","ts":"2026-04-18T12:00:00Z"}
{not valid json}
`
	if err := os.WriteFile(l.Path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := l.Load()
	if err == nil {
		t.Fatal("want error for malformed line")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error should include line number, got %v", err)
	}
}

func TestLoad_PartialLastLine_Errors(t *testing.T) {
	l := Open(filepath.Join(t.TempDir(), "a.jsonl"))
	_ = l.EnsureFile()
	// first line complete, second line truncated mid-json (no trailing newline).
	content := `{"id":"x","type":"system","ts":"2026-04-18T12:00:00Z"}
{"id":"y","type":"message`
	if err := os.WriteFile(l.Path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := l.Load()
	if err == nil {
		t.Fatal("want error for partial last line")
	}
}

func TestLoad_BlankLinesSkipped(t *testing.T) {
	l := Open(filepath.Join(t.TempDir(), "a.jsonl"))
	_ = l.EnsureFile()
	content := `{"id":"x","type":"system","ts":"2026-04-18T12:00:00Z"}

{"id":"y","type":"system","ts":"2026-04-18T12:00:01Z"}
`
	if err := os.WriteFile(l.Path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("want 2 events, got %d", len(loaded))
	}
}

func TestAppend_Concurrent_AllEventsLanded(t *testing.T) {
	l := fixedLog(t)
	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := l.Append(Event{Type: EventMessage, From: "alice", Body: fmt.Sprintf("msg-%d", i)})
			if err != nil {
				t.Errorf("append: %v", err)
			}
		}(i)
	}
	wg.Wait()
	loaded, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != n {
		t.Errorf("want %d events, got %d", n, len(loaded))
	}
}

// --- golden ---

func TestLog_Golden(t *testing.T) {
	l := fixedLog(t)
	_, _ = l.Append(Event{Type: EventSystem, Body: "pod started"})
	_, _ = l.Append(Event{Type: EventMessage, From: "alice", Body: "hi @bob, ready?"})
	_, _ = l.Append(Event{Type: EventMessage, From: "bob", Body: "@alice yep"})
	_, _ = l.Append(Event{
		Type:    EventPermissionRequest,
		From:    "alice",
		Action:  "run_shell",
		Payload: json.RawMessage(`{"cmd":"ls -la"}`),
	})
	_, _ = l.Append(Event{Type: EventPermissionGrant, From: "human", RequestID: "req-1"})
	_, _ = l.Append(Event{Type: EventHuman, Body: "ship it"})

	got, err := os.ReadFile(l.Path)
	if err != nil {
		t.Fatal(err)
	}
	goldenPath := "testdata/golden/thread_full.jsonl"
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run -update): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("golden mismatch\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestLog_Golden_Loadable verifies the committed golden file deserializes
// cleanly without any code changes producing incompatible schema drift.
func TestLog_Golden_Loadable(t *testing.T) {
	src, err := os.ReadFile("testdata/golden/thread_full.jsonl")
	if err != nil {
		t.Skipf("golden missing: %v", err)
	}
	tmp := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(tmp, src, 0o600); err != nil {
		t.Fatal(err)
	}
	l := Open(tmp)
	events, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("want some events from golden")
	}
}

// silence unused import when -update not passed
var _ = regexp.MustCompile
