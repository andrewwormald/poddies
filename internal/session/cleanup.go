package session

import (
	"context"
	"fmt"
	"os"
	"time"
)

// DefaultCleanupDays is the age threshold for stale-session cleanup.
const DefaultCleanupDays = 30

// CleanupStale removes sessions whose LastEditedAt is older than
// maxAge. Deletes each session's directory (thread log, meta sidecar,
// lock file) and drops the index entry. Returns the count of removed
// sessions and any non-fatal errors (one stuck session shouldn't block
// the rest).
//
// ctx lets the caller cap runtime — the launch-side cleanup goroutine
// passes a 1-hour context per user spec. Returning with ctx.Err()
// means the deadline fired; the index is still saved if any progress
// was made.
func CleanupStale(ctx context.Context, root string, maxAge time.Duration) (removed int, err error) {
	idx, err := LoadIndex(root)
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().UTC().Add(-maxAge)
	kept := make([]Session, 0, len(idx.Sessions))
	var firstErr error
	for _, s := range idx.Sessions {
		if err := ctx.Err(); err != nil {
			// Keep the rest so we don't lose index rows on timeout.
			kept = append(kept, s)
			continue
		}
		if s.LastEditedAt.After(cutoff) {
			kept = append(kept, s)
			continue
		}
		if err := os.RemoveAll(Dir(root, s.ID)); err != nil {
			// Non-fatal: record the first error, keep the entry so we
			// re-try next launch. Prevents a permission-locked dir
			// from losing its index row.
			if firstErr == nil {
				firstErr = fmt.Errorf("remove session %q: %w", s.ID, err)
			}
			kept = append(kept, s)
			continue
		}
		removed++
	}
	idx.Sessions = kept
	if err := SaveIndex(root, idx); err != nil {
		return removed, fmt.Errorf("save index: %w", err)
	}
	if ctx.Err() != nil {
		return removed, ctx.Err()
	}
	return removed, firstErr
}
