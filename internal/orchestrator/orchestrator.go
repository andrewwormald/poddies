// Package orchestrator runs turns against pod members. The v1 (M2)
// surface is a single-turn runner: Turn.Run loads pod + member config,
// invokes the configured adapter, and appends the response to the
// thread log. Multi-agent routing and the chief-of-staff component
// land in M3 / M3.5.
package orchestrator

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

// AdapterLookup resolves an adapter by name. Production wires this to
// adapter.Get; tests inject a map-based lookup so the mock adapter can
// be substituted without touching the global registry.
type AdapterLookup func(name string) (adapter.Adapter, error)

// Turn is a single invocation of a member against the current thread.
type Turn struct {
	// Root is the poddies root dir (contains pods/<Pod>/).
	Root string
	// Pod is the pod name.
	Pod string
	// Member is the member name to invoke.
	Member string
	// AdapterLookup resolves member.Adapter → adapter.Adapter.
	AdapterLookup AdapterLookup
	// Log is the thread log to read from and append to.
	Log *thread.Log
	// HumanMessage, if non-empty, is appended as a human event before
	// invoking the member. Useful for `poddies run --message "..."`.
	HumanMessage string
	// EffortOverride, if non-empty, overrides the member's configured
	// effort for this turn only.
	EffortOverride config.Effort
}

// RunResult is what Turn.Run returns on success.
type RunResult struct {
	// HumanEvent is the appended human event (zero if HumanMessage was empty).
	HumanEvent thread.Event
	// MemberEvent is the appended member response event.
	MemberEvent thread.Event
	// Response is the raw adapter response (for callers that want more
	// than just the appended event, e.g. permission requests).
	Response adapter.InvokeResponse
}

// Run executes a single turn. Errors bubble up verbatim with context.
func (t *Turn) Run(ctx context.Context) (RunResult, error) {
	if t.Log == nil {
		return RunResult{}, fmt.Errorf("orchestrator: Log must not be nil")
	}
	if t.AdapterLookup == nil {
		return RunResult{}, fmt.Errorf("orchestrator: AdapterLookup must not be nil")
	}
	podDir := filepath.Join(t.Root, "pods", t.Pod)

	pod, err := config.LoadPod(podDir)
	if err != nil {
		return RunResult{}, fmt.Errorf("load pod %q: %w", t.Pod, err)
	}
	member, err := config.LoadMember(podDir, t.Member)
	if err != nil {
		return RunResult{}, fmt.Errorf("load member %q: %w", t.Member, err)
	}

	var humanEvent thread.Event
	if t.HumanMessage != "" {
		e, err := t.Log.Append(thread.Event{Type: thread.EventHuman, Body: t.HumanMessage})
		if err != nil {
			return RunResult{}, fmt.Errorf("append human message: %w", err)
		}
		humanEvent = e
	}

	a, err := t.AdapterLookup(string(member.Adapter))
	if err != nil {
		return RunResult{}, fmt.Errorf("resolve adapter %q: %w", member.Adapter, err)
	}

	events, err := t.Log.Load()
	if err != nil {
		return RunResult{}, fmt.Errorf("load thread: %w", err)
	}

	effort := member.Effort
	if t.EffortOverride != "" {
		effort = t.EffortOverride
	}

	resp, err := a.Invoke(ctx, adapter.InvokeRequest{
		Role:   adapter.RoleMember,
		Member: *member,
		Pod:    *pod,
		Thread: events,
		Effort: effort,
	})
	if err != nil {
		return RunResult{}, fmt.Errorf("invoke %s: %w", t.Member, err)
	}

	memberEvent, err := t.Log.Append(thread.Event{
		Type: thread.EventMessage,
		From: t.Member,
		Body: resp.Body,
	})
	if err != nil {
		return RunResult{}, fmt.Errorf("append member event: %w", err)
	}
	return RunResult{
		HumanEvent:  humanEvent,
		MemberEvent: memberEvent,
		Response:    resp,
	}, nil
}

// MapLookup returns an AdapterLookup backed by a static map.
// Primarily for tests; production uses adapter.Get.
func MapLookup(m map[string]adapter.Adapter) AdapterLookup {
	return func(name string) (adapter.Adapter, error) {
		a, ok := m[name]
		if !ok {
			return nil, fmt.Errorf("no adapter for %q (known: %v)", name, keys(m))
		}
		return a, nil
	}
}

func keys(m map[string]adapter.Adapter) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
