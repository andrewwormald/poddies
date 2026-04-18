package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/adapter/mock"
	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

// TestE2E_PermissionFlow exercises: agent emits a permission request →
// loop halts with pending_permission → `thread approve` resolves it →
// `thread resume` picks up with a second scripted turn.
func TestE2E_PermissionFlow(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	runCmd(t, newApp(cwd, home), "init", "--local")
	runCmd(t, newApp(cwd, home), "pod", "create", "demo")
	runCmd(t, newApp(cwd, home), "member", "add",
		"--pod", "demo",
		"--name", "alice",
		"--title", "Staff",
		"--adapter", "mock",
		"--model", "m",
		"--effort", "high",
	)
	root := filepath.Join(cwd, "poddies")
	pdir := filepath.Join(root, "pods", "demo")
	p, err := config.LoadPod(pdir)
	if err != nil {
		t.Fatal(err)
	}
	p.Lead = "alice"
	if err := config.SavePod(pdir, p); err != nil {
		t.Fatal(err)
	}

	// Turn 1: alice produces a permission request. Loop must halt.
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{
			ForMember: "alice",
			Response: adapter.InvokeResponse{
				Body: "I need production access to reproduce",
				PermissionRequests: []adapter.PermissionRequest{
					{Action: "access_prod", Payload: []byte(`{"reason":"repro"}`)},
				},
			},
		},
	))
	app, out := appWithMockAdapter(t, cwd, home, m)
	rootCmd := app.NewRootCmd()
	rootCmd.SetArgs([]string{"run", "--pod", "demo", "--message", "ship the fix"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if !strings.Contains(out.String(), "pending_permission") {
		t.Errorf("want pending_permission stop reason, got:\n%s", out.String())
	}

	// Grab the request id from the persisted log so we can approve it.
	logPath := filepath.Join(pdir, "threads", "default.jsonl")
	events, err := thread.Open(logPath).Load()
	if err != nil {
		t.Fatal(err)
	}
	pending := thread.PendingPermissions(events)
	if len(pending) != 1 {
		t.Fatalf("want 1 pending, got %d", len(pending))
	}
	reqID := pending[0].ID

	// Approve via CLI.
	app2, out2 := appWithMockAdapter(t, cwd, home, mock.New())
	rootCmd2 := app2.NewRootCmd()
	rootCmd2.SetArgs([]string{"thread", "approve", "--pod", "demo", "default", reqID})
	if err := rootCmd2.Execute(); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if !strings.Contains(out2.String(), "granted") {
		t.Errorf("want granted output, got %q", out2.String())
	}

	// Turn 2: resume. Alice now has approval — script a plain response.
	// After the grant, the last conversational event is still alice's
	// "I need production access" message. Without an @mention there,
	// Route halts because lead=alice === the last speaker's identity
	// (no self-routing). So the loop quiesces immediately. That's the
	// expected behavior; the grant is recorded but no additional member
	// turn happens unless the human types a follow-up message.
	m3 := mock.New(mock.WithScript(mock.ScriptedResponse{
		ForMember: "alice",
		Response:  adapter.InvokeResponse{Body: "access confirmed, shipping"},
	}))
	app3, out3 := appWithMockAdapter(t, cwd, home, m3)
	rootCmd3 := app3.NewRootCmd()
	rootCmd3.SetArgs([]string{"thread", "resume", "--pod", "demo",
		"--member", "alice", "--message", "proceed", "default"})
	if err := rootCmd3.Execute(); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !strings.Contains(out3.String(), "access confirmed, shipping") {
		t.Errorf("want alice follow-up, got:\n%s", out3.String())
	}

	// Verify log structure: human, alice-msg, permission_request, grant,
	// human (resume), alice follow-up.
	events, err = thread.Open(logPath).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 6 {
		t.Fatalf("want 6 events, got %d", len(events))
	}
	wantTypes := []thread.EventType{
		thread.EventHuman,
		thread.EventMessage,
		thread.EventPermissionRequest,
		thread.EventPermissionGrant,
		thread.EventHuman,
		thread.EventMessage,
	}
	for i, e := range events {
		if e.Type != wantTypes[i] {
			t.Errorf("event %d: want %s, got %s", i, wantTypes[i], e.Type)
		}
	}
}
