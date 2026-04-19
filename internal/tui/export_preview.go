package tui

import "github.com/andrewwormald/poddies/internal/thread"

// exportPreview wraps the bundle bytes as a synthetic system event so
// the user can see and copy the exported TOML from the transcript pane.
// The event is not persisted to the log; it's a view-only artifact.
func exportPreview(data []byte) thread.Event {
	return thread.Event{
		Type: thread.EventSystem,
		Body: "--- exported pod bundle ---\n" + string(data) + "\n--- end bundle ---",
	}
}
