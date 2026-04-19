package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestRunCmd_TUIFlagExists confirms the TUI opt-in is wired up. We do
// not spin up a bubbletea program in tests — that requires a TTY-ish
// environment and is better exercised manually. Coverage of the Model
// itself lives in internal/tui/update_test.go.
func TestRunCmd_TUIFlagExists(t *testing.T) {
	a, _, _ := newTestApp(t.TempDir(), t.TempDir())
	root := a.NewRootCmd()
	var run *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "run" {
			run = c
			break
		}
	}
	if run == nil {
		t.Fatal("run command missing")
	}
	f := run.Flags().Lookup("tui")
	if f == nil {
		t.Fatal("--tui flag missing")
	}
	if f.DefValue != "false" {
		t.Errorf("want default false, got %q", f.DefValue)
	}
}

func TestListMemberNames_ReadsToml(t *testing.T) {
	_, root, _, _ := setupPodWithMember(t)
	names, err := listMemberNames(PodDir(root, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "alice" {
		t.Errorf("want [alice], got %v", names)
	}
}
