package session

import (
	"context"
	"os"
	"testing"
	"time"
)

// makeStaleSession injects a session directly into the index with a
// back-dated LastEditedAt so cleanup tests can exercise the purge
// path without waiting real time.
func makeStaleSession(t *testing.T, root, id string, age time.Duration) {
	t.Helper()
	s := Session{
		ID:           id,
		Pod:          "default",
		CreatedAt:    time.Now().UTC().Add(-age),
		LastEditedAt: time.Now().UTC().Add(-age),
	}
	idx, _ := LoadIndex(root)
	idx.Sessions = append(idx.Sessions, s)
	if err := SaveIndex(root, idx); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(Dir(root, id), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupStale_RemovesOld_KeepsFresh(t *testing.T) {
	root := t.TempDir()
	makeStaleSession(t, root, "old-1", 40*24*time.Hour)
	makeStaleSession(t, root, "old-2", 35*24*time.Hour)
	makeStaleSession(t, root, "recent", 1*time.Hour)

	removed, err := CleanupStale(context.Background(), root, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("CleanupStale: %v", err)
	}
	if removed != 2 {
		t.Errorf("want 2 removed, got %d", removed)
	}

	if _, err := os.Stat(Dir(root, "old-1")); !os.IsNotExist(err) {
		t.Errorf("old-1 should be deleted, err=%v", err)
	}
	if _, err := os.Stat(Dir(root, "recent")); err != nil {
		t.Errorf("recent should remain: %v", err)
	}

	idx, _ := LoadIndex(root)
	if len(idx.Sessions) != 1 || idx.Sessions[0].ID != "recent" {
		t.Errorf("index should only contain 'recent', got %+v", idx)
	}
}

func TestCleanupStale_EmptyIndex_NoOp(t *testing.T) {
	root := t.TempDir()
	removed, err := CleanupStale(context.Background(), root, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Errorf("want 0, got %d", removed)
	}
}

func TestCleanupStale_ContextTimeout_ReturnsCtxErr(t *testing.T) {
	root := t.TempDir()
	makeStaleSession(t, root, "old-1", 40*24*time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	removed, err := CleanupStale(ctx, root, 30*24*time.Hour)
	if err == nil {
		t.Error("want ctx error")
	}
	if removed != 0 {
		t.Errorf("want 0 removed on cancelled ctx, got %d", removed)
	}
	// index preserved — nothing lost
	idx, _ := LoadIndex(root)
	if len(idx.Sessions) != 1 {
		t.Errorf("want 1 session preserved, got %d", len(idx.Sessions))
	}
}
