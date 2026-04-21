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

// renderWizardModal renders the active wizard as a centered bordered
// box over the full terminal, replacing the thread layout entirely.
// This gives a distinct modal feel (vs. the old footer-replacement),
// matches how Claude Code presents its setup prompts, and keeps the
// wizard visually separate from the transcript.
func (m Model) renderWizardModal() string {
	w := m.wizard
	step := w.CurrentStep()
	if step == nil {
		return m.renderFooter() // shouldn't happen; defensive
	}
	cur, total := w.Progress()

	// Inner content width: total box width minus border (2) and padding (4).
	totalW := m.width - 6
	if totalW < 44 {
		totalW = 44
	}
	if totalW > 72 {
		totalW = 72
	}
	innerW := totalW - 6 // border(2) + padding(4)

	var b strings.Builder
	b.WriteString(headerStyle.Render(fmt.Sprintf("%s · step %d/%d", w.Title, cur, total)))
	b.WriteString("\n")
	if w.Preamble != "" && cur == 1 {
		b.WriteString("\n")
		b.WriteString(metaStyle.Render(wrapText(w.Preamble, innerW)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	choices := w.CurrentStepChoices()
	b.WriteString(wrapText(step.Question, innerW))
	b.WriteByte('\n')
	for i, c := range choices {
		b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, c))
	}
	if len(choices) > 0 && step.AllowCustom {
		b.WriteString(metaStyle.Render("  (or type your own value)\n"))
	}
	if step.Optional {
		b.WriteString(metaStyle.Render("  (optional — press Enter to skip)\n"))
	}
	b.WriteString("\n")
	b.WriteString(m.input.View())
	b.WriteString("\n\n")
	status := m.statusLine
	if m.lastErr != nil {
		status = errStyle.Render(status)
	}
	b.WriteString(metaStyle.Render(status + "   [Esc: cancel]"))

	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(1, 2).
		Width(innerW).
		Render(b.String())

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
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
	if roster := m.currentRoster(); len(roster) > 0 {
		var members []string
		for _, name := range roster {
			av := AvatarFor(name)
			members = append(members, av.RenderSmall()+" "+name)
		}
		parts = append(parts, metaStyle.Render("members: ")+strings.Join(members, "  "))
	}
	if m.opts.Lead != "" {
		lead := m.opts.Lead
		if lead == "human" {
			lead = "me"
		}
		parts = append(parts, metaStyle.Render("lead: "+lead))
	}
	return strings.Join(parts, "  ") + "\n" + metaStyle.Render(strings.Repeat("─", max(m.width, 20)))
}

func (m Model) renderFooter() string {
	divider := metaStyle.Render(strings.Repeat("─", max(m.width, 20)))
	status := m.statusLine
	if m.state == StateRunning {
		status = "running… (type ahead — Enter queues when done)"
	}
	if m.lastErr != nil {
		status = errStyle.Render(status)
	}
	if usage := m.renderUsage(); usage != "" {
		status = status + "  " + usage
	}
	pane := renderPermissionsPane(m.pendingRequests, m.width)
	if pane != "" {
		return pane + "\n" + divider + "\n" + m.renderInputLine() + "\n" + metaStyle.Render(status)
	}
	return divider + "\n" + m.renderInputLine() + "\n" + metaStyle.Render(status)
}

// renderInputLine returns the input view with a faint ghost-text suffix
// appended when an @mention suggestion is active. The ghost appears
// immediately after the cursor, giving the user a preview before Tab
// accepts it.
func (m Model) renderInputLine() string {
	base := m.input.View()
	// Try slash command suggestion first, then @mention.
	ghost, ok := findSlashSuggestion(m.input.Value())
	if !ok {
		ghost, ok = findMentionSuggestion(m.input.Value(), m.currentRoster(), m.opts.CoSName)
	}
	if !ok {
		return base
	}
	return base + lipgloss.NewStyle().Faint(true).Render(ghost)
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
// Per-user colouring and body wrapping are applied so long responses
// don't overflow and speakers are visually distinct.
func renderTranscript(events []thread.Event, cosName string, width int, avSize AvatarSize, debug bool) string {
	if len(events) == 0 {
		return metaStyle.Render("(no events yet — type a message below to kick off)")
	}
	var b strings.Builder
	for _, e := range events {
		// Hide CoS messages when debug is off.
		if !debug && cosName != "" && e.From == cosName {
			continue
		}
		b.WriteString(renderEvent(e, cosName, width, avSize))
		b.WriteByte('\n')
	}
	return b.String()
}

// renderSpeakerLine renders a chat bubble for a speaker event.
//
// Layout (AvatarSmall / AvatarLarge):
//
//	(o.o) alice
//	 ▌ message text here
//	 ▌ continued on next line
//
// The left bar ▌ is coloured with the member's assigned colour.
// AvatarOff omits the pea face but keeps the coloured bar.
func renderSpeakerLine(name, cosName string, body string, width int, avSize AvatarSize) string {
	av := AvatarFor(name)
	barStyle := lipgloss.NewStyle().Foreground(av.Color)
	bar := barStyle.Render("▌")

	// Body width accounts for the " ▌ " prefix (3 chars).
	bubbleWidth := width - 4
	if bubbleWidth < 20 {
		bubbleWidth = 20
	}

	var header string
	switch avSize {
	case AvatarLarge:
		large := av.RenderLarge()
		// If multi-line (has hat), put name on the face line.
		if strings.Contains(large, "\n") {
			parts := strings.SplitN(large, "\n", 2)
			header = " " + parts[0] + "\n" + parts[1] + " " + styledName(name, cosName)
		} else {
			header = large + " " + styledName(name, cosName)
		}
	case AvatarSmall:
		header = av.RenderSmall() + " " + styledName(name, cosName)
	default:
		header = styledName(name, cosName)
	}

	wrapped := wrapText(body, bubbleWidth)
	var b strings.Builder
	b.WriteString(header)
	for _, line := range strings.Split(wrapped, "\n") {
		b.WriteByte('\n')
		b.WriteString(" ")
		b.WriteString(bar)
		b.WriteString(" ")
		b.WriteString(line)
	}
	return b.String()
}

func renderEvent(e thread.Event, cosName string, width int, avSize AvatarSize) string {
	bodyWidth := width - 2
	if bodyWidth < 20 {
		bodyWidth = 20
	}
	switch e.Type {
	case thread.EventHuman:
		return renderSpeakerLine("me", cosName, e.Body, width, avSize)
	case thread.EventMessage:
		from := e.From
		if from == "" {
			from = "?"
		}
		return renderSpeakerLine(from, cosName, e.Body, width, avSize)
	case thread.EventSystem:
		return systemStyle.Render("[system] " + wrapText(e.Body, bodyWidth))
	case thread.EventToolUse:
		from := e.From
		if from == "" {
			from = "?"
		}
		label := fmt.Sprintf("[tool:%s]", e.Action)
		body := e.Body
		if body == "" {
			body = "-"
		}
		return metaStyle.Render(styledName(from, cosName)+" "+label+" "+wrapText(body, bodyWidth))
	case thread.EventPermissionRequest, thread.EventPermissionGrant, thread.EventPermissionDeny:
		return "" // shown in the dedicated permissions pane, not the transcript
	default:
		return systemStyle.Render(fmt.Sprintf("[%s] %s", e.Type, wrapText(e.Body, bodyWidth)))
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
