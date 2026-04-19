package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListThreads_CorruptFile_Flagged(t *testing.T) {
	_, root, _, _ := setupPodWithMember(t)
	threadsDir := filepath.Join(PodDir(root, "demo"), ThreadsDirName)
	if err := os.MkdirAll(threadsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(threadsDir, "bad.jsonl"), []byte("not valid json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ListThreads(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d", len(got))
	}
	if !got[0].Corrupt {
		t.Error("want Corrupt=true on broken file")
	}
}

func TestThreadListCmd_ShowsCorruptMarker(t *testing.T) {
	cwd, root, _, _ := setupPodWithMember(t)
	threadsDir := filepath.Join(PodDir(root, "demo"), ThreadsDirName)
	if err := os.MkdirAll(threadsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(threadsDir, "bad.jsonl"), []byte("garbage\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "thread", "list", "--pod", "demo"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "CORRUPT") {
		t.Errorf("want CORRUPT marker, got:\n%s", out.String())
	}
}
