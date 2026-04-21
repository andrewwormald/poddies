// Package adapter defines the contract between the poddies orchestrator
// and an agent backend (Claude Code CLI, Gemini CLI, or a test mock).
//
// Adapters are stateless: each Invoke() runs one turn of a named member
// against the current thread. Any session/conversation state lives in
// the pod's JSONL event log, which the adapter renders into the CLI's
// own prompt format per turn.
package adapter

import (
	"context"
	"errors"
	"fmt"

	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

// StopReason explains why an Invoke returned when it did.
type StopReason string

const (
	// StopDone is the normal case: the adapter produced a final message.
	StopDone StopReason = "done"
	// StopNeedsPermission means the adapter requested a human decision
	// (one or more entries in InvokeResponse.PermissionRequests).
	StopNeedsPermission StopReason = "needs_permission"
	// StopYield means the adapter voluntarily passed the turn back
	// without finishing (e.g. short thinking pause). Reserved; not
	// emitted by the v1 adapters.
	StopYield StopReason = "yield"
)

// Role describes who is being invoked in a turn. Most invocations are
// RoleMember; the facilitator role is invoked separately via the same
// interface so it can use any adapter backend.
type Role string

const (
	RoleMember       Role = "member"
	RoleChiefOfStaff Role = "chief_of_staff"
)

// InvokeRequest is a single turn's input to an adapter.
type InvokeRequest struct {
	// Role identifies whether this is a normal member turn or a
	// facilitator turn (chief of staff).
	Role Role

	// PriorSessionID, when non-empty, is the SessionID the adapter
	// returned on a prior Invoke for this member + thread. Adapters
	// that support resume (e.g. Claude's --resume) use this to append
	// the new prompt to an existing session instead of re-sending the
	// full thread. Empty means "fresh session".
	PriorSessionID string

	// Member is the full member config for the agent being invoked.
	// Only set when Role == RoleMember.
	Member config.Member

	// ChiefOfStaff is the chief-of-staff config. Only set when
	// Role == RoleChiefOfStaff.
	ChiefOfStaff config.ChiefOfStaff

	// Pod is the pod config the member belongs to.
	Pod config.Pod

	// Thread is the full event log, in order. The adapter renders it
	// into its own prompt format.
	Thread []thread.Event

	// Effort is a coarse hint mapped per-adapter to native knobs
	// (thinking budget, reasoning_effort, etc.). Ignored if the
	// adapter doesn't expose a comparable knob.
	Effort config.Effort

	// DispatchInstruction, when non-empty, is a targeted task from the
	// CoS dispatcher. The adapter uses this as the primary prompt
	// instead of deriving intent from the full thread. Only set for
	// dispatched member invocations.
	DispatchInstruction string

	// Roster lists the names of all pod members. Used by the mock
	// adapter's auto CoS dispatch to know who to route to. Real
	// adapters get this from the rendered system prompt.
	Roster []string
}

// PermissionRequest describes a structured ask from an agent that needs
// a human decision before proceeding (e.g., "may I run this shell
// command?"). In v1 these are purely semantic — poddies does not execute
// them; each CLI handles its own tool permissions. PermissionRequest is
// kept in the interface because orchestrators/facilitators may use it
// for cross-agent asks.
type PermissionRequest struct {
	Action  string
	Payload []byte // arbitrary JSON payload, opaque to the orchestrator
}

// ToolCall records one tool invocation observed during a streaming turn.
// Only populated by adapters that run in streaming mode and can see
// intermediate tool calls (e.g. Claude with --output-format stream-json).
type ToolCall struct {
	// Name is the tool name (e.g. "bash", "edit", "read").
	Name string
	// Input is a concise string representation of the call arguments,
	// suitable for display in the thread log. May be truncated.
	Input string
}

// InvokeResponse is a single turn's output from an adapter.
type InvokeResponse struct {
	// Body is the agent's final user-facing text for the turn.
	Body string

	// Mentions are @names parsed out of Body (convenience; orchestrator
	// may re-parse from the canonical log).
	Mentions []string

	// PermissionRequests lists any structured asks the agent emitted.
	// Typically empty.
	PermissionRequests []PermissionRequest

	// StopReason describes why the turn ended.
	StopReason StopReason

	// Usage records token counts + cost for this turn. Zero values
	// mean the adapter did not report usage (e.g. Gemini CLI in plain
	// stdout mode). Populated by Claude's JSON result and by the mock
	// adapter's Auto mode so the TUI can display burn rate.
	Usage Usage

	// ToolCalls lists tool invocations the adapter observed during the
	// turn, in order. Only populated in streaming mode. Empty in
	// non-streaming and mock adapters.
	ToolCalls []ToolCall

	// SessionID, when non-empty, identifies the adapter-side session
	// that produced this response. The orchestrator persists this and
	// passes it back on subsequent invocations so the adapter can
	// resume (e.g. `claude --resume <id>`) instead of re-rendering the
	// full thread. Empty when the adapter doesn't support resume.
	SessionID string
}

// Usage reports resource consumption for one adapter invocation.
type Usage struct {
	InputTokens  int
	OutputTokens int
	// TotalCostUSD is the adapter's self-reported dollar cost for the
	// turn. Sum of all turns gives session cost. Zero when unknown.
	TotalCostUSD float64
	// DurationMs is wall-clock milliseconds the adapter took.
	DurationMs int
}

// Add returns u + v element-wise.
func (u Usage) Add(v Usage) Usage {
	return Usage{
		InputTokens:  u.InputTokens + v.InputTokens,
		OutputTokens: u.OutputTokens + v.OutputTokens,
		TotalCostUSD: u.TotalCostUSD + v.TotalCostUSD,
		DurationMs:   u.DurationMs + v.DurationMs,
	}
}

// TotalTokens returns InputTokens + OutputTokens.
func (u Usage) TotalTokens() int { return u.InputTokens + u.OutputTokens }

// Adapter is implemented by each agent backend.
type Adapter interface {
	// Name returns the registry key (e.g. "claude", "gemini", "mock").
	Name() string

	// Invoke runs one turn and returns the response. Callers must honor
	// ctx; adapters should cancel subprocesses/HTTP on ctx cancellation.
	Invoke(ctx context.Context, req InvokeRequest) (InvokeResponse, error)
}

// Registry maps adapter name → factory constructor. Populated by
// adapter packages via Register() in their init() funcs.
var registry = map[string]Adapter{}

// ErrUnknownAdapter is returned by Get when no adapter is registered
// under the given name.
var ErrUnknownAdapter = errors.New("unknown adapter")

// Register adds a to the adapter registry under a.Name(). Intended to
// be called from adapter package init funcs. Panics on duplicate
// registration to surface configuration bugs loudly.
func Register(a Adapter) {
	if a == nil {
		panic("adapter.Register: nil adapter")
	}
	name := a.Name()
	if name == "" {
		panic("adapter.Register: adapter.Name() returned empty")
	}
	if _, dup := registry[name]; dup {
		panic(fmt.Sprintf("adapter.Register: duplicate name %q", name))
	}
	registry[name] = a
}

// Get returns the adapter registered under name, or ErrUnknownAdapter.
func Get(name string) (Adapter, error) {
	a, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownAdapter, name)
	}
	return a, nil
}

// Registered returns the sorted list of registered adapter names. Useful
// in doctor output and error messages.
func Registered() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}

// reset clears the registry; used in tests only.
func reset() {
	registry = map[string]Adapter{}
}

// ValidateRequest checks that req has the minimum fields for its role.
// Exported so tests and mock adapters can reuse the validation.
func ValidateRequest(req InvokeRequest) error {
	switch req.Role {
	case "", RoleMember:
		if req.Member.Name == "" {
			return fmt.Errorf("member role requires Member.Name")
		}
	case RoleChiefOfStaff:
		if !req.ChiefOfStaff.Enabled {
			return fmt.Errorf("chief-of-staff role requires ChiefOfStaff.Enabled=true")
		}
	default:
		return fmt.Errorf("unknown role %q", req.Role)
	}
	if req.Pod.Name == "" {
		return fmt.Errorf("Pod.Name required")
	}
	return nil
}
