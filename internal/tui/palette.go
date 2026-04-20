package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// paletteCommands maps command-palette tokens to the View they open.
// Keep tokens single-word, lowercase, k9s-style. Adding a new view
// means adding one entry here plus its render function.
var paletteCommands = map[string]View{
	"thread":  ViewThread,
	"chat":    ViewThread,
	"members": ViewMembers,
	"pods":    ViewPods,
	"threads": ViewThreads,
	"perms":   ViewPerms,
	"doctor":  ViewDoctor,
	"help":    ViewHelp,
	"?":       ViewHelp,
	"stats":   ViewStats,
}

// openPalette enters palette mode, clearing any prior input.
func (m Model) openPalette() Model {
	m.state = StatePalette
	m.paletteInput = ""
	return m
}

// closePalette returns to the prior state without applying a command.
func (m Model) closePalette() Model {
	m.paletteInput = ""
	// when palette is closed from any non-thread view, stay in that
	// view; otherwise fall back to the normal Idle state.
	m.state = m.stateForView()
	return m
}

// stateForView computes the "natural" state for a view. Thread is the
// only view that drives a chat loop; everything else is read-only and
// sits in StateIdle so input doesn't try to submit to a loop.
func (m Model) stateForView() State {
	return StateIdle
}

// applyPalette consumes m.paletteInput as a view token.
func (m Model) applyPalette() (tea.Model, tea.Cmd) {
	cmd := strings.ToLower(strings.TrimSpace(m.paletteInput))
	cmd = strings.TrimPrefix(cmd, ":")
	m.paletteInput = ""

	if cmd == "quit" || cmd == "exit" || cmd == "q" {
		m.state = StateQuit
		return m, tea.Quit
	}

	view, ok := paletteCommands[cmd]
	if !ok {
		m.statusLine = "unknown command: :" + cmd + " (press : then type 'help')"
		m.state = m.stateForView()
		return m, waitForSubMsg(m.sub)
	}

	m.view = view
	m.cursorPos = 0
	m.state = m.stateForView()
	m.statusLine = ":" + cmd
	return m, waitForSubMsg(m.sub)
}
