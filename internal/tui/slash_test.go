package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/andrewwormald/poddies/internal/orchestrator"
	"github.com/andrewwormald/poddies/internal/thread"
)

// submitString feeds the input box and simulates Enter, driving the
// submit path without running a real bubbletea Program.
func submitString(m Model, text string) (Model, tea.Cmd) {
	m, _ = updateAs(m, sizeMsg())
	m.input.SetValue(text)
	return updateAs(m, tea.KeyMsg{Type: tea.KeyEnter})
}

func TestSlash_Help_UpdatesStatus(t *testing.T) {
	m := NewModel(Options{PodName: "demo", StartLoop: okStartLoop})
	m, _ = submitString(m, "/help")
	if !strings.Contains(m.Status(), "/add") {
		t.Errorf("status should list commands, got %q", m.Status())
	}
}

func TestSlash_Unknown_FriendlyError(t *testing.T) {
	m := NewModel(Options{PodName: "demo", StartLoop: okStartLoop})
	m, _ = submitString(m, "/nope")
	if !strings.Contains(m.Status(), "unknown command") {
		t.Errorf("status should flag unknown, got %q", m.Status())
	}
}

func TestSlash_Quit_TransitionsToQuit(t *testing.T) {
	m := NewModel(Options{PodName: "demo", StartLoop: okStartLoop})
	m, cmd := submitString(m, "/quit")
	if m.CurrentState() != StateQuit {
		t.Errorf("want StateQuit, got %v", m.CurrentState())
	}
	if cmd == nil {
		t.Error("expect tea.Quit cmd")
	}
}

func TestSlash_Add_ActivatesWizard(t *testing.T) {
	m := NewModel(Options{
		PodName:     "demo",
		StartLoop:   okStartLoop,
		OnAddMember: func(AddMemberSpec) error { return nil },
	})
	m, _ = submitString(m, "/add")
	if m.CurrentState() != StatePrompting {
		t.Errorf("want StatePrompting, got %v", m.CurrentState())
	}
	if m.ActiveWizard() == nil {
		t.Error("wizard should be active")
	}
	if m.ActiveWizard().Title != "add member" {
		t.Errorf("want add member wizard, got %s", m.ActiveWizard().Title)
	}
}

func TestSlash_Remove_EmptyRoster_Status(t *testing.T) {
	m := NewModel(Options{PodName: "demo", StartLoop: okStartLoop})
	m, _ = submitString(m, "/remove")
	if !strings.Contains(m.Status(), "no members") {
		t.Errorf("want 'no members', got %q", m.Status())
	}
	if m.CurrentState() != StateIdle {
		t.Errorf("should stay idle, got %v", m.CurrentState())
	}
}

func TestSlash_Remove_WithRoster_ActivatesWizard(t *testing.T) {
	m := NewModel(Options{
		PodName:        "demo",
		StartLoop:      okStartLoop,
		OnListMembers:  func() []string { return []string{"alice", "bob"} },
		OnRemoveMember: func(string) error { return nil },
	})
	m, _ = submitString(m, "/remove")
	if m.CurrentState() != StatePrompting {
		t.Errorf("want StatePrompting, got %v", m.CurrentState())
	}
}

func TestSlash_Export_NotWired_StatusShowsError(t *testing.T) {
	m := NewModel(Options{PodName: "demo", StartLoop: okStartLoop})
	m, _ = submitString(m, "/export")
	if !strings.Contains(m.Status(), "not wired") {
		t.Errorf("want 'not wired', got %q", m.Status())
	}
}

func TestSlash_Export_Success_AppendsPreview(t *testing.T) {
	m := NewModel(Options{
		PodName:     "demo",
		StartLoop:   okStartLoop,
		OnExportPod: func() ([]byte, error) { return []byte("schema_version = 1"), nil },
	})
	m, _ = submitString(m, "/export")
	if len(m.Events()) != 1 {
		t.Fatalf("want 1 preview event, got %d", len(m.Events()))
	}
	if !strings.Contains(m.Events()[0].Body, "schema_version = 1") {
		t.Errorf("preview missing bundle content: %q", m.Events()[0].Body)
	}
}

func TestSubmit_NonSlash_RoutesToStartLoop(t *testing.T) {
	started := make(chan string, 1)
	m := NewModel(Options{
		PodName: "demo",
		StartLoop: func(_ context.Context, kickoff string, _ func(thread.Event)) (orchestrator.LoopResult, error) {
			started <- kickoff
			return orchestrator.LoopResult{StopReason: orchestrator.LoopQuiescent}, nil
		},
	})
	m, _ = submitString(m, "hello")
	if m.CurrentState() != StateRunning {
		t.Errorf("want running, got %v", m.CurrentState())
	}
	_ = started
}

func TestWizard_CompletesAndCallsOnAdd(t *testing.T) {
	got := make(chan AddMemberSpec, 1)
	m := NewModel(Options{
		PodName:     "demo",
		StartLoop:   okStartLoop,
		OnAddMember: func(spec AddMemberSpec) error { got <- spec; return nil },
	})
	m, _ = submitString(m, "/add")
	// step through all 6 questions
	for _, ans := range []string{"alice", "Staff", "1", "claude-opus-4-7", "3", "pragmatic"} {
		m.input.SetValue(ans)
		m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyEnter})
	}
	select {
	case s := <-got:
		if s.Name != "alice" || s.Title != "Staff" || s.Adapter != "claude" || s.Effort != "high" {
			t.Errorf("unexpected spec: %+v", s)
		}
	default:
		t.Fatal("OnAddMember never called")
	}
	if m.CurrentState() != StateIdle {
		t.Errorf("want idle after wizard, got %v", m.CurrentState())
	}
}

func TestWizard_ValidationError_KeepsWizardOnSameStep(t *testing.T) {
	m := NewModel(Options{
		PodName:     "demo",
		StartLoop:   okStartLoop,
		OnAddMember: func(AddMemberSpec) error { return nil },
	})
	m, _ = submitString(m, "/add")
	// step 1 expects a slug — submit an invalid value
	m.input.SetValue("Invalid Name With Spaces")
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.ActiveWizard() == nil {
		t.Fatal("wizard should still be active after validation error")
	}
	if cur, _ := m.ActiveWizard().Progress(); cur != 1 {
		t.Errorf("wizard should stay on step 1 after error, got %d", cur)
	}
	if !strings.Contains(m.Status(), "error") {
		t.Errorf("status should flag error, got %q", m.Status())
	}
}

func TestWizard_Escape_Cancels(t *testing.T) {
	m := NewModel(Options{
		PodName:     "demo",
		StartLoop:   okStartLoop,
		OnAddMember: func(AddMemberSpec) error { return nil },
	})
	m, _ = submitString(m, "/add")
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.ActiveWizard() != nil {
		t.Error("wizard should be nil after escape")
	}
	if m.CurrentState() != StateIdle {
		t.Errorf("want idle, got %v", m.CurrentState())
	}
	if !strings.Contains(m.Status(), "cancel") {
		t.Errorf("status should mention cancel, got %q", m.Status())
	}
}

// --- /resume tests ---

func sessions() []SessionSummary {
	return []SessionSummary{
		{ID: "sess-001", Pod: "demo", TurnCount: 3, LastSpeaker: "alice", LastEditedAt: "2026-04-19T10:00:00Z", IsCurrent: true},
		{ID: "sess-002", Pod: "demo", TurnCount: 1, LastSpeaker: "bob", LastEditedAt: "2026-04-18T09:00:00Z"},
	}
}

func TestSlash_Resume_NotWired_Status(t *testing.T) {
	m := NewModel(Options{PodName: "demo", StartLoop: okStartLoop}) // no OnListSessions
	m, _ = submitString(m, "/resume")
	if !strings.Contains(m.Status(), "not wired") {
		t.Errorf("want 'not wired' status, got %q", m.Status())
	}
}

func TestSlash_Resume_NoArg_OpensSessionsView(t *testing.T) {
	m := NewModel(Options{
		PodName:        "demo",
		StartLoop:      okStartLoop,
		OnListSessions: func() []SessionSummary { return nil },
	})
	m, _ = submitString(m, "/resume")
	if m.view != ViewSessions {
		t.Errorf("want ViewSessions, got view=%d", m.view)
	}
}

func TestSlash_Resume_NoArg_WithSessions_OpensView(t *testing.T) {
	m := NewModel(Options{
		PodName:        "demo",
		StartLoop:      okStartLoop,
		OnListSessions: sessions,
	})
	m, _ = submitString(m, "/resume")
	if m.view != ViewSessions {
		t.Errorf("want ViewSessions, got view=%d", m.view)
	}
	if m.cursorPos != 0 {
		t.Errorf("want cursorPos=0, got %d", m.cursorPos)
	}
}

func TestSlash_Resume_ByNumber_InvokesCallback(t *testing.T) {
	resumed := ""
	m := NewModel(Options{
		PodName:         "demo",
		StartLoop:       okStartLoop,
		OnListSessions:  sessions,
		OnResumeSession: func(id string) { resumed = id },
	})
	m, cmd := submitString(m, "/resume 2")
	if resumed != "sess-002" {
		t.Errorf("want sess-002 resumed, got %q", resumed)
	}
	if cmd == nil {
		t.Error("expect tea.Quit cmd")
	}
	if m.CurrentState() != StateQuit {
		t.Errorf("want StateQuit, got %v", m.CurrentState())
	}
}

func TestSlash_Resume_ByID_InvokesCallback(t *testing.T) {
	resumed := ""
	m := NewModel(Options{
		PodName:         "demo",
		StartLoop:       okStartLoop,
		OnListSessions:  sessions,
		OnResumeSession: func(id string) { resumed = id },
	})
	m, cmd := submitString(m, "/resume sess-001")
	if resumed != "sess-001" {
		t.Errorf("want sess-001 resumed, got %q", resumed)
	}
	if cmd == nil || m.CurrentState() != StateQuit {
		t.Error("expect quit after resume by ID")
	}
}

func TestSlash_Resume_BadNumber_Status(t *testing.T) {
	m := NewModel(Options{
		PodName:        "demo",
		StartLoop:      okStartLoop,
		OnListSessions: sessions,
	})
	m, _ = submitString(m, "/resume 9")
	if !strings.Contains(m.Status(), "out of range") {
		t.Errorf("want 'out of range', got %q", m.Status())
	}
}

func TestSlash_Resume_BadID_Status(t *testing.T) {
	m := NewModel(Options{
		PodName:        "demo",
		StartLoop:      okStartLoop,
		OnListSessions: sessions,
	})
	m, _ = submitString(m, "/resume no-such-session")
	if !strings.Contains(m.Status(), "no session matching") {
		t.Errorf("want 'no session matching', got %q", m.Status())
	}
}

func TestView_Wizard_RendersQuestionAndChoices(t *testing.T) {
	m := NewModel(Options{
		PodName:     "demo",
		StartLoop:   okStartLoop,
		OnAddMember: func(AddMemberSpec) error { return nil },
	})
	m, _ = submitString(m, "/add")
	view := m.View()
	for _, want := range []string{"add member", "step 1", "Name", "Esc"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	// advance to a choice step
	for _, ans := range []string{"alice", "Staff"} {
		m.input.SetValue(ans)
		m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyEnter})
	}
	view = m.View()
	for _, want := range []string{"Adapter", "1. claude", "2. gemini"} {
		if !strings.Contains(view, want) {
			t.Errorf("choice view missing %q:\n%s", want, view)
		}
	}
}

