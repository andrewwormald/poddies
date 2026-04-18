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
}

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
