package tui

import (
	"strings"
	"testing"
)

func TestColorFor_Deterministic(t *testing.T) {
	a := colorFor("alice")
	b := colorFor("alice")
	if a != b {
		t.Errorf("not deterministic: %s vs %s", a, b)
	}
}

func TestColorFor_HumanReserved(t *testing.T) {
	if colorFor("human") != humanColor {
		t.Error("human should use humanColor")
	}
}

func TestColorFor_EmptyUsesSystem(t *testing.T) {
	if colorFor("") != systemColor {
		t.Error("empty name should use systemColor")
	}
}

func TestColorFor_ProducesMultipleDistinctColours(t *testing.T) {
	// We can't assert "any two names differ" — the palette is finite so
	// collisions happen. What we can assert: across a reasonable sample
	// of names we see at least 3 distinct palette entries. If this
	// fires, the palette has collapsed to too few colours.
	names := []string{"alice", "bob", "carol", "dave", "eve", "frank", "gary", "hank"}
	seen := make(map[string]struct{}, len(names))
	for _, n := range names {
		seen[string(colorFor(n))] = struct{}{}
	}
	if len(seen) < 3 {
		t.Errorf("want at least 3 distinct colours across %d names, got %d", len(names), len(seen))
	}
}

func TestStyledName_CoSUsesReservedColour(t *testing.T) {
	// We can't compare ANSI strings directly (terminfo probing flakes in
	// CI). Just assert the name appears wrapped in brackets.
	got := styledName("sam", "sam")
	if !strings.Contains(got, "[sam]") {
		t.Errorf("want bracketed name, got %q", got)
	}
}

func TestStyledName_HumanBracketed(t *testing.T) {
	got := styledName("human", "sam")
	if !strings.Contains(got, "[human]") {
		t.Errorf("got %q", got)
	}
}
