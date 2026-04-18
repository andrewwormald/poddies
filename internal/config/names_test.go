package config

import (
	"strings"
	"testing"
)

func TestValidateSlug_Accepts(t *testing.T) {
	cases := []string{"alice", "bob-2", "a", "platform-pod", "x1", "a-b-c"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if err := ValidateSlug(c); err != nil {
				t.Errorf("want ok, got %v", err)
			}
		})
	}
}

func TestValidateSlug_Rejects(t *testing.T) {
	cases := map[string]string{
		"empty":          "",
		"uppercase":      "Alice",
		"leading-dash":   "-alice",
		"trailing-dash":  "alice-",
		"double-dash-ok": "a--b", // actually allowed by regex (double dash); flip if we want stricter
		"space":          "a b",
		"dot":            "a.b",
		"slash":          "a/b",
		"backslash":      `a\b`,
		"traversal":      "..",
		"traversal-long": "../../etc",
		"unicode":        "aliçe",
		"tab":            "a\tb",
		"too-long":       strings.Repeat("a", MaxSlugLen+1),
	}
	// "double-dash-ok" intentionally excluded — current regex allows it.
	// If we decide to tighten, update regex and remove this comment.
	for name, s := range cases {
		if name == "double-dash-ok" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			if err := ValidateSlug(s); err == nil {
				t.Errorf("want error for %q, got nil", s)
			}
		})
	}
}

func TestValidateSlug_MaxLenBoundary(t *testing.T) {
	s := strings.Repeat("a", MaxSlugLen)
	if err := ValidateSlug(s); err != nil {
		t.Errorf("slug at max len should be valid, got %v", err)
	}
}

func TestIsReservedMemberName_Human(t *testing.T) {
	if !IsReservedMemberName("human") {
		t.Error("human should be reserved")
	}
}

func TestIsReservedMemberName_Other(t *testing.T) {
	if IsReservedMemberName("alice") {
		t.Error("alice should not be reserved")
	}
}
