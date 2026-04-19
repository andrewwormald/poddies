package cli

import (
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/thread"
)

func TestDumpPrompt_Claude(t *testing.T) {
	cwd, root, _, _ := setupPodWithMember(t)
	log := thread.Open(ThreadPath(root, "demo", DefaultThreadName))
	_ = log.EnsureFile()
	_, _ = log.Append(thread.Event{Type: thread.EventHuman, Body: "hi there"})

	// Flip alice's adapter to claude so dumpPrompt hits that branch.
	patch := AdapterPatch("claude")
	_, err := EditMember(root, "demo", "alice", MemberPatch{Adapter: &patch})
	if err != nil {
		t.Fatal(err)
	}

	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "run", "--pod", "demo", "--member", "alice", "--dump-prompt"); err != nil {
		t.Fatalf("run --dump-prompt: %v", err)
	}
	for _, want := range []string{"dump-prompt", "--- SYSTEM ---", "--- USER ---", "You are", "hi there", "Be concise"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("dump output missing %q:\n%s", want, out.String())
		}
	}
}

func TestDumpPrompt_Gemini(t *testing.T) {
	cwd, root, _, _ := setupPodWithMember(t)
	patch := AdapterPatch("gemini")
	model := "gemini-2.5-flash"
	if _, err := EditMember(root, "demo", "alice", MemberPatch{Adapter: &patch, Model: &model}); err != nil {
		t.Fatal(err)
	}

	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "run", "--pod", "demo", "--member", "alice", "--dump-prompt"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"dump-prompt", "---- SYSTEM ----", "---- YOUR TURN ----", "You are"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("dump output missing %q:\n%s", want, out.String())
		}
	}
}

func TestDumpPrompt_Mock_FriendlyMessage(t *testing.T) {
	cwd, _, _, _ := setupPodWithMember(t)
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "run", "--pod", "demo", "--member", "alice", "--dump-prompt"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "mock adapter has no prompt") {
		t.Errorf("want mock note, got:\n%s", out.String())
	}
}

func TestDumpPrompt_DefaultsToFirstMember(t *testing.T) {
	cwd, _, _, _ := setupPodWithMember(t)
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "run", "--pod", "demo", "--dump-prompt"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "member=alice") {
		t.Errorf("want default-picked alice, got:\n%s", out.String())
	}
}
