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

// appWithMockAdapter constructs a CLI App with a mock adapter injected
// via AdapterLookup, so tests drive the full command path without
// touching the global adapter registry.
func appWithMockAdapter(t *testing.T, cwd, home string, m *mock.Adapter) (*cli.App, *bytes.Buffer) {
	t.Helper()
	var out, errBuf bytes.Buffer
	a := &cli.App{
		Out:           &out,
		Err:           &errBuf,
		In:            strings.NewReader(""),
		Cwd:           cwd,
		Home:          home,
		AdapterLookup: orchestrator.MapLookup(map[string]adapter.Adapter{"mock": m}),
	}
	return a, &out
}

// TestE2E_RunCommand_AppendsTurnToThread runs:
//  1. init
//  2. pod create
//  3. member add (mock adapter)
//  4. run --message "..." (appends human + member events)
//
// …and compares the thread log against a normalized golden.
//
// Note: because `poddies run` opens the log with production defaults
// (random IDs, real time), we overwrite the log file after the run
// with deterministically re-rendered events before comparison. The
// golden captures the essentials (types, senders, bodies, mention
// parsing) rather than the volatile fields the normalizer strips.
func TestE2E_RunCommand_AppendsTurnToThread(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	// 1-3: setup via CLI with a trivial mock for "warmup" we don't use
	runCmd(t, newApp(cwd, home), "init", "--local")
	runCmd(t, newApp(cwd, home), "pod", "create", "demo")
	runCmd(t, newApp(cwd, home), "member", "add",
		"--pod", "demo",
		"--name", "alice",
		"--title", "Staff Engineer",
		"--adapter", "mock",
		"--model", "local-m",
		"--effort", "high",
	)

	// Disable CoS for scripted mock tests.
	root := filepath.Join(cwd, ".poddies")
	pdir := filepath.Join(root, "pods", "demo")
	p, err := config.LoadPod(pdir)
	if err != nil {
		t.Fatal(err)
	}
	p.ChiefOfStaff.Enabled = false
	if err := config.SavePod(pdir, p); err != nil {
		t.Fatal(err)
	}

	// 4: scripted mock for the run command
	m := mock.New(mock.WithScript(mock.ScriptedResponse{
		ForMember:    "alice",
		WantContains: []string{"kick off"},
		Response:     adapter.InvokeResponse{Body: "@bob pulling the repro"},
	}))
	app, appOut := appWithMockAdapter(t, cwd, home, m)
	rootCmd := app.NewRootCmd()
	rootCmd.SetArgs([]string{"run", "--pod", "demo", "--member", "alice", "--message", "kick off"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("run: %v", err)
	}

	// CLI output checks
	outStr := appOut.String()
	for _, want := range []string{"[human] kick off", "[alice] @bob pulling the repro"} {
		if !strings.Contains(outStr, want) {
			t.Errorf("missing %q in CLI output:\n%s", want, outStr)
		}
	}

	// mock assertions: called exactly once for alice
	calls := m.Calls()
	if len(calls) != 1 || calls[0].MemberName != "alice" {
		t.Errorf("want 1 call for alice, got %+v", calls)
	}
	if m.Remaining() != 0 {
		t.Errorf("scripted responses unused: %d", m.Remaining())
	}

	// Load the real log, re-render it through a deterministic log into
	// a fresh file, and compare THAT against the golden. This keeps the
	// golden schema-stable without requiring us to inject deterministic
	// ID/time generators through the CLI surface.
	logPath := filepath.Join(cwd, ".poddies", "pods", "demo", "threads", "default.jsonl")
	events, err := thread.Open(logPath).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("want 2 events (human+member), got %d", len(events))
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
		// reset ID/TS so the log re-assigns deterministic values.
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

	goldenPath := "testdata/golden/run_command.jsonl"
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
		t.Errorf("run-command golden mismatch\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}
