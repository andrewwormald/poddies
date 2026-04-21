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

// setupPodWithMember scaffolds a local poddies root + pod + one mock member.
func setupPodWithMember(t *testing.T) (cwd, root, podName, member string) {
	t.Helper()
	cwd, root = initLocalRoot(t)
	if _, err := CreatePod(root, "demo"); err != nil {
		t.Fatal(err)
	}
	disableCoS(t, root, "demo")
	m := config.Member{
		Name: "alice", Title: "Staff", Adapter: config.AdapterMock,
		Model: "local-m", Effort: config.EffortHigh,
	}
	if err := AddMember(root, "demo", m); err != nil {
		t.Fatal(err)
	}
	return cwd, root, "demo", "alice"
}

// patchCoSToMock rewrites the pod's CoS adapter to "mock" so tests that
// only register a mock adapter can still exercise the dispatch path.
func patchCoSToMock(t *testing.T, root, pod string) {
	t.Helper()
	p, err := config.LoadPod(PodDir(root, pod))
	if err != nil {
		t.Fatal(err)
	}
	p.ChiefOfStaff.Adapter = config.AdapterMock
	p.ChiefOfStaff.Model = "mock-m"
	if err := config.SavePod(PodDir(root, pod), p); err != nil {
		t.Fatal(err)
	}
}

// disableCoS turns off the CoS for tests that use scripted mocks and
// don't want CoS dispatch interfering with their expected turn order.
func disableCoS(t *testing.T, root, pod string) {
	t.Helper()
	p, err := config.LoadPod(PodDir(root, pod))
	if err != nil {
		t.Fatal(err)
	}
	p.ChiefOfStaff.Enabled = false
	if err := config.SavePod(PodDir(root, pod), p); err != nil {
		t.Fatal(err)
	}
}

// setupPodWithTwoMembers scaffolds demo with alice (lead) and bob.
func setupPodWithTwoMembers(t *testing.T) (cwd, root string) {
	t.Helper()
	cwd, root = initLocalRoot(t)
	if _, err := CreatePod(root, "demo"); err != nil {
		t.Fatal(err)
	}
	disableCoS(t, root, "demo")
	// set lead=alice so human kickoff → lead → alice
	p, err := config.LoadPod(PodDir(root, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	p.Lead = "alice"
	if err := config.SavePod(PodDir(root, "demo"), p); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"alice", "bob"} {
		m := config.Member{
			Name: n, Title: "T", Adapter: config.AdapterMock,
			Model: "m", Effort: config.EffortMedium,
		}
		if err := AddMember(root, "demo", m); err != nil {
			t.Fatal(err)
		}
	}
	return cwd, root
}

func appWithMock(cwd, home string, m *mock.Adapter) *App {
	a, _, _ := newTestApp(cwd, home)
	a.AdapterLookup = orchestrator.MapLookup(map[string]adapter.Adapter{"mock": m})
	return a
}

func TestRunCmd_SingleMember_MemberFlag_Runs(t *testing.T) {
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
	for _, want := range []string{"[human] go", "[alice] @bob over to you", "quiescent"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestRunCmd_MultiTurn_Routing(t *testing.T) {
	cwd, _ := setupPodWithTwoMembers(t)
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@bob take this"}},
		mock.ScriptedResponse{ForMember: "bob", Response: adapter.InvokeResponse{Body: "done"}},
	))
	a := appWithMock(cwd, t.TempDir(), m)
	if err := runCmd(t, a, "run", "--pod", "demo", "--message", "kickoff"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := a.Out.(interface{ String() string }).String()
	for _, want := range []string{"[alice] @bob take this", "[bob] done", "quiescent"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestRunCmd_MaxTurnsFlag_Caps(t *testing.T) {
	cwd, _ := setupPodWithTwoMembers(t)
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@bob 1"}},
		mock.ScriptedResponse{ForMember: "bob", Response: adapter.InvokeResponse{Body: "@alice 2"}},
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@bob 3"}},
	))
	a := appWithMock(cwd, t.TempDir(), m)
	if err := runCmd(t, a, "run", "--pod", "demo", "--message", "go", "--max-turns", "2"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := a.Out.(interface{ String() string }).String()
	if !strings.Contains(out, "max_turns") {
		t.Errorf("want max_turns stop reason, got:\n%s", out)
	}
	if !strings.Contains(out, "turns=2") {
		t.Errorf("want turns=2, got:\n%s", out)
	}
}

func TestRunCmd_NoMembers_QuiescesImmediately(t *testing.T) {
	cwd, root := initLocalRoot(t)
	if _, err := CreatePod(root, "demo"); err != nil {
		t.Fatal(err)
	}
	disableCoS(t, root, "demo")
	m := mock.New()
	a := appWithMock(cwd, t.TempDir(), m)
	if err := runCmd(t, a, "run", "--pod", "demo"); err != nil {
		t.Fatal(err)
	}
	out := a.Out.(interface{ String() string }).String()
	if !strings.Contains(out, "quiescent") {
		t.Errorf("want quiescent, got:\n%s", out)
	}
}

func TestRunCmd_NonexistentPod_Errors(t *testing.T) {
	cwd, _ := initLocalRoot(t)
	a := appWithMock(cwd, t.TempDir(), mock.New())
	if err := runCmd(t, a, "run", "--pod", "ghost"); err == nil {
		t.Fatal("want error for ghost pod")
	}
}

func TestRunCmd_AutoSelectsPod_WhenOne(t *testing.T) {
	cwd, _, _, _ := setupPodWithMember(t)
	m := mock.New(mock.WithScript(mock.ScriptedResponse{Response: adapter.InvokeResponse{Body: "ok"}}))
	a := appWithMock(cwd, t.TempDir(), m)
	if err := runCmd(t, a, "run", "--member", "alice", "--message", "go"); err != nil {
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
	err := runCmd(t, a, "run")
	if err == nil || !strings.Contains(err.Error(), "multiple pods") {
		t.Errorf("want multiple-pods error, got %v", err)
	}
}

func TestRunCmd_CustomThreadName_CreatesNamedFile(t *testing.T) {
	cwd, root, _, _ := setupPodWithMember(t)
	m := mock.New(mock.WithScript(mock.ScriptedResponse{Response: adapter.InvokeResponse{Body: "ok"}}))
	a := appWithMock(cwd, t.TempDir(), m)
	if err := runCmd(t, a, "run", "--pod", "demo", "--member", "alice", "--message", "go", "--thread", "custom.jsonl"); err != nil {
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
	if err := runCmd(t, a, "run", "--pod", "demo", "--member", "alice", "--message", "go"); err != nil {
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
	if err := runCmd(t, a, "run", "--pod", "demo", "--member", "alice", "--message", "go", "--effort", "low"); err != nil {
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
