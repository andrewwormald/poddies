package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewID_Unique(t *testing.T) {
	const n = 500
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := NewID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id %q at i=%d", id, i)
		}
		seen[id] = struct{}{}
	}
}

func TestLoadIndex_Missing_Empty(t *testing.T) {
	idx, err := LoadIndex(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Sessions) != 0 {
		t.Errorf("want empty, got %+v", idx)
	}
}

func TestCreate_AppendsToIndexAndMakesDir(t *testing.T) {
	root := t.TempDir()
	s, err := Create(root, "default")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(Dir(root, s.ID)); err != nil {
		t.Errorf("session dir missing: %v", err)
	}
	if _, err := os.Stat(ThreadPath(root, s.ID)); err != nil {
		t.Errorf("thread file missing: %v", err)
	}
	idx, _ := LoadIndex(root)
	if len(idx.Sessions) != 1 || idx.Sessions[0].ID != s.ID {
		t.Errorf("index: want [%s], got %+v", s.ID, idx)
	}
}

func TestCreate_MultipleSessions_IndexedInOrder(t *testing.T) {
	root := t.TempDir()
	var ids []string
	for i := 0; i < 3; i++ {
		s, err := Create(root, "default")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, s.ID)
		time.Sleep(1 * time.Millisecond)
	}
	idx, _ := LoadIndex(root)
	if len(idx.Sessions) != 3 {
		t.Fatalf("want 3 sessions, got %d", len(idx.Sessions))
	}
}

func TestTouch_UpdatesFields(t *testing.T) {
	root := t.TempDir()
	s, _ := Create(root, "default")
	originalEdit := s.LastEditedAt
	time.Sleep(5 * time.Millisecond)
	if err := Touch(root, s.ID, 7, "alice"); err != nil {
		t.Fatal(err)
	}
	got, _ := Find(root, s.ID)
	if !got.LastEditedAt.After(originalEdit) {
		t.Errorf("LastEditedAt not advanced")
	}
	if got.TurnCount != 7 {
		t.Errorf("want turns=7, got %d", got.TurnCount)
	}
	if got.LastSpeaker != "alice" {
		t.Errorf("want lastSpeaker=alice, got %q", got.LastSpeaker)
	}
}

func TestTouch_MissingID_Errors(t *testing.T) {
	root := t.TempDir()
	if err := Touch(root, "ghost", 1, "x"); err == nil {
		t.Error("want error for unknown id")
	}
}

func TestListRecent_SortedNewestFirst(t *testing.T) {
	root := t.TempDir()
	a, _ := Create(root, "default")
	time.Sleep(5 * time.Millisecond)
	b, _ := Create(root, "default")
	time.Sleep(5 * time.Millisecond)
	_ = Touch(root, a.ID, 0, "") // a is now more recent than b

	list, err := ListRecent(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2, got %d", len(list))
	}
	if list[0].ID != a.ID {
		t.Errorf("want newest first (a), got %s (b=%s)", list[0].ID, b.ID)
	}
}

func TestFind_Found(t *testing.T) {
	root := t.TempDir()
	s, _ := Create(root, "default")
	got, err := Find(root, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != s.ID {
		t.Errorf("id mismatch")
	}
}

func TestFind_NotFound(t *testing.T) {
	if _, err := Find(t.TempDir(), "ghost"); err == nil {
		t.Error("want error")
	}
}

func TestThreadPath_Shape(t *testing.T) {
	got := ThreadPath("/r", "2026-01-01-000000-abcdef")
	want := filepath.Join("/r", "sessions", "2026-01-01-000000-abcdef", "thread.jsonl")
	if got != want {
		t.Errorf("want %s, got %s", want, got)
	}
}
