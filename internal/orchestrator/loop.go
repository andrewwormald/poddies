package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

// LoopStopReason explains why Loop.Run exited.
type LoopStopReason string

const (
	// LoopQuiescent — no actionable @mention after the last turn; the
	// agents have nothing more to say to each other.
	LoopQuiescent LoopStopReason = "quiescent"
	// LoopMaxTurns — hit the configured turn cap.
	LoopMaxTurns LoopStopReason = "max_turns"
	// LoopCancelled — ctx was cancelled.
	LoopCancelled LoopStopReason = "cancelled"
	// LoopError — an adapter or config error aborted the loop. The
	// LoopResult still reflects any events that were appended before
	// the error; the error itself is returned alongside.
	LoopError LoopStopReason = "error"
)

// DefaultMaxTurns is used when Loop.MaxTurns is zero.
const DefaultMaxTurns = 8

// SafetyMaxTurns caps any loop even when MaxTurns is set to a large or
// negative (unlimited) value, to prevent runaway billing / CPU.
const SafetyMaxTurns = 1000

// DefaultMilestoneEvery is the default number of member turns between
// milestone firings of the chief-of-staff facilitator.
const DefaultMilestoneEvery = 3

// Loop drives multiple member turns back-to-back using Route to pick
// the next speaker until quiescence / max turns / error.
type Loop struct {
	Root          string
	Pod           string
	AdapterLookup AdapterLookup
	Log           *thread.Log

	// HumanMessage, if non-empty, is appended as a human event before
	// the first iteration. Lets callers kick off a conversation.
	HumanMessage string

	// MaxTurns caps the number of member invocations. Zero means use
	// DefaultMaxTurns; negative means "unlimited" (still capped by
	// SafetyMaxTurns). One value = one member turn.
	MaxTurns int

	// EffortOverride, if non-empty, overrides every member's effort for
	// the duration of the loop.
	EffortOverride config.Effort

	// FirstMember, if non-empty, forces the first turn to invoke that
	// member regardless of what Route would have picked. Subsequent
	// turns use normal routing. Useful for `poddies run --member X`
	// where the caller wants to kick a specific member even if the
	// thread/lead would have routed somewhere else.
	FirstMember string

	// MilestoneEvery overrides DefaultMilestoneEvery. Only applies when
	// the pod's chief-of-staff is enabled with the "milestone" trigger.
	MilestoneEvery int

	// OnEvent, if non-nil, is called for every event the loop appends
	// (human kickoff, member responses, system routing notes). Lets
	// CLI / TUI callers stream updates without re-reading the log.
	OnEvent func(thread.Event)
}

// LoopResult summarizes a Loop.Run call.
type LoopResult struct {
	// Events is the list of events appended during this run, in order.
	Events []thread.Event
	// StopReason explains why the loop stopped.
	StopReason LoopStopReason
	// TurnsRun counts member invocations (not human/system events).
	TurnsRun int
	// LastDecision is the final Route decision (useful for debugging).
	LastDecision RoutingDecision
}

// Run executes the loop. On context cancellation or adapter error the
// loop exits early; any events already appended are returned so callers
// can render a partial thread to the user.
func (l *Loop) Run(ctx context.Context) (LoopResult, error) {
	if l.Log == nil {
		return LoopResult{}, fmt.Errorf("orchestrator: Log must not be nil")
	}
	if l.AdapterLookup == nil {
		return LoopResult{}, fmt.Errorf("orchestrator: AdapterLookup must not be nil")
	}

	podDir := filepath.Join(l.Root, "pods", l.Pod)
	pod, err := config.LoadPod(podDir)
	if err != nil {
		return LoopResult{}, fmt.Errorf("load pod %q: %w", l.Pod, err)
	}

	memberNames, members, err := loadMemberRoster(podDir)
	if err != nil {
		return LoopResult{}, fmt.Errorf("load roster: %w", err)
	}
	memberSet := MemberSet(memberNames)

	existing, err := l.Log.Load()
	if err != nil {
		return LoopResult{}, fmt.Errorf("load thread: %w", err)
	}

	var appended []thread.Event
	emit := func(e thread.Event) {
		appended = append(appended, e)
		existing = append(existing, e)
		if l.OnEvent != nil {
			l.OnEvent(e)
		}
	}

	// Optional kickoff.
	if l.HumanMessage != "" {
		e, err := l.Log.Append(thread.Event{Type: thread.EventHuman, Body: l.HumanMessage})
		if err != nil {
			return LoopResult{Events: appended, StopReason: LoopError}, fmt.Errorf("append human: %w", err)
		}
		emit(e)
	}

	cap := l.MaxTurns
	switch {
	case cap == 0:
		cap = DefaultMaxTurns
	case cap < 0:
		cap = SafetyMaxTurns
	case cap > SafetyMaxTurns:
		cap = SafetyMaxTurns
	}

	milestoneEvery := l.MilestoneEvery
	if milestoneEvery <= 0 {
		milestoneEvery = DefaultMilestoneEvery
	}

	var lastDecision RoutingDecision
	turnsRun := 0
	turnsSinceMilestone := 0
	// cosRescued flips the first time the CoS intervenes on an
	// unresolved-routing halt during this Run. We do not reset it: a
	// second halt later in the same run is accepted as genuine, since
	// the CoS has already had its chance to steer the conversation.
	cosRescued := false
	for turnsRun < cap {
		if err := ctx.Err(); err != nil {
			return LoopResult{
				Events:       appended,
				StopReason:   LoopCancelled,
				TurnsRun:     turnsRun,
				LastDecision: lastDecision,
			}, nil
		}

		// Milestone trigger fires before routing, once we have at least
		// one member turn under our belt.
		if turnsRun > 0 && turnsSinceMilestone >= milestoneEvery && hasTrigger(pod.ChiefOfStaff, config.TriggerMilestone) {
			if err := l.invokeChiefOfStaff(ctx, pod, existing, emit); err != nil {
				return LoopResult{
						Events:       appended,
						StopReason:   LoopError,
						TurnsRun:     turnsRun,
						LastDecision: lastDecision,
					},
					fmt.Errorf("chief_of_staff milestone: %w", err)
			}
			turnsSinceMilestone = 0
			continue
		}

		var decision RoutingDecision
		if turnsRun == 0 && l.FirstMember != "" {
			decision = RoutingDecision{
				Action: ActionInvoke,
				Member: l.FirstMember,
				Reason: "first-member override: " + l.FirstMember,
			}
		} else {
			decision = Route(existing, memberSet, pod.Lead)
		}
		lastDecision = decision
		if decision.Action == ActionHalt {
			// Unresolved-routing rescue: give the chief-of-staff one
			// chance per halt to break the quiescence. If CoS also fails
			// to produce a routable response, the next Route halts again
			// and we exit cleanly.
			if !cosRescued && hasTrigger(pod.ChiefOfStaff, config.TriggerUnresolvedRouting) {
				cosRescued = true
				if err := l.invokeChiefOfStaff(ctx, pod, existing, emit); err != nil {
					return LoopResult{
							Events:       appended,
							StopReason:   LoopError,
							TurnsRun:     turnsRun,
							LastDecision: decision,
						},
						fmt.Errorf("chief_of_staff rescue: %w", err)
				}
				continue
			}
			return LoopResult{
				Events:       appended,
				StopReason:   LoopQuiescent,
				TurnsRun:     turnsRun,
				LastDecision: decision,
			}, nil
		}

		member, ok := members[decision.Member]
		if !ok {
			return LoopResult{
					Events:       appended,
					StopReason:   LoopError,
					TurnsRun:     turnsRun,
					LastDecision: decision,
				},
				fmt.Errorf("routed to unknown member %q", decision.Member)
		}

		a, err := l.AdapterLookup(string(member.Adapter))
		if err != nil {
			return LoopResult{
					Events:       appended,
					StopReason:   LoopError,
					TurnsRun:     turnsRun,
					LastDecision: decision,
				},
				fmt.Errorf("resolve adapter %q: %w", member.Adapter, err)
		}

		effort := member.Effort
		if l.EffortOverride != "" {
			effort = l.EffortOverride
		}

		resp, err := a.Invoke(ctx, adapter.InvokeRequest{
			Role:   adapter.RoleMember,
			Member: *member,
			Pod:    *pod,
			Thread: existing,
			Effort: effort,
		})
		if err != nil {
			return LoopResult{
					Events:       appended,
					StopReason:   LoopError,
					TurnsRun:     turnsRun,
					LastDecision: decision,
				},
				fmt.Errorf("invoke %s: %w", member.Name, err)
		}

		e, err := l.Log.Append(thread.Event{
			Type: thread.EventMessage,
			From: member.Name,
			Body: resp.Body,
		})
		if err != nil {
			return LoopResult{
					Events:       appended,
					StopReason:   LoopError,
					TurnsRun:     turnsRun,
					LastDecision: decision,
				},
				fmt.Errorf("append member event: %w", err)
		}
		emit(e)
		turnsRun++
		turnsSinceMilestone++
	}

	return LoopResult{
		Events:       appended,
		StopReason:   LoopMaxTurns,
		TurnsRun:     turnsRun,
		LastDecision: lastDecision,
	}, nil
}

// hasTrigger reports whether cos is enabled and lists t among its
// configured triggers.
func hasTrigger(cos config.ChiefOfStaff, t config.Trigger) bool {
	if !cos.Enabled {
		return false
	}
	for _, x := range cos.Triggers {
		if x == t {
			return true
		}
	}
	return false
}

// invokeChiefOfStaff runs one CoS turn and appends the response as a
// visible message event under the CoS's configured name. Wrapped so
// both the milestone trigger path and the unresolved-routing rescue
// path share identical semantics.
func (l *Loop) invokeChiefOfStaff(ctx context.Context, pod *config.Pod, events []thread.Event, emit func(thread.Event)) error {
	cos := pod.ChiefOfStaff
	a, err := l.AdapterLookup(string(cos.Adapter))
	if err != nil {
		return fmt.Errorf("resolve adapter %q: %w", cos.Adapter, err)
	}
	resp, err := a.Invoke(ctx, adapter.InvokeRequest{
		Role:         adapter.RoleChiefOfStaff,
		ChiefOfStaff: cos,
		Pod:          *pod,
		Thread:       events,
		Effort:       config.EffortLow,
	})
	if err != nil {
		return fmt.Errorf("invoke: %w", err)
	}
	e, err := l.Log.Append(thread.Event{
		Type: thread.EventMessage,
		From: cos.ResolvedName(),
		Body: resp.Body,
	})
	if err != nil {
		return fmt.Errorf("append CoS event: %w", err)
	}
	emit(e)
	return nil
}

// loadMemberRoster returns (names, name→*Member) for the pod. Names are
// sorted for stable iteration.
func loadMemberRoster(podDir string) ([]string, map[string]*config.Member, error) {
	dir := filepath.Join(podDir, config.MembersDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, map[string]*config.Member{}, nil
		}
		return nil, nil, err
	}
	members := make(map[string]*config.Member, len(entries))
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".toml")
		m, err := config.LoadMember(podDir, name)
		if err != nil {
			return nil, nil, fmt.Errorf("load member %q: %w", name, err)
		}
		members[name] = m
		names = append(names, name)
	}
	return names, members, nil
}
