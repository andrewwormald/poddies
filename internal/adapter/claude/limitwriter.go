package claude

import (
	"io"
)

// limitedWriter wraps an io.Writer with a byte cap. Writes past the cap
// are silently dropped; no error is returned so the subprocess isn't
// killed mid-stream. Cap of 0 means unlimited.
type limitedWriter struct {
	w   io.Writer
	cap int
	n   int
}

func limitWriter(w io.Writer, cap int) io.Writer {
	if cap <= 0 {
		return w
	}
	return &limitedWriter{w: w, cap: cap}
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n >= l.cap {
		return len(p), nil
	}
	room := l.cap - l.n
	if room > len(p) {
		room = len(p)
	}
	nn, err := l.w.Write(p[:room])
	l.n += nn
	if err != nil {
		return nn, err
	}
	return len(p), nil
}
