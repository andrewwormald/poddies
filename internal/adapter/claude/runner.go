package claude

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// Runner executes a subprocess and returns its stdout + stderr.
// Abstracted so tests can inject a fake and exercise the adapter's
// flag construction + output parsing without depending on a real
// claude CLI binary.
type Runner interface {
	Run(ctx context.Context, bin string, args []string, stdin []byte) (stdout, stderr []byte, err error)
}

// ExecRunner runs commands via os/exec.CommandContext. It caps captured
// stdout/stderr at MaxOutputBytes to prevent runaway allocations; use
// NewExecRunner to construct.
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
		// return captured output alongside the error so callers can
		// include stderr in their error messages.
		return outBuf.Bytes(), errBuf.Bytes(), err
	}
	return outBuf.Bytes(), errBuf.Bytes(), nil
}
