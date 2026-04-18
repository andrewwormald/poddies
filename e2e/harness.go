// Package e2e holds end-to-end tests that exercise the CLI, config,
// thread log, and mock adapter together. Tests use only the mock
// adapter so they are deterministic and cache-cheap.
package e2e

import (
	"bytes"
	"regexp"
)

// rfc3339Re matches RFC 3339 / RFC 3339 Nano timestamps emitted by
// encoding/json's time.Time serialization (always UTC in our code).
var rfc3339Re = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z`)

// hex32Re matches 32-lowercase-hex IDs produced by thread.NewID.
// Deterministic test IDs like "evt-000" don't match, so they stay put.
var hex32Re = regexp.MustCompile(`[0-9a-f]{32}`)

// Normalize scrubs volatile fields from a JSONL blob so it can be
// compared against a golden file across runs. It replaces:
//   - occurrences of tmpRoot with "<ROOT>"
//   - RFC 3339 timestamps with "<TS>"
//   - 32-char hex IDs with "<ID>"
//
// Deterministic test fixtures (evt-NNN ids, fixed UTC times) are
// intentionally left untouched so the golden is still informative.
func Normalize(b []byte, tmpRoot string) []byte {
	if tmpRoot != "" {
		b = bytes.ReplaceAll(b, []byte(tmpRoot), []byte("<ROOT>"))
	}
	b = rfc3339Re.ReplaceAll(b, []byte("<TS>"))
	b = hex32Re.ReplaceAll(b, []byte("<ID>"))
	return b
}
