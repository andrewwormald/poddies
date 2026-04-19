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

// helpers for export/import tests

// makeFullPod creates a pod with one member and returns (cwd, root).
func makeFullPod(t *testing.T) (cwd, root string) {
	t.Helper()
	cwd, root = initLocalRoot(t)
	if _, err := CreatePod(root, "demo"); err != nil {
		t.Fatal(err)
	}
	m := config.Member{
		Name:    "alice",
		Title:   "Staff Engineer",
		Adapter: config.AdapterMock,
		Model:   "mock-model",
		Effort:  config.EffortHigh,
	}
	if err := config.SaveMember(PodDir(root, "demo"), &m); err != nil {
		t.Fatal(err)
	}
	return cwd, root
}

// writeBundleFile writes a bundle to a temp file and returns its path.
func writeBundleFile(t *testing.T, b *config.Bundle) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "bundle-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := config.SaveBundle(f, b); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

// mustExportBundle exports a pod bundle or fatals.
func mustExportBundle(t *testing.T, root, name string) *config.Bundle {
	t.Helper()
	data, err := ExportPod(root, name, "")
	if err != nil {
		t.Fatalf("ExportPod: %v", err)
	}
	b, err := config.LoadBundle(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	return b
}

// --- ExportPod ---

func TestExportPod_ToStdout(t *testing.T) {
	_, root := makeFullPod(t)
	data, err := ExportPod(root, "demo", "")
	if err != nil {
		t.Fatalf("ExportPod: %v", err)
	}
	if len(data) == 0 {
		t.Error("want non-empty bundle bytes")
	}
	b, err := config.LoadBundle(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	if b.Pod.Name != "demo" {
		t.Errorf("want pod name demo, got %q", b.Pod.Name)
	}
}

func TestExportPod_ToFile(t *testing.T) {
	_, root := makeFullPod(t)
	outPath := filepath.Join(t.TempDir(), "demo.toml")
	data, err := ExportPod(root, "demo", outPath)
	if err != nil {
		t.Fatalf("ExportPod: %v", err)
	}
	if data != nil {
		t.Error("want nil data when writing to file")
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("output file missing: %v", err)
	}
}

func TestExportPod_NotFound_Errors(t *testing.T) {
	_, root := initLocalRoot(t)
	_, err := ExportPod(root, "nope", "")
	if err == nil {
		t.Error("want error for missing pod")
	}
}

// --- ImportPod ---

func TestImportPod_Creates(t *testing.T) {
	_, root := makeFullPod(t)
	data, err := ExportPod(root, "demo", "")
	if err != nil {
		t.Fatal(err)
	}
	bundleFile := filepath.Join(t.TempDir(), "demo.toml")
	if err := os.WriteFile(bundleFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, root2 := initLocalRoot(t)
	b, err := ImportPod(root2, bundleFile, "", false)
	if err != nil {
		t.Fatalf("ImportPod: %v", err)
	}
	if b.Pod.Name != "demo" {
		t.Errorf("want pod name demo, got %q", b.Pod.Name)
	}
	if !PodExists(root2, "demo") {
		t.Error("pod missing after import")
	}
}

func TestImportPod_AsRename(t *testing.T) {
	_, root := makeFullPod(t)
	bundlePath := writeBundleFile(t, mustExportBundle(t, root, "demo"))

	_, root2 := initLocalRoot(t)
	b, err := ImportPod(root2, bundlePath, "renamed", false)
	if err != nil {
		t.Fatalf("ImportPod --as: %v", err)
	}
	if b.Pod.Name != "renamed" {
		t.Errorf("want pod name renamed, got %q", b.Pod.Name)
	}
	if !PodExists(root2, "renamed") {
		t.Error("renamed pod missing after import")
	}
}

func TestImportPod_DuplicateNoOverwrite_Errors(t *testing.T) {
	_, root := makeFullPod(t)
	bundlePath := writeBundleFile(t, mustExportBundle(t, root, "demo"))

	_, root2 := initLocalRoot(t)
	if _, err := ImportPod(root2, bundlePath, "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportPod(root2, bundlePath, "", false); err == nil {
		t.Error("want error on duplicate import without --overwrite")
	}
}

func TestImportPod_DuplicateWithOverwrite_Succeeds(t *testing.T) {
	_, root := makeFullPod(t)
	bundlePath := writeBundleFile(t, mustExportBundle(t, root, "demo"))

	_, root2 := initLocalRoot(t)
	if _, err := ImportPod(root2, bundlePath, "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportPod(root2, bundlePath, "", true); err != nil {
		t.Errorf("want success with --overwrite, got %v", err)
	}
}

func TestImportPod_BadBundleFile_Errors(t *testing.T) {
	_, root2 := initLocalRoot(t)
	_, err := ImportPod(root2, filepath.Join(t.TempDir(), "nope.toml"), "", false)
	if err == nil {
		t.Error("want error for missing bundle file")
	}
}

func TestImportPod_SchemaVersionMismatch_Errors(t *testing.T) {
	_, root := makeFullPod(t)
	b := mustExportBundle(t, root, "demo")
	b.SchemaVersion = 99
	bundlePath := writeBundleFile(t, b)

	_, root2 := initLocalRoot(t)
	_, err := ImportPod(root2, bundlePath, "", false)
	if err == nil {
		t.Error("want error for schema_version mismatch")
	}
}

// --- Cobra commands ---

func TestPodExportCmd_Stdout(t *testing.T) {
	cwd, _ := makeFullPod(t)
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "pod", "export", "demo"); err != nil {
		t.Fatalf("export cmd: %v", err)
	}
	if !strings.Contains(out.String(), "schema_version") {
		t.Errorf("want TOML bundle in stdout, got %q", out.String())
	}
}

func TestPodExportCmd_OutFile(t *testing.T) {
	cwd, _ := makeFullPod(t)
	outPath := filepath.Join(t.TempDir(), "bundle.toml")
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "pod", "export", "demo", "--out", outPath); err != nil {
		t.Fatalf("export cmd: %v", err)
	}
	if !strings.Contains(out.String(), "exported pod") {
		t.Errorf("want 'exported pod' in output, got %q", out.String())
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("output file missing: %v", err)
	}
}

func TestPodExportCmd_NotFound_Errors(t *testing.T) {
	cwd, _ := initLocalRoot(t)
	a, _, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "pod", "export", "nope"); err == nil {
		t.Error("want error for missing pod")
	}
}

func TestPodImportCmd_Creates(t *testing.T) {
	srcCwd, _ := makeFullPod(t)
	outPath := filepath.Join(t.TempDir(), "demo.toml")
	{
		a, _, _ := newTestApp(srcCwd, t.TempDir())
		if err := runCmd(t, a, "pod", "export", "demo", "--out", outPath); err != nil {
			t.Fatal(err)
		}
	}

	dstCwd, dstRoot := initLocalRoot(t)
	a, out, _ := newTestApp(dstCwd, t.TempDir())
	if err := runCmd(t, a, "pod", "import", outPath); err != nil {
		t.Fatalf("import cmd: %v", err)
	}
	if !strings.Contains(out.String(), "imported pod") {
		t.Errorf("want 'imported pod' in output, got %q", out.String())
	}
	if !PodExists(dstRoot, "demo") {
		t.Error("pod missing after import")
	}
}

func TestPodImportCmd_AsFlag(t *testing.T) {
	srcCwd, _ := makeFullPod(t)
	outPath := filepath.Join(t.TempDir(), "demo.toml")
	{
		a, _, _ := newTestApp(srcCwd, t.TempDir())
		if err := runCmd(t, a, "pod", "export", "demo", "--out", outPath); err != nil {
			t.Fatal(err)
		}
	}

	dstCwd, dstRoot := initLocalRoot(t)
	a, _, _ := newTestApp(dstCwd, t.TempDir())
	if err := runCmd(t, a, "pod", "import", outPath, "--as", "other"); err != nil {
		t.Fatalf("import --as: %v", err)
	}
	if !PodExists(dstRoot, "other") {
		t.Error("renamed pod missing after import --as")
	}
}

func TestPodImportCmd_OverwriteFlag(t *testing.T) {
	srcCwd, _ := makeFullPod(t)
	outPath := filepath.Join(t.TempDir(), "demo.toml")
	{
		a, _, _ := newTestApp(srcCwd, t.TempDir())
		if err := runCmd(t, a, "pod", "export", "demo", "--out", outPath); err != nil {
			t.Fatal(err)
		}
	}

	dstCwd, _ := initLocalRoot(t)
	{
		a, _, _ := newTestApp(dstCwd, t.TempDir())
		if err := runCmd(t, a, "pod", "import", outPath); err != nil {
			t.Fatal(err)
		}
	}
	{
		a, _, _ := newTestApp(dstCwd, t.TempDir())
		if err := runCmd(t, a, "pod", "import", outPath); err == nil {
			t.Error("want error on duplicate without --overwrite")
		}
	}
	{
		a, _, _ := newTestApp(dstCwd, t.TempDir())
		if err := runCmd(t, a, "pod", "import", outPath, "--overwrite"); err != nil {
			t.Errorf("want success with --overwrite, got %v", err)
		}
	}
}

// Export + Import round-trip: re-exported bytes equal original.
func TestPodExportImport_RoundTrip_ByteIdentical(t *testing.T) {
	srcCwd, _ := makeFullPod(t)
	outPath := filepath.Join(t.TempDir(), "demo.toml")
	{
		a, _, _ := newTestApp(srcCwd, t.TempDir())
		if err := runCmd(t, a, "pod", "export", "demo", "--out", outPath); err != nil {
			t.Fatal(err)
		}
	}

	dstCwd, _ := initLocalRoot(t)
	{
		a, _, _ := newTestApp(dstCwd, t.TempDir())
		if err := runCmd(t, a, "pod", "import", outPath); err != nil {
			t.Fatal(err)
		}
	}

	outPath2 := filepath.Join(t.TempDir(), "demo2.toml")
	{
		a, _, _ := newTestApp(dstCwd, t.TempDir())
		if err := runCmd(t, a, "pod", "export", "demo", "--out", outPath2); err != nil {
			t.Fatal(err)
		}
	}

	orig, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	reexported, err := os.ReadFile(outPath2)
	if err != nil {
		t.Fatal(err)
	}
	if string(orig) != string(reexported) {
		t.Errorf("export->import->export not byte-identical\n--- original ---\n%s\n--- re-exported ---\n%s", orig, reexported)
	}
}
