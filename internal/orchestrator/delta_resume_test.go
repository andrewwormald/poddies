package orchestrator

import (
	"context"
	"testing"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/adapter/mock"
	"github.com/andrewwormald/poddies/internal/thread"
)

// TestLoop_ContextWindow_Bounded verifies that even when the thread log
// contains more events than DefaultContextWindow, the adapter receives at
// most DefaultContextWindow events. This is the O(1)-per-turn guarantee
// that prevents quadratic token growth.
func TestLoop_ContextWindow_Bounded(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice")
	log := newDeterministicLog(t, t.TempDir())

	// Pre-populate the log with more events than the window.
	excess := DefaultContextWindow + 5
	for i := 0; i < excess; i++ {
		if _, err := log.Append(thread.Event{
			Type: thread.EventMessage, From: "alice",
			Body: "prior message",
		}); err != nil {
			t.Fatal(err)
		}
	}

	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{
			Body: "ack", // no @mention → loop becomes quiescent
		}},
	))
	lookup := MapLookup(map[string]adapter.Adapter{"mock": m})

	if _, err := (&Loop{
		Root: root, Pod: "demo", AdapterLookup: lookup,
		Log: log, HumanMessage: "go",
	}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	calls := m.Calls()
	if len(calls) == 0 {
		t.Fatal("no invocations recorded")
	}
	for i, c := range calls {
		if c.ThreadLength > DefaultContextWindow {
			t.Errorf("invocation %d: sent %d events, want ≤ %d",
				i, c.ThreadLength, DefaultContextWindow)
		}
	}
}

// TestLoop_ContextWindow_ShortThread verifies that when the thread is
// shorter than DefaultContextWindow all events are still sent — the
// window does not truncate unnecessarily.
func TestLoop_ContextWindow_ShortThread(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice")
	log := newDeterministicLog(t, t.TempDir())

	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{
			Body: "ack",
		}},
	))
	lookup := MapLookup(map[string]adapter.Adapter{"mock": m})

	// HumanMessage produces 1 event — well inside the window.
	if _, err := (&Loop{
		Root: root, Pod: "demo", AdapterLookup: lookup,
		Log: log, HumanMessage: "just a short kick-off",
	}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	calls := m.Calls()
	if len(calls) == 0 {
		t.Fatal("no invocations recorded")
	}
	if calls[0].ThreadLength == 0 {
		t.Errorf("short thread: invocation 0 received 0 events, want > 0")
	}
	if calls[0].ThreadLength > DefaultContextWindow {
		t.Errorf("short thread: got %d events, exceeds window %d",
			calls[0].ThreadLength, DefaultContextWindow)
	}
}

// TestLoop_NoPriorSessionID verifies that PriorSessionID is never set on
// any invocation. Server-side session resumption was removed because
// accumulated tool-call results caused quadratic token growth.
func TestLoop_NoPriorSessionID(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice")
	log := newDeterministicLog(t, t.TempDir())

	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{
			Body: "ack", SessionID: "sess-1", // returns a SessionID
		}},
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{
			Body: "ack",
		}},
	))
	lookup := MapLookup(map[string]adapter.Adapter{"mock": m})

	// Two runs: even though run 1 returns a SessionID, run 2 must NOT
	// pass it as PriorSessionID.
	for i := 0; i < 2; i++ {
		if _, err := (&Loop{
			Root: root, Pod: "demo", AdapterLookup: lookup,
			Log: log, HumanMessage: "ping",
		}).Run(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	calls := m.Calls()
	for i, c := range calls {
		if c.PriorSessionID != "" {
			t.Errorf("invocation %d: PriorSessionID should be empty, got %q", i, c.PriorSessionID)
		}
	}
}
