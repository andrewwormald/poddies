package thread

import (
	"path/filepath"
	"testing"
)

func TestLoadMeta_Missing_ReturnsEmpty(t *testing.T) {
	m, err := LoadMeta(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if m == nil {
		t.Fatal("want non-nil meta")
	}
	if len(m.LastSessionIDs) != 0 || m.InputTokens != 0 {
		t.Errorf("want empty, got %+v", m)
	}
}

func TestSaveLoadMeta_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "t.jsonl")
	want := &Meta{
		LastSessionIDs: map[string]string{"alice": "sess-1", "bob": "sess-2"},
		InputTokens:    100,
		OutputTokens:   200,
		CostUSD:        0.042,
		DurationMs:     5000,
		TurnCount:      3,
	}
	if err := SaveMeta(logPath, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadMeta(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.InputTokens != 100 || got.OutputTokens != 200 || got.CostUSD != 0.042 || got.TurnCount != 3 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.LastSessionIDs["alice"] != "sess-1" || got.LastSessionIDs["bob"] != "sess-2" {
		t.Errorf("sessions lost: %v", got.LastSessionIDs)
	}
}

func TestRecordTurn_AccumulatesAndSetsSession(t *testing.T) {
	m := &Meta{}
	m.RecordTurn("alice", "sess-a", 10, 5, 0.001, 123)
	m.RecordTurn("alice", "sess-a2", 20, 10, 0.002, 456)
	m.RecordTurn("bob", "", 7, 3, 0, 50)
	if m.TurnCount != 3 {
		t.Errorf("turns: want 3, got %d", m.TurnCount)
	}
	if m.InputTokens != 37 || m.OutputTokens != 18 {
		t.Errorf("tokens: %+v", m)
	}
	if m.LastSessionIDs["alice"] != "sess-a2" {
		t.Errorf("want latest alice session 'sess-a2', got %q", m.LastSessionIDs["alice"])
	}
	// bob had no session; not stored.
	if _, ok := m.LastSessionIDs["bob"]; ok {
		t.Error("bob should not have a stored session (none reported)")
	}
}

func TestRecordTurn_NilMap_Initialised(t *testing.T) {
	m := &Meta{}
	m.RecordTurn("alice", "s1", 1, 1, 0, 0)
	if m.LastSessionIDs == nil {
		t.Error("map should be initialised on first RecordTurn")
	}
}

func TestMetaPath_AddsSuffix(t *testing.T) {
	if got := MetaPath("/tmp/foo.jsonl"); got != "/tmp/foo.jsonl.meta.toml" {
		t.Errorf("got %s", got)
	}
}

func TestSaveLoadMeta_LastEventIdx_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "t.jsonl")
	want := &Meta{
		LastSessionIDs: map[string]string{"alice": "sess-1"},
		LastEventIdx:   map[string]int{"alice": 7, "bob": 3},
		TurnCount:      5,
	}
	if err := SaveMeta(logPath, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadMeta(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastEventIdx["alice"] != 7 || got.LastEventIdx["bob"] != 3 {
		t.Errorf("LastEventIdx round-trip failed: got %v", got.LastEventIdx)
	}
}

func TestLoadMeta_Missing_LastEventIdx_InitialisedEmpty(t *testing.T) {
	m, err := LoadMeta(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if m.LastEventIdx == nil {
		t.Error("LastEventIdx should be initialised to empty map")
	}
}

func TestMeta_TotalTokens(t *testing.T) {
	m := &Meta{InputTokens: 10, OutputTokens: 5}
	if m.TotalTokens() != 15 {
		t.Errorf("want 15, got %d", m.TotalTokens())
	}
}
