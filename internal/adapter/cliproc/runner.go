// Package cliproc holds the subprocess runner shared by the CLI-based
// agent adapters (claude, gemini, and future additions). It abstracts
// exec.CommandContext so adapters can be unit-tested against a fake
// runner without depending on real external binaries.
package cliproc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// Runner executes a subprocess and returns its stdout + stderr.
type Runner interface {
	Run(ctx context.Context, bin string, args []string, stdin []byte) (stdout, stderr []byte, err error)
}

// ExecRunner runs commands via os/exec.CommandContext. It caps captured
// stdout/stderr at MaxOutputBytes to prevent runaway allocations.
type ExecRunner struct {
	// MaxOutputBytes caps each of stdout and stderr. Zero disables the
	// cap (unlimited). Default 8 MiB via NewExecRunner.
	MaxOutputBytes int
}

// NewExecRunner returns a Runner with conservative defaults.
func NewExecRunner() *ExecRunner {
	return &ExecRunner{MaxOutputBytes: 8 * 1024 * 1024}
}

// ErrBinaryMissing is returned when the configured binary cannot be
// found on PATH. Wrapped so callers can detect it via errors.Is.
var ErrBinaryMissing = errors.New("binary not found on PATH")

// Run implements Runner.
func (r *ExecRunner) Run(ctx context.Context, bin string, args []string, stdin []byte) ([]byte, []byte, error) {
	if _, err := exec.LookPath(bin); err != nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrBinaryMissing, bin)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = limitWriter(&outBuf, r.MaxOutputBytes)
	cmd.Stderr = limitWriter(&errBuf, r.MaxOutputBytes)
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	if err := cmd.Run(); err != nil {
		return outBuf.Bytes(), errBuf.Bytes(), err
	}
	return outBuf.Bytes(), errBuf.Bytes(), nil
}

// Truncate returns the first n bytes of b as a string, appending an
// ellipsis marker if b was longer. Handy for embedding captured stderr
// in adapter error messages without exploding their size.
func Truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "...(truncated)"
}
