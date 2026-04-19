package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// onPaletteKey handles keyboard input while the ':' command palette
// is open. Keeps it lightweight — no full textinput component because
// palette entries are always single tokens. Enter submits, Esc
// cancels, Backspace erases one rune, printable runes append.
func (m Model) onPaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		if m.cancelLoop != nil {
			m.cancelLoop()
		}
		m.state = StateQuit
		return m, tea.Quit
	case tea.KeyEsc:
		m = m.closePalette()
		return m, waitForSubMsg(m.sub)
	case tea.KeyEnter:
		return m.applyPalette()
	case tea.KeyBackspace:
		if len(m.paletteInput) > 0 {
			m.paletteInput = m.paletteInput[:len(m.paletteInput)-1]
		}
		return m, waitForSubMsg(m.sub)
	case tea.KeyRunes:
		m.paletteInput += string(msg.Runes)
		return m, waitForSubMsg(m.sub)
	case tea.KeySpace:
		m.paletteInput += " "
		return m, waitForSubMsg(m.sub)
	}
	return m, waitForSubMsg(m.sub)
}
