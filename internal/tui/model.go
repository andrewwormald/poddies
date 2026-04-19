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
	// StatePalette means the command palette is open (user pressed ':').
	StatePalette
	// StateQuit means the program has been asked to exit.
	StateQuit
)

// View identifies which pane the TUI is currently rendering. Most
// views are read-only in v1; rich CRUD happens via slash commands
// inside ViewThread or by exiting back to the thread view.
type View int

const (
	// ViewThread is the chat pane. Default landing view.
	ViewThread View = iota
	// ViewMembers is a read-only list of pod members.
	ViewMembers
	// ViewPods is a read-only list of pods under the root.
	ViewPods
	// ViewThreads is a read-only list of threads for the current pod.
	ViewThreads
	// ViewPerms is a dedicated view for pending permission requests
	// (same data as the thread-embedded pane, just full-screen).
	ViewPerms
	// ViewDoctor runs the adapter/root health checks and displays them.
	ViewDoctor
	// ViewHelp is the help overlay.
	ViewHelp
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

	// Read-only callbacks for the :pods / :threads / :doctor views.
	OnListPods    func() []string
	OnListThreads func() []ThreadSummary
	OnDoctor      func() []DoctorCheck
}

// ThreadSummary is the minimum info the threads-view needs.
type ThreadSummary struct {
	Name       string
	Events     int
	LastFrom   string
	ModifiedAt string
	Corrupt    bool
}

// DoctorCheck is one row of the :doctor view.
type DoctorCheck struct {
	Name    string
	Status  string // "pass" | "warn" | "fail"
	Message string
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

	// view is the currently-rendered pane. Command palette and :cmd
	// invocations mutate this; Esc returns to ViewThread from any
	// other view.
	view View

	// paletteInput is the text buffer while the : palette is open.
	paletteInput string
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

// ActiveView returns which pane the TUI is rendering. Exported for
// tests so they don't need to rely on View() string output.
func (m Model) ActiveView() View { return m.view }

// CurrentState returns the coarse phase of the Model. For tests.
func (m Model) CurrentState() State { return m.state }

// Status returns the current status line. For tests.
func (m Model) Status() string { return m.statusLine }
