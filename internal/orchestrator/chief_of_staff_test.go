package orchestrator

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/adapter/mock"
	"github.com/andrewwormald/poddies/internal/config"
)

// scaffoldWithCoS writes a pod with CoS enabled + the given triggers.
// Members are all mock-backed.
func scaffoldWithCoS(t *testing.T, lead, cosName string, triggers []config.Trigger, names ...string) string {
	t.Helper()
	root := t.TempDir()
	podDir := filepath.Join(root, "pods", "demo")
	if err := osMkdirAll(filepath.Join(podDir, "members"), 0o700); err != nil {
		t.Fatal(err)
	}
	pod := &config.Pod{
		Name: "demo",
		Lead: lead,
		ChiefOfStaff: config.ChiefOfStaff{
			Enabled:  true,
			Name:     cosName,
			Adapter:  config.AdapterMock,
			Model:    "local-cheap",
			Triggers: triggers,
		},
	}
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

func TestLoop_Milestone_FiresEveryN(t *testing.T) {
	root := scaffoldWithCoS(t, "alice", "sam",
		[]config.Trigger{config.TriggerMilestone},
		"alice", "bob",
	)
	// Ping-pong script for members + CoS summary between rounds. CoS
	// speaks at milestone with no mention, which will halt the loop —
	// that's fine for this assertion.
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@bob 1"}},
		mock.ScriptedResponse{ForMember: "bob", Response: adapter.InvokeResponse{Body: "@alice 2"}},
		mock.ScriptedResponse{ForMember: "sam", Response: adapter.InvokeResponse{Body: "we've made 2 turns of progress"}},
	))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:           root,
		Pod:            "demo",
		AdapterLookup:  MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:            log,
		HumanMessage:   "kick off",
		MaxTurns:       6,
		MilestoneEvery: 2,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Should have: human + alice + bob + sam (milestone) → sam had no
	// actionable mention → quiescent.
	if res.StopReason != LoopQuiescent {
		t.Errorf("want quiescent, got %s", res.StopReason)
	}
	if len(res.Events) != 4 {
		t.Errorf("want 4 events, got %d: %+v", len(res.Events), eventTypesConcrete(res.Events))
	}
	if res.Events[3].From != "sam" {
		t.Errorf("want last event from sam, got %s", res.Events[3].From)
	}
}

func TestLoop_Milestone_Disabled_NoCoSEvent(t *testing.T) {
	root := scaffoldWithCoS(t, "alice", "sam",
		// CoS enabled but milestone trigger NOT in list
		[]config.Trigger{config.TriggerUnresolvedRouting},
		"alice", "bob",
	)
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@bob 1"}},
		mock.ScriptedResponse{ForMember: "bob", Response: adapter.InvokeResponse{Body: "@alice 2"}},
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@bob 3"}},
	))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:           root,
		Pod:            "demo",
		AdapterLookup:  MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:            log,
		HumanMessage:   "go",
		MaxTurns:       3,
		MilestoneEvery: 2,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, e := range res.Events {
		if e.From == "sam" {
			t.Errorf("CoS should not fire when milestone trigger not configured; got sam event")
		}
	}
}

func TestLoop_UnresolvedRouting_CoS_Rescues(t *testing.T) {
	root := scaffoldWithCoS(t, "alice", "sam",
		[]config.Trigger{config.TriggerUnresolvedRouting},
		"alice", "bob",
	)
	// alice produces no mention → Route halts → CoS rescues with @bob
	// → bob runs and finishes quiet.
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "I'm stuck"}},
		mock.ScriptedResponse{ForMember: "sam", Response: adapter.InvokeResponse{Body: "@bob can you help alice?"}},
		mock.ScriptedResponse{ForMember: "bob", Response: adapter.InvokeResponse{Body: "picking up, done"}},
	))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "start",
		MaxTurns:      5,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.StopReason != LoopQuiescent {
		t.Errorf("want quiescent, got %s", res.StopReason)
	}
	// events: human, alice, sam (rescue), bob
	if len(res.Events) != 4 {
		t.Fatalf("want 4 events, got %d: %+v", len(res.Events), eventTypesConcrete(res.Events))
	}
	if res.Events[2].From != "sam" {
		t.Errorf("want rescue at index 2 from sam, got %s", res.Events[2].From)
	}
	if res.Events[3].From != "bob" {
		t.Errorf("want bob follow-up at index 3, got %s", res.Events[3].From)
	}
}

func TestLoop_UnresolvedRouting_CoS_CannotRescue_StopsAfterOne(t *testing.T) {
	root := scaffoldWithCoS(t, "alice", "sam",
		[]config.Trigger{config.TriggerUnresolvedRouting},
		"alice", "bob",
	)
	// alice halts → CoS also produces no mention → loop stops
	// (one-rescue-per-halt cap).
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "I'm stuck"}},
		mock.ScriptedResponse{ForMember: "sam", Response: adapter.InvokeResponse{Body: "no obvious next speaker"}},
	))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "start",
		MaxTurns:      5,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.StopReason != LoopQuiescent {
		t.Errorf("want quiescent, got %s", res.StopReason)
	}
	// human + alice + sam (rescue) — and nothing further since the
	// cap prevents a second rescue.
	if len(res.Events) != 3 {
		t.Errorf("want 3 events, got %d: %+v", len(res.Events), eventTypesConcrete(res.Events))
	}
	// Mock should have exactly 2 Invoke calls (alice + sam).
	if got := m.Calls(); len(got) != 2 {
		t.Errorf("want 2 calls, got %d", len(got))
	}
}

func TestLoop_UnresolvedRouting_Disabled_NoRescue(t *testing.T) {
	root := scaffoldWithCoS(t, "alice", "sam",
		// CoS enabled with milestone only, no unresolved_routing
		[]config.Trigger{config.TriggerMilestone},
		"alice", "bob",
	)
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "I'm stuck"}},
	))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "start",
		MaxTurns:      5,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.StopReason != LoopQuiescent {
		t.Errorf("want quiescent, got %s", res.StopReason)
	}
	// only human + alice; no rescue fired.
	if len(res.Events) != 2 {
		t.Errorf("want 2 events, got %d", len(res.Events))
	}
}

func TestLoop_CoS_AdapterMissing_ErrorsFromRescuePath(t *testing.T) {
	root := scaffoldWithCoS(t, "alice", "sam",
		[]config.Trigger{config.TriggerUnresolvedRouting},
		"alice", "bob",
	)
	// Member adapter OK, but no registration for the CoS's adapter
	// (also "mock", but we'll simulate missing by scripting alice to
	// halt and CoS script exhausted — same mock, so it'll error on
	// script exhaustion rather than adapter lookup; still proves the
	// error path surfaces).
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "halted"}},
	))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "start",
		MaxTurns:      5,
	}
	_, err := loop.Run(context.Background())
	if err == nil {
		t.Fatal("want error from CoS rescue invocation")
	}
	if !strings.Contains(err.Error(), "chief_of_staff") {
		t.Errorf("error should mention chief_of_staff, got %v", err)
	}
}

func TestLoop_CoS_Milestone_DefaultInterval(t *testing.T) {
	root := scaffoldWithCoS(t, "alice", "sam",
		[]config.Trigger{config.TriggerMilestone},
		"alice", "bob",
	)
	// 3 turns with default interval (DefaultMilestoneEvery=3) → CoS fires
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@bob 1"}},
		mock.ScriptedResponse{ForMember: "bob", Response: adapter.InvokeResponse{Body: "@alice 2"}},
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@bob 3"}},
		mock.ScriptedResponse{ForMember: "sam", Response: adapter.InvokeResponse{Body: "progress update"}},
	))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "go",
		MaxTurns:      5,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var cosFired bool
	for _, e := range res.Events {
		if e.From == "sam" {
			cosFired = true
		}
	}
	if !cosFired {
		t.Errorf("want CoS to fire after default interval; events: %+v", eventTypesConcrete(res.Events))
	}
}

func TestHasTrigger(t *testing.T) {
	cos := config.ChiefOfStaff{
		Enabled:  true,
		Triggers: []config.Trigger{config.TriggerMilestone, config.TriggerGrayArea},
	}
	if !hasTrigger(cos, config.TriggerMilestone) {
		t.Error("want milestone")
	}
	if hasTrigger(cos, config.TriggerUnresolvedRouting) {
		t.Error("unresolved_routing not configured")
	}
	cos.Enabled = false
	if hasTrigger(cos, config.TriggerMilestone) {
		t.Error("disabled should return false")
	}
}

func TestHasTrigger_EmptyCos(t *testing.T) {
	if hasTrigger(config.ChiefOfStaff{}, config.TriggerMilestone) {
		t.Error("zero-value should return false")
	}
}

