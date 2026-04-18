package config

import (
	"bytes"
	"flag"
	"os"
	"testing"
)

var updateGolden = flag.Bool("update", false, "regenerate golden files in testdata/")

// goldenCompare fails t unless the on-disk golden at path matches got.
// Run `go test -update` to (re)generate the golden file.
func goldenCompare(t *testing.T, path string, got []byte) {
	t.Helper()
	if *updateGolden {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatalf("writing golden %q: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden %q (run with -update to create): %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("golden %q mismatch\n--- want ---\n%s\n--- got ---\n%s", path, want, got)
	}
}
