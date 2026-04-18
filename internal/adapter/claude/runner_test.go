package claude

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExecRunner_MissingBinary_ReturnsErrBinaryMissing(t *testing.T) {
	r := NewExecRunner()
	_, _, err := r.Run(context.Background(), "definitely-not-a-real-binary-xyz-123", nil, nil)
	if !errors.Is(err, ErrBinaryMissing) {
		t.Errorf("want ErrBinaryMissing, got %v", err)
	}
}

func TestExecRunner_EchoesStdout(t *testing.T) {
	r := NewExecRunner()
	// `sh -c 'printf hello'` is portable across darwin/linux.
	stdout, _, err := r.Run(context.Background(), "sh", []string{"-c", "printf hello"}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(stdout) != "hello" {
		t.Errorf("want hello, got %q", stdout)
	}
}

func TestExecRunner_CapturesStderr(t *testing.T) {
	r := NewExecRunner()
	_, stderr, err := r.Run(context.Background(), "sh", []string{"-c", "printf oops 1>&2; exit 7"}, nil)
	if err == nil {
		t.Fatal("want error on non-zero exit")
	}
	if !strings.Contains(string(stderr), "oops") {
		t.Errorf("want oops in stderr, got %q", stderr)
	}
}

func TestExecRunner_PassesStdin(t *testing.T) {
	r := NewExecRunner()
	stdout, _, err := r.Run(context.Background(), "cat", nil, []byte("hello from stdin"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(stdout) != "hello from stdin" {
		t.Errorf("want hello from stdin, got %q", stdout)
	}
}

func TestExecRunner_ContextCancel_KillsSubprocess(t *testing.T) {
	r := NewExecRunner()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := r.Run(ctx, "sh", []string{"-c", "sleep 60"}, nil)
	if err == nil {
		t.Error("want error when ctx cancelled")
	}
}

func TestExecRunner_RespectsMaxOutputBytes(t *testing.T) {
	r := &ExecRunner{MaxOutputBytes: 5}
	stdout, _, err := r.Run(context.Background(), "sh", []string{"-c", "printf 0123456789"}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(stdout) != "01234" {
		t.Errorf("want truncated 01234, got %q", stdout)
	}
}
