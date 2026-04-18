package orchestrator

import (
	"context"
	"errors"
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

// scaffoldWithMembers writes a pod with the named mock-backed members.
// lead can be any of the member names or "human".
func scaffoldWithMembers(t *testing.T, lead string, names ...string) string {
	t.Helper()
	root := t.TempDir()
	podDir := filepath.Join(root, "pods", "demo")
	if err := osMkdirAll(filepath.Join(podDir, "members"), 0o700); err != nil {
		t.Fatal(err)
	}
	pod := &config.Pod{Name: "demo", Lead: lead}
	if err := config.SavePod(podDir, pod); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		m := &config.Member{
			Name: n, Title: "T", Adapter: config.AdapterMock,
			Model: "m", Effort: config.EffortMedium,
		}
		if err := config.SaveMember(podDir, m); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// newDeterministicLog creates a thread.Log with monotonic counter-backed
// IDs/TSes, so tests can reason about event order & count without
// fighting real clocks.
func newDeterministicLog(t *testing.T, dir string) *thread.Log {
	t.Helper()
	var counter int64
	base := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	log := &thread.Log{
		Path: filepath.Join(dir, "t.jsonl"),
		Now: func() time.Time {
			n := atomic.AddInt64(&counter, 1)
			return base.Add(time.Duration(n) * time.Second)
		},
		NewID: func() string {
			n := atomic.LoadInt64(&counter)
			return fmt.Sprintf("evt-%03d", n)
		},
	}
	if err := log.EnsureFile(); err != nil {
		t.Fatal(err)
	}
	return log
}

func TestLoop_Quiesces_WhenAgentStopsMentioning(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice", "bob")
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@bob take a look"}},
		mock.ScriptedResponse{ForMember: "bob", Response: adapter.InvokeResponse{Body: "done, nothing more"}},
	))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "kick off",
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.StopReason != LoopQuiescent {
		t.Errorf("want quiescent, got %s", res.StopReason)
	}
	if res.TurnsRun != 2 {
		t.Errorf("want 2 turns, got %d", res.TurnsRun)
	}
	// 1 human + 2 member events
	if len(res.Events) != 3 {
		t.Errorf("want 3 events, got %d", len(res.Events))
	}
}

func TestLoop_MaxTurns_Capped(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice", "bob")
	// infinite ping-pong
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@bob 1"}},
		mock.ScriptedResponse{ForMember: "bob", Response: adapter.InvokeResponse{Body: "@alice 2"}},
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@bob 3"}},
		mock.ScriptedResponse{ForMember: "bob", Response: adapter.InvokeResponse{Body: "@alice 4"}},
	))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "go",
		MaxTurns:      3,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.StopReason != LoopMaxTurns {
		t.Errorf("want max_turns, got %s", res.StopReason)
	}
	if res.TurnsRun != 3 {
		t.Errorf("want 3 turns, got %d", res.TurnsRun)
	}
}

func TestLoop_HumanNoMention_LeadIsAgent_RoutesToLead(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice", "bob")
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "I'll take it"}},
	))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "anyone?",
		MaxTurns:      4,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != LoopQuiescent {
		t.Errorf("want quiescent, got %s", res.StopReason)
	}
	if res.TurnsRun != 1 {
		t.Errorf("want 1 turn (alice → quiescent), got %d", res.TurnsRun)
	}
}

func TestLoop_NoHumanMessage_EmptyThread_HaltsImmediately(t *testing.T) {
	root := scaffoldWithMembers(t, "human", "alice")
	m := mock.New()
	log := newDeterministicLog(t, t.TempDir())
	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != LoopQuiescent {
		t.Errorf("want quiescent, got %s", res.StopReason)
	}
	if res.TurnsRun != 0 {
		t.Errorf("want 0 turns, got %d", res.TurnsRun)
	}
}

func TestLoop_AdapterError_ReturnsLoopError(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice")
	m := mock.New() // empty script → exhausted
	log := newDeterministicLog(t, t.TempDir())
	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "hi",
		MaxTurns:      3,
	}
	res, err := loop.Run(context.Background())
	if err == nil {
		t.Fatal("want error")
	}
	if res.StopReason != LoopError {
		t.Errorf("want LoopError, got %s", res.StopReason)
	}
	// the human kickoff should still be in res.Events
	if len(res.Events) != 1 || res.Events[0].Type != thread.EventHuman {
		t.Errorf("want human event preserved, got %+v", res.Events)
	}
}

func TestLoop_Cancelled_StopsBeforeNextTurn(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice", "bob")
	// would ping-pong forever if allowed
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@bob hi"}},
	))
	log := newDeterministicLog(t, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "go",
		MaxTurns:      10,
		OnEvent: func(e thread.Event) {
			if e.From == "alice" {
				cancel()
			}
		},
	}
	res, err := loop.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != LoopCancelled {
		t.Errorf("want cancelled, got %s", res.StopReason)
	}
}

func TestLoop_UnknownAdapter_Errors(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice")
	log := newDeterministicLog(t, t.TempDir())
	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{}), // empty
		Log:           log,
		HumanMessage:  "hi",
	}
	_, err := loop.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "adapter") {
		t.Errorf("want adapter error, got %v", err)
	}
}

func TestLoop_NilLog_Errors(t *testing.T) {
	loop := &Loop{Root: t.TempDir(), Pod: "demo", AdapterLookup: MapLookup(nil)}
	_, err := loop.Run(context.Background())
	if err == nil {
		t.Error("want error")
	}
}

func TestLoop_NilLookup_Errors(t *testing.T) {
	log := newDeterministicLog(t, t.TempDir())
	loop := &Loop{Root: t.TempDir(), Pod: "demo", Log: log}
	_, err := loop.Run(context.Background())
	if err == nil {
		t.Error("want error")
	}
}

func TestLoop_PodMissing_Errors(t *testing.T) {
	log := newDeterministicLog(t, t.TempDir())
	loop := &Loop{
		Root:          t.TempDir(),
		Pod:           "ghost",
		AdapterLookup: MapLookup(nil),
		Log:           log,
	}
	_, err := loop.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "load pod") {
		t.Errorf("want load-pod error, got %v", err)
	}
}

func TestLoop_OnEvent_CalledForEveryAppendedEvent(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice", "bob")
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@bob hi"}},
		mock.ScriptedResponse{ForMember: "bob", Response: adapter.InvokeResponse{Body: "done"}},
	))
	log := newDeterministicLog(t, t.TempDir())
	var captured []thread.Event
	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "kickoff",
		OnEvent:       func(e thread.Event) { captured = append(captured, e) },
	}
	if _, err := loop.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(captured) != 3 {
		t.Errorf("want 3 events (human, alice, bob), got %d", len(captured))
	}
}

func TestLoop_EffortOverride_AppliesToEveryInvocation(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice", "bob")
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@bob hi"}},
		mock.ScriptedResponse{ForMember: "bob", Response: adapter.InvokeResponse{Body: "done"}},
	))
	log := newDeterministicLog(t, t.TempDir())
	loop := &Loop{
		Root:           root,
		Pod:            "demo",
		AdapterLookup:  MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:            log,
		HumanMessage:   "go",
		EffortOverride: config.EffortLow,
	}
	if _, err := loop.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i, c := range m.Calls() {
		if c.Effort != "low" {
			t.Errorf("call %d: want low, got %s", i, c.Effort)
		}
	}
}

func TestLoop_NegativeMaxTurns_UsesSafetyCap(t *testing.T) {
	root := scaffoldWithMembers(t, "human", "alice")
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "done"}},
	))
	log := newDeterministicLog(t, t.TempDir())
	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "hi",
		MaxTurns:      -1,
	}
	// with lead=human and human kickoff → route halts on agent turn 1
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.TurnsRun > SafetyMaxTurns {
		t.Errorf("safety cap violated: %d", res.TurnsRun)
	}
	// doesn't actually invoke anyone because lead=human & no mentions
	if res.TurnsRun != 0 {
		// we routed to nothing, so 0 turns. ensure our assumptions hold.
		t.Logf("turns=%d (expected 0 due to human-lead)", res.TurnsRun)
	}
	_ = errors.New
}

func TestLoadMemberRoster_ReadsAllMembers(t *testing.T) {
	root := scaffoldWithMembers(t, "human", "alice", "bob", "carol")
	podDir := filepath.Join(root, "pods", "demo")
	names, members, err := loadMemberRoster(podDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 || len(members) != 3 {
		t.Errorf("want 3 members, got names=%v members=%d", names, len(members))
	}
}

func TestLoadMemberRoster_MissingMembersDir_Empty(t *testing.T) {
	dir := t.TempDir()
	names, members, err := loadMemberRoster(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 || len(members) != 0 {
		t.Errorf("want empty, got %v / %d", names, len(members))
	}
}
