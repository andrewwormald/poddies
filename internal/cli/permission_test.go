package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/thread"
)

// seedPermissionThread creates a thread with one unresolved permission
// request and returns the request's ID along with cwd and root paths.
func seedPermissionThread(t *testing.T) (cwd, root, reqID string) {
	t.Helper()
	cwd, root, _, _ = setupPodWithMember(t)
	path := ThreadPath(root, "demo", normalizeThreadName("t"))
	log := thread.Open(path)
	if err := log.EnsureFile(); err != nil {
		t.Fatal(err)
	}
	if _, err := log.Append(thread.Event{Type: thread.EventHuman, Body: "start"}); err != nil {
		t.Fatal(err)
	}
	e, err := log.Append(thread.Event{
		Type: thread.EventPermissionRequest, From: "alice", Action: "run_shell",
	})
	if err != nil {
		t.Fatal(err)
	}
	return cwd, root, e.ID
}

func TestAppendGrant_Success(t *testing.T) {
	_, root, reqID := seedPermissionThread(t)
	path := ThreadPath(root, "demo", "t.jsonl")
	log := thread.Open(path)
	events, err := log.Load()
	if err != nil {
		t.Fatal(err)
	}
	e, err := AppendGrant(log, events, reqID, "human")
	if err != nil {
		t.Fatal(err)
	}
	if e.Type != thread.EventPermissionGrant || e.RequestID != reqID {
		t.Errorf("bad grant event: %+v", e)
	}
}

func TestAppendGrant_NotFound_Errors(t *testing.T) {
	_, root, _ := seedPermissionThread(t)
	path := ThreadPath(root, "demo", "t.jsonl")
	log := thread.Open(path)
	events, _ := log.Load()
	_, err := AppendGrant(log, events, "ghost", "human")
	if !errors.Is(err, ErrPermissionNotFound) {
		t.Errorf("want ErrPermissionNotFound, got %v", err)
	}
}

func TestAppendGrant_AlreadyResolved_Errors(t *testing.T) {
	_, root, reqID := seedPermissionThread(t)
	path := ThreadPath(root, "demo", "t.jsonl")
	log := thread.Open(path)
	events, _ := log.Load()
	if _, err := AppendGrant(log, events, reqID, "human"); err != nil {
		t.Fatal(err)
	}
	events, _ = log.Load()
	_, err := AppendGrant(log, events, reqID, "human")
	if !errors.Is(err, ErrPermissionAlreadyResolved) {
		t.Errorf("want ErrPermissionAlreadyResolved, got %v", err)
	}
}

func TestAppendDeny_StoresReasonInBody(t *testing.T) {
	_, root, reqID := seedPermissionThread(t)
	path := ThreadPath(root, "demo", "t.jsonl")
	log := thread.Open(path)
	events, _ := log.Load()
	e, err := AppendDeny(log, events, reqID, "human", "too risky")
	if err != nil {
		t.Fatal(err)
	}
	if e.Type != thread.EventPermissionDeny || e.Body != "too risky" {
		t.Errorf("bad deny event: %+v", e)
	}
}

// --- cobra ---

func TestThreadPermissionsCmd_Empty(t *testing.T) {
	cwd, _ := seedThread(t, "empty", []thread.Event{
		{Type: thread.EventHuman, Body: "hi"},
	})
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "thread", "permissions", "--pod", "demo", "empty"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no pending") {
		t.Errorf("want 'no pending', got %q", out.String())
	}
}

func TestThreadPermissionsCmd_ShowsPending(t *testing.T) {
	cwd, _, reqID := seedPermissionThread(t)
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "thread", "permissions", "--pod", "demo", "t"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{reqID, "alice", "run_shell"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in:\n%s", want, out.String())
		}
	}
}

func TestThreadApproveCmd_Success(t *testing.T) {
	cwd, root, reqID := seedPermissionThread(t)
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "thread", "approve", "--pod", "demo", "t", reqID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "granted") {
		t.Errorf("want 'granted', got %q", out.String())
	}
	events, _ := thread.Open(ThreadPath(root, "demo", "t.jsonl")).Load()
	if !thread.IsResolved(events, reqID) {
		t.Error("request should be resolved after approve")
	}
}

func TestThreadDenyCmd_WithReason(t *testing.T) {
	cwd, root, reqID := seedPermissionThread(t)
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "thread", "deny", "--pod", "demo", "--reason", "not now", "t", reqID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "denied") {
		t.Errorf("want 'denied', got %q", out.String())
	}
	events, _ := thread.Open(ThreadPath(root, "demo", "t.jsonl")).Load()
	if !thread.IsResolved(events, reqID) {
		t.Error("request should be resolved after deny")
	}
	// check reason stored in body
	for _, e := range events {
		if e.Type == thread.EventPermissionDeny && e.RequestID == reqID {
			if e.Body != "not now" {
				t.Errorf("want body 'not now', got %q", e.Body)
			}
		}
	}
}

func TestThreadApproveCmd_NotFound(t *testing.T) {
	cwd, _, _ := seedPermissionThread(t)
	a, _, _ := newTestApp(cwd, t.TempDir())
	err := runCmd(t, a, "thread", "approve", "--pod", "demo", "t", "ghost-id")
	if !errors.Is(err, ErrPermissionNotFound) {
		t.Errorf("want ErrPermissionNotFound, got %v", err)
	}
}

func TestThreadApproveCmd_AlreadyResolved(t *testing.T) {
	cwd, _, reqID := seedPermissionThread(t)
	a, _, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "thread", "approve", "--pod", "demo", "t", reqID); err != nil {
		t.Fatal(err)
	}
	a2, _, _ := newTestApp(cwd, t.TempDir())
	err := runCmd(t, a2, "thread", "approve", "--pod", "demo", "t", reqID)
	if !errors.Is(err, ErrPermissionAlreadyResolved) {
		t.Errorf("want ErrPermissionAlreadyResolved, got %v", err)
	}
}
