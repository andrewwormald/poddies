// Package thread owns the append-only JSONL event log for a pod.
// The log is the single source of truth for a running/resumed pod
// conversation; orchestrators and adapters render from it and write to it.
package thread

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// EventType identifies a category of log entry.
type EventType string

const (
	// EventMessage is an agent speaking in the thread.
	EventMessage EventType = "message"
	// EventHuman is the human (pod lead) speaking.
	EventHuman EventType = "human"
	// EventSystem is a facilitator/orchestrator bookkeeping event
	// (routing picks, thread compaction notes, doctor warnings).
	EventSystem EventType = "system"
	// EventPermissionRequest is an agent-to-human request for a decision.
	EventPermissionRequest EventType = "permission_request"
	// EventPermissionGrant records the human (or lead agent) approving
	// a prior permission_request.
	EventPermissionGrant EventType = "permission_grant"
	// EventPermissionDeny records denial of a prior permission_request.
	EventPermissionDeny EventType = "permission_deny"
	// EventToolUse records an agent invoking an internal tool (bash, edit,
	// etc.) during a streaming turn. Action = tool name; Body = input summary.
	// Only captured when the adapter runs in streaming mode.
	EventToolUse EventType = "tool_use"
)

// KnownEventTypes lists EventType values the orchestrator understands.
// Unknown types loaded from the log are preserved (forward-compat) but
// are ignored by routing/rendering logic.
var KnownEventTypes = []EventType{
	EventMessage,
	EventHuman,
	EventSystem,
	EventPermissionRequest,
	EventPermissionGrant,
	EventPermissionDeny,
	EventToolUse,
}

// IsKnown reports whether t is a recognized EventType.
func (t EventType) IsKnown() bool {
	for _, k := range KnownEventTypes {
		if t == k {
			return true
		}
	}
	return false
}

// Event is one line in the JSONL log. Fields are type-dependent; unused
// ones are omitted on marshal via omitempty. Forward-compat: unknown
// types still round-trip through From/Body/Payload where present.
type Event struct {
	ID        string          `json:"id"`
	Type      EventType       `json:"type"`
	TS        time.Time       `json:"ts"`
	From      string          `json:"from,omitempty"`
	To        []string        `json:"to,omitempty"`
	Mentions  []string        `json:"mentions,omitempty"`
	Body      string          `json:"body,omitempty"`
	Action    string          `json:"action,omitempty"`
	RequestID string          `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// Validate checks the Event has the fields required by its Type.
// Called before append so the log doesn't accumulate malformed events.
func (e *Event) Validate() error {
	if e.Type == "" {
		return fmt.Errorf("event type must not be empty")
	}
	switch e.Type {
	case EventMessage:
		if e.From == "" {
			return fmt.Errorf("message event: from must not be empty")
		}
	case EventHuman:
		if e.From == "" {
			e.From = "human"
		}
		if e.From != "human" {
			return fmt.Errorf("human event: from must be %q, got %q", "human", e.From)
		}
	case EventSystem:
		// body optional, from optional (typically "system" or facilitator name)
	case EventPermissionRequest:
		if e.From == "" {
			return fmt.Errorf("permission_request: from must not be empty")
		}
		if e.Action == "" {
			return fmt.Errorf("permission_request: action must not be empty")
		}
	case EventPermissionGrant, EventPermissionDeny:
		if e.RequestID == "" {
			return fmt.Errorf("%s: request_id must not be empty", e.Type)
		}
	case EventToolUse:
		if e.From == "" {
			return fmt.Errorf("tool_use: from must not be empty")
		}
		if e.Action == "" {
			return fmt.Errorf("tool_use: action (tool name) must not be empty")
		}
	default:
		// unknown type: accept so forward-compat load/save works,
		// but do not validate fields.
	}
	return nil
}

// NewID returns a 16-byte random hex identifier suitable for event IDs.
// We use 128 bits of entropy so collisions are negligible in practice.
// Not a UUID (avoids a dependency); callers that need one can plug in
// a different generator via Log.NewID.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand.Read only fails on severe syscall errors; this
		// is effectively unreachable. Panic is acceptable.
		panic(fmt.Errorf("rand: %w", err))
	}
	return hex.EncodeToString(b[:])
}

// mentionRe matches @mentions. We require the character before the '@'
// to be a non-alphanumeric (word-boundary-ish) so "email@host" doesn't
// match "host". The captured name must be a slug (letters, digits, hyphens).
var mentionRe = regexp.MustCompile(`(?:^|[^a-zA-Z0-9_])@([a-zA-Z0-9][a-zA-Z0-9-]*)`)

// ParseMentions extracts @mentions from body in order of appearance,
// deduplicated.
func ParseMentions(body string) []string {
	matches := mentionRe.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, m := range matches {
		name := m[1]
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}
