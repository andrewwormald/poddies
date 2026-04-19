package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/andrewwormald/poddies/internal/thread"
)

var (
	permPaneStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	permItemStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	permHintStyle   = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("220"))
	permHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
)

// renderPermissionsPane renders the pending-permissions pane.
// Returns an empty string when there are no pending requests.
func renderPermissionsPane(pending []thread.Event, width int) string {
	if len(pending) == 0 {
		return ""
	}

	var b strings.Builder
	divider := permPaneStyle.Render(strings.Repeat("─", max(width, 20)))
	b.WriteString(divider + "\n")
	b.WriteString(permHeaderStyle.Render(fmt.Sprintf("⚠  %d pending permission request(s)  [a=approve  d=deny  A=approve-all  D=deny-all]", len(pending))))
	b.WriteByte('\n')

	for _, e := range pending {
		shortID := e.ID
		if len(shortID) > 6 {
			shortID = shortID[:6]
		}
		line := fmt.Sprintf("  %s  from=%-12s  action=%s", shortID, e.From, e.Action)
		b.WriteString(permItemStyle.Render(line))
		b.WriteByte('\n')
	}

	hint := permHintStyle.Render("oldest request is acted on first for a/d")
	b.WriteString(hint)
	return b.String()
}

// shortID returns the first 6 characters of id, or the whole id if shorter.
func shortID(id string) string {
	if len(id) > 6 {
		return id[:6]
	}
	return id
}
