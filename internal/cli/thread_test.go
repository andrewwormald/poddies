package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/adapter/mock"
	"github.com/andrewwormald/poddies/internal/thread"
)

// seedThread creates a pod, a member, and a named thread with the given
// events. Returns cwd and root paths.
func seedThread(t *testing.T, threadName string, events []thread.Event) (cwd, root string) {
	t.Helper()
	cwd, root, _, _ = setupPodWithMember(t)
	path := ThreadPath(root, "demo", normalizeThreadName(threadName))
	log := thread.Open(path)
	if err := log.EnsureFile(); err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if _, err := log.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	return cwd, root
}

func TestNormalizeThreadName_AddsSuffix(t *testing.T) {
	if got := normalizeThreadName("default"); got != "default.jsonl" {
		t.Errorf("want default.jsonl, got %s", got)
	}
}

func TestNormalizeThreadName_PreservesSuffix(t *testing.T) {
	if got := normalizeThreadName("default.jsonl"); got != "default.jsonl" {
		t.Errorf("want unchanged, got %s", got)
	}
}

func TestListThreads_Empty(t *testing.T) {
	_, root, _, _ := setupPodWithMember(t)
	got, err := ListThreads(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("want empty, got %v", got)
	}
}

func TestListThreads_ReturnsEntries_SortedByMtime(t *testing.T) {
	_, root := seedThread(t, "older", []thread.Event{
		{Type: thread.EventHuman, Body: "first"},
	})
	// ensure the second file has a strictly newer mtime
	time.Sleep(20 * time.Millisecond)
	path := ThreadPath(root, "demo", "newer.jsonl")
	log := thread.Open(path)
	_ = log.EnsureFile()
	_, _ = log.Append(thread.Event{Type: thread.EventHuman, Body: "hi"})
	_, _ = log.Append(thread.Event{Type: thread.EventMessage, From: "alice", Body: "ok"})
	// nudge mtime forward
	now := time.Now().Add(time.Second)
	_ = os.Chtimes(path, now, now)

	got, err := ListThreads(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 threads, got %d", len(got))
	}
	if got[0].Name != "newer" {
		t.Errorf("want newer first (mtime desc), got %+v", got)
	}
	if got[0].Events != 2 {
		t.Errorf("want 2 events on newer, got %d", got[0].Events)
	}
	if got[0].LastFrom != "alice" {
		t.Errorf("want last-from=alice, got %s", got[0].LastFrom)
	}
}

func TestListThreads_SkipsNonJSONL(t *testing.T) {
	_, root := seedThread(t, "real", []thread.Event{
		{Type: thread.EventHuman, Body: "x"},
	})
	stray := filepath.Join(PodDir(root, "demo"), ThreadsDirName, "notes.txt")
	if err := os.WriteFile(stray, []byte("ignore me"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ListThreads(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "real" {
		t.Errorf("want [real], got %v", got)
	}
}

func TestLoadThread_NotFound_ErrThreadNotFound(t *testing.T) {
	_, root, _, _ := setupPodWithMember(t)
	_, err := LoadThread(root, "demo", "ghost")
	if !errors.Is(err, ErrThreadNotFound) {
		t.Errorf("want ErrThreadNotFound, got %v", err)
	}
}

func TestLoadThread_ReturnsEvents(t *testing.T) {
	_, root := seedThread(t, "t", []thread.Event{
		{Type: thread.EventHuman, Body: "hi"},
	})
	events, err := LoadThread(root, "demo", "t")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Body != "hi" {
		t.Errorf("unexpected events: %+v", events)
	}
}

// --- cobra ---

func TestThreadListCmd_Empty(t *testing.T) {
	cwd, _, _, _ := setupPodWithMember(t)
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "thread", "list", "--pod", "demo"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no threads") {
		t.Errorf("want 'no threads', got %q", out.String())
	}
}

func TestThreadListCmd_ShowsEntries(t *testing.T) {
	cwd, _ := seedThread(t, "demo_thread", []thread.Event{
		{Type: thread.EventHuman, Body: "hi"},
		{Type: thread.EventMessage, From: "alice", Body: "yo"},
	})
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "thread", "list", "--pod", "demo"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"demo_thread", "events=2", "last=alice"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in:\n%s", want, out.String())
		}
	}
}

func TestThreadShowCmd_Human(t *testing.T) {
	cwd, _ := seedThread(t, "t", []thread.Event{
		{Type: thread.EventHuman, Body: "kick off"},
		{Type: thread.EventMessage, From: "alice", Body: "@bob go"},
	})
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "thread", "show", "--pod", "demo", "t"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[human] kick off", "[alice] @bob go"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in:\n%s", want, out.String())
		}
	}
}

func TestThreadShowCmd_JSON(t *testing.T) {
	cwd, _ := seedThread(t, "t", []thread.Event{
		{Type: thread.EventHuman, Body: "kick off"},
	})
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "thread", "show", "--pod", "demo", "--json", "t"); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, `"type":"human"`) {
		t.Errorf("want JSON 'type' field, got %q", s)
	}
	if !strings.Contains(s, `"body":"kick off"`) {
		t.Errorf("want body in JSON, got %q", s)
	}
}

func TestThreadShowCmd_NotFound_Errors(t *testing.T) {
	cwd, _, _, _ := setupPodWithMember(t)
	a, _, _ := newTestApp(cwd, t.TempDir())
	err := runCmd(t, a, "thread", "show", "--pod", "demo", "ghost")
	if !errors.Is(err, ErrThreadNotFound) {
		t.Errorf("want ErrThreadNotFound, got %v", err)
	}
}

func TestThreadShowCmd_AcceptsBothShortAndLongName(t *testing.T) {
	cwd, _ := seedThread(t, "short", []thread.Event{{Type: thread.EventHuman, Body: "x"}})
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "thread", "show", "--pod", "demo", "short"); err != nil {
		t.Fatalf("short name: %v", err)
	}
	if !strings.Contains(out.String(), "[human] x") {
		t.Errorf("want content, got %q", out.String())
	}

	a2, out2, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a2, "thread", "show", "--pod", "demo", "short.jsonl"); err != nil {
		t.Fatalf("long name: %v", err)
	}
	if !strings.Contains(out2.String(), "[human] x") {
		t.Errorf("want content via long name, got %q", out2.String())
	}
}

func TestThreadResumeCmd_NonexistentThread_Errors(t *testing.T) {
	cwd, _, _, _ := setupPodWithMember(t)
	a, _, _ := newTestApp(cwd, t.TempDir())
	a.AdapterLookup = nil // irrelevant; should error before any loop
	err := runCmd(t, a, "thread", "resume", "--pod", "demo", "ghost")
	if !errors.Is(err, ErrThreadNotFound) {
		t.Errorf("want ErrThreadNotFound, got %v", err)
	}
}

func TestThreadResumeCmd_RunsLoop(t *testing.T) {
	cwd, _ := seedThread(t, "ongoing", []thread.Event{
		{Type: thread.EventHuman, Body: "earlier"},
	})
	m := mock.New(mock.WithScript(mock.ScriptedResponse{
		ForMember: "alice",
		Response:  adapter.InvokeResponse{Body: "follow-up done"},
	}))
	a := appWithMock(cwd, t.TempDir(), m)
	if err := runCmd(t, a, "thread", "resume", "--pod", "demo",
		"--member", "alice", "--message", "continue please", "ongoing"); err != nil {
		t.Fatalf("resume: %v", err)
	}
	out := a.Out.(interface{ String() string }).String()
	for _, want := range []string{"[human] continue please", "[alice] follow-up done", "quiescent"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestThreadResumeCmd_AppendsToExistingThread(t *testing.T) {
	cwd, root := seedThread(t, "ongoing", []thread.Event{
		{Type: thread.EventHuman, Body: "earlier"},
	})
	m := mock.New(mock.WithScript(mock.ScriptedResponse{
		ForMember: "alice",
		Response:  adapter.InvokeResponse{Body: "adding to thread"},
	}))
	a := appWithMock(cwd, t.TempDir(), m)
	if err := runCmd(t, a, "thread", "resume", "--pod", "demo",
		"--member", "alice", "--message", "go", "ongoing"); err != nil {
		t.Fatal(err)
	}
	events, err := LoadThread(root, "demo", "ongoing")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Errorf("want 3 events (earlier + go + alice), got %d", len(events))
	}
}
