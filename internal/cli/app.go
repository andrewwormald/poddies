// Package cli wires the poddies cobra command tree. All subcommands
// hang off App.NewRootCmd so tests can construct an App bound to
// temporary directories and in-memory I/O without touching the user's
// real filesystem or environment.
package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"
)

// Version is set by cmd/poddies/main.go (overridable via -ldflags).
var Version = "0.0.0-dev"

// App holds the per-invocation I/O and environment. Subcommands read
// from these instead of the process globals so tests stay hermetic.
type App struct {
	Out     io.Writer
	Err     io.Writer
	In      io.Reader
	Cwd     string
	Home    string
	EnvRoot string // POD_ROOT override; empty = unset
}

// NewAppFromEnv builds an App from the current process environment,
// using os.Stdin/Stdout/Stderr and the real cwd/home/POD_ROOT.
// Exists as a constructor so cmd/poddies/main.go doesn't hand-assemble.
func NewAppFromEnv() (*App, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return &App{
		Out:     os.Stdout,
		Err:     os.Stderr,
		In:      os.Stdin,
		Cwd:     cwd,
		Home:    os.Getenv("HOME"),
		EnvRoot: os.Getenv("POD_ROOT"),
	}, nil
}

// NewRootCmd constructs the root "poddies" cobra command with all
// subcommands attached. Each call produces a fresh tree so parallel
// tests don't share flag state.
func (a *App) NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "poddies",
		Short:         "Run a pod of AI agents as a shared Slack-thread-style conversation.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       Version,
	}
	root.SetOut(a.Out)
	root.SetErr(a.Err)
	root.SetIn(a.In)

	root.AddCommand(a.newInitCmd(), a.newPodCmd(), a.newMemberCmd(), a.newDoctorCmd())
	return root
}
