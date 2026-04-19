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
	"github.com/andrewwormald/poddies/internal/cli"
	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/orchestrator"
	"github.com/andrewwormald/poddies/internal/thread"
)

// TestE2E_MultiTurn_Loop exercises the full multi-agent loop via the
// CLI. Two members (alice, bob), alice is lead. Human kicks off with no
// mention → Route sends to lead (alice) → alice @mentions bob → bob
// replies without mentioning anyone → quiescent.
//
// Golden compares the thread log after deterministic replay.
func TestE2E_MultiTurn_Loop(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	runCmd(t, newApp(cwd, home), "init", "--local")
	runCmd(t, newApp(cwd, home), "pod", "create", "demo")
	for _, spec := range []struct{ name, title string }{
		{"alice", "Staff Engineer"},
		{"bob", "Senior Engineer"},
	} {
		runCmd(t, newApp(cwd, home), "member", "add",
			"--pod", "demo",
			"--name", spec.name,
			"--title", spec.title,
			"--adapter", "mock",
			"--model", "local-m",
			"--effort", "medium",
		)
	}

	// set lead=alice so routing flows: human → alice (lead) → bob (mention) → quiescent.
	root := filepath.Join(cwd, ".poddies")
	podDir := filepath.Join(root, "pods", "demo")
	p, err := config.LoadPod(podDir)
	if err != nil {
		t.Fatal(err)
	}
	p.Lead = "alice"
	if err := config.SavePod(podDir, p); err != nil {
		t.Fatal(err)
	}

	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{
			ForMember:    "alice",
			WantContains: []string{"investigate the auth bug"},
			Response:     adapter.InvokeResponse{Body: "@bob can you pull the repro while I read the auth code?"},
		},
		mock.ScriptedResponse{
			ForMember:    "bob",
			WantContains: []string{"pull the repro"},
			Response:     adapter.InvokeResponse{Body: "repro is ready, no further questions"},
		},
	))
	app, appOut := appWithMockAdapter(t, cwd, home, m)

	rootCmd := app.NewRootCmd()
	rootCmd.SetArgs([]string{"run", "--pod", "demo", "--message", "investigate the auth bug", "--max-turns", "5"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := appOut.String()
	for _, want := range []string{
		"[human] investigate the auth bug",
		"[alice] @bob can you pull the repro",
		"[bob] repro is ready",
		"quiescent",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in CLI output:\n%s", want, out)
		}
	}

	if m.Remaining() != 0 {
		t.Errorf("scripted responses unused: %d", m.Remaining())
	}
	calls := m.Calls()
	if len(calls) != 2 || calls[0].MemberName != "alice" || calls[1].MemberName != "bob" {
		t.Errorf("want [alice, bob] call order, got %+v", calls)
	}

	// Re-render the thread into a deterministic log for golden compare.
	logPath := filepath.Join(root, "pods", "demo", "threads", "default.jsonl")
	events, err := thread.Open(logPath).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("want 3 events (human + 2 members), got %d", len(events))
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

	goldenPath := "testdata/golden/multi_turn.jsonl"
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
		t.Fatalf("read golden (run -update): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("multi-turn golden mismatch\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// silence unused-import warnings in files that only reference cli.App via alias.
var _ = cli.App{}
var _ = orchestrator.Loop{}
