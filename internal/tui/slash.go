package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// dispatchSlashCommand routes /-prefixed input to the matching handler.
// Unknown commands leave a friendly error in the status line.
func (m Model) dispatchSlashCommand(raw string) (tea.Model, tea.Cmd) {
	cmd := strings.TrimSpace(strings.TrimPrefix(raw, "/"))
	head, _, _ := strings.Cut(cmd, " ")
	_, arg, _ := strings.Cut(cmd, " ")
	arg = strings.TrimSpace(arg)
	switch head {
	case "help", "?":
		m.statusLine = "commands: /add /remove /edit /export /resume /new /help /quit"
	case "quit", "exit":
		if m.cancelLoop != nil {
			m.cancelLoop()
		}
		m.state = StateQuit
		return m, tea.Quit
	case "new":
		// Treat /new like a resume-to-no-target: tea.Quit and let the
		// launch wrapper spin up a fresh session. We piggyback on the
		// resume callback with a sentinel — "" means "fresh session"
		// in the launch loop already, so we just quit.
		if m.cancelLoop != nil {
			m.cancelLoop()
		}
		m.state = StateQuit
		return m, tea.Quit
	case "resume":
		return m.handleResume(arg)
	case "stats":
		m.view = ViewStats
	case "add":
		m = m.activateWizard(addMemberWizard(m.opts))
	case "remove":
		w := removeMemberWizard(m.opts)
		if w == nil {
			m.statusLine = "no members to remove"
			return m, waitForSubMsg(m.sub)
		}
		m = m.activateWizard(w)
	case "edit":
		w := editMemberWizard(m.opts)
		if w == nil {
			m.statusLine = "no members to edit"
			return m, waitForSubMsg(m.sub)
		}
		m = m.activateWizard(w)
	case "export":
		return m.handleExport()
	default:
		m.statusLine = fmt.Sprintf("unknown command: /%s (try /help)", head)
	}
	return m, waitForSubMsg(m.sub)
}

// activateWizard enters StatePrompting with the given wizard. Clears
// the input field so the next keystrokes target the wizard.
func (m Model) activateWizard(w *Wizard) Model {
	m.wizard = w
	m.state = StatePrompting
	m.input.SetValue("")
	m.statusLine = "wizard: " + w.Title
	return m
}

// wizardSubmit feeds the current input to the active wizard's Next
// function and advances / completes it.
func (m Model) wizardSubmit(text string) (tea.Model, tea.Cmd) {
	if m.wizard == nil {
		m.state = StateIdle
		return m, waitForSubMsg(m.sub)
	}
	done, err := m.wizard.Next(text)
	if err != nil {
		m.statusLine = "error: " + err.Error()
		return m, waitForSubMsg(m.sub)
	}
	if done {
		if err := m.wizard.Complete(); err != nil {
			m.lastErr = err
			m.statusLine = "error: " + err.Error()
		} else {
			m.statusLine = m.wizard.Title + ": done"
			m.lastErr = nil
		}
		m.wizard = nil
		m.state = StateIdle
	}
	return m, waitForSubMsg(m.sub)
}

// cancelWizard aborts the active wizard without completing it.
func (m Model) cancelWizard() Model {
	if m.wizard == nil {
		return m
	}
	m.wizard.Cancel()
	m.wizard = nil
	m.state = StateIdle
	m.statusLine = "cancelled"
	return m
}

// handleResume drives /resume.
//
//   - No arg: renders a numbered session list into the transcript and
//     tells the user to pick with /resume 1, /resume 2, … or /resume <id>.
//   - Numeric arg (1-based): resumes that list position.
//   - String arg: ID or prefix match, same as before.
//
// In all resume cases the TUI quits so the launch wrapper can restart
// it bound to the chosen session.
func (m Model) handleResume(arg string) (tea.Model, tea.Cmd) {
	if m.opts.OnListSessions == nil {
		m.statusLine = "/resume is not wired in this TUI session"
		return m, waitForSubMsg(m.sub)
	}
	list := m.opts.OnListSessions()
	if arg == "" {
		if len(list) == 0 {
			m.statusLine = "no prior sessions to resume"
			return m, waitForSubMsg(m.sub)
		}
		var b strings.Builder
		b.WriteString("recent sessions (type /resume <n> or /resume <id>):\n")
		for i, s := range list {
			marker := "» " // current session marker
			if !s.IsCurrent {
				marker = "  "
			}
			last := s.LastSpeaker
			if last == "" {
				last = "-"
			}
			fmt.Fprintf(&b, "%s%d. %s  pod=%s  turns=%d  last=%s  edited=%s\n",
				marker, i+1, s.ID, s.Pod, s.TurnCount, last, s.LastEditedAt)
		}
		m.events = append(m.events, resumePreview(b.String()))
		if m.ready {
			m.viewport.SetContent(renderTranscript(m.events, m.opts.CoSName, m.viewport.Width))
			m.viewport.GotoBottom()
		}
		m.statusLine = fmt.Sprintf("%d session(s) — pick one with /resume <n> or /resume <id>", len(list))
		return m, waitForSubMsg(m.sub)
	}
	// Numeric pick: /resume 1 resumes list[0].
	if idx, err := strconv.Atoi(arg); err == nil {
		if idx < 1 || idx > len(list) {
			m.statusLine = fmt.Sprintf("out of range: %d (1..%d)", idx, len(list))
			return m, waitForSubMsg(m.sub)
		}
		return m.doResume(list[idx-1].ID)
	}
	// ID / prefix match.
	for _, s := range list {
		if s.ID == arg || strings.HasPrefix(s.ID, arg) {
			return m.doResume(s.ID)
		}
	}
	m.statusLine = "no session matching " + arg
	return m, waitForSubMsg(m.sub)
}

// doResume invokes OnResumeSession and quits the TUI so the launch
// wrapper can restart bound to the chosen session.
func (m Model) doResume(id string) (tea.Model, tea.Cmd) {
	if m.opts.OnResumeSession == nil {
		m.statusLine = "/resume is not wired (no callback)"
		return m, waitForSubMsg(m.sub)
	}
	m.opts.OnResumeSession(id)
	if m.cancelLoop != nil {
		m.cancelLoop()
	}
	m.state = StateQuit
	return m, tea.Quit
}

// selectCurrentPod switches to the pod at cursorPos in the :pods view.
func (m Model) selectCurrentPod() (tea.Model, tea.Cmd) {
	if m.opts.OnListPods == nil {
		m.statusLine = "pod listing not wired"
		return m, waitForSubMsg(m.sub)
	}
	pods := m.opts.OnListPods()
	if len(pods) == 0 {
		m.statusLine = "no pods available"
		return m, waitForSubMsg(m.sub)
	}
	pos := m.cursorPos
	if pos >= len(pods) {
		pos = len(pods) - 1
	}
	return m.doSwitchPod(pods[pos])
}

// doSwitchPod records the target pod and quits so the launch wrapper
// can restart bound to the chosen pod.
func (m Model) doSwitchPod(name string) (tea.Model, tea.Cmd) {
	if m.opts.OnSwitchPod == nil {
		m.statusLine = "pod switching not wired"
		return m, waitForSubMsg(m.sub)
	}
	if name == m.opts.PodName {
		// Already on this pod; just return to thread view.
		m.view = ViewThread
		m.statusLine = "already on pod " + name
		return m, waitForSubMsg(m.sub)
	}
	m.opts.OnSwitchPod(name)
	m.switchPodTarget = name
	if m.cancelLoop != nil {
		m.cancelLoop()
	}
	m.state = StateQuit
	return m, tea.Quit
}

// selectCurrentThread resumes the thread at cursorPos in the :threads view.
func (m Model) selectCurrentThread() (tea.Model, tea.Cmd) {
	if m.opts.OnListThreads == nil {
		m.statusLine = "thread listing not wired"
		return m, waitForSubMsg(m.sub)
	}
	threads := m.opts.OnListThreads()
	if len(threads) == 0 {
		m.statusLine = "no threads available"
		return m, waitForSubMsg(m.sub)
	}
	pos := m.cursorPos
	if pos >= len(threads) {
		pos = len(threads) - 1
	}
	// ThreadSummary.Name is the session/thread ID.
	return m.doResume(threads[pos].Name)
}

// handleExport invokes OnExportPod and writes the resulting TOML bundle
// into the thread as a system event so the user has a copy they can
// scroll back to. Non-destructive.
func (m Model) handleExport() (tea.Model, tea.Cmd) {
	if m.opts.OnExportPod == nil {
		m.statusLine = "/export is not wired in this TUI session"
		return m, waitForSubMsg(m.sub)
	}
	data, err := m.opts.OnExportPod()
	if err != nil {
		m.lastErr = err
		m.statusLine = "export failed: " + err.Error()
		return m, waitForSubMsg(m.sub)
	}
	m.statusLine = fmt.Sprintf("exported pod bundle (%d bytes) — copy from below", len(data))
	// push the bundle text into the viewport as a synthetic system-style
	// block so the user can select/copy it without leaving the TUI.
	m.events = append(m.events, exportPreview(data))
	if m.ready {
		m.viewport.SetContent(renderTranscript(m.events, m.opts.CoSName, m.viewport.Width))
		m.viewport.GotoBottom()
	}
	return m, waitForSubMsg(m.sub)
}
