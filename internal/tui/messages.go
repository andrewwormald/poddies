// Package tui renders the poddies interactive terminal UI built on
// bubbletea. The TUI is a thin client on top of orchestrator.Loop:
// the loop runs in its own goroutine, streams events into a channel,
// and the Model drains that channel via a self-re-arming tea.Cmd.
package tui

import (
	"github.com/andrewwormald/poddies/internal/orchestrator"
	"github.com/andrewwormald/poddies/internal/thread"
)

// EventMsg carries a single event the orchestrator appended to the log.
type EventMsg struct{ Event thread.Event }

// LoopDoneMsg is sent when an orchestrator.Loop.Run call completes.
// When Err is non-nil, the TUI shows it in the status line and the
// result's StopReason is "error".
type LoopDoneMsg struct {
	Result orchestrator.LoopResult
	Err    error
}

// errorMsg is sent for non-loop errors (adapter registration, I/O, …).
// Kept distinct from LoopDoneMsg so the Model can differentiate a
// failed one-shot from a completed loop.
type errorMsg struct{ err error }
