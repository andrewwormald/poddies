package tui

import (
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	vizPanelW   = 24            // columns occupied by the viz panel (incl. divider)
	nodeSpacing = 3             // connector rows between each node
	linkDur     = 1200 * time.Millisecond
	animTickMs  = 60 * time.Millisecond
)

// vizLink records a single message-flow event between two participants.
// from and to are member names; "" represents the human ("you").
type vizLink struct {
	from    string
	to      string
	startAt time.Time
}

func (l vizLink) progress(now time.Time) float64 {
	p := now.Sub(l.startAt).Seconds() / linkDur.Seconds()
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}

func (l vizLink) expired(now time.Time) bool {
	return now.Sub(l.startAt) >= linkDur
}

// animTickMsg triggers a single animation frame redraw.
type animTickMsg struct{ t time.Time }

func animTick() tea.Cmd {
	return tea.Tick(animTickMs, func(t time.Time) tea.Msg {
		return animTickMsg{t: t}
	})
}

// renderVizPanel draws the right-hand pod visualisation panel.
// panelH is the height to fill (should match viewport height).
func (m Model) renderVizPanel(panelH int) string {
	roster := m.currentRoster()
	now := time.Now()

	// nodes[i]: member names + "" sentinel for the human ("you").
	nodes := make([]string, 0, len(roster)+1)
	nodes = append(nodes, roster...)
	nodes = append(nodes, "") // human always at the bottom

	nodeRow := func(i int) int { return i * (1 + nodeSpacing) }

	// Collect animated dot positions.
	type dot struct {
		row  int
		down bool
	}
	var dots []dot
	for _, lnk := range m.activeLinks {
		if lnk.expired(now) {
			continue
		}
		fi := vizSliceIdx(nodes, lnk.from)
		ti := vizSliceIdx(nodes, lnk.to)
		if fi < 0 || ti < 0 {
			continue
		}
		p := lnk.progress(now)
		fr := float64(nodeRow(fi))
		tr := float64(nodeRow(ti))
		row := int(math.Round(fr + p*(tr-fr)))
		dots = append(dots, dot{row: row, down: tr >= fr})
	}

	dotAt := func(r int) (bool, bool) { // hasDot, goingDown
		for _, d := range dots {
			if d.row == r {
				return true, d.down
			}
		}
		return false, false
	}

	isActive := func(name string) bool {
		for _, lnk := range m.activeLinks {
			if lnk.expired(now) {
				continue
			}
			if lnk.from == name || lnk.to == name {
				return true
			}
		}
		return false
	}

	dotStyle  := lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true)
	liveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true)
	dimStyle  := metaStyle

	totalRows := (len(nodes)-1)*(1+nodeSpacing) + 1

	var rows []string

	// Header line.
	rows = append(rows,
		headerStyle.Render("pod")+"  "+dimStyle.Render("[ v to close ]"))
	rows = append(rows, dimStyle.Render(strings.Repeat("─", vizPanelW-1)))

	for r := 0; r < totalRows && len(rows) < panelH+1; r++ {
		hasDot, goingDown := dotAt(r)
		isNode := r%(1+nodeSpacing) == 0
		idx := r / (1 + nodeSpacing)

		switch {
		case isNode && idx < len(nodes):
			name := nodes[idx]
			displayName := name
			if displayName == "" {
				displayName = "you"
			}
			if hasDot {
				// Dot is passing through / at a node — show a flowing marker.
				ch := "↓"
				if !goingDown {
					ch = "↑"
				}
				rows = append(rows, "  "+dotStyle.Render(ch+" "+displayName))
			} else if isActive(name) {
				node := "◉"
				if name == "" {
					node = "◎"
				}
				rows = append(rows, "  "+liveStyle.Render(node+" "+displayName))
			} else {
				node := "◯"
				if name == "" {
					node = "◎"
				}
				rows = append(rows, "  "+dimStyle.Render(node+" "+displayName))
			}
		case hasDot:
			ch := "↓"
			if !goingDown {
				ch = "↑"
			}
			rows = append(rows, "  "+dotStyle.Render(ch))
		default:
			rows = append(rows, "  "+dimStyle.Render("│"))
		}
	}

	// Pad to panelH so the panel height stays stable.
	for len(rows) < panelH+2 {
		rows = append(rows, "")
	}

	return lipgloss.NewStyle().
		Width(vizPanelW).
		Render(strings.Join(rows, "\n"))
}

// vizSliceIdx returns the index of v in s, or -1.
func vizSliceIdx(s []string, v string) int {
	for i, e := range s {
		if e == v {
			return i
		}
	}
	return -1
}
