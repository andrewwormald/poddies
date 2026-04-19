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
	// StatePrompting means a Wizard is active; input is captured by the
	// wizard, not the chat loop.
	StatePrompting
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

	// OnApprove is called when the user approves a pending permission
	// request. requestID is the ID of the permission_request event.
	OnApprove func(requestID string) error
	// OnDeny is called when the user denies a pending permission request.
	OnDeny func(requestID, reason string) error
	// GetPending returns the current list of unresolved permission_request
	// events from the backing log. Called after a loop halts with
	// LoopPendingPermission so the Model can populate its pane.
	GetPending func() []thread.Event

	// Slash-command callbacks. Nil means the corresponding command is
	// unavailable and the TUI responds with an error in the status line.
	OnAddMember    func(spec AddMemberSpec) error
	OnRemoveMember func(name string) error
	OnEditMember   func(name, field, value string) error
	OnListMembers  func() []string
	OnExportPod    func() ([]byte, error)
}

// AddMemberSpec bundles the answers from an addMemberWizard so the CLI
// layer can call through to AddMember without the TUI package importing
// cli/config-level types.
type AddMemberSpec struct {
	Name    string
	Title   string
	Adapter string
	Model   string
	Effort  string
	Persona string
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

	// pendingRequests holds unresolved permission_request events.
	// Populated when the loop halts with LoopPendingPermission.
	pendingRequests []thread.Event

	// wizard is active when state == StatePrompting. Input is routed to
	// the wizard's Next() instead of the chat loop. Nil means no wizard.
	wizard *Wizard
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

// PendingRequests returns the pending permission requests held by the
// model. Exported for tests.
func (m Model) PendingRequests() []thread.Event { return m.pendingRequests }

// ActiveWizard returns the currently active wizard, or nil. Exported
// for tests.
func (m Model) ActiveWizard() *Wizard { return m.wizard }

// CurrentState returns the coarse phase of the Model. For tests.
func (m Model) CurrentState() State { return m.state }

// Status returns the current status line. For tests.
func (m Model) Status() string { return m.statusLine }
