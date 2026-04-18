package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"

	"github.com/andrewwormald/poddies/internal/orchestrator"
	"github.com/andrewwormald/poddies/internal/thread"
)

// State is the Model's coarse phase.
type State int

const (
	// StateIdle means no loop is running; the user can type.
	StateIdle State = iota
	// StateRunning means a loop is streaming events into the Model.
	StateRunning
	// StateQuit means the program has been asked to exit.
	StateQuit
)

// StartLoopFn starts an orchestrator loop with the given kickoff
// message. The TUI drains events via onEvent (typically a channel
// send). Implementations run synchronously; the TUI invokes this from
// a goroutine via a tea.Cmd.
type StartLoopFn func(ctx context.Context, kickoff string, onEvent func(thread.Event)) (orchestrator.LoopResult, error)

// Options is the constructor input for Model / Run.
type Options struct {
	PodName   string
	Members   []string // display-only roster for the header
	Lead      string
	StartLoop StartLoopFn
	// InitialKickoff auto-submits this message on first Update tick.
	// Set by `poddies run --tui --message "..."` callers.
	InitialKickoff string
}

// Model is the bubbletea model for the poddies TUI.
type Model struct {
	opts Options

	state    State
	events   []thread.Event
	input    textinput.Model
	viewport viewport.Model

	// streaming subscription: background loop goroutines push tea.Msg
	// values here; Update re-arms a waitForMsg Cmd after each read.
	sub chan any

	// status + error surfaces
	statusLine string
	lastErr    error

	// loop bookkeeping
	lastStop orchestrator.LoopStopReason
	turnsRun int

	// cancel currently-running loop on ctrl-c
	cancelLoop context.CancelFunc

	// layout
	width, height int
	ready         bool
}

// NewModel constructs an initial Model. Callers typically hand it to
// Run which wires a tea.Program around it.
func NewModel(opts Options) Model {
	ti := textinput.New()
	ti.Placeholder = "type a message, Enter to send, Ctrl-C to quit"
	ti.CharLimit = 2000
	ti.Focus()

	vp := viewport.New(80, 10)

	return Model{
		opts:       opts,
		state:      StateIdle,
		input:      ti,
		viewport:   vp,
		sub:        make(chan any, 64),
		statusLine: "ready",
	}
}

// Events returns the events the Model has accumulated. Exported for
// tests that want to assert transcript state without introspecting
// View output.
func (m Model) Events() []thread.Event { return m.events }

// CurrentState returns the coarse phase of the Model. For tests.
func (m Model) CurrentState() State { return m.state }

// Status returns the current status line. For tests.
func (m Model) Status() string { return m.statusLine }
