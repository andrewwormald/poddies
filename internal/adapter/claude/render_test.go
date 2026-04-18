package claude

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

var updateGolden = flag.Bool("update", false, "regenerate golden files in testdata/")

func alice() config.Member {
	return config.Member{
		Name:    "alice",
		Title:   "Staff Engineer",
		Adapter: config.AdapterClaude,
		Model:   "claude-opus-4-7",
		Effort:  config.EffortHigh,
		Persona: "Pragmatic, terse.",
	}
}

func bob() config.Member {
	return config.Member{
		Name:    "bob",
		Title:   "Senior Engineer",
		Adapter: config.AdapterClaude,
		Model:   "claude-sonnet-4-6",
		Effort:  config.EffortMedium,
	}
}

func demoPod() config.Pod {
	return config.Pod{Name: "demo", Lead: "human"}
}

func TestRenderSystemPrompt_IncludesCoreFields(t *testing.T) {
	got := RenderSystemPrompt(alice(), demoPod(), []config.Member{alice(), bob()})
	for _, want := range []string{"alice", "Staff Engineer", "demo", "Pragmatic, terse", "alice: Staff Engineer (you)", "bob: Senior Engineer", "Lead: human"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderSystemPrompt_EmptyRoster_OmitsSection(t *testing.T) {
	got := RenderSystemPrompt(alice(), demoPod(), nil)
	if strings.Contains(got, "Pod members:") {
		t.Errorf("should omit member list when roster is empty:\n%s", got)
	}
}

func TestRenderSystemPrompt_EmptyPersona_NoPersonaLine(t *testing.T) {
	m := alice()
	m.Persona = ""
	got := RenderSystemPrompt(m, demoPod(), nil)
	if strings.Contains(got, "Persona:") {
		t.Errorf("should omit persona when empty, got:\n%s", got)
	}
}

func TestRenderSystemPrompt_SystemPromptExtra_Appended(t *testing.T) {
	m := alice()
	m.SystemPromptExtra = "Prefer short answers."
	got := RenderSystemPrompt(m, demoPod(), nil)
	if !strings.Contains(got, "Prefer short answers.") {
		t.Errorf("system_prompt_extra missing:\n%s", got)
	}
}

func TestRenderUserPrompt_EmptyThread(t *testing.T) {
	got := RenderUserPrompt(alice(), nil)
	if !strings.Contains(got, "thread is empty") {
		t.Errorf("want empty-thread note, got:\n%s", got)
	}
	if !strings.Contains(got, "You are alice") {
		t.Errorf("want CTA for alice, got:\n%s", got)
	}
}

func TestRenderUserPrompt_RendersAllEventTypes(t *testing.T) {
	ts := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	events := []thread.Event{
		{Type: thread.EventHuman, From: "human", Body: "kick off", TS: ts},
		{Type: thread.EventMessage, From: "alice", Body: "hi @bob", TS: ts},
		{Type: thread.EventSystem, Body: "routed to alice", TS: ts},
		{Type: thread.EventPermissionRequest, From: "alice", Action: "run_shell", TS: ts},
		{Type: thread.EventPermissionGrant, From: "human", RequestID: "r1", TS: ts},
		{Type: thread.EventPermissionDeny, From: "human", RequestID: "r2", TS: ts},
		{Type: "future_type", Body: "unknown payload", TS: ts},
	}
	got := RenderUserPrompt(alice(), events)
	for _, want := range []string{
		"[human] kick off",
		"[alice] hi @bob",
		"[system] routed to alice",
		"[permission_request from alice]",
		"[permission_grant by human for r1]",
		"[permission_deny by human for r2]",
		"[unknown:future_type]",
		"You are alice",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderUserPrompt_MessageWithoutFrom_RendersPlaceholder(t *testing.T) {
	got := RenderUserPrompt(alice(), []thread.Event{{Type: thread.EventMessage, Body: "mystery"}})
	if !strings.Contains(got, "[?]") {
		t.Errorf("expected [?] placeholder, got:\n%s", got)
	}
}

// --- golden ---

func TestRender_Golden(t *testing.T) {
	ts := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	sys := RenderSystemPrompt(alice(), demoPod(), []config.Member{alice(), bob()})
	usr := RenderUserPrompt(alice(), []thread.Event{
		{Type: thread.EventHuman, From: "human", Body: "kick off the investigation", TS: ts},
		{Type: thread.EventMessage, From: "bob", Body: "@alice can you take the first look?", TS: ts},
	})
	got := []byte("=== SYSTEM ===\n" + sys + "\n=== USER ===\n" + usr)

	path := "testdata/golden/render_full.txt"
	if *updateGolden {
		if err := os.MkdirAll("testdata/golden", 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run -update first): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("render golden mismatch\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}
