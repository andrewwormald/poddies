package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/andrewwormald/poddies/internal/orchestrator"
	"github.com/andrewwormald/poddies/internal/thread"
)

// Init implements tea.Model. It arms the initial subscription Cmd.
// When the pod has no members yet, Init queues an onboarding-trigger
// message that opens the addMemberWizard on first tick. Otherwise, if
// an InitialKickoff was configured, it auto-submits that instead.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{waitForSubMsg(m.sub)}
	if m.needsOnboarding() {
		cmds = append(cmds, func() tea.Msg { return startOnboardingMsg{} })
	} else if m.opts.InitialKickoff != "" {
		cmds = append(cmds, func() tea.Msg {
			return autoSubmitMsg{text: m.opts.InitialKickoff}
		})
	}
	return tea.Batch(cmds...)
}

// needsOnboarding reports whether the TUI should open an addMember
// wizard on first tick — true when the pod has no members yet.
func (m Model) needsOnboarding() bool {
	if m.opts.OnAddMember == nil {
		// we can't add members via the TUI in this session, so there's
		// nothing useful onboarding can do. Skip.
		return false
	}
	if m.opts.OnListMembers != nil {
		return len(m.opts.OnListMembers()) == 0
	}
	return len(m.opts.Members) == 0
}

// startOnboardingMsg is an internal signal handled by Update to kick
// off the onboarding wizard on the first tick.
type startOnboardingMsg struct{}

// autoSubmitMsg is an internal signal used by Init to trigger a
// first-tick kickoff without the user having to press Enter. Exported
// via the message interface for clarity in Update.
type autoSubmitMsg struct{ text string }

// waitForSubMsg returns a Cmd that reads one message from the Model's
// subscription channel. After every channel-driven Update, we re-arm
// this Cmd so the channel keeps being drained. The channel is a
// reference type, so the value-semantics Model doesn't lose the
// subscription across Update calls.
func waitForSubMsg(sub <-chan any) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-sub
		if !ok {
			return nil
		}
		return msg
	}
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onResize(msg)
	case tea.KeyMsg:
		return m.onKey(msg)
	case autoSubmitMsg:
		return m.submit(msg.text)
	case startOnboardingMsg:
		m.statusLine = "welcome — let's add your first member"
		m = m.activateWizard(onboardingAddMemberWizard(m.opts))
		return m, waitForSubMsg(m.sub)
	case EventMsg:
		return m.onEvent(msg)
	case LoopDoneMsg:
		return m.onLoopDone(msg)
	case errorMsg:
		m.lastErr = msg.err
		m.statusLine = "error: " + msg.err.Error()
		return m, waitForSubMsg(m.sub)
	}
	return m, nil
}

// onResize updates the viewport and input widths, caching dimensions
// so View can lay out cleanly.
func (m Model) onResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	headerH := 3
	footerH := 3
	vpH := msg.Height - headerH - footerH
	if vpH < 3 {
		vpH = 3
	}
	m.viewport.Width = msg.Width
	m.viewport.Height = vpH
	m.input.Width = msg.Width - 4
	m.ready = true
	m.viewport.SetContent(renderTranscript(m.events, msg.Width))
	m.viewport.GotoBottom()
	return m, nil
}

// onKey dispatches keyboard input based on state.
func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Palette capture takes precedence over everything except Ctrl-C.
	if m.state == StatePalette {
		return m.onPaletteKey(msg)
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		// always quits, cancelling any in-flight loop
		if m.cancelLoop != nil {
			m.cancelLoop()
		}
		m.state = StateQuit
		return m, tea.Quit
	case tea.KeyEsc:
		if m.state == StatePrompting {
			m = m.cancelWizard()
			return m, waitForSubMsg(m.sub)
		}
		// From any non-thread view, Esc returns home.
		if m.view != ViewThread {
			m.view = ViewThread
			m.statusLine = ""
			return m, waitForSubMsg(m.sub)
		}
	case tea.KeyEnter:
		if m.state == StatePrompting {
			text := m.input.Value()
			m.input.SetValue("")
			return m.wizardSubmit(text)
		}
		if m.state == StateIdle && m.view == ViewThread {
			text := m.input.Value()
			if text == "" {
				return m, waitForSubMsg(m.sub)
			}
			m.input.SetValue("")
			return m.submit(text)
		}
	}

	// Global key shortcuts (available from any view in Idle/Palette-less state):
	//   : → open command palette
	//   ? → open help view
	// These are intercepted before input field updates so typing them
	// in the chat would need Shift/etc. (future refinement if needed).
	if m.state == StateIdle && msg.Type == tea.KeyRunes {
		switch string(msg.Runes) {
		case ":":
			if m.view != ViewThread || m.input.Value() == "" {
				m = m.openPalette()
				return m, waitForSubMsg(m.sub)
			}
		case "?":
			if m.view != ViewThread || m.input.Value() == "" {
				m.view = ViewHelp
				return m, waitForSubMsg(m.sub)
			}
		}
	}

	if m.state == StateIdle && len(m.pendingRequests) > 0 {
		switch msg.Type {
		case tea.KeyRunes:
			switch string(msg.Runes) {
			case "a":
				return m.onApprove()
			case "d":
				return m.onDeny()
			case "A":
				return m.onApproveAll()
			case "D":
				return m.onDenyAll()
			}
		}
	}

	if m.state == StateIdle && m.view == ViewThread {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	if m.state == StateIdle || m.state == StatePrompting {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	// during a running loop, scroll the viewport instead of typing.
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// submit appends a human event to the local view (the real event
// lands in the log via the loop's HumanMessage kickoff) and launches
// the loop in a goroutine. If the input starts with "/", it is routed
// to the slash-command dispatcher instead.
func (m Model) submit(text string) (tea.Model, tea.Cmd) {
	if len(text) > 0 && text[0] == '/' {
		return m.dispatchSlashCommand(text)
	}
	if m.state == StateRunning {
		// ignore double-submits (e.g. from maybeResume firing while a
		// previous loop's goroutine is still draining into sub).
		return m, waitForSubMsg(m.sub)
	}
	if m.opts.StartLoop == nil {
		m.lastErr = fmt.Errorf("start-loop not configured")
		m.statusLine = "error: start-loop not configured"
		return m, waitForSubMsg(m.sub)
	}
	m.state = StateRunning
	m.statusLine = "running…"
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelLoop = cancel
	start := m.opts.StartLoop
	sub := m.sub
	go func() {
		result, err := start(ctx, text, func(e thread.Event) {
			sub <- EventMsg{Event: e}
		})
		sub <- LoopDoneMsg{Result: result, Err: err}
	}()
	return m, waitForSubMsg(m.sub)
}

// onEvent appends an event to the transcript and re-arms the
// subscription.
func (m Model) onEvent(msg EventMsg) (tea.Model, tea.Cmd) {
	m.events = append(m.events, msg.Event)
	if m.ready {
		m.viewport.SetContent(renderTranscript(m.events, m.viewport.Width))
		m.viewport.GotoBottom()
	}
	return m, waitForSubMsg(m.sub)
}

// onLoopDone transitions back to Idle and summarizes the run.
// When the loop stopped for pending permissions, populates pendingRequests.
func (m Model) onLoopDone(msg LoopDoneMsg) (tea.Model, tea.Cmd) {
	m.state = StateIdle
	m.cancelLoop = nil
	if msg.Err != nil {
		m.lastErr = msg.Err
		m.statusLine = fmt.Sprintf("error: %v", msg.Err)
	} else {
		m.lastStop = msg.Result.StopReason
		m.turnsRun = msg.Result.TurnsRun
		m.statusLine = fmt.Sprintf("stopped: %s (turns=%d)", msg.Result.StopReason, msg.Result.TurnsRun)
		if msg.Result.StopReason == orchestrator.LoopPendingPermission && m.opts.GetPending != nil {
			m.pendingRequests = m.opts.GetPending()
		}
	}
	return m, waitForSubMsg(m.sub)
}

// onApprove approves the oldest pending request.
func (m Model) onApprove() (tea.Model, tea.Cmd) {
	if len(m.pendingRequests) == 0 || m.opts.OnApprove == nil {
		return m, waitForSubMsg(m.sub)
	}
	req := m.pendingRequests[0]
	m.pendingRequests = m.pendingRequests[1:]
	if err := m.opts.OnApprove(req.ID); err != nil {
		m.lastErr = err
		m.statusLine = "error: " + err.Error()
		return m, waitForSubMsg(m.sub)
	}
	return m.maybeResume()
}

// onDeny denies the oldest pending request.
func (m Model) onDeny() (tea.Model, tea.Cmd) {
	if len(m.pendingRequests) == 0 || m.opts.OnDeny == nil {
		return m, waitForSubMsg(m.sub)
	}
	req := m.pendingRequests[0]
	m.pendingRequests = m.pendingRequests[1:]
	if err := m.opts.OnDeny(req.ID, ""); err != nil {
		m.lastErr = err
		m.statusLine = "error: " + err.Error()
		return m, waitForSubMsg(m.sub)
	}
	return m.maybeResume()
}

// onApproveAll approves all pending requests.
func (m Model) onApproveAll() (tea.Model, tea.Cmd) {
	if len(m.pendingRequests) == 0 || m.opts.OnApprove == nil {
		return m, waitForSubMsg(m.sub)
	}
	for _, req := range m.pendingRequests {
		if err := m.opts.OnApprove(req.ID); err != nil {
			m.lastErr = err
			m.statusLine = "error: " + err.Error()
			m.pendingRequests = nil
			return m, waitForSubMsg(m.sub)
		}
	}
	m.pendingRequests = nil
	return m.maybeResume()
}

// onDenyAll denies all pending requests.
func (m Model) onDenyAll() (tea.Model, tea.Cmd) {
	if len(m.pendingRequests) == 0 || m.opts.OnDeny == nil {
		return m, waitForSubMsg(m.sub)
	}
	for _, req := range m.pendingRequests {
		if err := m.opts.OnDeny(req.ID, ""); err != nil {
			m.lastErr = err
			m.statusLine = "error: " + err.Error()
			m.pendingRequests = nil
			return m, waitForSubMsg(m.sub)
		}
	}
	m.pendingRequests = nil
	return m.maybeResume()
}

// maybeResume re-kicks the loop with an empty message when all pending
// requests have been handled, so agents can continue automatically.
// Guards against re-entry while a loop is already running — without
// this, rapid-fire approve/deny presses (or programmatic sequences)
// can start a second goroutine that races with the first, drops
// messages, and leaks ctx cancellation handles.
func (m Model) maybeResume() (tea.Model, tea.Cmd) {
	if len(m.pendingRequests) > 0 {
		return m, waitForSubMsg(m.sub)
	}
	if m.state == StateRunning {
		return m, waitForSubMsg(m.sub)
	}
	return m.submit("")
}
