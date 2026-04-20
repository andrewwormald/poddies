package orchestrator

import (
	"context"
	"testing"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/adapter/mock"
)

// TestLoop_DeltaResume_ThreadShrinks runs the same thread twice. On the
// second run the adapter should receive only the incremental events since
// the member last spoke, not the full history. This is the A2 efficiency
// win: Claude already has prior context via --resume so re-sending it is
// pure waste.
func TestLoop_DeltaResume_ThreadShrinks(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice")
	log := newDeterministicLog(t, t.TempDir())

	m := mock.New(mock.WithScript(
		// Run 1: alice sees [human "kick off"] = 1 event, returns SessionID.
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{
			Body: "done first", SessionID: "sess-alice",
		}},
		// Run 2: alice should receive only the delta since her last turn.
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{
			Body: "done second",
		}},
	))
	lookup := MapLookup(map[string]adapter.Adapter{"mock": m})

	// Run 1 — fresh session, no prior session ID in meta.
	if _, err := (&Loop{
		Root: root, Pod: "demo", AdapterLookup: lookup,
		Log: log, HumanMessage: "kick off",
	}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Run 2 — meta has alice's session ID; delta should activate.
	if _, err := (&Loop{
		Root: root, Pod: "demo", AdapterLookup: lookup,
		Log: log, HumanMessage: "follow up",
	}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	calls := m.Calls()
	if len(calls) != 2 {
		t.Fatalf("want 2 invocations, got %d", len(calls))
	}

	// Run 1: full thread = [human "kick off"] = 1 event.
	if calls[0].ThreadLength != 1 {
		t.Errorf("run 1: want ThreadLength=1 (full thread), got %d", calls[0].ThreadLength)
	}
	if calls[0].PriorSessionID != "" {
		t.Errorf("run 1: want no prior session ID, got %q", calls[0].PriorSessionID)
	}

	// Run 2: full thread at invocation time = 3 events
	// (human "kick off" + alice "done first" + human "follow up").
	// Delta = events since alice's last turn = [human "follow up"] = 1 event.
	if calls[1].ThreadLength >= 3 {
		t.Errorf("run 2: want delta (< 3 events), got ThreadLength=%d — delta not applied",
			calls[1].ThreadLength)
	}
	if calls[1].PriorSessionID != "sess-alice" {
		t.Errorf("run 2: want PriorSessionID=sess-alice, got %q", calls[1].PriorSessionID)
	}
}

// TestLoop_DeltaResume_NoSessionID_FullThreadSent verifies that when an
// adapter does not return a SessionID (e.g. Gemini CLI plain mode), the
// delta path is NOT activated and the full thread is always sent.
func TestLoop_DeltaResume_NoSessionID_FullThreadSent(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice")
	log := newDeterministicLog(t, t.TempDir())

	m := mock.New(mock.WithScript(
		// Run 1: no SessionID returned → delta must NOT activate on run 2.
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{
			Body: "done first", // SessionID deliberately empty
		}},
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{
			Body: "done second",
		}},
	))
	lookup := MapLookup(map[string]adapter.Adapter{"mock": m})

	if _, err := (&Loop{
		Root: root, Pod: "demo", AdapterLookup: lookup,
		Log: log, HumanMessage: "kick off",
	}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	if _, err := (&Loop{
		Root: root, Pod: "demo", AdapterLookup: lookup,
		Log: log, HumanMessage: "follow up",
	}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	calls := m.Calls()
	if len(calls) != 2 {
		t.Fatalf("want 2 calls, got %d", len(calls))
	}
	// Run 2 without a session ID: full thread (3 events) must be sent.
	if calls[1].ThreadLength < 3 {
		t.Errorf("run 2: want full thread (3 events) when no session ID, got %d", calls[1].ThreadLength)
	}
	if calls[1].PriorSessionID != "" {
		t.Errorf("run 2: want no prior session ID, got %q", calls[1].PriorSessionID)
	}
}
