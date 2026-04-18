package thread

import (
	"testing"
)

func TestPendingPermissions_None(t *testing.T) {
	events := []Event{
		{ID: "1", Type: EventHuman, Body: "hi"},
		{ID: "2", Type: EventMessage, From: "alice", Body: "yo"},
	}
	if got := PendingPermissions(events); len(got) != 0 {
		t.Errorf("want none, got %v", got)
	}
}

func TestPendingPermissions_Unresolved(t *testing.T) {
	events := []Event{
		{ID: "req1", Type: EventPermissionRequest, From: "alice", Action: "run"},
	}
	got := PendingPermissions(events)
	if len(got) != 1 || got[0].ID != "req1" {
		t.Errorf("want [req1], got %+v", got)
	}
}

func TestPendingPermissions_ResolvedByGrant(t *testing.T) {
	events := []Event{
		{ID: "req1", Type: EventPermissionRequest, From: "alice", Action: "run"},
		{ID: "g1", Type: EventPermissionGrant, From: "human", RequestID: "req1"},
	}
	if got := PendingPermissions(events); len(got) != 0 {
		t.Errorf("want none after grant, got %+v", got)
	}
}

func TestPendingPermissions_ResolvedByDeny(t *testing.T) {
	events := []Event{
		{ID: "req1", Type: EventPermissionRequest, From: "alice", Action: "run"},
		{ID: "d1", Type: EventPermissionDeny, From: "human", RequestID: "req1"},
	}
	if got := PendingPermissions(events); len(got) != 0 {
		t.Errorf("want none after deny, got %+v", got)
	}
}

func TestPendingPermissions_MixedPendingAndResolved(t *testing.T) {
	events := []Event{
		{ID: "req1", Type: EventPermissionRequest, From: "alice"},
		{ID: "g1", Type: EventPermissionGrant, From: "human", RequestID: "req1"},
		{ID: "req2", Type: EventPermissionRequest, From: "bob"},
	}
	got := PendingPermissions(events)
	if len(got) != 1 || got[0].ID != "req2" {
		t.Errorf("want [req2], got %+v", got)
	}
}

func TestPendingPermissions_OrderPreserved(t *testing.T) {
	events := []Event{
		{ID: "a", Type: EventPermissionRequest, From: "alice"},
		{ID: "b", Type: EventPermissionRequest, From: "bob"},
		{ID: "c", Type: EventPermissionRequest, From: "carol"},
	}
	got := PendingPermissions(events)
	if len(got) != 3 {
		t.Fatalf("want 3, got %d", len(got))
	}
	for i, id := range []string{"a", "b", "c"} {
		if got[i].ID != id {
			t.Errorf("order[%d]: want %s, got %s", i, id, got[i].ID)
		}
	}
}

func TestHasPendingPermissions(t *testing.T) {
	empty := []Event{{Type: EventMessage, From: "alice", Body: "hi"}}
	if HasPendingPermissions(empty) {
		t.Error("want false, got true")
	}
	withReq := []Event{{ID: "r", Type: EventPermissionRequest, From: "alice"}}
	if !HasPendingPermissions(withReq) {
		t.Error("want true, got false")
	}
}

func TestFindRequest(t *testing.T) {
	events := []Event{
		{ID: "a", Type: EventPermissionRequest, From: "alice"},
		{ID: "b", Type: EventMessage, Body: "unrelated"},
	}
	if _, ok := FindRequest(events, "a"); !ok {
		t.Error("want found")
	}
	if _, ok := FindRequest(events, "b"); ok {
		t.Error("should not match non-request event")
	}
	if _, ok := FindRequest(events, "ghost"); ok {
		t.Error("should not find missing id")
	}
}

func TestIsResolved(t *testing.T) {
	events := []Event{
		{ID: "req1", Type: EventPermissionRequest, From: "alice"},
		{ID: "g1", Type: EventPermissionGrant, From: "human", RequestID: "req1"},
	}
	if !IsResolved(events, "req1") {
		t.Error("want resolved")
	}
	if IsResolved(events, "req2") {
		t.Error("ghost should not be resolved")
	}
}
