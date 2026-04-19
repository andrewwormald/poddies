package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fullBundle returns a Bundle exercising all fields, suitable for golden tests.
func fullBundle() *Bundle {
	return &Bundle{
		SchemaVersion: 1,
		Pod:           *fullPod(),
		Members: []Member{
			*fullMember(),
			{
				Name:    "bob",
				Title:   "Senior Engineer",
				Adapter: AdapterGemini,
				Model:   "gemini-2-5-pro",
				Effort:  EffortMedium,
			},
		},
	}
}

func TestSaveBundle_ProducesGolden(t *testing.T) {
	b := fullBundle()
	var buf bytes.Buffer
	if err := SaveBundle(&buf, b); err != nil {
		t.Fatalf("SaveBundle: %v", err)
	}
	goldenCompare(t, "testdata/golden/bundle_full.toml", buf.Bytes())
}

func TestLoadBundle_Golden_RoundTrips(t *testing.T) {
	src, err := os.ReadFile("testdata/golden/bundle_full.toml")
	if err != nil {
		t.Skipf("golden missing (run -update first): %v", err)
	}
	got, err := LoadBundle(bytes.NewReader(src))
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	if !reflect.DeepEqual(got, fullBundle()) {
		t.Errorf("round-trip mismatch\nwant %#v\ngot  %#v", fullBundle(), got)
	}
}

func TestSaveBundle_ThenLoadBundle_Equivalent(t *testing.T) {
	want := fullBundle()
	var buf bytes.Buffer
	if err := SaveBundle(&buf, want); err != nil {
		t.Fatalf("SaveBundle: %v", err)
	}
	got, err := LoadBundle(&buf)
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mismatch\nwant %#v\ngot  %#v", want, got)
	}
}

func TestLoadBundle_UnknownField_Errors(t *testing.T) {
	const toml = `schema_version = 1
bogus_field = "oops"
[pod]
name = "demo"
lead = "human"
`
	_, err := LoadBundle(strings.NewReader(toml))
	if err == nil {
		t.Fatal("want error for unknown field")
	}
	if !strings.Contains(err.Error(), "strict") {
		t.Errorf("want strict-mode error, got %v", err)
	}
}

// --- Bundle.Validate ---

func TestBundle_Validate_OK(t *testing.T) {
	if err := fullBundle().Validate(); err != nil {
		t.Errorf("expected valid, got %v", err)
	}
}

func TestBundle_Validate_WrongSchemaVersion(t *testing.T) {
	b := fullBundle()
	b.SchemaVersion = 99
	if err := b.Validate(); err == nil {
		t.Error("want error for wrong schema_version")
	}
}

func TestBundle_Validate_BadPod(t *testing.T) {
	b := fullBundle()
	b.Pod.Name = ""
	if err := b.Validate(); err == nil {
		t.Error("want error for empty pod name")
	}
}

func TestBundle_Validate_BadMember(t *testing.T) {
	b := fullBundle()
	b.Members[0].Title = ""
	if err := b.Validate(); err == nil {
		t.Error("want error for empty member title")
	}
}

func TestBundle_Validate_DuplicateMemberName(t *testing.T) {
	b := fullBundle()
	dup := b.Members[0]
	b.Members = append(b.Members, dup)
	if err := b.Validate(); err == nil {
		t.Error("want error for duplicate member name")
	}
}

// --- NewBundleFromPodDir ---

func writePodDir(t *testing.T, podDir string, b *Bundle) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(podDir, MembersDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := SavePod(podDir, &b.Pod); err != nil {
		t.Fatal(err)
	}
	for i := range b.Members {
		if err := SaveMember(podDir, &b.Members[i]); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNewBundleFromPodDir_RoundTrip(t *testing.T) {
	want := fullBundle()
	podDir := t.TempDir()
	writePodDir(t, podDir, want)

	got, err := NewBundleFromPodDir(podDir)
	if err != nil {
		t.Fatalf("NewBundleFromPodDir: %v", err)
	}
	if got.SchemaVersion != currentSchemaVersion {
		t.Errorf("want schema_version %d, got %d", currentSchemaVersion, got.SchemaVersion)
	}
	if !reflect.DeepEqual(got.Pod, want.Pod) {
		t.Errorf("pod mismatch\nwant %#v\ngot  %#v", want.Pod, got.Pod)
	}
	// Members order is filesystem order (sorted by os.ReadDir); build a name map.
	if len(got.Members) != len(want.Members) {
		t.Fatalf("want %d members, got %d", len(want.Members), len(got.Members))
	}
	byName := make(map[string]Member, len(got.Members))
	for _, m := range got.Members {
		byName[m.Name] = m
	}
	for _, wm := range want.Members {
		gm, ok := byName[wm.Name]
		if !ok {
			t.Errorf("member %q missing", wm.Name)
			continue
		}
		if !reflect.DeepEqual(gm, wm) {
			t.Errorf("member %q mismatch\nwant %#v\ngot  %#v", wm.Name, wm, gm)
		}
	}
}

func TestNewBundleFromPodDir_MissingPodToml_Errors(t *testing.T) {
	_, err := NewBundleFromPodDir(t.TempDir())
	if err == nil {
		t.Error("want error for missing pod.toml")
	}
}

func TestNewBundleFromPodDir_NoMembersDir_OK(t *testing.T) {
	podDir := t.TempDir()
	p := &Pod{Name: "empty", Lead: "human"}
	if err := SavePod(podDir, p); err != nil {
		t.Fatal(err)
	}
	b, err := NewBundleFromPodDir(podDir)
	if err != nil {
		t.Fatalf("NewBundleFromPodDir: %v", err)
	}
	if len(b.Members) != 0 {
		t.Errorf("want 0 members, got %d", len(b.Members))
	}
}

// --- WriteBundleToPodDir ---

func TestWriteBundleToPodDir_Creates(t *testing.T) {
	b := fullBundle()
	podDir := t.TempDir()
	if err := WriteBundleToPodDir(podDir, b, false); err != nil {
		t.Fatalf("WriteBundleToPodDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(podDir, PodFileName)); err != nil {
		t.Errorf("pod.toml missing: %v", err)
	}
	for _, m := range b.Members {
		if _, err := os.Stat(MemberPath(podDir, m.Name)); err != nil {
			t.Errorf("member %q missing: %v", m.Name, err)
		}
	}
}

func TestWriteBundleToPodDir_ExistingNoOverwrite_Errors(t *testing.T) {
	b := fullBundle()
	podDir := t.TempDir()
	if err := WriteBundleToPodDir(podDir, b, false); err != nil {
		t.Fatal(err)
	}
	if err := WriteBundleToPodDir(podDir, b, false); err == nil {
		t.Error("want error on existing pod without --overwrite")
	}
}

func TestWriteBundleToPodDir_ExistingWithOverwrite_Succeeds(t *testing.T) {
	b := fullBundle()
	podDir := t.TempDir()
	if err := WriteBundleToPodDir(podDir, b, false); err != nil {
		t.Fatal(err)
	}
	if err := WriteBundleToPodDir(podDir, b, true); err != nil {
		t.Errorf("want success with overwrite, got %v", err)
	}
}

// Full round-trip: NewBundleFromPodDir -> SaveBundle -> LoadBundle -> WriteBundleToPodDir -> NewBundleFromPodDir
func TestBundle_FullRoundTrip(t *testing.T) {
	src := fullBundle()
	srcDir := t.TempDir()
	writePodDir(t, srcDir, src)

	b1, err := NewBundleFromPodDir(srcDir)
	if err != nil {
		t.Fatalf("NewBundleFromPodDir: %v", err)
	}

	var buf bytes.Buffer
	if err := SaveBundle(&buf, b1); err != nil {
		t.Fatalf("SaveBundle: %v", err)
	}

	b2, err := LoadBundle(&buf)
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}

	dstDir := t.TempDir()
	if err := WriteBundleToPodDir(dstDir, b2, false); err != nil {
		t.Fatalf("WriteBundleToPodDir: %v", err)
	}

	b3, err := NewBundleFromPodDir(dstDir)
	if err != nil {
		t.Fatalf("NewBundleFromPodDir (dst): %v", err)
	}

	if !reflect.DeepEqual(b1.Pod, b3.Pod) {
		t.Errorf("pod mismatch after full round-trip")
	}
	if len(b1.Members) != len(b3.Members) {
		t.Errorf("member count mismatch: %d vs %d", len(b1.Members), len(b3.Members))
	}
}
