package tui

import (
	"testing"
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
