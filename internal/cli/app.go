// Package cli wires the poddies cobra command tree. All subcommands
// hang off App.NewRootCmd so tests can construct an App bound to
// temporary directories and in-memory I/O without touching the user's
// real filesystem or environment.
package cli

import (
	"io"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/orchestrator"
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

	// AdapterLookup resolves adapter names to implementations. Nil means
	// "use the global adapter registry" (production wiring). Tests
	// inject a map-backed lookup to sidestep the registry entirely.
	AdapterLookup orchestrator.AdapterLookup
}

// stdinIsTTY reports whether a.In is a real terminal. Returns false
// when a.In isn't an *os.File (bytes.Buffer in tests, pipes in CI),
// so those cases correctly skip the interactive TUI launch.
func (a *App) stdinIsTTY() bool {
	f, ok := a.In.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd())
}

// stdoutIsTTY mirrors stdinIsTTY for a.Out.
func (a *App) stdoutIsTTY() bool {
	f, ok := a.Out.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd())
}

// adapterLookup returns a.AdapterLookup if set, otherwise adapter.Get.
func (a *App) adapterLookup() orchestrator.AdapterLookup {
	if a.AdapterLookup != nil {
		return a.AdapterLookup
	}
	return func(name string) (adapter.Adapter, error) {
		return adapter.Get(name)
	}
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
//
// poddies is TUI-first: `poddies` with no subcommand launches the
// bubbletea interface. Subcommands stay wired up as a scripting
// back-door (CI, automation, test harnesses) but are hidden from
// `poddies --help` so the day-to-day surface stays one command.
func (a *App) NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "poddies",
		Short:         "Run a pod of AI agents as a shared Slack-thread-style conversation.",
		Long:          "Run `poddies` to open the TUI. All pod / member / thread management happens inside.\n\nScripting subcommands exist but are hidden — pass --help-scripting to see them.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       Version,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Skip the TUI when stdin/stdout aren't real terminals — CI,
			// `go run .` tests, and piped invocations would hang forever
			// on a TUI. Print help instead so the exit is clean and fast.
			if !a.stdinIsTTY() || !a.stdoutIsTTY() {
				return cmd.Help()
			}
			return a.launchTUI(cmd.Context())
		},
	}
	root.SetOut(a.Out)
	root.SetErr(a.Err)
	root.SetIn(a.In)

	subs := []*cobra.Command{
		a.newInitCmd(),
		a.newPodCmd(),
		a.newMemberCmd(),
		a.newRunCmd(),
		a.newThreadCmd(),
	}
	for _, s := range subs {
		s.Hidden = true
		root.AddCommand(s)
	}
	// doctor stays visible — it's read-only diagnostics, useful before
	// launching the TUI to confirm adapter binaries are present.
	root.AddCommand(a.newDoctorCmd())

	// --help-scripting reveals the hidden commands for users who want
	// to automate pod setup from CI.
	root.Flags().Bool("help-scripting", false, "show hidden scripting subcommands")
	root.PreRun = func(cmd *cobra.Command, args []string) {
		if show, _ := cmd.Flags().GetBool("help-scripting"); show {
			for _, c := range cmd.Commands() {
				c.Hidden = false
			}
		}
	}
	return root
}
