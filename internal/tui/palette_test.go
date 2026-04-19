package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// palettePress types each rune in s as a KeyRunes message to the Model.
func palettePress(m Model, s string) Model {
	for _, r := range s {
		m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func TestPalette_ColonOpensPalette(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	if m.state != StatePalette {
		t.Errorf("want StatePalette after ':', got %v", m.state)
	}
}

func TestPalette_TypingAppendsToBuffer(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	m = palettePress(m, "pods")
	if m.paletteInput != "pods" {
		t.Errorf("want 'pods' in buffer, got %q", m.paletteInput)
	}
}

func TestPalette_BackspaceErases(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	m = palettePress(m, "pods")
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.paletteInput != "pod" {
		t.Errorf("want 'pod' after backspace, got %q", m.paletteInput)
	}
}

func TestPalette_EscCancels(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	m = palettePress(m, "pods")
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.state == StatePalette {
		t.Error("esc should leave palette")
	}
	if m.paletteInput != "" {
		t.Error("buffer should be cleared on esc")
	}
}

func TestPalette_EnterSwitchesView(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	m = palettePress(m, "pods")
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.ActiveView() != ViewPods {
		t.Errorf("want ViewPods, got %v", m.ActiveView())
	}
	if m.state == StatePalette {
		t.Error("palette should close after submit")
	}
}

func TestPalette_UnknownCommand_Status(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	m = palettePress(m, "nope")
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.Status(), "unknown command") {
		t.Errorf("want 'unknown command' in status, got %q", m.Status())
	}
}

func TestPalette_QuitExits(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	m = palettePress(m, "quit")
	m, cmd := updateAs(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != StateQuit {
		t.Errorf("want StateQuit, got %v", m.state)
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestPalette_HelpOpensHelpView(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	m = palettePress(m, "help")
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.ActiveView() != ViewHelp {
		t.Errorf("want ViewHelp, got %v", m.ActiveView())
	}
}

func TestEscFromNonThreadView_ReturnsToThread(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	m, _ = updateAs(m, sizeMsg())
	m.view = ViewPods
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.ActiveView() != ViewThread {
		t.Errorf("want ViewThread after Esc, got %v", m.ActiveView())
	}
}

func TestQuestionMark_OpensHelpFromIdle(t *testing.T) {
	m := NewModel(Options{PodName: "demo"})
	m, _ = updateAs(m, sizeMsg())
	// move to a non-thread view first so '?' isn't interpreted as input
	m.view = ViewPods
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if m.ActiveView() != ViewHelp {
		t.Errorf("want ViewHelp after '?', got %v", m.ActiveView())
	}
}

func TestView_Members_RendersRoster(t *testing.T) {
	m := NewModel(Options{
		PodName:       "demo",
		OnListMembers: func() []string { return []string{"alice", "bob"} },
	})
	m, _ = updateAs(m, sizeMsg())
	m.view = ViewMembers
	out := m.View()
	for _, want := range []string{":members", "alice", "bob"} {
		if !strings.Contains(out, want) {
			t.Errorf("members view missing %q:\n%s", want, out)
		}
	}
}

func TestView_Pods_RendersList(t *testing.T) {
	m := NewModel(Options{
		PodName:    "demo",
		OnListPods: func() []string { return []string{"demo", "other"} },
	})
	m, _ = updateAs(m, sizeMsg())
	m.view = ViewPods
	out := m.View()
	if !strings.Contains(out, "demo") || !strings.Contains(out, "other") {
		t.Errorf("pods view missing entries:\n%s", out)
	}
}

func TestView_Doctor_RendersChecks(t *testing.T) {
	m := NewModel(Options{
		PodName: "demo",
		OnDoctor: func() []DoctorCheck {
			return []DoctorCheck{
				{Name: "claude CLI", Status: "pass", Message: "/usr/local/bin/claude"},
				{Name: "gemini CLI", Status: "warn", Message: "not installed"},
			}
		},
	})
	m, _ = updateAs(m, sizeMsg())
	m.view = ViewDoctor
	out := m.View()
	for _, want := range []string{"claude CLI", "gemini CLI", "pass", "warn"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor view missing %q:\n%s", want, out)
		}
	}
}
