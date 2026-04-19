package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyRoot_NothingToMigrate(t *testing.T) {
	migrated, err := MigrateLegacyRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if migrated {
		t.Error("want migrated=false on empty cwd")
	}
}

func TestMigrateLegacyRoot_MovesLegacyDir(t *testing.T) {
	cwd := t.TempDir()
	legacy := filepath.Join(cwd, "poddies")
	if err := os.MkdirAll(filepath.Join(legacy, "pods"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "config.toml"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	migrated, err := MigrateLegacyRoot(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if !migrated {
		t.Error("want migrated=true")
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("legacy should be gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".poddies", "pods")); err != nil {
		t.Errorf("new root missing pods/: %v", err)
	}
}

func TestMigrateLegacyRoot_BothExist_Errors(t *testing.T) {
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, "poddies"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cwd, ".poddies"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := MigrateLegacyRoot(cwd)
	if err == nil {
		t.Error("want error when both exist")
	}
}
