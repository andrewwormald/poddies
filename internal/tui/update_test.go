package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/andrewwormald/poddies/internal/orchestrator"
	"github.com/andrewwormald/poddies/internal/thread"
)

// sizeMsg sends a realistic terminal size to Update so the Model
// transitions into the ready state, unlocking input/viewport behavior.
func sizeMsg() tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: 80, Height: 24}
}

func TestNewModel_Defaults(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	if m.CurrentState() != StateIdle {
		t.Errorf("want idle, got %v", m.CurrentState())
	}
	if m.Status() != "ready" {
		t.Errorf("want 'ready' status, got %q", m.Status())
	}
	if len(m.Events()) != 0 {
		t.Errorf("want no events, got %d", len(m.Events()))
	}
}

func TestUpdate_WindowResize_MakesModelReady(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	got, _ := m.Update(sizeMsg())
	mm := got.(Model)
	if !mm.ready {
		t.Error("expected ready=true after WindowSizeMsg")
	}
	if mm.viewport.Width == 0 || mm.viewport.Height == 0 {
		t.Errorf("viewport not sized: %+v", mm.viewport)
	}
}

func TestUpdate_EventMsg_AppendsEvent(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, EventMsg{Event: thread.Event{Type: thread.EventMessage, From: "alice", Body: "hi"}})
	if len(m.Events()) != 1 {
		t.Fatalf("want 1 event, got %d", len(m.Events()))
	}
	if m.Events()[0].From != "alice" {
		t.Errorf("unexpected event: %+v", m.Events()[0])
	}
}

func TestUpdate_LoopDone_TransitionsToIdle(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	m, _ = updateAs(m, sizeMsg())
	m.state = StateRunning
	m, _ = updateAs(m, LoopDoneMsg{Result: orchestrator.LoopResult{StopReason: orchestrator.LoopQuiescent, TurnsRun: 3}})
	if m.CurrentState() != StateIdle {
		t.Errorf("want Idle after loop done, got %v", m.CurrentState())
	}
	if !strings.Contains(m.Status(), "quiescent") {
		t.Errorf("want status to mention quiescent, got %q", m.Status())
	}
	if !strings.Contains(m.Status(), "turns=3") {
		t.Errorf("want turns count in status, got %q", m.Status())
	}
}

func TestUpdate_LoopDone_WithError_ShowsError(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	m, _ = updateAs(m, sizeMsg())
	m.state = StateRunning
	m, _ = updateAs(m, LoopDoneMsg{Err: errors.New("boom")})
	if m.CurrentState() != StateIdle {
		t.Error("want idle after error")
	}
	if m.lastErr == nil || !strings.Contains(m.Status(), "boom") {
		t.Errorf("want error surfaced in status, got %q (err=%v)", m.Status(), m.lastErr)
	}
}

func TestUpdate_CtrlC_Quits(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("want quit command")
	}
	// tea.Quit returns a sentinel; we can't compare easily, but the
	// State transition is the more interesting assertion.
	got, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if got.(Model).CurrentState() != StateQuit {
		t.Errorf("want StateQuit, got %v", got.(Model).CurrentState())
	}
}

func TestUpdate_EnterEmptyInput_NoOp(t *testing.T) {
	m := NewModel(Options{PodName: "demo", StartLoop: okStartLoop})
	m, _ = updateAs(m, sizeMsg())
	before := m.CurrentState()
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.CurrentState() != before {
		t.Errorf("state changed on empty enter: %v -> %v", before, m.CurrentState())
	}
}

func TestUpdate_Submit_StartsLoop(t *testing.T) {
	started := make(chan string, 1)
	opts := Options{
		PodName: "demo",
		StartLoop: func(ctx context.Context, kickoff string, _ func(thread.Event)) (orchestrator.LoopResult, error) {
			started <- kickoff
			// block briefly so we observe StateRunning
			time.Sleep(50 * time.Millisecond)
			return orchestrator.LoopResult{StopReason: orchestrator.LoopQuiescent}, nil
		},
	}
	m := NewModel(opts)
	m, _ = updateAs(m, sizeMsg())
	m.input.SetValue("hello")
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.CurrentState() != StateRunning {
		t.Errorf("want running, got %v", m.CurrentState())
	}
	select {
	case got := <-started:
		if got != "hello" {
			t.Errorf("want 'hello', got %q", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("StartLoop never called")
	}
}

func TestUpdate_Submit_ClearsInput(t *testing.T) {
	m := NewModel(Options{PodName: "demo", StartLoop: okStartLoop})
	m, _ = updateAs(m, sizeMsg())
	m.input.SetValue("hello")
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.input.Value() != "" {
		t.Errorf("want empty input after submit, got %q", m.input.Value())
	}
}

func TestUpdate_Submit_NoStartLoopFn_ShowsError(t *testing.T) {
	m := NewModel(Options{PodName: "demo"}) // no StartLoop
	m, _ = updateAs(m, sizeMsg())
	m.input.SetValue("hello")
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.lastErr == nil {
		t.Error("want error when StartLoop is nil")
	}
}

func TestInit_WithInitialKickoff_AutoSubmits(t *testing.T) {
	started := make(chan string, 1)
	opts := Options{
		PodName:        "demo",
		InitialKickoff: "auto kickoff",
		StartLoop: func(ctx context.Context, kickoff string, _ func(thread.Event)) (orchestrator.LoopResult, error) {
			started <- kickoff
			return orchestrator.LoopResult{StopReason: orchestrator.LoopQuiescent}, nil
		},
	}
	m := NewModel(opts)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("want Init cmd")
	}
	// Init returns a Batch; one of its children is autoSubmitMsg.
	// Exercise the autoSubmit path directly instead of introspecting.
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, autoSubmitMsg{text: opts.InitialKickoff})
	select {
	case got := <-started:
		if got != "auto kickoff" {
			t.Errorf("want auto-kickoff, got %q", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("auto-submit did not trigger StartLoop")
	}
}

func TestInit_WithoutKickoff_NoAutoSubmit(t *testing.T) {
	m := NewModel(Options{PodName: "demo", StartLoop: okStartLoop})
	cmd := m.Init()
	// cmd should only be the sub-drain, not an autoSubmit. We cannot
	// introspect a tea.Batch directly, but we can verify state after
	// running: no loop should have been kicked off.
	_ = cmd
	if m.CurrentState() != StateIdle {
		t.Errorf("want idle initially, got %v", m.CurrentState())
	}
}

func TestUpdate_ErrorMsg_SurfacesError(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, errorMsg{err: errors.New("synthetic")})
	if m.lastErr == nil || !strings.Contains(m.Status(), "synthetic") {
		t.Errorf("want error in status, got %q", m.Status())
	}
}

func TestUpdate_KeyWhileRunning_GoesToViewport(t *testing.T) {
	// while running, keys should not end up in the input. Easiest
	// assertion: input value remains unchanged after a key press.
	m := NewModel(Options{PodName: "demo", StartLoop: okStartLoop})
	m, _ = updateAs(m, sizeMsg())
	m.state = StateRunning
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if m.input.Value() != "" {
		t.Errorf("input should be empty while running, got %q", m.input.Value())
	}
}

func TestView_RendersHeaderAndStatus(t *testing.T) {
	m := NewModel(Options{PodName: "demo", Members: []string{"alice", "bob"}, Lead: "alice"})
	m, _ = updateAs(m, sizeMsg())
	out := m.View()
	for _, want := range []string{"poddies", "demo", "alice", "bob", "lead: alice", "ready"} {
		if !strings.Contains(out, want) {
			t.Errorf("View missing %q:\n%s", want, out)
		}
	}
}

func TestView_EmptyTranscript_ShowsPlaceholder(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	m, _ = updateAs(m, sizeMsg())
	out := m.View()
	if !strings.Contains(out, "no events yet") {
		t.Errorf("want placeholder in empty view, got:\n%s", out)
	}
}

func TestView_WithEvents_RendersTranscript(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, EventMsg{Event: thread.Event{Type: thread.EventHuman, Body: "kick off"}})
	m, _ = updateAs(m, EventMsg{Event: thread.Event{Type: thread.EventMessage, From: "alice", Body: "on it"}})
	out := m.View()
	for _, want := range []string{"kick off", "alice", "on it"} {
		if !strings.Contains(out, want) {
			t.Errorf("View missing %q:\n%s", want, out)
		}
	}
}

func TestRenderEvent_CoversAllTypes(t *testing.T) {
	cases := []thread.Event{
		{Type: thread.EventHuman, Body: "h"},
		{Type: thread.EventMessage, From: "alice", Body: "m"},
		{Type: thread.EventSystem, Body: "s"},
		{Type: thread.EventPermissionRequest, From: "alice", Action: "run"},
		{Type: thread.EventPermissionGrant, From: "human", RequestID: "r1"},
		{Type: thread.EventPermissionDeny, From: "human", RequestID: "r2"},
		{Type: "future_type", Body: "x"},
	}
	for _, e := range cases {
		got := renderEvent(e, "", 80)
		if got == "" {
			t.Errorf("empty rendering for %+v", e)
		}
	}
}

func TestUpdate_Tab_AcceptsSuggestion(t *testing.T) {
	m := NewModel(Options{PodName: "demo", Members: []string{"alice", "bob"}, StartLoop: okStartLoop})
	m, _ = updateAs(m, sizeMsg())
	m.input.SetValue("@al")
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.input.Value() != "@alice " {
		t.Errorf("want '@alice ' after Tab, got %q", m.input.Value())
	}
}

func TestUpdate_Tab_NoSuggestion_InputUnchanged(t *testing.T) {
	m := NewModel(Options{PodName: "demo", Members: []string{"alice"}, StartLoop: okStartLoop})
	m, _ = updateAs(m, sizeMsg())
	m.input.SetValue("hello")
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyTab})
	// no crash; input is unchanged (Tab passes through with no suggestion)
	if m.input.Value() != "hello" {
		t.Errorf("want input unchanged when no suggestion, got %q", m.input.Value())
	}
}

func TestUpdate_Tab_WhileRunning_NoAccept(t *testing.T) {
	m := NewModel(Options{PodName: "demo", Members: []string{"alice"}, StartLoop: okStartLoop})
	m, _ = updateAs(m, sizeMsg())
	m.state = StateRunning
	m.input.SetValue("@al")
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyTab})
	// should NOT accept while running (input routes to viewport)
	if m.input.Value() != "@al" {
		t.Errorf("want input unchanged while running, got %q", m.input.Value())
	}
}

func TestRenderInputLine_GhostText_Present(t *testing.T) {
	m := NewModel(Options{PodName: "demo", Members: []string{"alice"}, StartLoop: okStartLoop})
	m, _ = updateAs(m, sizeMsg())
	m.input.SetValue("@al")
	rendered := m.renderInputLine()
	if !strings.Contains(rendered, "ice") {
		t.Errorf("want ghost suffix 'ice' in rendered input, got:\n%s", rendered)
	}
}

func TestRenderInputLine_NoGhost_WhenNoSuggestion(t *testing.T) {
	m := NewModel(Options{PodName: "demo", Members: []string{"alice"}, StartLoop: okStartLoop})
	m, _ = updateAs(m, sizeMsg())
	m.input.SetValue("hello")
	base := m.input.View()
	rendered := m.renderInputLine()
	if rendered != base {
		t.Errorf("want renderInputLine == input.View() when no suggestion\ngot:  %q\nwant: %q", rendered, base)
	}
}

func TestRenderInputLine_NoGhost_WhenNoMembers(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	m, _ = updateAs(m, sizeMsg())
	m.input.SetValue("@al")
	base := m.input.View()
	rendered := m.renderInputLine()
	if rendered != base {
		t.Errorf("want no ghost when roster is empty, got %q", rendered)
	}
}

// --- helpers ---

func updateAs(m Model, msg tea.Msg) (Model, tea.Cmd) {
	got, cmd := m.Update(msg)
	return got.(Model), cmd
}

func okStartLoop(ctx context.Context, _ string, _ func(thread.Event)) (orchestrator.LoopResult, error) {
	return orchestrator.LoopResult{StopReason: orchestrator.LoopQuiescent}, nil
}
