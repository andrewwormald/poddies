package tui

import (
	"strings"
	"testing"
)

func TestWrapText_ShortLine_Unchanged(t *testing.T) {
	if got := wrapText("hello", 40); got != "hello" {
		t.Errorf("got %q", got)
	}
}

func TestWrapText_LongLine_WrapsAtSpace(t *testing.T) {
	got := wrapText("hello world foo bar baz", 11)
	// "hello world" = 11 chars, "foo bar baz" = 11 chars → two lines
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), got)
	}
	for _, l := range lines {
		if lenRunes(l) > 11 {
			t.Errorf("line exceeds width: %q", l)
		}
	}
}

func TestWrapText_VeryLongWord_HardBreaks(t *testing.T) {
	got := wrapText("supercalifragilistic", 5)
	lines := strings.Split(got, "\n")
	if len(lines) < 4 {
		t.Errorf("want hard-breaks, got %q", got)
	}
}

func TestWrapText_PreservesExistingNewlines(t *testing.T) {
	got := wrapText("first\nsecond line that is long", 10)
	if !strings.Contains(got, "first\n") {
		t.Errorf("lost first line: %q", got)
	}
}

func TestWrapText_ZeroWidth_Unchanged(t *testing.T) {
	if got := wrapText("hello world", 0); got != "hello world" {
		t.Errorf("got %q", got)
	}
}
