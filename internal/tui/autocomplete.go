package tui

import (
	"sort"
	"strings"
)

// MentionCandidate is one possible completion for an @mention.
type MentionCandidate struct {
	Name   string // the completed name (without the '@')
	IsCoS  bool   // true if this is the chief-of-staff
}

// mentionPrefix returns the partial @mention at the end of input — the
// text after the last '@' if the current run of trailing characters
// started with '@' and no whitespace intervened. Returns ok=false
// when there's no active @mention being typed.
func mentionPrefix(input string) (prefix string, ok bool) {
	// Walk backward over a slug-compatible run of characters until we
	// hit '@' (accept) or whitespace / start-of-string (reject).
	for i := len(input) - 1; i >= 0; i-- {
		c := input[i]
		if c == '@' {
			return input[i+1:], true
		}
		if c == ' ' || c == '\t' || c == '\n' {
			return "", false
		}
		if !isSlugByte(c) {
			return "", false
		}
	}
	return "", false
}

// isSlugByte matches the same charset as config.ValidateSlug so we
// auto-complete against real member names and don't trigger on, say,
// mid-word punctuation.
func isSlugByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z':
		return true
	case b >= 'A' && b <= 'Z':
		return true
	case b >= '0' && b <= '9':
		return true
	case b == '-':
		return true
	default:
		return false
	}
}

// slashCommands is the list of available slash commands for autocomplete.
var slashCommands = []string{
	"add", "clear", "debug-restart", "edit", "export",
	"help", "new", "quit", "remove", "resume", "stats",
}

// slashPrefix returns the partial command after '/' at the start of
// input. Returns ok=false when input doesn't start with '/'.
func slashPrefix(input string) (prefix string, ok bool) {
	if !strings.HasPrefix(input, "/") {
		return "", false
	}
	// Only the first word matters — no space yet.
	if strings.Contains(input[1:], " ") {
		return "", false
	}
	return input[1:], true
}

// findSlashSuggestion picks the best slash command to show as ghost text.
func findSlashSuggestion(input string) (suggestion string, ok bool) {
	prefix, active := slashPrefix(input)
	if !active {
		return "", false
	}
	for _, cmd := range slashCommands {
		if cmd == prefix {
			return "", false // exact match
		}
		if strings.HasPrefix(cmd, prefix) {
			return cmd[len(prefix):], true
		}
	}
	return "", false
}

// applySlashSuggestion returns input with the ghost suffix accepted.
func applySlashSuggestion(input string) string {
	suffix, ok := findSlashSuggestion(input)
	if !ok {
		return input
	}
	return input + suffix
}

// findMentionSuggestion picks the best single candidate to show as a
// ghost suffix. Deterministic: sorted alphabetical, first prefix match
// wins. Returns ("", false) when no active @mention or no match.
func findMentionSuggestion(input string, members []string, cosName string) (suggestion string, ok bool) {
	prefix, active := mentionPrefix(input)
	if !active {
		return "", false
	}
	lowerPrefix := strings.ToLower(prefix)
	candidates := mentionCandidates(members, cosName)
	for _, c := range candidates {
		if strings.EqualFold(c.Name, prefix) {
			// exact match → no ghost needed
			return "", false
		}
		if strings.HasPrefix(strings.ToLower(c.Name), lowerPrefix) {
			return c.Name[len(prefix):], true
		}
	}
	return "", false
}

// mentionCandidates returns the sorted slice of names eligible for
// mention completion: members first, then the CoS if set and not
// already included.
func mentionCandidates(members []string, cosName string) []MentionCandidate {
	out := make([]MentionCandidate, 0, len(members)+1)
	for _, m := range members {
		out = append(out, MentionCandidate{Name: m})
	}
	if cosName != "" {
		already := false
		for _, m := range members {
			if m == cosName {
				already = true
				break
			}
		}
		if !already {
			out = append(out, MentionCandidate{Name: cosName, IsCoS: true})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// applySuggestion returns input with the ghost suffix accepted. Always
// appends a space so the user can keep typing. Returns input unchanged
// when there's no active suggestion.
func applySuggestion(input string, members []string, cosName string) string {
	suffix, ok := findMentionSuggestion(input, members, cosName)
	if !ok {
		return input
	}
	return input + suffix + " "
}
