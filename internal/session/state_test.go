package session

import (
	"path/filepath"
	"testing"
)

func TestLoadLastSession_Missing_ReturnsEmpty(t *testing.T) {
	root := t.TempDir()
	id, err := LoadLastSession(root, "demo")
	if err != nil {
		t.Fatalf("LoadLastSession: %v", err)
	}
	if id != "" {
		t.Errorf("want empty ID when state absent, got %q", id)
	}
}

func TestSaveLoadLastSession_RoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := SaveLastSession(root, "demo", "sess-001"); err != nil {
		t.Fatal(err)
	}
	id, err := LoadLastSession(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if id != "sess-001" {
		t.Errorf("want sess-001, got %q", id)
	}
}

func TestSaveLastSession_UpdatesExisting(t *testing.T) {
	root := t.TempDir()
	if err := SaveLastSession(root, "demo", "sess-001"); err != nil {
		t.Fatal(err)
	}
	if err := SaveLastSession(root, "demo", "sess-002"); err != nil {
		t.Fatal(err)
	}
	id, _ := LoadLastSession(root, "demo")
	if id != "sess-002" {
		t.Errorf("want sess-002 after update, got %q", id)
	}
}

func TestSaveLastSession_MultiPod(t *testing.T) {
	root := t.TempDir()
	if err := SaveLastSession(root, "demo", "demo-sess"); err != nil {
		t.Fatal(err)
	}
	if err := SaveLastSession(root, "work", "work-sess"); err != nil {
		t.Fatal(err)
	}
	demo, _ := LoadLastSession(root, "demo")
	work, _ := LoadLastSession(root, "work")
	if demo != "demo-sess" || work != "work-sess" {
		t.Errorf("multi-pod: demo=%q work=%q", demo, work)
	}
}

func TestStatePath_ExpectedLocation(t *testing.T) {
	got := StatePath("/root")
	want := filepath.Join("/root", StateFileName)
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
