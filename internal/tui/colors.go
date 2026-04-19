package tui

import (
	"hash/fnv"

	"github.com/charmbracelet/lipgloss"
)

// palette is a small set of visually-distinct ANSI-256 colours used to
// tint participant names in the transcript. Avoids reds/yellows so
// they stay reserved for warnings / errors.
var palette = []lipgloss.Color{
	"33",  // blue
	"36",  // cyan
	"42",  // green
	"141", // purple
	"208", // orange
	"170", // pink
	"75",  // sky
	"108", // teal
	"180", // tan
	"117", // light blue
}

// Reserved colours for special participants. Using them keeps
// human/CoS visually distinct from the round-robin palette regardless
// of how their names hash.
var (
	humanColor = lipgloss.Color("39")  // bold blue
	cosColor   = lipgloss.Color("201") // magenta — stands out
	systemColor = lipgloss.Color("245") // gray
)

// colorFor returns a deterministic palette colour for a participant
// name. "human" and the zero value map to reserved colours so the UI
// is consistent regardless of who hashes where. Callers wanting the
// chief-of-staff's dedicated colour can use colorForCoS.
func colorFor(name string) lipgloss.Color {
	if name == "" {
		return systemColor
	}
	if name == "human" {
		return humanColor
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return palette[int(h.Sum32())%len(palette)]
}

// colorForCoS returns the reserved chief-of-staff colour. Passed the
// configured CoS name so tests / renderers can style it without
// threading the pod config everywhere.
func colorForCoS() lipgloss.Color { return cosColor }

// styledName renders "[name]" with a bold colour tint. cosName, if
// non-empty, reserves the CoS palette slot for that specific name so
// it pops regardless of hash.
func styledName(name, cosName string) string {
	var c lipgloss.Color
	switch {
	case name == "":
		c = systemColor
	case cosName != "" && name == cosName:
		c = cosColor
	case name == "human":
		c = humanColor
	default:
		c = colorFor(name)
	}
	return lipgloss.NewStyle().Foreground(c).Bold(true).Render("[" + name + "]")
}
