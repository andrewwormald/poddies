package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
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
	"github.com/andrewwormald/poddies/internal/thread"
)

var updateGolden = flag.Bool("update", false, "regenerate golden files in e2e/testdata/golden")

// runCmd invokes a cobra command tree rooted at a.NewRootCmd with args.
func runCmd(t *testing.T, a *cli.App, args ...string) {
	t.Helper()
	cmd := a.NewRootCmd()
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd %v: %v", args, err)
	}
}

func newApp(cwd, home string) *cli.App {
	var out, errBuf bytes.Buffer
	return &cli.App{
		Out:  &out,
		Err:  &errBuf,
		In:   strings.NewReader(""),
		Cwd:  cwd,
		Home: home,
	}
}

// TestE2E_SetupAndScriptedThread runs the full M1-available surface:
//  1. Initialize a local poddies root via `poddies init`.
//  2. Create a pod via `poddies pod create`.
//  3. Add two members via `poddies member add`.
//  4. Open a thread log with deterministic ID/time generators.
//  5. Drive a scripted mock adapter through 3 turns, appending each
//     response to the log.
//  6. Normalize the resulting JSONL and compare against a golden file.
//  7. Assert the mock was invoked as expected.
func TestE2E_SetupAndScriptedThread(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	// 1+2+3: set up via CLI
	runCmd(t, newApp(cwd, home), "init", "--local")
	runCmd(t, newApp(cwd, home), "pod", "create", "demo")
	for _, spec := range []struct {
		name, title, adapter, model, effort string
	}{
		{"alice", "Staff Engineer", "mock", "local-llama-3", "high"},
		{"bob", "Senior Engineer", "mock", "local-llama-3", "medium"},
	} {
		runCmd(t, newApp(cwd, home), "member", "add",
			"--pod", "demo",
			"--name", spec.name,
			"--title", spec.title,
			"--adapter", spec.adapter,
			"--model", spec.model,
			"--effort", spec.effort,
		)
	}

	root := filepath.Join(cwd, "poddies")

	// sanity: members exist on disk
	for _, n := range []string{"alice", "bob"} {
		if _, err := config.LoadMember(filepath.Join(root, "pods", "demo"), n); err != nil {
			t.Fatalf("load member %s: %v", n, err)
		}
	}

	// 4: deterministic thread log
	var counter int64
	base := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	logPath := filepath.Join(root, "pods", "demo", "threads", "e2e.jsonl")
	log := &thread.Log{
		Path: logPath,
		Now: func() time.Time {
			n := atomic.AddInt64(&counter, 1)
			return base.Add(time.Duration(n) * time.Second)
		},
		NewID: func() string {
			n := atomic.LoadInt64(&counter)
			return fmt.Sprintf("evt-%03d", n)
		},
	}
	if err := log.EnsureFile(); err != nil {
		t.Fatal(err)
	}

	// 5: scripted mock adapter
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{
			ForMember: "alice",
			Response:  adapter.InvokeResponse{Body: "@bob ready to look at the auth bug?"},
		},
		mock.ScriptedResponse{
			ForMember:    "bob",
			WantContains: []string{"@bob ready"},
			Response:     adapter.InvokeResponse{Body: "@alice yes, pulling the repro now"},
		},
	))

	// seed a kickoff from the human, then invoke each member in order.
	if _, err := log.Append(thread.Event{Type: thread.EventHuman, Body: "kick off the auth bug investigation"}); err != nil {
		t.Fatal(err)
	}
	members := []string{"alice", "bob"}
	for _, name := range members {
		member, err := config.LoadMember(filepath.Join(root, "pods", "demo"), name)
		if err != nil {
			t.Fatal(err)
		}
		existing, err := log.Load()
		if err != nil {
			t.Fatal(err)
		}
		resp, err := m.Invoke(context.Background(), adapter.InvokeRequest{
			Role:   adapter.RoleMember,
			Member: *member,
			Pod:    config.Pod{Name: "demo"},
			Thread: existing,
			Effort: member.Effort,
		})
		if err != nil {
			t.Fatalf("invoke %s: %v", name, err)
		}
		if _, err := log.Append(thread.Event{
			Type: thread.EventMessage,
			From: name,
			Body: resp.Body,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := log.Append(thread.Event{Type: thread.EventHuman, Body: "ship the fix when ready"}); err != nil {
		t.Fatal(err)
	}

	// 6: golden compare
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := Normalize(raw, cwd)
	goldenPath := "testdata/golden/setup_and_run.jsonl"
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, got, 0o600); err != nil {
			t.Fatal(err)
		}
	} else {
		want, err := os.ReadFile(goldenPath)
		if err != nil {
			t.Fatalf("read golden (run -update first): %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("golden mismatch\n--- want ---\n%s\n--- got ---\n%s", want, got)
		}
	}

	// 7: mock assertion
	calls := m.Calls()
	if len(calls) != 2 {
		t.Fatalf("want 2 mock calls, got %d", len(calls))
	}
	if calls[0].MemberName != "alice" || calls[1].MemberName != "bob" {
		t.Errorf("unexpected call order: %+v", calls)
	}
	if m.Remaining() != 0 {
		t.Errorf("scripted responses unused: %d remaining", m.Remaining())
	}

	// 8: log is parseable (sanity)
	events, err := log.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Errorf("want 4 events (human, alice, bob, human), got %d", len(events))
	}
	// strip json raw usage warning
	_ = json.Valid
}

// TestE2E_ScriptExhausted_ReportsClearly guards that the mock's
// exhaustion error is surfaced verbatim to the caller — so an E2E
// under-scripted by a test writer fails loudly, not silently.
func TestE2E_ScriptExhausted_ReportsClearly(t *testing.T) {
	m := mock.New() // empty script
	_, err := m.Invoke(context.Background(), adapter.InvokeRequest{
		Role:   adapter.RoleMember,
		Member: config.Member{Name: "alice"},
		Pod:    config.Pod{Name: "demo"},
	})
	if err == nil || !strings.Contains(err.Error(), "exhausted") {
		t.Errorf("want exhausted error, got %v", err)
	}
}
