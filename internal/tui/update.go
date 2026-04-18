package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/andrewwormald/poddies/internal/thread"
)

// Init implements tea.Model. It arms the initial subscription Cmd and,
// if an InitialKickoff was configured, auto-submits it.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{waitForSubMsg(m.sub)}
	if m.opts.InitialKickoff != "" {
		cmds = append(cmds, func() tea.Msg {
			return autoSubmitMsg{text: m.opts.InitialKickoff}
		})
	}
	return tea.Batch(cmds...)
}

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
	switch msg.Type {
	case tea.KeyCtrlC:
		// always quits, cancelling any in-flight loop
		if m.cancelLoop != nil {
			m.cancelLoop()
		}
		m.state = StateQuit
		return m, tea.Quit
	case tea.KeyEnter:
		if m.state == StateIdle {
			text := m.input.Value()
			if text == "" {
				return m, waitForSubMsg(m.sub)
			}
			m.input.SetValue("")
			return m.submit(text)
		}
	}

	if m.state == StateIdle {
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
// the loop in a goroutine.
func (m Model) submit(text string) (tea.Model, tea.Cmd) {
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
	}
	return m, waitForSubMsg(m.sub)
}
