package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/andrewwormald/poddies/internal/thread"
)

func vizModel() Model {
	m := NewModel(Options{
		PodName:   "demo",
		StartLoop: okStartLoop,
		Members:   []string{"alice", "bob", "charlie"},
	})
	m, _ = updateAs(m, sizeMsg())
	return m
}

// --- toggle ---

func TestViz_Toggle_V_OpensPanel(t *testing.T) {
	m := vizModel()
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if !m.vizOpen {
		t.Error("pressing v should open the viz panel")
	}
}

func TestViz_Toggle_V_Twice_Closes(t *testing.T) {
	m := vizModel()
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if m.vizOpen {
		t.Error("pressing v twice should close the viz panel")
	}
}

func TestViz_Toggle_ViewportNarrows(t *testing.T) {
	m := vizModel()
	fullW := m.viewport.Width
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if m.viewport.Width >= fullW {
		t.Errorf("viewport should narrow when viz opens: was %d, got %d", fullW, m.viewport.Width)
	}
	if m.viewport.Width != fullW-vizPanelW-1 {
		t.Errorf("want %d, got %d", fullW-vizPanelW-1, m.viewport.Width)
	}
}

func TestViz_Toggle_ViewportRestoresOnClose(t *testing.T) {
	m := vizModel()
	fullW := m.viewport.Width
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if m.viewport.Width != fullW {
		t.Errorf("viewport should restore to %d on close, got %d", fullW, m.viewport.Width)
	}
}

// --- anim tick ---

func TestViz_AnimTick_PrunesExpiredLinks(t *testing.T) {
	m := vizModel()
	m.vizOpen = true
	m.activeLinks = []vizLink{
		{from: "alice", to: "", startAt: time.Now().Add(-2 * linkDur)}, // expired
		{from: "bob", to: "", startAt: time.Now()},                     // live
	}
	m, _ = updateAs(m, animTickMsg{t: time.Now()})
	if len(m.activeLinks) != 1 {
		t.Errorf("want 1 live link after prune, got %d", len(m.activeLinks))
	}
	if m.activeLinks[0].from != "bob" {
		t.Errorf("expected bob's link to survive, got %+v", m.activeLinks)
	}
}

func TestViz_AnimTick_StopsWhenPanelClosed(t *testing.T) {
	m := vizModel()
	m.vizOpen = false
	m.activeLinks = []vizLink{
		{from: "alice", to: "", startAt: time.Now()},
	}
	_, cmd := updateAs(m, animTickMsg{t: time.Now()})
	// When closed, no follow-up tick cmd should be returned.
	if cmd != nil {
		t.Error("animTick should not re-arm when viz panel is closed")
	}
}

// --- link creation ---

func TestViz_OnEvent_MessageCreatesLink(t *testing.T) {
	m := vizModel()
	m.lastSpeaker = "bob"
	m, _ = updateAs(m, EventMsg{Event: thread.Event{
		Type: thread.EventMessage, From: "alice", Body: "hi",
	}})
	if len(m.activeLinks) == 0 {
		t.Fatal("expected a viz link after EventMessage")
	}
	l := m.activeLinks[0]
	if l.from != "alice" || l.to != "bob" {
		t.Errorf("want from=alice to=bob, got from=%s to=%s", l.from, l.to)
	}
}

func TestViz_OnEvent_HumanCreatesLink(t *testing.T) {
	m := vizModel()
	m.lastSpeaker = "charlie"
	m, _ = updateAs(m, EventMsg{Event: thread.Event{
		Type: thread.EventHuman, Body: "hello",
	}})
	if len(m.activeLinks) == 0 {
		t.Fatal("expected a viz link after EventHuman")
	}
	l := m.activeLinks[0]
	if l.from != "" || l.to != "charlie" {
		t.Errorf("want from='' to=charlie, got from=%q to=%q", l.from, l.to)
	}
}

func TestViz_OnEvent_NoSelfLink(t *testing.T) {
	m := vizModel()
	m.lastSpeaker = "alice"
	m, _ = updateAs(m, EventMsg{Event: thread.Event{
		Type: thread.EventMessage, From: "alice", Body: "hi",
	}})
	for _, l := range m.activeLinks {
		if l.from == l.to {
			t.Errorf("self-link created: %+v", l)
		}
	}
}

// --- rendering ---

func TestViz_RenderPanel_ShowsAllMembers(t *testing.T) {
	m := vizModel()
	panel := m.renderVizPanel(20)
	for _, name := range []string{"alice", "bob", "charlie", "you"} {
		if !strings.Contains(panel, name) {
			t.Errorf("viz panel should contain %q:\n%s", name, panel)
		}
	}
}

func TestViz_RenderPanel_ActiveNodeHighlighted(t *testing.T) {
	m := vizModel()
	m.activeLinks = []vizLink{
		{from: "alice", to: "bob", startAt: time.Now()},
	}
	panel := m.renderVizPanel(20)
	// Panel should contain ◉ (active node marker) for alice or bob.
	if !strings.Contains(panel, "◉") {
		t.Errorf("active node should render ◉, panel:\n%s", panel)
	}
}

func TestViz_View_SplitsLayoutWhenOpen(t *testing.T) {
	m := vizModel()
	m, _ = updateAs(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	view := m.View()
	// Both transcript area and viz panel must appear in the view.
	if !strings.Contains(view, "pod") {
		t.Errorf("viz header 'pod' should appear when panel is open:\n%s", view)
	}
	if !strings.Contains(view, "alice") {
		t.Errorf("member names should appear in viz panel:\n%s", view)
	}
}
