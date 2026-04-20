package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderActiveView dispatches to the view-specific renderer based on
// m.view. The chat view (ViewThread) keeps the existing header +
// viewport + footer layout; other views draw a simple title + list.
func (m Model) renderActiveView() string {
	switch m.view {
	case ViewMembers:
		return m.renderMembersView()
	case ViewPods:
		return m.renderPodsView()
	case ViewThreads:
		return m.renderThreadsView()
	case ViewPerms:
		return m.renderPermsView()
	case ViewDoctor:
		return m.renderDoctorView()
	case ViewHelp:
		return m.renderHelpView()
	case ViewStats:
		return m.renderStatsView()
	default:
		return m.renderThreadView()
	}
}

func (m Model) renderThreadView() string {
	header := m.renderHeader()
	body := m.viewport.View()
	var footer string
	if m.state == StatePalette {
		footer = m.renderPaletteFooter()
	} else if m.state == StatePrompting && m.wizard != nil {
		return m.renderWizardModal()
	} else {
		footer = m.renderFooter()
	}
	return lipgloss.JoinVertical(lipgloss.Top, header, body, footer)
}

func (m Model) renderViewFrame(title, body string) string {
	header := headerStyle.Render("poddies · " + title) + "\n" +
		metaStyle.Render(strings.Repeat("─", max(m.width, 20)))
	var footer string
	if m.state == StatePalette {
		footer = m.renderPaletteFooter()
	} else {
		hints := ":cmd for palette · Esc returns to :thread · ? help"
		footer = metaStyle.Render(strings.Repeat("─", max(m.width, 20))) + "\n" + metaStyle.Render(hints)
	}
	return lipgloss.JoinVertical(lipgloss.Top, header, body, footer)
}

func (m Model) renderMembersView() string {
	roster := m.currentRoster()
	if len(roster) == 0 {
		return m.renderViewFrame(":members", metaStyle.Render("\n  (no members — return to :thread and type /add)\n"))
	}
	var b strings.Builder
	b.WriteString("\n")
	for _, name := range roster {
		fmt.Fprintf(&b, "  %s\n", lipgloss.NewStyle().Bold(true).Render(name))
	}
	b.WriteString("\n")
	b.WriteString(metaStyle.Render("  use /add /remove /edit from :thread to modify"))
	return m.renderViewFrame(":members · "+m.opts.PodName, b.String())
}

func (m Model) renderPodsView() string {
	if m.opts.OnListPods == nil {
		return m.renderViewFrame(":pods", metaStyle.Render("\n  (pod listing not wired in this session)\n"))
	}
	pods := m.opts.OnListPods()
	var b strings.Builder
	b.WriteString("\n")
	if len(pods) == 0 {
		b.WriteString(metaStyle.Render("  (no pods)\n"))
	} else {
		for _, p := range pods {
			marker := "  "
			if p == m.opts.PodName {
				marker = "» "
			}
			fmt.Fprintf(&b, "%s%s\n", marker, lipgloss.NewStyle().Bold(true).Render(p))
		}
	}
	b.WriteString("\n")
	b.WriteString(metaStyle.Render("  » current pod · switching via TUI lands in a future phase"))
	return m.renderViewFrame(":pods", b.String())
}

func (m Model) renderThreadsView() string {
	if m.opts.OnListThreads == nil {
		return m.renderViewFrame(":threads", metaStyle.Render("\n  (thread listing not wired in this session)\n"))
	}
	ts := m.opts.OnListThreads()
	var b strings.Builder
	b.WriteString("\n")
	if len(ts) == 0 {
		b.WriteString(metaStyle.Render("  (no threads yet)\n"))
	} else {
		for _, t := range ts {
			if t.Corrupt {
				fmt.Fprintf(&b, "  %s  %s\n", errStyle.Render("["+t.Name+"]"), metaStyle.Render("CORRUPT"))
				continue
			}
			last := t.LastFrom
			if last == "" {
				last = "-"
			}
			fmt.Fprintf(&b, "  %-24s events=%-4d last=%-12s  %s\n",
				lipgloss.NewStyle().Bold(true).Render(t.Name),
				t.Events, last, t.ModifiedAt)
		}
	}
	return m.renderViewFrame(":threads · "+m.opts.PodName, b.String())
}

func (m Model) renderPermsView() string {
	var b strings.Builder
	b.WriteString("\n")
	if len(m.pendingRequests) == 0 {
		b.WriteString(metaStyle.Render("  (no pending permission requests)\n"))
	} else {
		for _, r := range m.pendingRequests {
			fmt.Fprintf(&b, "  %s  from=%s  action=%s\n",
				lipgloss.NewStyle().Bold(true).Render(shortID(r.ID)),
				r.From, r.Action)
		}
		b.WriteString("\n")
		b.WriteString(metaStyle.Render("  keys: a approve · d deny · A approve all · D deny all"))
	}
	return m.renderViewFrame(":perms", b.String())
}

func (m Model) renderDoctorView() string {
	if m.opts.OnDoctor == nil {
		return m.renderViewFrame(":doctor", metaStyle.Render("\n  (doctor not wired in this session)\n"))
	}
	checks := m.opts.OnDoctor()
	var b strings.Builder
	b.WriteString("\n")
	for _, c := range checks {
		badge := "[" + c.Status + "]"
		switch c.Status {
		case "pass":
			badge = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(badge)
		case "warn":
			badge = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render(badge)
		case "fail":
			badge = errStyle.Render(badge)
		}
		fmt.Fprintf(&b, "  %s %-18s %s\n", badge, c.Name, c.Message)
	}
	return m.renderViewFrame(":doctor", b.String())
}

func (m Model) renderHelpView() string {
	body := `
  Command palette:  press ':' then type one of
    :thread    chat view (default)
    :members   pod member list
    :pods      pods in this root
    :threads   threads in this pod
    :perms     pending permission requests
    :doctor    adapter + root health check
    :help      this screen
    :quit      exit

  In the chat view:
    /add, /remove, /edit, /export, /help, /quit
    Esc cancels an active wizard
    a / d / A / D  approve / deny pending permissions

  Global:
    :   open palette
    ?   open this help
    Esc go back to :thread
    Ctrl-C  quit (also cancels an in-flight loop)
`
	return m.renderViewFrame(":help", body)
}

func (m Model) renderStatsView() string {
	var b strings.Builder

	// Thread totals section.
	b.WriteString("\n  Thread totals\n")
	b.WriteString(metaStyle.Render("  " + strings.Repeat("─", 36)) + "\n")
	if m.opts.OnUsageSnapshot != nil {
		snap := m.opts.OnUsageSnapshot()
		fmt.Fprintf(&b, "  input tokens   %d\n", snap.InputTokens)
		fmt.Fprintf(&b, "  output tokens  %d\n", snap.OutputTokens)
		fmt.Fprintf(&b, "  cost USD       %.4f\n", snap.CostUSD)
		fmt.Fprintf(&b, "  turns          %d\n", snap.TurnCount)
	} else {
		b.WriteString(metaStyle.Render("  (stats not wired — OnUsageSnapshot is nil)") + "\n")
	}

	// Per-member message counts derived from in-session events.
	counts := map[string]int{}
	humanCount := 0
	for _, e := range m.events {
		switch e.Type {
		case "message":
			if e.From != "" {
				counts[e.From]++
			}
		case "human":
			humanCount++
		}
	}

	b.WriteString("\n  Message counts (this session)\n")
	b.WriteString(metaStyle.Render("  "+strings.Repeat("─", 36)) + "\n")
	for name, n := range counts {
		fmt.Fprintf(&b, "  %-20s %d\n", name, n)
	}
	if humanCount > 0 || len(counts) == 0 {
		fmt.Fprintf(&b, "  %-20s %d\n", "human", humanCount)
	}

	return m.renderViewFrame(":stats", b.String())
}

// currentRoster returns the member names for the :members view,
// falling back to Options.Members when the dynamic callback isn't
// wired.
func (m Model) currentRoster() []string {
	if m.opts.OnListMembers != nil {
		return m.opts.OnListMembers()
	}
	return m.opts.Members
}

// renderPaletteFooter replaces the footer while StatePalette is active.
func (m Model) renderPaletteFooter() string {
	divider := metaStyle.Render(strings.Repeat("─", max(m.width, 20)))
	return divider + "\n" + ":" + m.paletteInput + "_" + "\n" +
		metaStyle.Render("type a command (thread, members, pods, threads, perms, doctor, help, quit) · Enter submits · Esc cancels")
}
