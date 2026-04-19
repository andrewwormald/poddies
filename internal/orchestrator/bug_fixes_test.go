package orchestrator

import (
	"context"
	"testing"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/adapter/mock"
	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

// TestLoop_CoS_EmptyBody_NotAppended guards the B1 regression: prior
// to the fix, invokeChiefOfStaff would append an empty-body message
// event when the CoS adapter returned Body="". That event then became
// the last non-meta event, permanently halting routing.
func TestLoop_CoS_EmptyBody_NotAppended(t *testing.T) {
	root := scaffoldWithCoS(t, "alice", "sam",
		[]config.Trigger{config.TriggerUnresolvedRouting},
		"alice",
	)
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "I'm stuck"}},
		mock.ScriptedResponse{ForMember: "sam", Response: adapter.InvokeResponse{Body: ""}},
	))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "start",
		MaxTurns:      3,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.StopReason != LoopQuiescent {
		t.Errorf("want quiescent, got %s", res.StopReason)
	}
	// only human + alice; CoS empty response NOT appended.
	if len(res.Events) != 2 {
		for i, e := range res.Events {
			t.Logf("event[%d]: %s from=%s body=%q", i, e.Type, e.From, e.Body)
		}
		t.Errorf("want 2 events (human + alice), got %d", len(res.Events))
	}
}

// TestLoop_RescueResetsMilestoneCounter guards S3: after the rescue
// path fires, turnsSinceMilestone must reset so the next member turn
// doesn't double-trigger (rescue + milestone).
func TestLoop_RescueResetsMilestoneCounter(t *testing.T) {
	root := scaffoldWithCoS(t, "alice", "sam",
		[]config.Trigger{config.TriggerUnresolvedRouting, config.TriggerMilestone},
		"alice", "bob",
	)
	// Scenario: alice halts → sam rescues with @bob → bob runs.
	// If milestone counter wasn't reset, the next iteration would see
	// turnsSinceMilestone >= MilestoneEvery (1) and fire sam again.
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "stuck"}},
		mock.ScriptedResponse{ForMember: "sam", Response: adapter.InvokeResponse{Body: "@bob help?"}},
		mock.ScriptedResponse{ForMember: "bob", Response: adapter.InvokeResponse{Body: "done"}},
	))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:           root,
		Pod:            "demo",
		AdapterLookup:  MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:            log,
		HumanMessage:   "start",
		MaxTurns:       5,
		MilestoneEvery: 2,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Expect exactly: human, alice, sam (rescue), bob. No extra sam.
	if len(res.Events) != 4 {
		for i, e := range res.Events {
			t.Logf("event[%d]: %s from=%s", i, e.Type, e.From)
		}
		t.Fatalf("want 4 events, got %d", len(res.Events))
	}
	samCount := 0
	for _, e := range res.Events {
		if e.From == "sam" {
			samCount++
		}
	}
	if samCount != 1 {
		t.Errorf("want exactly 1 CoS event, got %d", samCount)
	}
}

// TestLoop_CoS_UsesRoleChiefOfStaff_InAdapterCall guards the identity
// fix: when invokeChiefOfStaff runs, the adapter must receive
// Role=RoleChiefOfStaff and a populated ChiefOfStaff config, so the
// adapter can render a CoS-specific prompt rather than a zero-value
// Member one.
func TestLoop_CoS_UsesRoleChiefOfStaff_InAdapterCall(t *testing.T) {
	root := scaffoldWithCoS(t, "alice", "sam",
		[]config.Trigger{config.TriggerUnresolvedRouting},
		"alice",
	)
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "halt"}},
		mock.ScriptedResponse{ForMember: "sam", Response: adapter.InvokeResponse{Body: "handling it"}},
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
	if _, err := loop.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := m.Calls()
	if len(calls) < 2 {
		t.Fatalf("want >=2 calls, got %d", len(calls))
	}
	// The second call is the CoS invocation.
	if calls[1].Role != adapter.RoleChiefOfStaff {
		t.Errorf("CoS call role: want %s, got %s", adapter.RoleChiefOfStaff, calls[1].Role)
	}
	if calls[1].MemberName != "sam" {
		t.Errorf("CoS call should be attributed to sam, got %q", calls[1].MemberName)
	}
}

// ensure unused import gracefully — thread.EventMessage may not be
// referenced if tests above drop it.
var _ = thread.EventMessage
