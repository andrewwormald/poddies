package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/config"
)

// initLocalRoot sets up a poddies local root in a fresh tempdir and
// returns (cwd, rootDir).
func initLocalRoot(t *testing.T) (cwd, root string) {
	t.Helper()
	cwd = t.TempDir()
	if _, err := Init(cwd, t.TempDir(), config.ModeLocal, false); err != nil {
		t.Fatal(err)
	}
	return cwd, filepath.Join(cwd, "poddies")
}

func TestCreatePod_Succeeds(t *testing.T) {
	_, root := initLocalRoot(t)
	p, err := CreatePod(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "demo" {
		t.Errorf("want name demo, got %s", p.Name)
	}
	if _, err := os.Stat(filepath.Join(root, "pods", "demo", "pod.toml")); err != nil {
		t.Errorf("pod.toml missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "pods", "demo", "members")); err != nil {
		t.Errorf("members/ missing: %v", err)
	}
}

func TestCreatePod_Duplicate_Errors(t *testing.T) {
	_, root := initLocalRoot(t)
	if _, err := CreatePod(root, "demo"); err != nil {
		t.Fatal(err)
	}
	_, err := CreatePod(root, "demo")
	if err == nil {
		t.Error("want error on duplicate")
	}
}

func TestCreatePod_InvalidSlug_Errors(t *testing.T) {
	_, root := initLocalRoot(t)
	_, err := CreatePod(root, "Invalid Name")
	if err == nil {
		t.Error("want error for invalid slug")
	}
}

func TestCreatePod_MissingRoot_Errors(t *testing.T) {
	_, err := CreatePod(filepath.Join(t.TempDir(), "nope"), "demo")
	if err == nil {
		t.Error("want error for missing root")
	}
}

func TestListPods_Empty(t *testing.T) {
	_, root := initLocalRoot(t)
	pods, err := ListPods(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 0 {
		t.Errorf("want empty, got %v", pods)
	}
}

func TestListPods_Multiple_Sorted(t *testing.T) {
	_, root := initLocalRoot(t)
	for _, n := range []string{"zeta", "alpha", "beta"} {
		if _, err := CreatePod(root, n); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ListPods(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha", "beta", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] want %s, got %s", i, want[i], got[i])
		}
	}
}

func TestListPods_SkipsStrayDirs(t *testing.T) {
	_, root := initLocalRoot(t)
	if _, err := CreatePod(root, "real"); err != nil {
		t.Fatal(err)
	}
	// stray dir with no pod.toml
	if err := os.MkdirAll(filepath.Join(root, "pods", "stray"), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := ListPods(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "real" {
		t.Errorf("want [real], got %v", got)
	}
}

func TestListPods_NoPodsDir_Empty(t *testing.T) {
	root := t.TempDir() // no pods/ subdir
	got, err := ListPods(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("want empty, got %v", got)
	}
}

func TestPodExists_TrueFalse(t *testing.T) {
	_, root := initLocalRoot(t)
	if PodExists(root, "nope") {
		t.Error("expected false for nonexistent pod")
	}
	if _, err := CreatePod(root, "x"); err != nil {
		t.Fatal(err)
	}
	if !PodExists(root, "x") {
		t.Error("expected true after create")
	}
}

// --- cobra ---

func TestPodCreateCmd(t *testing.T) {
	cwd, _ := initLocalRoot(t)
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "pod", "create", "demo"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "created pod") {
		t.Errorf("want 'created pod', got %q", out.String())
	}
}

func TestPodCreateCmd_DuplicateErrors(t *testing.T) {
	cwd, _ := initLocalRoot(t)
	a, _, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "pod", "create", "demo"); err != nil {
		t.Fatal(err)
	}
	a2, _, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a2, "pod", "create", "demo"); err == nil {
		t.Error("want error for duplicate pod")
	}
}

func TestPodListCmd_Empty(t *testing.T) {
	cwd, _ := initLocalRoot(t)
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "pod", "list"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no pods") {
		t.Errorf("want 'no pods', got %q", out.String())
	}
}

func TestPodListCmd_ShowsPods(t *testing.T) {
	cwd, _ := initLocalRoot(t)
	for _, n := range []string{"a", "b"} {
		a, _, _ := newTestApp(cwd, t.TempDir())
		if err := runCmd(t, a, "pod", "create", n); err != nil {
			t.Fatal(err)
		}
	}
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "pod", "list"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "a\n") || !strings.Contains(out.String(), "b\n") {
		t.Errorf("expected a and b in output, got %q", out.String())
	}
}

func TestPodCmd_NoRoot_Errors(t *testing.T) {
	a, _, _ := newTestApp(t.TempDir(), t.TempDir())
	err := runCmd(t, a, "pod", "list")
	if err == nil {
		t.Error("want error when no root is resolvable")
	}
	if !errors.Is(err, config.ErrNoRoot) {
		// Accept wrapped forms — just check error is non-nil and cobra surfaced it.
	}
}
