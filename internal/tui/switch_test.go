package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// podsModel returns a Model wired with two pods and OnSwitchPod.
func podsModel(switchedTo *string) Model {
	m := NewModel(Options{
		PodName:   "alpha",
		StartLoop: okStartLoop,
		OnListPods: func() []string { return []string{"alpha", "beta", "gamma"} },
		OnSwitchPod: func(name string) {
			if switchedTo != nil {
				*switchedTo = name
			}
		},
		OnListThreads: func() []ThreadSummary {
			return []ThreadSummary{
				{Name: "sess-001", Events: 5, LastFrom: "alice"},
				{Name: "sess-002", Events: 2, LastFrom: "bob"},
			}
		},
		OnResumeSession: func(id string) {},
		OnListSessions:  func() []SessionSummary { return nil },
	})
	m, _ = updateAs(m, sizeMsg())
	return m
}

// switchToPods navigates to the :pods view.
func switchToPods(m Model) Model {
	m.view = ViewPods
	m.cursorPos = 0
	return m
}

// switchToThreads navigates to the :threads view.
func switchToThreads(m Model) Model {
	m.view = ViewThreads
	m.cursorPos = 0
	return m
}

func TestPodsView_CursorMovesDown(t *testing.T) {
	m := switchToPods(podsModel(nil))
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cursorPos != 1 {
		t.Errorf("want cursorPos=1 after down, got %d", m.cursorPos)
	}
}

func TestPodsView_CursorMovesUp(t *testing.T) {
	m := switchToPods(podsModel(nil))
	m.cursorPos = 2
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.cursorPos != 1 {
		t.Errorf("want cursorPos=1 after up, got %d", m.cursorPos)
	}
}

func TestPodsView_CursorDoesNotGoNegative(t *testing.T) {
	m := switchToPods(podsModel(nil))
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.cursorPos != 0 {
		t.Errorf("want cursorPos=0 at top, got %d", m.cursorPos)
	}
}

func TestPodsView_EnterOnCurrentPod_StaysInView(t *testing.T) {
	// cursor on "alpha" (index 0) which is the current pod → no switch
	m := switchToPods(podsModel(nil))
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.CurrentState() == StateQuit {
		t.Error("should not quit when selecting the current pod")
	}
}

func TestPodsView_EnterOnOtherPod_InvokesSwitchAndQuits(t *testing.T) {
	var switched string
	m := switchToPods(podsModel(&switched))
	m.cursorPos = 1 // "beta"
	m, cmd := updateAs(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.CurrentState() != StateQuit {
		t.Error("want StateQuit after switching pod")
	}
	if cmd == nil {
		t.Error("want tea.Quit cmd")
	}
	if switched != "beta" {
		t.Errorf("want OnSwitchPod called with 'beta', got %q", switched)
	}
	if m.SwitchPodTarget() != "beta" {
		t.Errorf("want SwitchPodTarget='beta', got %q", m.SwitchPodTarget())
	}
}

func TestPodsView_NotWired_StatusError(t *testing.T) {
	m := NewModel(Options{PodName: "demo", StartLoop: okStartLoop})
	m, _ = updateAs(m, sizeMsg())
	m.view = ViewPods
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.CurrentState() == StateQuit {
		t.Error("should not quit when OnListPods is nil")
	}
}

func TestPodsView_PaletteEntry_ResetsCursor(t *testing.T) {
	m := podsModel(nil)
	m.cursorPos = 99
	// simulate palette entry by going through applyPalette path
	m.state = StatePalette
	m.paletteInput = "pods"
	got, _ := m.applyPalette()
	mm := got.(Model)
	if mm.cursorPos != 0 {
		t.Errorf("want cursorPos=0 after palette switch, got %d", mm.cursorPos)
	}
}

func TestThreadsView_EnterResumesThread(t *testing.T) {
	var resumed string
	m := NewModel(Options{
		PodName:   "demo",
		StartLoop: okStartLoop,
		OnListThreads: func() []ThreadSummary {
			return []ThreadSummary{
				{Name: "sess-001", Events: 5},
				{Name: "sess-002", Events: 2},
			}
		},
		OnResumeSession: func(id string) { resumed = id },
		OnListSessions:  func() []SessionSummary { return nil },
	})
	m, _ = updateAs(m, sizeMsg())
	m = switchToThreads(m)
	m.cursorPos = 1 // "sess-002"
	m, cmd := updateAs(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.CurrentState() != StateQuit {
		t.Error("want StateQuit after thread selection")
	}
	if cmd == nil {
		t.Error("want tea.Quit cmd")
	}
	if resumed != "sess-002" {
		t.Errorf("want OnResumeSession('sess-002'), got %q", resumed)
	}
}

func TestThreadsView_CursorMovement(t *testing.T) {
	m := NewModel(Options{
		PodName:   "demo",
		StartLoop: okStartLoop,
		OnListThreads: func() []ThreadSummary {
			return []ThreadSummary{{Name: "a"}, {Name: "b"}}
		},
		OnResumeSession: func(string) {},
		OnListSessions:  func() []SessionSummary { return nil },
	})
	m, _ = updateAs(m, sizeMsg())
	m.view = ViewThreads
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cursorPos != 1 {
		t.Errorf("want cursorPos=1, got %d", m.cursorPos)
	}
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.cursorPos != 0 {
		t.Errorf("want cursorPos=0 after up, got %d", m.cursorPos)
	}
}

func TestRenderPodsView_ShowsCursor(t *testing.T) {
	m := switchToPods(podsModel(nil))
	m.cursorPos = 1
	out := m.View()
	if !strings.Contains(out, "▶") {
		t.Errorf("want cursor indicator ▶ in pods view:\n%s", out)
	}
}

func TestRenderPodsView_ShowsHint(t *testing.T) {
	m := switchToPods(podsModel(nil))
	out := m.View()
	if !strings.Contains(out, "Enter") {
		t.Errorf("want Enter hint in pods view:\n%s", out)
	}
}

func TestRenderThreadsView_ShowsCursorAndHint(t *testing.T) {
	m := switchToThreads(podsModel(nil))
	out := m.View()
	if !strings.Contains(out, "▶") {
		t.Errorf("want cursor indicator ▶ in threads view:\n%s", out)
	}
}
