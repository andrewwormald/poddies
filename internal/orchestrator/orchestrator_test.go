package orchestrator

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/adapter/mock"
	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

// helper: scaffold a pod with alice on disk.
func scaffold(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	podDir := filepath.Join(root, "pods", "demo")
	if err := mkdirAll(podDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := mkdirAll(filepath.Join(podDir, "members"), 0o700); err != nil {
		t.Fatal(err)
	}
	pod := &config.Pod{Name: "demo", Lead: "human"}
	if err := config.SavePod(podDir, pod); err != nil {
		t.Fatal(err)
	}
	m := &config.Member{
		Name: "alice", Title: "Staff", Adapter: config.AdapterMock,
		Model: "m", Effort: config.EffortHigh,
	}
	if err := config.SaveMember(podDir, m); err != nil {
		t.Fatal(err)
	}
	return root
}

// mkdirAll is a tiny wrapper so the test file doesn't need an os import.
func mkdirAll(path string, perm uint32) error {
	return (func() error {
		return osMkdirAll(path, perm)
	})()
}

func deterministicLog(t *testing.T, path string) *thread.Log {
	t.Helper()
	var counter int64
	base := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	return &thread.Log{
		Path: path,
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

func TestRun_HappyPath(t *testing.T) {
	root := scaffold(t)
	m := mock.New(mock.WithScript(mock.ScriptedResponse{
		ForMember: "alice",
		Response:  adapter.InvokeResponse{Body: "@bob how are you?"},
	}))
	log := deterministicLog(t, filepath.Join(root, "pods", "demo", "threads", "t.jsonl"))
	_ = log.EnsureFile()

	turn := &Turn{
		Root:          root,
		Pod:           "demo",
		Member:        "alice",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
	}
	res, err := turn.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.MemberEvent.From != "alice" {
		t.Errorf("want from=alice, got %s", res.MemberEvent.From)
	}
	if res.MemberEvent.Body != "@bob how are you?" {
		t.Errorf("unexpected body: %q", res.MemberEvent.Body)
	}
	if len(res.MemberEvent.Mentions) != 1 || res.MemberEvent.Mentions[0] != "bob" {
		t.Errorf("mentions: want [bob], got %v", res.MemberEvent.Mentions)
	}
}

func TestRun_WithHumanKickoff_AppendsHumanFirst(t *testing.T) {
	root := scaffold(t)
	m := mock.New(mock.WithScript(mock.ScriptedResponse{
		ForMember:    "alice",
		WantContains: []string{"start please"},
		Response:     adapter.InvokeResponse{Body: "on it"},
	}))
	log := deterministicLog(t, filepath.Join(root, "pods", "demo", "threads", "t.jsonl"))
	_ = log.EnsureFile()

	turn := &Turn{
		Root:          root,
		Pod:           "demo",
		Member:        "alice",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "start please",
	}
	res, err := turn.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.HumanEvent.Type != thread.EventHuman {
		t.Errorf("want human event, got %s", res.HumanEvent.Type)
	}

	events, _ := log.Load()
	if len(events) != 2 {
		t.Fatalf("want 2 events (human+member), got %d", len(events))
	}
	if events[0].Type != thread.EventHuman || events[1].Type != thread.EventMessage {
		t.Errorf("wrong order: %v %v", events[0].Type, events[1].Type)
	}
}

func TestRun_EffortOverride_PassedToAdapter(t *testing.T) {
	root := scaffold(t)
	m := mock.New(mock.WithScript(mock.ScriptedResponse{Response: adapter.InvokeResponse{Body: "ok"}}))
	log := deterministicLog(t, filepath.Join(root, "pods", "demo", "threads", "t.jsonl"))
	_ = log.EnsureFile()

	turn := &Turn{
		Root:           root,
		Pod:            "demo",
		Member:         "alice",
		AdapterLookup:  MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:            log,
		EffortOverride: config.EffortLow,
	}
	if _, err := turn.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := m.Calls()
	if len(calls) != 1 || calls[0].Effort != "low" {
		t.Errorf("want effort=low, got %+v", calls)
	}
}

func TestRun_PodNotFound_Errors(t *testing.T) {
	root := t.TempDir()
	log := deterministicLog(t, filepath.Join(root, "t.jsonl"))
	_ = log.EnsureFile()
	turn := &Turn{
		Root:          root,
		Pod:           "ghost",
		Member:        "alice",
		AdapterLookup: MapLookup(nil),
		Log:           log,
	}
	_, err := turn.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "load pod") {
		t.Errorf("want load pod error, got %v", err)
	}
}

func TestRun_MemberNotFound_Errors(t *testing.T) {
	root := scaffold(t)
	log := deterministicLog(t, filepath.Join(root, "t.jsonl"))
	_ = log.EnsureFile()
	turn := &Turn{
		Root:          root,
		Pod:           "demo",
		Member:        "ghost",
		AdapterLookup: MapLookup(nil),
		Log:           log,
	}
	_, err := turn.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "load member") {
		t.Errorf("want load member error, got %v", err)
	}
}

func TestRun_AdapterNotFound_Errors(t *testing.T) {
	root := scaffold(t)
	log := deterministicLog(t, filepath.Join(root, "t.jsonl"))
	_ = log.EnsureFile()
	turn := &Turn{
		Root:          root,
		Pod:           "demo",
		Member:        "alice",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{}), // empty
		Log:           log,
	}
	_, err := turn.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "adapter") {
		t.Errorf("want adapter error, got %v", err)
	}
}

func TestRun_AdapterError_Surfaces(t *testing.T) {
	root := scaffold(t)
	// empty-script mock: first Invoke errors with "exhausted"
	m := mock.New()
	log := deterministicLog(t, filepath.Join(root, "t.jsonl"))
	_ = log.EnsureFile()
	turn := &Turn{
		Root:          root,
		Pod:           "demo",
		Member:        "alice",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
	}
	_, err := turn.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "exhausted") {
		t.Errorf("want exhausted error, got %v", err)
	}
}

func TestRun_NilLog_Errors(t *testing.T) {
	root := scaffold(t)
	turn := &Turn{Root: root, Pod: "demo", Member: "alice", AdapterLookup: MapLookup(nil)}
	_, err := turn.Run(context.Background())
	if err == nil {
		t.Error("want error for nil log")
	}
}

func TestRun_NilLookup_Errors(t *testing.T) {
	root := scaffold(t)
	log := deterministicLog(t, filepath.Join(root, "t.jsonl"))
	_ = log.EnsureFile()
	turn := &Turn{Root: root, Pod: "demo", Member: "alice", Log: log}
	_, err := turn.Run(context.Background())
	if err == nil {
		t.Error("want error for nil lookup")
	}
}

func TestMapLookup_FindsAdapter(t *testing.T) {
	m := mock.New()
	lookup := MapLookup(map[string]adapter.Adapter{"mock": m})
	a, err := lookup("mock")
	if err != nil {
		t.Fatal(err)
	}
	if a.Name() != "mock" {
		t.Errorf("want mock, got %s", a.Name())
	}
}

func TestMapLookup_NotFound_Errors(t *testing.T) {
	lookup := MapLookup(map[string]adapter.Adapter{})
	_, err := lookup("ghost")
	if err == nil {
		t.Error("want error")
	}
}

func TestMapLookup_NilMap_Errors(t *testing.T) {
	lookup := MapLookup(nil)
	_, err := lookup("x")
	if err == nil {
		t.Error("want error")
	}
}
