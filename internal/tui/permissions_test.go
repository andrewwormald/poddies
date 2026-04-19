package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/andrewwormald/poddies/internal/orchestrator"
	"github.com/andrewwormald/poddies/internal/thread"
)

// --- helpers ---

func makePendingEvent(id, from, action string) thread.Event {
	return thread.Event{
		ID:     id,
		Type:   thread.EventPermissionRequest,
		From:   from,
		Action: action,
	}
}

func modelWithPending(pending []thread.Event, onApprove func(string) error, onDeny func(string, string) error) Model {
	m := NewModel(Options{
		PodName:   "demo",
		StartLoop: okStartLoop,
		OnApprove: onApprove,
		OnDeny:    onDeny,
		GetPending: func() []thread.Event {
			return pending
		},
	})
	m.pendingRequests = pending
	m.state = StateIdle
	return m
}

// --- renderPermissionsPane ---

func TestRenderPermissionsPane_EmptyReturnsEmpty(t *testing.T) {
	out := renderPermissionsPane(nil, 80)
	if out != "" {
		t.Errorf("want empty output for no pending, got %q", out)
	}
}

func TestRenderPermissionsPane_ShowsRequestDetails(t *testing.T) {
	pending := []thread.Event{
		makePendingEvent("abcdef123", "alice", "run_command"),
		makePendingEvent("xyz999", "bob", "write_file"),
	}
	out := renderPermissionsPane(pending, 80)
	for _, want := range []string{"abcdef", "alice", "run_command", "xyz999", "bob", "write_file"} {
		if !strings.Contains(out, want) {
			t.Errorf("pane missing %q:\n%s", want, out)
		}
	}
}

func TestRenderPermissionsPane_ShowsCount(t *testing.T) {
	pending := []thread.Event{
		makePendingEvent("id1", "alice", "act"),
	}
	out := renderPermissionsPane(pending, 80)
	if !strings.Contains(out, "1") {
		t.Errorf("want count in pane, got:\n%s", out)
	}
}

func TestRenderPermissionsPane_ShowsKeyHints(t *testing.T) {
	pending := []thread.Event{makePendingEvent("id1", "alice", "act")}
	out := renderPermissionsPane(pending, 80)
	for _, want := range []string{"a=approve", "d=deny", "A=approve-all", "D=deny-all"} {
		if !strings.Contains(out, want) {
			t.Errorf("pane missing hint %q:\n%s", want, out)
		}
	}
}

func TestShortID(t *testing.T) {
	if shortID("abcdefghij") != "abcdef" {
		t.Error("want 6-char short ID")
	}
	if shortID("abc") != "abc" {
		t.Error("want full id when shorter than 6")
	}
	if shortID("abcdef") != "abcdef" {
		t.Error("want exact 6-char id unchanged")
	}
}

// --- key: a (approve oldest) ---

func TestUpdate_KeyA_ApprovesOldest(t *testing.T) {
	var approved []string
	pending := []thread.Event{
		makePendingEvent("id1", "alice", "act"),
		makePendingEvent("id2", "bob", "act"),
	}
	m := modelWithPending(pending, func(id string) error {
		approved = append(approved, id)
		return nil
	}, nil)
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if len(approved) != 1 || approved[0] != "id1" {
		t.Errorf("want id1 approved, got %v", approved)
	}
	if len(m.PendingRequests()) != 1 {
		t.Errorf("want 1 remaining pending, got %d", len(m.PendingRequests()))
	}
}

func TestUpdate_KeyA_NoPending_NoOp(t *testing.T) {
	called := false
	m := modelWithPending(nil, func(id string) error {
		called = true
		return nil
	}, nil)
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if called {
		t.Error("OnApprove should not be called when no pending")
	}
}

func TestUpdate_KeyA_CallbackError_SurfacesError(t *testing.T) {
	pending := []thread.Event{makePendingEvent("id1", "alice", "act")}
	m := modelWithPending(pending, func(id string) error {
		return errors.New("approve-fail")
	}, nil)
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if m.lastErr == nil || !strings.Contains(m.Status(), "approve-fail") {
		t.Errorf("want error surfaced, got status=%q err=%v", m.Status(), m.lastErr)
	}
}

// --- key: d (deny oldest) ---

func TestUpdate_KeyD_DeniesOldest(t *testing.T) {
	var denied []string
	pending := []thread.Event{
		makePendingEvent("id1", "alice", "act"),
		makePendingEvent("id2", "bob", "act"),
	}
	m := modelWithPending(pending, nil, func(id, reason string) error {
		denied = append(denied, id)
		return nil
	})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if len(denied) != 1 || denied[0] != "id1" {
		t.Errorf("want id1 denied, got %v", denied)
	}
	if len(m.PendingRequests()) != 1 {
		t.Errorf("want 1 remaining pending, got %d", len(m.PendingRequests()))
	}
}

func TestUpdate_KeyD_NoPending_NoOp(t *testing.T) {
	called := false
	m := modelWithPending(nil, nil, func(id, reason string) error {
		called = true
		return nil
	})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if called {
		t.Error("OnDeny should not be called when no pending")
	}
}

func TestUpdate_KeyD_CallbackError_SurfacesError(t *testing.T) {
	pending := []thread.Event{makePendingEvent("id1", "alice", "act")}
	m := modelWithPending(pending, nil, func(id, reason string) error {
		return errors.New("deny-fail")
	})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if m.lastErr == nil || !strings.Contains(m.Status(), "deny-fail") {
		t.Errorf("want error surfaced, got status=%q err=%v", m.Status(), m.lastErr)
	}
}

// --- key: A (approve all) ---

func TestUpdate_KeyCapA_ApprovesAll(t *testing.T) {
	var approved []string
	pending := []thread.Event{
		makePendingEvent("id1", "alice", "act"),
		makePendingEvent("id2", "bob", "act"),
		makePendingEvent("id3", "carol", "act"),
	}
	m := modelWithPending(pending, func(id string) error {
		approved = append(approved, id)
		return nil
	}, nil)
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	if len(approved) != 3 {
		t.Errorf("want 3 approvals, got %d: %v", len(approved), approved)
	}
	if len(m.PendingRequests()) != 0 {
		t.Errorf("want no pending after approve-all, got %d", len(m.PendingRequests()))
	}
}

func TestUpdate_KeyCapA_NoPending_NoOp(t *testing.T) {
	called := false
	m := modelWithPending(nil, func(id string) error {
		called = true
		return nil
	}, nil)
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	if called {
		t.Error("OnApprove should not be called when no pending")
	}
}

func TestUpdate_KeyCapA_ErrorOnFirst_ClearsAllPending(t *testing.T) {
	pending := []thread.Event{
		makePendingEvent("id1", "alice", "act"),
		makePendingEvent("id2", "bob", "act"),
	}
	m := modelWithPending(pending, func(id string) error {
		return errors.New("approve-all-fail")
	}, nil)
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	if m.lastErr == nil {
		t.Error("want error after approve-all failure")
	}
	if len(m.PendingRequests()) != 0 {
		t.Errorf("want pending cleared on error, got %d", len(m.PendingRequests()))
	}
}

// --- key: D (deny all) ---

func TestUpdate_KeyCapD_DeniesAll(t *testing.T) {
	var denied []string
	pending := []thread.Event{
		makePendingEvent("id1", "alice", "act"),
		makePendingEvent("id2", "bob", "act"),
	}
	m := modelWithPending(pending, nil, func(id, reason string) error {
		denied = append(denied, id)
		return nil
	})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	if len(denied) != 2 {
		t.Errorf("want 2 denials, got %d: %v", len(denied), denied)
	}
	if len(m.PendingRequests()) != 0 {
		t.Errorf("want no pending after deny-all, got %d", len(m.PendingRequests()))
	}
}

func TestUpdate_KeyCapD_NoPending_NoOp(t *testing.T) {
	called := false
	m := modelWithPending(nil, nil, func(id, reason string) error {
		called = true
		return nil
	})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	if called {
		t.Error("OnDeny should not be called when no pending")
	}
}

// --- keybindings don't fire while running ---

func TestUpdate_KeyA_WhileRunning_NoOp(t *testing.T) {
	called := false
	pending := []thread.Event{makePendingEvent("id1", "alice", "act")}
	m := modelWithPending(pending, func(id string) error {
		called = true
		return nil
	}, nil)
	m, _ = updateAs(m, sizeMsg())
	m.state = StateRunning
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if called {
		t.Error("OnApprove should not be called while loop is running")
	}
}

// --- onLoopDone populates pending ---

func TestUpdate_LoopDone_PendingPermission_PopulatesPending(t *testing.T) {
	expected := []thread.Event{makePendingEvent("req1", "alice", "exec")}
	m := NewModel(Options{
		PodName:   "demo",
		StartLoop: okStartLoop,
		GetPending: func() []thread.Event {
			return expected
		},
	})
	m, _ = updateAs(m, sizeMsg())
	m.state = StateRunning
	m, _ = updateAs(m, LoopDoneMsg{Result: orchestrator.LoopResult{
		StopReason: orchestrator.LoopPendingPermission,
		TurnsRun:   1,
	}})
	if len(m.PendingRequests()) != 1 {
		t.Fatalf("want 1 pending, got %d", len(m.PendingRequests()))
	}
	if m.PendingRequests()[0].ID != "req1" {
		t.Errorf("unexpected pending event: %+v", m.PendingRequests()[0])
	}
}

func TestUpdate_LoopDone_NonPending_DoesNotPopulate(t *testing.T) {
	called := false
	m := NewModel(Options{
		PodName:   "demo",
		StartLoop: okStartLoop,
		GetPending: func() []thread.Event {
			called = true
			return nil
		},
	})
	m, _ = updateAs(m, sizeMsg())
	m.state = StateRunning
	m, _ = updateAs(m, LoopDoneMsg{Result: orchestrator.LoopResult{
		StopReason: orchestrator.LoopQuiescent,
	}})
	if called {
		t.Error("GetPending should not be called for non-pending stop reason")
	}
}

// --- view renders pane only when pending > 0 ---

func TestView_ShowsPermissionPane_WhenPending(t *testing.T) {
	pending := []thread.Event{makePendingEvent("abc123", "alice", "run_tool")}
	m := modelWithPending(pending, nil, nil)
	m, _ = updateAs(m, sizeMsg())
	out := m.View()
	for _, want := range []string{"abc123", "alice", "run_tool", "approve", "deny"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q when pending:\n%s", want, out)
		}
	}
}

func TestView_NoPermissionPane_WhenNoPending(t *testing.T) {
	m := NewModel(Options{PodName: "demo", StartLoop: okStartLoop})
	m, _ = updateAs(m, sizeMsg())
	out := m.View()
	if strings.Contains(out, "pending permission") {
		t.Errorf("view should not show permission pane when no pending:\n%s", out)
	}
}
