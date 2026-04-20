package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/andrewwormald/poddies/internal/thread"
)

func TestNeedsOnboarding_NoCallback_False(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	if m.needsOnboarding() {
		t.Error("without OnAddMember, onboarding cannot be run")
	}
}

func TestNeedsOnboarding_EmptyListMembers_True(t *testing.T) {
	m := NewModel(Options{
		PodName:       "demo",
		OnAddMember:   func(AddMemberSpec) error { return nil },
		OnListMembers: func() []string { return nil },
	})
	if !m.needsOnboarding() {
		t.Error("empty roster should trigger onboarding")
	}
}

func TestNeedsOnboarding_PopulatedListMembers_False(t *testing.T) {
	m := NewModel(Options{
		PodName:       "demo",
		OnAddMember:   func(AddMemberSpec) error { return nil },
		OnListMembers: func() []string { return []string{"alice"} },
	})
	if m.needsOnboarding() {
		t.Error("populated roster should skip onboarding")
	}
}

func TestNeedsOnboarding_FallsBackToMembers(t *testing.T) {
	mEmpty := NewModel(Options{
		PodName:     "demo",
		OnAddMember: func(AddMemberSpec) error { return nil },
	})
	if !mEmpty.needsOnboarding() {
		t.Error("empty Members slice should trigger onboarding")
	}
	mWith := NewModel(Options{
		PodName:     "demo",
		OnAddMember: func(AddMemberSpec) error { return nil },
		Members:     []string{"alice"},
	})
	if mWith.needsOnboarding() {
		t.Error("populated Members slice should skip onboarding")
	}
}

func TestOnboarding_StartMessage_ActivatesAddMemberWizard(t *testing.T) {
	m := NewModel(Options{
		PodName:     "demo",
		StartLoop:   okStartLoop,
		OnAddMember: func(AddMemberSpec) error { return nil },
	})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, startOnboardingMsg{})
	if m.CurrentState() != StatePrompting {
		t.Errorf("want StatePrompting, got %v", m.CurrentState())
	}
	if m.ActiveWizard() == nil {
		t.Fatal("wizard should be active")
	}
	if m.ActiveWizard().Title != "onboarding · add member" {
		t.Errorf("want onboarding title, got %s", m.ActiveWizard().Title)
	}
}

func TestOnboarding_Wizard_HasPreamble(t *testing.T) {
	m := NewModel(Options{
		PodName:     "demo",
		StartLoop:   okStartLoop,
		OnAddMember: func(AddMemberSpec) error { return nil },
	})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, startOnboardingMsg{})
	w := m.ActiveWizard()
	if w == nil {
		t.Fatal("wizard should be active")
	}
	if w.Preamble == "" {
		t.Error("onboarding wizard should have a preamble")
	}
	if !strings.Contains(w.Preamble, "pod") {
		t.Errorf("preamble should mention pod, got %q", w.Preamble)
	}
}

func TestOnboarding_WizardModal_RendersPreamble(t *testing.T) {
	m := NewModel(Options{
		PodName:     "demo",
		StartLoop:   okStartLoop,
		OnAddMember: func(AddMemberSpec) error { return nil },
	})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, startOnboardingMsg{})
	if m.CurrentState() != StatePrompting {
		t.Skip("not in wizard state")
	}
	view := m.View()
	if !strings.Contains(view, "Welcome") {
		t.Errorf("modal should render preamble with 'Welcome', got:\n%s", view)
	}
}

func TestOnboarding_Complete_AppendsGuideEvent(t *testing.T) {
	var added AddMemberSpec
	m := NewModel(Options{
		PodName:   "demo",
		StartLoop: okStartLoop,
		OnAddMember: func(s AddMemberSpec) error {
			added = s
			return nil
		},
		OnListMembers: func() []string { return []string{"alice"} },
	})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, startOnboardingMsg{})

	// Advance through all wizard steps.
	answers := []string{"alice", "Engineer", "1", "2", "2", ""}
	for _, a := range answers {
		m.input.SetValue(a)
		m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyEnter})
	}
	_ = added

	// A guide event should have been appended.
	found := false
	for _, e := range m.Events() {
		if e.Type == thread.EventSystem && strings.Contains(e.Body, "Type a message") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("guide system event not found after onboarding; events: %v", m.Events())
	}
}

func TestThreadView_EmptyWithMembers_ShowsHint(t *testing.T) {
	m := NewModel(Options{
		PodName:   "demo",
		StartLoop: okStartLoop,
		Members:   []string{"alice"},
	})
	m, _ = updateAs(m, sizeMsg())
	view := m.View()
	if !strings.Contains(view, "Type a message") {
		t.Errorf("empty thread with members should show hint; view:\n%s", view)
	}
}

func TestThreadView_EmptyNoMembers_NoHint(t *testing.T) {
	m := NewModel(Options{
		PodName:   "demo",
		StartLoop: okStartLoop,
	})
	m, _ = updateAs(m, sizeMsg())
	view := m.View()
	if strings.Contains(view, "Type a message") {
		t.Errorf("empty thread without members should NOT show type-hint; view:\n%s", view)
	}
}
