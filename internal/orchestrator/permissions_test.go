package orchestrator

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/adapter/mock"
	"github.com/andrewwormald/poddies/internal/thread"
)

func TestLoop_AgentPermissionRequest_HaltsWithPendingPermission(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice", "bob")
	m := mock.New(mock.WithScript(mock.ScriptedResponse{
		ForMember: "alice",
		Response: adapter.InvokeResponse{
			Body: "I need access to prod",
			PermissionRequests: []adapter.PermissionRequest{
				{Action: "access_prod", Payload: []byte(`{"reason":"debug"}`)},
			},
		},
	}))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "ship the fix",
		MaxTurns:      5,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.StopReason != LoopPendingPermission {
		t.Errorf("want pending_permission, got %s", res.StopReason)
	}
	// expect: human + alice_message + alice_permission_request
	if len(res.Events) != 3 {
		t.Fatalf("want 3 events, got %d: %+v", len(res.Events), eventTypesConcrete(res.Events))
	}
	if res.Events[2].Type != thread.EventPermissionRequest {
		t.Errorf("want permission_request last, got %s", res.Events[2].Type)
	}
}

func TestLoop_AgentEmitsRequestWithoutBody_NoEmptyMessageEvent(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice")
	m := mock.New(mock.WithScript(mock.ScriptedResponse{
		ForMember: "alice",
		Response: adapter.InvokeResponse{
			Body: "",
			PermissionRequests: []adapter.PermissionRequest{
				{Action: "run_shell"},
			},
		},
	}))
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
	// human + permission_request only (no empty message event)
	if len(res.Events) != 2 {
		t.Fatalf("want 2 events, got %d: %+v", len(res.Events), eventTypesConcrete(res.Events))
	}
	if res.Events[1].Type != thread.EventPermissionRequest {
		t.Errorf("want permission_request last, got %s", res.Events[1].Type)
	}
}

func TestLoop_PreexistingUnresolvedRequest_HaltsImmediately(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice")
	m := mock.New() // script doesn't matter; loop should halt before invoking
	dir := t.TempDir()
	logPath := filepath.Join(dir, "t.jsonl")
	log := newDeterministicLog(t, dir)
	log.Path = logPath
	_ = log.EnsureFile()
	// seed an unresolved permission_request
	if _, err := log.Append(thread.Event{
		Type: thread.EventPermissionRequest, From: "alice", Action: "run",
	}); err != nil {
		t.Fatal(err)
	}

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		MaxTurns:      3,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.StopReason != LoopPendingPermission {
		t.Errorf("want pending_permission, got %s", res.StopReason)
	}
	if res.TurnsRun != 0 {
		t.Errorf("should not invoke any members; got %d turns", res.TurnsRun)
	}
	if len(m.Calls()) != 0 {
		t.Errorf("mock should not have been called: %+v", m.Calls())
	}
}

func TestLoop_ResumedAfterGrant_Continues(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice", "bob")
	m := mock.New(mock.WithScript(mock.ScriptedResponse{
		ForMember: "alice",
		Response:  adapter.InvokeResponse{Body: "thanks for the approval, done"},
	}))
	dir := t.TempDir()
	log := newDeterministicLog(t, dir)
	_ = log.EnsureFile()

	// simulate a previous turn that produced a request, then a grant.
	if _, err := log.Append(thread.Event{
		Type: thread.EventHuman, Body: "ship the fix",
	}); err != nil {
		t.Fatal(err)
	}
	req, err := log.Append(thread.Event{
		Type: thread.EventPermissionRequest, From: "alice", Action: "access_prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := log.Append(thread.Event{
		Type: thread.EventPermissionGrant, From: "human", RequestID: req.ID,
	}); err != nil {
		t.Fatal(err)
	}

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		MaxTurns:      3,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// lead=alice + last-conversational-event is human (ignoring
	// perm events) → routes to alice (lead fallback); alice then
	// quiesces after bob fallback would exhaust script. Bob has no
	// script response → loop halts after alice's single turn.
	if res.StopReason != LoopQuiescent {
		t.Errorf("want quiescent after resume, got %s", res.StopReason)
	}
	if res.TurnsRun != 1 {
		t.Errorf("want 1 new turn, got %d", res.TurnsRun)
	}
}

func TestLoop_MultipleRequestsInOneTurn_AllAppended(t *testing.T) {
	root := scaffoldWithMembers(t, "alice", "alice")
	m := mock.New(mock.WithScript(mock.ScriptedResponse{
		ForMember: "alice",
		Response: adapter.InvokeResponse{
			Body: "asking two things",
			PermissionRequests: []adapter.PermissionRequest{
				{Action: "a"},
				{Action: "b"},
			},
		},
	}))
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
	if res.StopReason != LoopPendingPermission {
		t.Errorf("want pending_permission, got %s", res.StopReason)
	}
	// human + alice_message + 2 requests = 4
	if len(res.Events) != 4 {
		t.Fatalf("want 4 events, got %d: %+v", len(res.Events), eventTypesConcrete(res.Events))
	}
	reqCount := 0
	for _, e := range res.Events {
		if e.Type == thread.EventPermissionRequest {
			reqCount++
		}
	}
	if reqCount != 2 {
		t.Errorf("want 2 requests, got %d", reqCount)
	}
}
