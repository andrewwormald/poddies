package tui

import (
	"context"
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the TUI. It blocks until the user quits or an error
// occurs. Stdout/stderr are routed through the tea.Program so
// callers that want to capture output for tests can pass custom
// writers. In production wire this to os.Stdout / os.Stderr.
//
// The provided context is not currently attached to the tea.Program
// (older bubbletea versions lack WithContext); Ctrl-C is the primary
// shutdown path. Cancelling the context stops any in-flight loop via
// the Model's cancelLoop closure, but does not terminate the Program
// on its own.
func Run(ctx context.Context, opts Options, in io.Reader, out io.Writer) error {
	if in == nil || out == nil {
		// tea.NewProgram defaults these to os.Stdin/Stdout.
		p := tea.NewProgram(NewModel(opts), tea.WithAltScreen())
		_, err := p.Run()
		return err
	}
	p := tea.NewProgram(
		NewModel(opts),
		tea.WithAltScreen(),
		tea.WithInput(in),
		tea.WithOutput(out),
	)
	_, err := p.Run()
	return err
}
