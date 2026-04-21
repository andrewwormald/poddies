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

	for i, e := range pending {
		num := fmt.Sprintf(" %d.", i+1)
		av := AvatarFor(e.From)
		if e.Action == "dispatch" {
			// Dispatch permission: show agent avatar + task instruction.
			task := e.Body
			if len(task) > 80 {
				task = task[:77] + "..."
			}
			b.WriteString(permItemStyle.Render(num) + " " + av.RenderSmall() + " " + permItemStyle.Render(e.From+": ") + task)
		} else {
			b.WriteString(permItemStyle.Render(fmt.Sprintf("%s %s  action=%s", num, e.From, e.Action)))
		}
		b.WriteByte('\n')
	}

	hint := permHintStyle.Render("  a=approve  d=deny  A=approve-all  D=deny-all  (oldest first)")
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
