package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/config"
)

func TestInit_Local_CreatesRoot(t *testing.T) {
	cwd := t.TempDir()
	res, err := Init(cwd, t.TempDir(), config.ModeLocal, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != config.SourceLocal {
		t.Errorf("want local, got %s", res.Source)
	}
	if res.Dir != filepath.Join(cwd, "poddies") {
		t.Errorf("unexpected dir %s", res.Dir)
	}
	if res.AlreadyExisted {
		t.Error("should not be marked already existed")
	}
	if _, err := os.Stat(filepath.Join(res.Dir, ConfigFileName)); err != nil {
		t.Errorf("config.toml missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(res.Dir, PodsDirName)); err != nil {
		t.Errorf("pods/ missing: %v", err)
	}
}

func TestInit_Global_CreatesRoot(t *testing.T) {
	home := t.TempDir()
	res, err := Init(t.TempDir(), home, config.ModeGlobal, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != config.SourceGlobal {
		t.Errorf("want global, got %s", res.Source)
	}
	if res.Dir != filepath.Join(home, ".poddies") {
		t.Errorf("unexpected dir %s", res.Dir)
	}
}

func TestInit_Global_EmptyHome_Errors(t *testing.T) {
	_, err := Init(t.TempDir(), "", config.ModeGlobal, false)
	if err == nil {
		t.Error("want error for empty home")
	}
}

func TestInit_Idempotent_AlreadyInitialized(t *testing.T) {
	cwd := t.TempDir()
	if _, err := Init(cwd, t.TempDir(), config.ModeLocal, false); err != nil {
		t.Fatal(err)
	}
	res2, err := Init(cwd, t.TempDir(), config.ModeLocal, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res2.AlreadyExisted {
		t.Error("want AlreadyExisted on second init")
	}
}

func TestInit_NonEmptyDir_WithoutMarker_Errors(t *testing.T) {
	cwd := t.TempDir()
	dir := filepath.Join(cwd, "poddies")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "random.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Init(cwd, t.TempDir(), config.ModeLocal, false)
	if err == nil {
		t.Error("want error when dir has unrelated content")
	}
}

func TestInit_NonEmptyDir_Force_OK(t *testing.T) {
	cwd := t.TempDir()
	dir := filepath.Join(cwd, "poddies")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "random.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(cwd, t.TempDir(), config.ModeLocal, true); err != nil {
		t.Fatalf("force init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ConfigFileName)); err != nil {
		t.Errorf("config.toml missing after --force: %v", err)
	}
}

func TestInit_RootPath_IsFile_Errors(t *testing.T) {
	cwd := t.TempDir()
	bad := filepath.Join(cwd, "poddies")
	if err := os.WriteFile(bad, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Init(cwd, t.TempDir(), config.ModeLocal, false)
	if err == nil {
		t.Error("want error when path is a file")
	}
}

func TestInit_ConfigFilePermissions(t *testing.T) {
	cwd := t.TempDir()
	res, err := Init(cwd, t.TempDir(), config.ModeLocal, false)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(res.Dir, ConfigFileName))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("want 0600, got %o", got)
	}
}

func TestInit_RootDirPermissions(t *testing.T) {
	cwd := t.TempDir()
	res, err := Init(cwd, t.TempDir(), config.ModeLocal, false)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(res.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("want 0700, got %o", got)
	}
}

func TestInit_UnknownMode_Errors(t *testing.T) {
	_, err := Init(t.TempDir(), t.TempDir(), config.Mode(99), false)
	if err == nil {
		t.Error("want error for unknown mode")
	}
}

// --- cobra wiring ---

func TestInitCmd_Local(t *testing.T) {
	cwd := t.TempDir()
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "init", "--local"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "initialized") {
		t.Errorf("want 'initialized' in output, got %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(cwd, "poddies", ConfigFileName)); err != nil {
		t.Errorf("config.toml missing: %v", err)
	}
}

func TestInitCmd_Global(t *testing.T) {
	home := t.TempDir()
	a, out, _ := newTestApp(t.TempDir(), home)
	if err := runCmd(t, a, "init", "--global"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "initialized") {
		t.Errorf("want 'initialized' in output, got %q", out.String())
	}
}

func TestInitCmd_BothFlagsErrors(t *testing.T) {
	a, _, _ := newTestApp(t.TempDir(), t.TempDir())
	err := runCmd(t, a, "init", "--local", "--global")
	if err == nil {
		t.Error("want error for mutually exclusive flags")
	}
}

func TestInitCmd_Idempotent(t *testing.T) {
	cwd := t.TempDir()
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "init", "--local"); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	a2, out2, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a2, "init", "--local"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out2.String(), "already initialized") {
		t.Errorf("want already-initialized message, got %q", out2.String())
	}
}

func TestInitCmd_DefaultsToLocal(t *testing.T) {
	cwd := t.TempDir()
	a, _, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "init"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "poddies", ConfigFileName)); err != nil {
		t.Errorf("expected local init by default: %v", err)
	}
}
