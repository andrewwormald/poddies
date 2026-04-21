package tui

import (
	"bytes"
	"flag"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/thread"
)

var updateTUIGolden = flag.Bool("update", false, "regenerate TUI golden files in testdata/")

// ansiEscape matches ANSI CSI escape sequences (colours, cursor, erase).
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[mKHJABCDsuhl]`)

// normalizeView strips ANSI codes and trailing whitespace per line so
// golden files are human-readable and not sensitive to colour-profile
// variations across terminals and CI environments.
func normalizeView(s string) string {
	stripped := ansiEscape.ReplaceAllString(s, "")
	lines := strings.Split(stripped, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	// drop trailing blank lines
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n") + "\n"
}

func writeOrCompareGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *updateTUIGolden {
		if err := os.MkdirAll("testdata/golden", 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated %s", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with -update to generate): %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", path, want, got)
	}
}

func TestViewGolden_ThreadIdle(t *testing.T) {
	m := NewModel(Options{
		PodName: "demo",
		Members: []string{"alice", "bob"},
		Lead:    "alice",
		CoSName: "sage",
	})
	m, _ = updateAs(m, sizeMsg())
	m, _ = updateAs(m, EventMsg{Event: thread.Event{
		Type: thread.EventHuman, Body: "kick off the investigation",
	}})
	m, _ = updateAs(m, EventMsg{Event: thread.Event{
		Type: thread.EventMessage, From: "alice", Body: "on it — reviewing now",
	}})
	m.statusLine = "stopped: quiescent (turns=1)"

	got := []byte(normalizeView(m.View()))
	writeOrCompareGolden(t, "testdata/golden/view_thread_idle.txt", got)
}

func TestViewGolden_WizardModal(t *testing.T) {
	w := &Wizard{
		Title: "Add member",
		Steps: []WizardStep{
			{Question: "Name (slug, e.g. alice):"},
			{Question: "Adapter?", Choices: []string{"claude", "gemini", "mock"}, AllowCustom: true},
			{Question: "Persona (optional)", Optional: true},
		},
	}
	m := NewModel(Options{PodName: "demo", Members: []string{"alice"}})
	m, _ = updateAs(m, sizeMsg())
	m = m.activateWizard(w)
	m.statusLine = "welcome — let's add your first member"

	got := []byte(normalizeView(m.View()))
	writeOrCompareGolden(t, "testdata/golden/view_wizard_modal.txt", got)
}

func TestViewGolden_FooterGhostText(t *testing.T) {
	m := NewModel(Options{
		PodName: "demo",
		Members: []string{"alice", "bob"},
	})
	m, _ = updateAs(m, sizeMsg())
	m.input.SetValue("@al")
	m.statusLine = "ready"

	// Capture only the footer portion (last 3 lines of the view) to keep
	// the golden focused on the ghost-text rendering, not the full layout.
	full := normalizeView(m.View())
	lines := strings.Split(strings.TrimRight(full, "\n"), "\n")
	start := len(lines) - 3
	if start < 0 {
		start = 0
	}
	got := []byte(strings.Join(lines[start:], "\n") + "\n")
	writeOrCompareGolden(t, "testdata/golden/view_footer_ghost.txt", got)
}
