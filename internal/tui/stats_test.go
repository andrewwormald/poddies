package tui

import (
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/thread"
)

func statsModel() Model {
	m := NewModel(Options{
		PodName: "demo",
		Members: []string{"alice", "bob"},
		CoSName: "sage",
		OnUsageSnapshot: func() UsageSnapshot {
			return UsageSnapshot{
				InputTokens:  18_200,
				OutputTokens: 3_400,
				CostUSD:      0.0824,
				TurnCount:    12,
			}
		},
		StartLoop: okStartLoop,
	})
	m, _ = updateAs(m, sizeMsg())
	// Inject some message events so per-member counts can be derived.
	for _, e := range []thread.Event{
		{Type: thread.EventMessage, From: "alice", Body: "a1"},
		{Type: thread.EventMessage, From: "bob", Body: "b1"},
		{Type: thread.EventMessage, From: "alice", Body: "a2"},
		{Type: thread.EventMessage, From: "sage", Body: "s1"},
		{Type: thread.EventHuman, Body: "h1"},
	} {
		m, _ = updateAs(m, EventMsg{Event: e})
	}
	return m
}

func TestSlash_Stats_SwitchesToStatsView(t *testing.T) {
	m, _ := submitString(statsModel(), "/stats")
	if m.ActiveView() != ViewStats {
		t.Errorf("want ViewStats, got %v", m.ActiveView())
	}
}

func TestRenderStatsView_ShowsTotals(t *testing.T) {
	m, _ := submitString(statsModel(), "/stats")
	out := m.View()
	for _, want := range []string{"18", "3", "0.0824", "12"} {
		if !strings.Contains(out, want) {
			t.Errorf("stats view missing %q:\n%s", want, out)
		}
	}
}

func TestRenderStatsView_ShowsMemberCounts(t *testing.T) {
	m, _ := submitString(statsModel(), "/stats")
	out := m.View()
	for _, want := range []string{"alice", "bob", "sage"} {
		if !strings.Contains(out, want) {
			t.Errorf("stats view missing member %q:\n%s", want, out)
		}
	}
}

func TestRenderStatsView_NotWired_ShowsNote(t *testing.T) {
	m := NewModel(Options{PodName: "demo", StartLoop: okStartLoop}) // no OnUsageSnapshot
	m, _ = updateAs(m, sizeMsg())
	m, _ = submitString(m, "/stats")
	out := m.View()
	if !strings.Contains(out, "stats") {
		t.Errorf("want stats-related content even without wired callback:\n%s", out)
	}
}

func TestRenderStatsView_HumanMessagesCounted(t *testing.T) {
	m, _ := submitString(statsModel(), "/stats")
	out := m.View()
	if !strings.Contains(out, "me") {
		t.Errorf("stats view should show 'me' message count:\n%s", out)
	}
}
