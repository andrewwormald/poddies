package tui

import (
	"strings"
	"unicode"
)

// wrapText wraps s to at most width cells per line, breaking on word
// boundaries when possible. Preserves existing newlines in s. Used
// for event bodies in the transcript and question text in the wizard
// modal. Width <=0 returns s unchanged (no-op).
func wrapText(s string, width int) string {
	if width <= 0 {
		return s
	}
	// If the text has explicit newlines, wrap each line individually so
	// multi-paragraph bodies preserve their own structure.
	if strings.ContainsRune(s, '\n') {
		parts := strings.Split(s, "\n")
		for i, p := range parts {
			parts[i] = wrapLine(p, width)
		}
		return strings.Join(parts, "\n")
	}
	return wrapLine(s, width)
}

// wrapLine wraps a single logical line to width, breaking on the
// nearest preceding whitespace when a word would overflow. Hard-breaks
// single words longer than width.
func wrapLine(line string, width int) string {
	if width <= 0 || lenRunes(line) <= width {
		return line
	}
	var b strings.Builder
	cur := 0 // runes on current output line
	lastSpace := -1
	runes := []rune(line)
	lineStart := 0
	for i, r := range runes {
		if unicode.IsSpace(r) {
			lastSpace = i
		}
		cur++
		if cur > width {
			// break point: prefer lastSpace (within this output line)
			brk := lastSpace
			if brk < lineStart {
				brk = -1
			}
			if brk >= 0 {
				b.WriteString(string(runes[lineStart:brk]))
				b.WriteByte('\n')
				lineStart = brk + 1 // skip the space
				cur = i - brk
			} else {
				// single long word; hard-break at i
				b.WriteString(string(runes[lineStart:i]))
				b.WriteByte('\n')
				lineStart = i
				cur = 1
			}
			lastSpace = -1
		}
	}
	b.WriteString(string(runes[lineStart:]))
	return b.String()
}

// lenRunes returns the number of runes in s. Used instead of len(s) so
// multi-byte characters count as one column each (close enough for
// ASCII-and-Latin1; a full wcwidth implementation is out of scope).
func lenRunes(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}
