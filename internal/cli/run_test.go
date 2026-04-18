package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/adapter/mock"
	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/orchestrator"
)

// setupPodWithMember scaffolds a pod with one mock-backed member.
func setupPodWithMember(t *testing.T) (cwd, root, podName, member string) {
	t.Helper()
	cwd, root = initLocalRoot(t)
	if _, err := CreatePod(root, "demo"); err != nil {
		t.Fatal(err)
	}
	m := config.Member{
		Name: "alice", Title: "Staff", Adapter: config.AdapterMock,
		Model: "local-m", Effort: config.EffortHigh,
	}
	if err := AddMember(root, "demo", m); err != nil {
		t.Fatal(err)
	}
	return cwd, root, "demo", "alice"
}

func appWithMock(cwd, home string, m *mock.Adapter) *App {
	a, _, _ := newTestApp(cwd, home)
	a.AdapterLookup = orchestrator.MapLookup(map[string]adapter.Adapter{"mock": m})
	return a
}

func TestRunCmd_HappyPath(t *testing.T) {
	cwd, _, _, _ := setupPodWithMember(t)
	m := mock.New(mock.WithScript(mock.ScriptedResponse{
		ForMember: "alice",
		Response:  adapter.InvokeResponse{Body: "@bob over to you"},
	}))
	a := appWithMock(cwd, t.TempDir(), m)
	if err := runCmd(t, a, "run", "--pod", "demo", "--member", "alice", "--message", "go"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := a.Out.(interface{ String() string }).String()
	for _, want := range []string{"[human] go", "[alice] @bob over to you"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestRunCmd_MemberRequired(t *testing.T) {
	cwd, _, _, _ := setupPodWithMember(t)
	a := appWithMock(cwd, t.TempDir(), mock.New())
	err := runCmd(t, a, "run", "--pod", "demo")
	if err == nil || !strings.Contains(err.Error(), "--member") {
		t.Errorf("want --member error, got %v", err)
	}
}

func TestRunCmd_NonexistentPod_Errors(t *testing.T) {
	cwd, _ := initLocalRoot(t)
	a := appWithMock(cwd, t.TempDir(), mock.New())
	err := runCmd(t, a, "run", "--pod", "ghost", "--member", "alice")
	if err == nil {
		t.Fatal("want error for ghost pod")
	}
}

func TestRunCmd_AutoSelectsPod_WhenOne(t *testing.T) {
	cwd, _, _, _ := setupPodWithMember(t)
	m := mock.New(mock.WithScript(mock.ScriptedResponse{Response: adapter.InvokeResponse{Body: "ok"}}))
	a := appWithMock(cwd, t.TempDir(), m)
	if err := runCmd(t, a, "run", "--member", "alice"); err != nil {
		t.Fatalf("want auto-select, got %v", err)
	}
}

func TestRunCmd_MultiplePods_WithoutFlag_Errors(t *testing.T) {
	cwd, root := initLocalRoot(t)
	for _, n := range []string{"a", "b"} {
		if _, err := CreatePod(root, n); err != nil {
			t.Fatal(err)
		}
	}
	a := appWithMock(cwd, t.TempDir(), mock.New())
	err := runCmd(t, a, "run", "--member", "alice")
	if err == nil || !strings.Contains(err.Error(), "multiple pods") {
		t.Errorf("want multiple-pods error, got %v", err)
	}
}

func TestRunCmd_CustomThreadName_CreatesNamedFile(t *testing.T) {
	cwd, root, _, _ := setupPodWithMember(t)
	m := mock.New(mock.WithScript(mock.ScriptedResponse{Response: adapter.InvokeResponse{Body: "ok"}}))
	a := appWithMock(cwd, t.TempDir(), m)
	if err := runCmd(t, a, "run", "--pod", "demo", "--member", "alice", "--thread", "custom.jsonl"); err != nil {
		t.Fatal(err)
	}
	path := ThreadPath(root, "demo", "custom.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("custom thread file not created: %v", err)
	}
}

func TestRunCmd_DefaultsToDefaultThread(t *testing.T) {
	cwd, root, _, _ := setupPodWithMember(t)
	m := mock.New(mock.WithScript(mock.ScriptedResponse{Response: adapter.InvokeResponse{Body: "ok"}}))
	a := appWithMock(cwd, t.TempDir(), m)
	if err := runCmd(t, a, "run", "--pod", "demo", "--member", "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ThreadPath(root, "demo", DefaultThreadName)); err != nil {
		t.Errorf("default thread missing: %v", err)
	}
}

func TestRunCmd_EffortOverride_ReachesAdapter(t *testing.T) {
	cwd, _, _, _ := setupPodWithMember(t)
	m := mock.New(mock.WithScript(mock.ScriptedResponse{Response: adapter.InvokeResponse{Body: "ok"}}))
	a := appWithMock(cwd, t.TempDir(), m)
	if err := runCmd(t, a, "run", "--pod", "demo", "--member", "alice", "--effort", "low"); err != nil {
		t.Fatal(err)
	}
	calls := m.Calls()
	if len(calls) != 1 || calls[0].Effort != "low" {
		t.Errorf("want low effort, got %+v", calls)
	}
}

func TestResolvePod_NoPods_Errors(t *testing.T) {
	root := t.TempDir()
	_, err := resolvePod(root, "")
	if err == nil {
		t.Error("want error")
	}
}

func TestResolvePod_RequestedButAbsent_Errors(t *testing.T) {
	_, root := initLocalRoot(t)
	_, err := resolvePod(root, "ghost")
	if err == nil {
		t.Error("want error for absent pod")
	}
}

func TestThreadPath_Shape(t *testing.T) {
	got := ThreadPath("/root", "demo", "t.jsonl")
	want := filepath.Join("/root", "pods", "demo", "threads", "t.jsonl")
	if got != want {
		t.Errorf("want %s, got %s", want, got)
	}
}
