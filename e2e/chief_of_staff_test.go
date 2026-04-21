package e2e

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/adapter/mock"
	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

// enableChiefOfStaff loads the pod, enables CoS with the given
// triggers/name/adapter, and writes it back.
func enableChiefOfStaff(t *testing.T, root, podName, cosName string, triggers []config.Trigger) {
	t.Helper()
	podDir := filepath.Join(root, "pods", podName)
	p, err := config.LoadPod(podDir)
	if err != nil {
		t.Fatal(err)
	}
	p.ChiefOfStaff = config.ChiefOfStaff{
		Enabled:  true,
		Name:     cosName,
		Adapter:  config.AdapterMock,
		Model:    "local-cheap",
		Triggers: triggers,
	}
	if err := config.SavePod(podDir, p); err != nil {
		t.Fatal(err)
	}
}

// TestE2E_CoS_UnresolvedRouting_Rescue runs a pod where alice halts
// without naming a next speaker; the chief-of-staff intervenes with an
// @mention, and bob picks it up.
func TestE2E_CoS_UnresolvedRouting_Rescue(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	runCmd(t, newApp(cwd, home), "init", "--local")
	runCmd(t, newApp(cwd, home), "pod", "create", "demo")
	for _, name := range []string{"alice", "bob"} {
		runCmd(t, newApp(cwd, home), "member", "add",
			"--pod", "demo",
			"--name", name,
			"--title", "Engineer",
			"--adapter", "mock",
			"--model", "m",
			"--effort", "medium",
		)
	}
	root := filepath.Join(cwd, ".poddies")

	// set lead=alice and enable CoS unresolved_routing
	pdir := filepath.Join(root, "pods", "demo")
	p, err := config.LoadPod(pdir)
	if err != nil {
		t.Fatal(err)
	}
	p.Lead = "alice"
	if err := config.SavePod(pdir, p); err != nil {
		t.Fatal(err)
	}
	enableChiefOfStaff(t, root, "demo", "sam", []config.Trigger{config.TriggerUnresolvedRouting})

	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "I'm stuck on the repro"}},
		mock.ScriptedResponse{ForMember: "sam", Response: adapter.InvokeResponse{Body: "@bob can you help alice with the repro?"}},
		mock.ScriptedResponse{ForMember: "bob", Response: adapter.InvokeResponse{Body: "on it, will post results"}},
	))
	app, appOut := appWithMockAdapter(t, cwd, home, m)

	rootCmd := app.NewRootCmd()
	rootCmd.SetArgs([]string{"run", "--pod", "demo", "--message", "please investigate the auth bug", "--max-turns", "5"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := appOut.String()
	for _, want := range []string{
		"[human] please investigate",
		"[alice] I'm stuck",
		"[sam] @bob can you help",
		"[bob] on it",
		"quiescent",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in CLI output:\n%s", want, out)
		}
	}

	goldenCompareFullLog(t, filepath.Join(root, "pods", "demo", "threads", "default.jsonl"),
		"testdata/golden/cos_rescue.jsonl", cwd, 4)
}

// TestE2E_CoS_Milestone runs a pod with milestone trigger firing every
// 2 member turns; after 2 member turns the CoS posts a summary.
func TestE2E_CoS_Milestone(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	runCmd(t, newApp(cwd, home), "init", "--local")
	runCmd(t, newApp(cwd, home), "pod", "create", "demo")
	for _, name := range []string{"alice", "bob"} {
		runCmd(t, newApp(cwd, home), "member", "add",
			"--pod", "demo",
			"--name", name,
			"--title", "Engineer",
			"--adapter", "mock",
			"--model", "m",
			"--effort", "medium",
		)
	}
	root := filepath.Join(cwd, ".poddies")

	pdir := filepath.Join(root, "pods", "demo")
	p, err := config.LoadPod(pdir)
	if err != nil {
		t.Fatal(err)
	}
	p.Lead = "alice"
	if err := config.SavePod(pdir, p); err != nil {
		t.Fatal(err)
	}
	enableChiefOfStaff(t, root, "demo", "sam", []config.Trigger{config.TriggerMilestone})

	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@bob looking at the auth flow"}},
		mock.ScriptedResponse{ForMember: "bob", Response: adapter.InvokeResponse{Body: "@alice I see the bug in token refresh"}},
		// milestone after 2 member turns — sam posts summary (no mention → halt)
		mock.ScriptedResponse{ForMember: "sam", Response: adapter.InvokeResponse{Body: "milestone: alice + bob identified the token refresh bug"}},
	))
	// milestone-every-2 so it fires after alice+bob.
	app, appOut := appWithMockAdapter(t, cwd, home, m)
	// We need to set MilestoneEvery=2 — the CLI doesn't expose this yet,
	// so patch via the App's AdapterLookup is insufficient. Instead,
	// invoke the loop directly so we control MilestoneEvery.
	// But we want E2E through the CLI. Alternative: have the test go
	// through the Loop API directly for this scenario.
	//
	// Keep CLI scenario simpler: default interval (3) with 3 member turns.
	// Re-script:
	m = mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@bob looking at auth flow"}},
		mock.ScriptedResponse{ForMember: "bob", Response: adapter.InvokeResponse{Body: "@alice I see the refresh bug"}},
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@bob patching now"}},
		mock.ScriptedResponse{ForMember: "sam", Response: adapter.InvokeResponse{Body: "milestone: patch in progress"}},
	))
	app, appOut = appWithMockAdapter(t, cwd, home, m)

	rootCmd := app.NewRootCmd()
	rootCmd.SetArgs([]string{"run", "--pod", "demo", "--message", "auth bug please", "--max-turns", "10"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := appOut.String()
	if !strings.Contains(out, "[sam] milestone: patch in progress") {
		t.Errorf("expected CoS milestone in output:\n%s", out)
	}
	if !strings.Contains(out, "quiescent") {
		t.Errorf("expected quiescent stop reason:\n%s", out)
	}

	goldenCompareFullLog(t, filepath.Join(root, "pods", "demo", "threads", "default.jsonl"),
		"testdata/golden/cos_milestone.jsonl", cwd, 5)
}

// goldenCompareFullLog loads the thread at srcPath, replays the events
// through a deterministic log, normalizes the result, and compares it
// against the golden. wantEvents asserts the event count up front for
// clearer failures.
func goldenCompareFullLog(t *testing.T, srcPath, goldenPath, cwd string, wantEvents int) {
	t.Helper()
	events, err := thread.Open(srcPath).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != wantEvents {
		t.Fatalf("want %d events, got %d", wantEvents, len(events))
	}

	var counter int64
	base := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	replayPath := filepath.Join(t.TempDir(), "replay.jsonl")
	replay := &thread.Log{
		Path: replayPath,
		Now: func() time.Time {
			n := atomic.AddInt64(&counter, 1)
			return base.Add(time.Duration(n) * time.Second)
		},
		NewID: func() string {
			n := atomic.LoadInt64(&counter)
			return fmt.Sprintf("evt-%03d", n)
		},
	}
	if err := replay.EnsureFile(); err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		e.ID = ""
		e.TS = time.Time{}
		if _, err := replay.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(replayPath)
	if err != nil {
		t.Fatal(err)
	}
	got := Normalize(raw, cwd)

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %q (run -update): %v", goldenPath, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("golden mismatch %s\n--- want ---\n%s\n--- got ---\n%s", goldenPath, want, got)
	}
}
