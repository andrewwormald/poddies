package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/andrewwormald/poddies/internal/thread"
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	metaStyle   = lipgloss.NewStyle().Faint(true)
	humanStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true)
	systemStyle = lipgloss.NewStyle().Faint(true)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)

// View implements tea.Model. Dispatches to the active view's renderer.
func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}
	return m.renderActiveView()
}

// renderWizard replaces the footer pane when a wizard is active.
func (m Model) renderWizard() string {
	w := m.wizard
	step := w.CurrentStep()
	if step == nil {
		return m.renderFooter()
	}
	cur, total := w.Progress()
	var b strings.Builder
	b.WriteString(metaStyle.Render(strings.Repeat("─", max(m.width, 20))))
	b.WriteByte('\n')
	b.WriteString(headerStyle.Render(fmt.Sprintf("%s · step %d/%d", w.Title, cur, total)))
	b.WriteByte('\n')
	b.WriteString(step.Question)
	b.WriteByte('\n')
	for i, c := range step.Choices {
		b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, c))
	}
	if len(step.Choices) > 0 && step.AllowCustom {
		b.WriteString(metaStyle.Render("  (or type your own value)\n"))
	}
	if step.Optional {
		b.WriteString(metaStyle.Render("  (optional — press Enter to skip)\n"))
	}
	b.WriteString(m.input.View())
	b.WriteByte('\n')
	b.WriteString(metaStyle.Render(m.statusLine + "   [Esc: cancel]"))
	return b.String()
}

func (m Model) renderHeader() string {
	var parts []string
	parts = append(parts, headerStyle.Render(fmt.Sprintf("poddies · %s", m.opts.PodName)))
	if m.opts.SessionID != "" {
		short := m.opts.SessionID
		if len(short) > 19 {
			short = short[:19]
		}
		parts = append(parts, metaStyle.Render("session: "+short))
	}
	if len(m.opts.Members) > 0 {
		parts = append(parts, metaStyle.Render("members: "+strings.Join(m.opts.Members, ", ")))
	}
	if m.opts.Lead != "" {
		parts = append(parts, metaStyle.Render("lead: "+m.opts.Lead))
	}
	return strings.Join(parts, "  ") + "\n" + metaStyle.Render(strings.Repeat("─", max(m.width, 20)))
}

func (m Model) renderFooter() string {
	divider := metaStyle.Render(strings.Repeat("─", max(m.width, 20)))
	status := m.statusLine
	if m.state == StateRunning {
		status = "running… (input disabled)"
	}
	if m.lastErr != nil {
		status = errStyle.Render(status)
	}
	if usage := m.renderUsage(); usage != "" {
		status = status + "  " + usage
	}
	pane := renderPermissionsPane(m.pendingRequests, m.width)
	if pane != "" {
		return pane + "\n" + divider + "\n" + m.input.View() + "\n" + metaStyle.Render(status)
	}
	return divider + "\n" + m.input.View() + "\n" + metaStyle.Render(status)
}

// renderUsage formats the cumulative token counter for the footer.
// Empty when the callback isn't wired or the counter is still zero —
// avoids drawing "0 tokens" before anything has happened.
func (m Model) renderUsage() string {
	if m.opts.OnUsageSnapshot == nil {
		return ""
	}
	s := m.opts.OnUsageSnapshot()
	if s.TurnCount == 0 {
		return ""
	}
	txt := fmt.Sprintf("%d turns · %d tokens (in %d / out %d)", s.TurnCount, s.TotalTokens(), s.InputTokens, s.OutputTokens)
	if s.CostUSD > 0 {
		txt += fmt.Sprintf(" · $%.4f", s.CostUSD)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(txt)
}

// renderTranscript formats the event list into the viewport-ready text.
// Keeps formatting consistent with the stdout renderer so users moving
// between modes see the same shape.
func renderTranscript(events []thread.Event, width int) string {
	if len(events) == 0 {
		return metaStyle.Render("(no events yet — type a message below to kick off)")
	}
	var b strings.Builder
	for _, e := range events {
		b.WriteString(renderEvent(e, width))
		b.WriteByte('\n')
	}
	return b.String()
}

func renderEvent(e thread.Event, width int) string {
	switch e.Type {
	case thread.EventHuman:
		return humanStyle.Render("[human]") + " " + e.Body
	case thread.EventMessage:
		from := e.From
		if from == "" {
			from = "?"
		}
		return lipgloss.NewStyle().Bold(true).Render("["+from+"]") + " " + e.Body
	case thread.EventSystem:
		return systemStyle.Render("[system] " + e.Body)
	case thread.EventPermissionRequest:
		return errStyle.Render(fmt.Sprintf("[permission_request from %s] action=%s", e.From, e.Action))
	case thread.EventPermissionGrant:
		return systemStyle.Render(fmt.Sprintf("[permission_grant by %s for %s]", e.From, e.RequestID))
	case thread.EventPermissionDeny:
		return systemStyle.Render(fmt.Sprintf("[permission_deny by %s for %s]", e.From, e.RequestID))
	default:
		return systemStyle.Render(fmt.Sprintf("[%s] %s", e.Type, e.Body))
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
