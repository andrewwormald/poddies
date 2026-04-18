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

// View implements tea.Model.
func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}

	header := m.renderHeader()
	body := m.viewport.View()
	footer := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Top, header, body, footer)
}

func (m Model) renderHeader() string {
	var parts []string
	parts = append(parts, headerStyle.Render(fmt.Sprintf("poddies · %s", m.opts.PodName)))
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
	return divider + "\n" + m.input.View() + "\n" + metaStyle.Render(status)
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
