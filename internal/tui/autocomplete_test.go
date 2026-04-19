package tui

import "testing"

func TestMentionPrefix_Active(t *testing.T) {
	cases := map[string]string{
		"@":            "",
		"@a":           "a",
		"@ali":         "ali",
		"hi @bo":       "bo",
		"@alice @bo":   "bo",
		"@-foo":        "-foo", // not a valid slug prefix but still extracted
		"(cc @carol":   "carol",
	}
	for in, want := range cases {
		got, ok := mentionPrefix(in)
		if !ok {
			t.Errorf("mentionPrefix(%q) ok=false, want true", in)
			continue
		}
		if got != want {
			t.Errorf("mentionPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMentionPrefix_Inactive(t *testing.T) {
	for _, in := range []string{
		"",
		"plain text",
		"@alice done",                 // trailing space closes it
		"@alice\nsecond line",         // newline closes
		"alice",                       // no @ at all
	} {
		if _, ok := mentionPrefix(in); ok {
			t.Errorf("mentionPrefix(%q) ok=true, want false", in)
		}
	}
}

func TestFindMentionSuggestion_PrefixMatch(t *testing.T) {
	suffix, ok := findMentionSuggestion("hi @al", []string{"alice", "bob"}, "")
	if !ok || suffix != "ice" {
		t.Errorf("want 'ice', got %q ok=%v", suffix, ok)
	}
}

func TestFindMentionSuggestion_ExactMatch_NoGhost(t *testing.T) {
	_, ok := findMentionSuggestion("@alice", []string{"alice", "bob"}, "")
	if ok {
		t.Error("exact match should produce no ghost")
	}
}

func TestFindMentionSuggestion_NoMembers(t *testing.T) {
	_, ok := findMentionSuggestion("@al", nil, "")
	if ok {
		t.Error("empty roster should produce no suggestion")
	}
}

func TestFindMentionSuggestion_CoS_Included(t *testing.T) {
	suffix, ok := findMentionSuggestion("@sa", []string{"alice"}, "sam")
	if !ok || suffix != "m" {
		t.Errorf("want 'm' for CoS sam, got %q ok=%v", suffix, ok)
	}
}

func TestFindMentionSuggestion_CoS_DedupedWithMember(t *testing.T) {
	// CoS name happens to equal a member name (config validator blocks
	// this at pod level, but the autocomplete function should be
	// robust). Should not double-count.
	_, ok := findMentionSuggestion("@al", []string{"alice"}, "alice")
	if !ok {
		t.Error("should still suggest")
	}
}

func TestFindMentionSuggestion_AlphabeticalTiebreak(t *testing.T) {
	// Both "alice" and "allan" start with "al" — alphabetical first
	// (alice) wins.
	suffix, ok := findMentionSuggestion("@al", []string{"allan", "alice"}, "")
	if !ok || suffix != "ice" {
		t.Errorf("want alphabetical 'alice' first; got suffix %q ok=%v", suffix, ok)
	}
}

func TestApplySuggestion_AcceptsAndTrailingSpace(t *testing.T) {
	got := applySuggestion("hi @al", []string{"alice"}, "")
	if got != "hi @alice " {
		t.Errorf("want 'hi @alice ', got %q", got)
	}
}

func TestApplySuggestion_NoSuggestion_Unchanged(t *testing.T) {
	got := applySuggestion("hello", []string{"alice"}, "")
	if got != "hello" {
		t.Errorf("want unchanged, got %q", got)
	}
}
