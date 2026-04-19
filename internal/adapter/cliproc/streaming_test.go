package cliproc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestExecRunner_Start_ReadsStdout(t *testing.T) {
	r := NewExecRunner()
	stdout, wait, err := r.Start(context.Background(), "sh", []string{"-c", "printf hello"}, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	data, _ := io.ReadAll(stdout)
	if _, werr := wait(); werr != nil {
		t.Fatalf("wait: %v", werr)
	}
	if string(data) != "hello" {
		t.Errorf("want hello, got %q", string(data))
	}
}

func TestExecRunner_Start_PassesStdin(t *testing.T) {
	r := NewExecRunner()
	stdout, wait, err := r.Start(context.Background(), "cat", nil, []byte("from stdin"))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	data, _ := io.ReadAll(stdout)
	if _, werr := wait(); werr != nil {
		t.Fatalf("wait: %v", werr)
	}
	if string(data) != "from stdin" {
		t.Errorf("want 'from stdin', got %q", string(data))
	}
}

func TestExecRunner_Start_CapturesStderrOnError(t *testing.T) {
	r := NewExecRunner()
	stdout, wait, err := r.Start(context.Background(), "sh", []string{"-c", "printf oops 1>&2; exit 7"}, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	io.Copy(io.Discard, stdout)
	stderr, waitErr := wait()
	if waitErr == nil {
		t.Fatal("want error from wait on non-zero exit")
	}
	if !strings.Contains(string(stderr), "oops") {
		t.Errorf("want oops in stderr, got %q", stderr)
	}
}

func TestExecRunner_Start_MissingBinary_Errors(t *testing.T) {
	r := NewExecRunner()
	_, _, err := r.Start(context.Background(), "definitely-not-a-real-binary-xyz-123", nil, nil)
	if !errors.Is(err, ErrBinaryMissing) {
		t.Errorf("want ErrBinaryMissing, got %v", err)
	}
}

func TestExecRunner_Start_StreamsMultipleLines(t *testing.T) {
	r := NewExecRunner()
	stdout, wait, err := r.Start(context.Background(), "printf", []string{"line1\nline2\nline3\n"}, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	data, _ := io.ReadAll(stdout)
	if _, werr := wait(); werr != nil {
		t.Fatalf("wait: %v", werr)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 3 {
		t.Errorf("want 3 lines, got %d: %v", len(lines), lines)
	}
}

func TestExecRunner_Start_CtxCancel(t *testing.T) {
	r := NewExecRunner()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stdout, wait, err := r.Start(ctx, "sh", []string{"-c", "sleep 60"}, nil)
	// Either Start itself fails or wait fails after draining
	if err != nil {
		return
	}
	io.Copy(io.Discard, stdout)
	_, waitErr := wait()
	if waitErr == nil {
		t.Error("want error when ctx cancelled before process finishes")
	}
}

func TestExecRunner_ImplementsStreamingRunner(t *testing.T) {
	var _ StreamingRunner = NewExecRunner()
	_ = bytes.NewReader // suppress unused import if needed
}
