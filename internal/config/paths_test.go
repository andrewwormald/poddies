package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// helper — create a directory tree and return the path
func mkTmpDir(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	return dir
}

func TestResolveRoot_LocalExists_NoFlag_PrefersLocal(t *testing.T) {
	cwd := t.TempDir()
	if err := os.Mkdir(filepath.Join(cwd, "poddies"), 0o700); err != nil {
		t.Fatal(err)
	}
	home := mkTmpDir(t, ".poddies")
	homeDir := filepath.Dir(home)

	got, err := ResolveRoot(ModeAuto, cwd, homeDir, "")
	if err != nil {
		t.Fatalf("ResolveRoot: %v", err)
	}
	if got.Source != SourceLocal {
		t.Errorf("want SourceLocal, got %v", got.Source)
	}
	if got.Dir != filepath.Join(cwd, "poddies") {
		t.Errorf("want local dir, got %s", got.Dir)
	}
}

func TestResolveRoot_OnlyGlobalExists_NoFlag_UsesGlobal(t *testing.T) {
	cwd := t.TempDir()
	home := mkTmpDir(t, ".poddies")
	homeDir := filepath.Dir(home)

	got, err := ResolveRoot(ModeAuto, cwd, homeDir, "")
	if err != nil {
		t.Fatalf("ResolveRoot: %v", err)
	}
	if got.Source != SourceGlobal {
		t.Errorf("want SourceGlobal, got %v", got.Source)
	}
	if got.Dir != home {
		t.Errorf("want %s, got %s", home, got.Dir)
	}
}

func TestResolveRoot_NeitherExists_ReturnsErrNoRoot(t *testing.T) {
	cwd := t.TempDir()
	homeDir := t.TempDir()

	_, err := ResolveRoot(ModeAuto, cwd, homeDir, "")
	if !errors.Is(err, ErrNoRoot) {
		t.Fatalf("want ErrNoRoot, got %v", err)
	}
}

func TestResolveRoot_GlobalFlag_IgnoresLocal(t *testing.T) {
	cwd := t.TempDir()
	if err := os.Mkdir(filepath.Join(cwd, "poddies"), 0o700); err != nil {
		t.Fatal(err)
	}
	home := mkTmpDir(t, ".poddies")
	homeDir := filepath.Dir(home)

	got, err := ResolveRoot(ModeGlobal, cwd, homeDir, "")
	if err != nil {
		t.Fatalf("ResolveRoot: %v", err)
	}
	if got.Source != SourceGlobal {
		t.Errorf("want SourceGlobal, got %v", got.Source)
	}
}

func TestResolveRoot_LocalFlag_IgnoresGlobal(t *testing.T) {
	cwd := t.TempDir()
	if err := os.Mkdir(filepath.Join(cwd, "poddies"), 0o700); err != nil {
		t.Fatal(err)
	}
	home := mkTmpDir(t, ".poddies")
	homeDir := filepath.Dir(home)

	got, err := ResolveRoot(ModeLocal, cwd, homeDir, "")
	if err != nil {
		t.Fatalf("ResolveRoot: %v", err)
	}
	if got.Source != SourceLocal {
		t.Errorf("want SourceLocal, got %v", got.Source)
	}
}

func TestResolveRoot_LocalFlag_MissingLocal_ReturnsErrNoRoot(t *testing.T) {
	cwd := t.TempDir()
	home := mkTmpDir(t, ".poddies")
	homeDir := filepath.Dir(home)

	_, err := ResolveRoot(ModeLocal, cwd, homeDir, "")
	if !errors.Is(err, ErrNoRoot) {
		t.Fatalf("want ErrNoRoot, got %v", err)
	}
}

func TestResolveRoot_GlobalFlag_MissingGlobal_ReturnsErrNoRoot(t *testing.T) {
	cwd := t.TempDir()
	homeDir := t.TempDir() // no .poddies inside

	_, err := ResolveRoot(ModeGlobal, cwd, homeDir, "")
	if !errors.Is(err, ErrNoRoot) {
		t.Fatalf("want ErrNoRoot, got %v", err)
	}
}

func TestResolveRoot_EnvOverride_WinsOverLocalAndGlobal(t *testing.T) {
	cwd := t.TempDir()
	if err := os.Mkdir(filepath.Join(cwd, "poddies"), 0o700); err != nil {
		t.Fatal(err)
	}
	home := mkTmpDir(t, ".poddies")
	homeDir := filepath.Dir(home)
	envRoot := mkTmpDir(t, "env-root")

	got, err := ResolveRoot(ModeAuto, cwd, homeDir, envRoot)
	if err != nil {
		t.Fatalf("ResolveRoot: %v", err)
	}
	if got.Source != SourceEnv {
		t.Errorf("want SourceEnv, got %v", got.Source)
	}
	if got.Dir != envRoot {
		t.Errorf("want %s, got %s", envRoot, got.Dir)
	}
}

func TestResolveRoot_EnvOverride_MissingDir_ReturnsError(t *testing.T) {
	cwd := t.TempDir()
	homeDir := t.TempDir()

	_, err := ResolveRoot(ModeAuto, cwd, homeDir, "/definitely/not/a/real/path/xyz")
	if err == nil {
		t.Fatal("expected error for non-existent env root")
	}
}

func TestResolveRoot_HomeUnset_StillFindsLocal(t *testing.T) {
	cwd := t.TempDir()
	if err := os.Mkdir(filepath.Join(cwd, "poddies"), 0o700); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveRoot(ModeAuto, cwd, "", "")
	if err != nil {
		t.Fatalf("ResolveRoot: %v", err)
	}
	if got.Source != SourceLocal {
		t.Errorf("want SourceLocal, got %v", got.Source)
	}
}

func TestResolveRoot_HomeUnset_GlobalMode_Errors(t *testing.T) {
	cwd := t.TempDir()

	_, err := ResolveRoot(ModeGlobal, cwd, "", "")
	if err == nil {
		t.Fatal("expected error when home is unset and global is requested")
	}
}

func TestResolveRoot_LocalPathIsFile_NotDir_Errors(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "poddies"), []byte("oops"), 0o600); err != nil {
		t.Fatal(err)
	}
	homeDir := t.TempDir()

	_, err := ResolveRoot(ModeAuto, cwd, homeDir, "")
	if err == nil {
		t.Fatal("expected error when ./poddies is a file, not a dir")
	}
}

func TestLocalDir_KnownPath(t *testing.T) {
	cwd := "/some/cwd"
	if got := LocalDir(cwd); got != "/some/cwd/poddies" {
		t.Errorf("got %s", got)
	}
}

func TestGlobalDir_KnownPath(t *testing.T) {
	home := "/users/x"
	if got := GlobalDir(home); got != "/users/x/.poddies" {
		t.Errorf("got %s", got)
	}
}

func TestGlobalDir_EmptyHome(t *testing.T) {
	if got := GlobalDir(""); got != "" {
		t.Errorf("want empty string, got %s", got)
	}
}
