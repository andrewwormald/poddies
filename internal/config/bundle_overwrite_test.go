package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteBundleToPodDir_OverwriteClearsStale guards the data-integrity
// fix: an --overwrite import must not leave members from the previous
// pod behind when the incoming bundle doesn't include them.
func TestWriteBundleToPodDir_OverwriteClearsStale(t *testing.T) {
	podDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(podDir, MembersDirName), 0o700); err != nil {
		t.Fatal(err)
	}

	// pre-populate with a stale member and a pod.toml
	stale := &Member{Name: "old", Title: "T", Adapter: AdapterMock, Model: "m", Effort: EffortHigh}
	if err := SaveMember(podDir, stale); err != nil {
		t.Fatal(err)
	}
	existingPod := &Pod{Name: "target", Lead: "human"}
	if err := SavePod(podDir, existingPod); err != nil {
		t.Fatal(err)
	}

	b := &Bundle{
		SchemaVersion: 1,
		Pod:           Pod{Name: "target", Lead: "human"},
		Members: []Member{
			{Name: "fresh", Title: "T", Adapter: AdapterMock, Model: "m", Effort: EffortHigh},
		},
	}
	if err := WriteBundleToPodDir(podDir, b, true); err != nil {
		t.Fatalf("WriteBundleToPodDir: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(podDir, MembersDirName))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "fresh.toml" {
		t.Errorf("want only fresh.toml after overwrite, got %v", names)
	}
}

// TestWriteBundleToPodDir_NonOverwrite_LeavesStaleAlone confirms that a
// non-overwrite write (pod.toml absent, fresh dir) doesn't accidentally
// touch members that happened to pre-exist.
func TestWriteBundleToPodDir_NonOverwrite_LeavesStaleAlone(t *testing.T) {
	podDir := t.TempDir()
	// members dir exists but no pod.toml yet (unusual but defensible)
	if err := os.MkdirAll(filepath.Join(podDir, MembersDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	stale := &Member{Name: "stray", Title: "T", Adapter: AdapterMock, Model: "m", Effort: EffortHigh}
	if err := SaveMember(podDir, stale); err != nil {
		t.Fatal(err)
	}

	b := &Bundle{
		SchemaVersion: 1,
		Pod:           Pod{Name: "target", Lead: "human"},
		Members: []Member{
			{Name: "alice", Title: "T", Adapter: AdapterMock, Model: "m", Effort: EffortHigh},
		},
	}
	if err := WriteBundleToPodDir(podDir, b, false); err != nil {
		t.Fatalf("WriteBundleToPodDir: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(podDir, MembersDirName))
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	// non-overwrite doesn't clean up; the stray member remains alongside alice.
	if len(names) != 2 {
		t.Errorf("want 2 member files (no overwrite means no cleanup), got %v", names)
	}
}

func TestBundle_Validate_RejectsMemberNameCollidingWithCoS(t *testing.T) {
	b := &Bundle{
		SchemaVersion: 1,
		Pod: Pod{
			Name: "demo",
			Lead: "human",
			ChiefOfStaff: ChiefOfStaff{
				Enabled:  true,
				Name:     "sam",
				Adapter:  AdapterMock,
				Model:    "m",
				Triggers: []Trigger{TriggerMilestone},
			},
		},
		Members: []Member{
			{Name: "sam", Title: "T", Adapter: AdapterMock, Model: "m", Effort: EffortHigh},
		},
	}
	if err := b.Validate(); err == nil {
		t.Error("want collision error for member named 'sam' when CoS is also 'sam'")
	}
}
