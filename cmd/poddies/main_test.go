package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_NoArgs_PrintsVersion(t *testing.T) {
	var buf bytes.Buffer
	if err := run(nil, &buf); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !strings.Contains(buf.String(), "poddies ") {
		t.Fatalf("expected version output, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), version) {
		t.Fatalf("expected output to contain version %q, got %q", version, buf.String())
	}
}

func TestRun_VersionSubcommand(t *testing.T) {
	for _, arg := range []string{"version", "--version", "-v"} {
		t.Run(arg, func(t *testing.T) {
			var buf bytes.Buffer
			if err := run([]string{arg}, &buf); err != nil {
				t.Fatalf("run(%q) returned error: %v", arg, err)
			}
			if !strings.Contains(buf.String(), version) {
				t.Fatalf("expected %q in output, got %q", version, buf.String())
			}
		})
	}
}

func TestRun_UnknownCommand_ReturnsError(t *testing.T) {
	var buf bytes.Buffer
	err := run([]string{"bogus"}, &buf)
	if err == nil {
		t.Fatalf("expected error for unknown command, got nil")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("expected error to mention the unknown command, got %v", err)
	}
}
