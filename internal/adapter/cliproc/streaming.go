package cliproc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
)

// StreamingRunner starts a subprocess and exposes its stdout as an io.Reader
// so callers can process output incrementally. The Wait func must be called
// after the reader is drained to reap the process and collect stderr.
type StreamingRunner interface {
	Start(ctx context.Context, bin string, args []string, stdin []byte) (stdout io.Reader, wait func() (stderr []byte, err error), err error)
}

// Start implements StreamingRunner for ExecRunner.
func (r *ExecRunner) Start(ctx context.Context, bin string, args []string, stdin []byte) (io.Reader, func() ([]byte, error), error) {
	if _, err := exec.LookPath(bin); err != nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrBinaryMissing, bin)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	var errBuf bytes.Buffer
	cmd.Stderr = limitWriter(&errBuf, r.MaxOutputBytes)
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	pr, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("cliproc: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("cliproc: start: %w", err)
	}
	wait := func() ([]byte, error) {
		err := cmd.Wait()
		return errBuf.Bytes(), err
	}
	return pr, wait, nil
}
