package orchestrator

import (
	"testing"

	"github.com/andrewwormald/poddies/internal/thread"
)

func TestRoute_SkipsPermissionEvents_RoutesOnRealLastTurn(t *testing.T) {
	events := []thread.Event{
		{Type: thread.EventMessage, From: "alice", Body: "@bob please help", Mentions: []string{"bob"}},
		{Type: thread.EventPermissionRequest, From: "alice", Action: "run"},
	}
	got := Route(events, abMembers, "human")
	if got.Action != ActionInvoke || got.Member != "bob" {
		t.Errorf("want invoke bob, got %+v", got)
	}
}

func TestRoute_SkipsPermissionGrant(t *testing.T) {
	events := []thread.Event{
		{Type: thread.EventMessage, From: "alice", Body: "@bob help", Mentions: []string{"bob"}},
		{Type: thread.EventPermissionRequest, From: "alice"},
		{Type: thread.EventPermissionGrant, From: "human", RequestID: "req1"},
	}
	got := Route(events, abMembers, "human")
	if got.Action != ActionInvoke || got.Member != "bob" {
		t.Errorf("want invoke bob after grant, got %+v", got)
	}
}

func TestRoute_OnlyPermissionEvents_Halts(t *testing.T) {
	events := []thread.Event{
		{Type: thread.EventPermissionRequest, From: "alice"},
		{Type: thread.EventPermissionGrant, From: "human", RequestID: "req1"},
	}
	got := Route(events, abMembers, "human")
	if got.Action != ActionHalt {
		t.Errorf("want halt, got %+v", got)
	}
}
