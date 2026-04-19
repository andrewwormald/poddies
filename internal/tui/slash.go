package tui

import (
	"fmt"
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

// handleResume drives /resume. With no arg, lists recent sessions
// into the transcript (system events). With an ID, invokes
// OnResumeSession and tea.Quit — the launch wrapper restarts the TUI
// bound to that session.
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
		b.WriteString("recent sessions (type /resume <id>):\n")
		for _, s := range list {
			marker := "  "
			if s.IsCurrent {
				marker = "» "
			}
			last := s.LastSpeaker
			if last == "" {
				last = "-"
			}
			fmt.Fprintf(&b, "%s%s  pod=%s  turns=%d  last=%s  edited=%s\n",
				marker, s.ID, s.Pod, s.TurnCount, last, s.LastEditedAt)
		}
		m.events = append(m.events, resumePreview(b.String()))
		if m.ready {
			m.viewport.SetContent(renderTranscript(m.events, m.opts.CoSName, m.viewport.Width))
			m.viewport.GotoBottom()
		}
		m.statusLine = fmt.Sprintf("%d session(s) — pick one with /resume <id>", len(list))
		return m, waitForSubMsg(m.sub)
	}
	// User supplied an ID (or a prefix of one). Find the best match.
	matched := ""
	for _, s := range list {
		if s.ID == arg || strings.HasPrefix(s.ID, arg) {
			matched = s.ID
			break
		}
	}
	if matched == "" {
		m.statusLine = "no session matching " + arg
		return m, waitForSubMsg(m.sub)
	}
	if m.opts.OnResumeSession == nil {
		m.statusLine = "/resume is not wired (no callback)"
		return m, waitForSubMsg(m.sub)
	}
	m.opts.OnResumeSession(matched)
	if m.cancelLoop != nil {
		m.cancelLoop()
	}
	m.state = StateQuit
	return m, tea.Quit
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
