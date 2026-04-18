package main

import (
	"os/exec"
	"strings"
	"testing"
)

// TestBinary_Version builds the binary and runs `poddies --version` to
// verify the cobra wiring actually produces a usable CLI from main().
// Kept as a smoke test — heavier command-level behavior is covered in
// internal/cli tests.
func TestBinary_Version(t *testing.T) {
	out, err := exec.Command("go", "run", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go run .: %v\n%s", err, out)
	}
	// no-arg run: cobra prints usage (contains the command name).
	if !strings.Contains(string(out), "poddies") {
		t.Errorf("expected 'poddies' in output, got %q", out)
	}
}

func TestBinary_VersionFlag(t *testing.T) {
	out, err := exec.Command("go", "run", ".", "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("go run . --version: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), version) {
		t.Errorf("expected version %q in output, got %q", version, out)
	}
}
