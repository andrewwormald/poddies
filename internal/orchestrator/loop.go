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
	// LoopPendingPermission — at least one permission_request event in
	// the thread is unresolved. The loop halts until the user grants
	// or denies via `poddies thread approve/deny`, at which point
	// `poddies run` or `thread resume` picks up where it paused.
	LoopPendingPermission LoopStopReason = "pending_permission"
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

	// If the loaded thread already has unresolved permission requests,
	// halt before doing any work so the user (or `thread approve/deny`)
	// can resolve them first.
	if thread.HasPendingPermissions(existing) {
		return LoopResult{
			Events:     appended,
			StopReason: LoopPendingPermission,
			TurnsRun:   0,
		}, nil
	}

	var lastDecision RoutingDecision
	turnsRun := 0
	turnsSinceMilestone := 0
	// cosRescued flips the first time the CoS intervenes on an
	// unresolved-routing halt during this Run. We do not reset it: a
	// second halt later in the same run is accepted as genuine, since
	// the CoS has already had its chance to steer the conversation.
	cosRescued := false
	// firstMember is consumed after the first invocation attempt so
	// the override doesn't re-fire on subsequent iterations when the
	// first turn took a non-member path (e.g. @CoS routing) and left
	// turnsRun at zero.
	firstMember := l.FirstMember
	for turnsRun < cap {
		if err := ctx.Err(); err != nil {
			return LoopResult{
				Events:       appended,
				StopReason:   LoopCancelled,
				TurnsRun:     turnsRun,
				LastDecision: lastDecision,
			}, nil
		}

		// Gray-area trigger fires whenever the most recent real event
		// is a human message the CoS has not yet addressed. Lets the
		// CoS either route to a member (@mention) or answer directly
		// when the request doesn't clearly land in anyone's domain.
		// Shares the one-rescue-per-run budget with unresolved_routing
		// so a pod configured with both triggers doesn't fire the CoS
		// twice (gray_area answer → halt → rescue → duplicate).
		if shouldFireGrayArea(existing, pod.ChiefOfStaff) {
			if err := l.invokeChiefOfStaff(ctx, pod, existing, emit); err != nil {
				return LoopResult{
						Events:       appended,
						StopReason:   LoopError,
						TurnsRun:     turnsRun,
						LastDecision: lastDecision,
					},
					fmt.Errorf("chief_of_staff gray_area: %w", err)
			}
			cosRescued = true
			turnsSinceMilestone = 0
			continue
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
		if firstMember != "" {
			decision = RoutingDecision{
				Action: ActionInvoke,
				Member: firstMember,
				Reason: "first-member override: " + firstMember,
			}
			firstMember = "" // consume, regardless of which path handles it
		} else {
			cosName := ""
			if pod.ChiefOfStaff.Enabled {
				cosName = pod.ChiefOfStaff.ResolvedName()
			}
			decision = Route(existing, memberSet, pod.Lead, cosName)
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
				// Reset the milestone counter so the next member turn
				// doesn't double-fire (rescue + milestone). Without this
				// reset a pod with both triggers configured would see
				// two CoS invocations in quick succession the moment the
				// rescued turn lands on a milestone boundary.
				turnsSinceMilestone = 0
				continue
			}
			return LoopResult{
				Events:       appended,
				StopReason:   LoopQuiescent,
				TurnsRun:     turnsRun,
				LastDecision: decision,
			}, nil
		}

		// If Route picked the CoS (via @mention), detour through the
		// CoS invocation path instead of treating it as a member turn.
		// Consumes the rescue budget so a subsequent Route halt doesn't
		// also fire unresolved_routing — the CoS has already had its say.
		if pod.ChiefOfStaff.Enabled && decision.Member == pod.ChiefOfStaff.ResolvedName() {
			if err := l.invokeChiefOfStaff(ctx, pod, existing, emit); err != nil {
				return LoopResult{
						Events:       appended,
						StopReason:   LoopError,
						TurnsRun:     turnsRun,
						LastDecision: decision,
					},
					fmt.Errorf("chief_of_staff mention: %w", err)
			}
			cosRescued = true
			turnsSinceMilestone = 0
			continue
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

		if resp.Body != "" {
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
		}

		// Append any permission requests from the response. Each gets
		// its own thread event; the loop then halts before the next
		// member turn (see pending-permission check at top of iter).
		for _, pr := range resp.PermissionRequests {
			e, err := l.Log.Append(thread.Event{
				Type:    thread.EventPermissionRequest,
				From:    member.Name,
				Action:  pr.Action,
				Payload: pr.Payload,
			})
			if err != nil {
				return LoopResult{
						Events:       appended,
						StopReason:   LoopError,
						TurnsRun:     turnsRun,
						LastDecision: decision,
					},
					fmt.Errorf("append permission_request: %w", err)
			}
			emit(e)
		}

		turnsRun++
		turnsSinceMilestone++

		if thread.HasPendingPermissions(existing) {
			return LoopResult{
				Events:       appended,
				StopReason:   LoopPendingPermission,
				TurnsRun:     turnsRun,
				LastDecision: decision,
			}, nil
		}
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

// shouldFireGrayArea reports whether the CoS should pre-emptively
// respond to the current thread. True when:
//   - the gray_area trigger is configured, and
//   - the most recent non-meta event is a human message, and
//   - that human message has no @mention (explicit mentions respect
//     the human's stated intent — the CoS stays out of the way), and
//   - no member and no CoS has yet responded since that human message.
//
// Meta events (system, permission_*) are skipped. A CoS response or a
// member response "closes" the human turn — the next firing needs a
// fresh human event.
func shouldFireGrayArea(events []thread.Event, cos config.ChiefOfStaff) bool {
	if !hasTrigger(cos, config.TriggerGrayArea) {
		return false
	}
	for i := len(events) - 1; i >= 0; i-- {
		switch events[i].Type {
		case thread.EventSystem,
			thread.EventPermissionRequest,
			thread.EventPermissionGrant,
			thread.EventPermissionDeny:
			continue
		case thread.EventMessage:
			// Any response closes the turn (member or CoS).
			return false
		case thread.EventHuman:
			// Explicit @mention → defer to the human's choice.
			return len(events[i].Mentions) == 0
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
	// Skip empty-body responses entirely. Appending an empty message
	// event would poison the thread: Route's last-non-meta-event lookup
	// would return the empty event, which has no mentions, halting the
	// loop forever. The CoS simply having nothing to add is a valid
	// outcome — it just means the loop continues via whatever the last
	// real turn was.
	if resp.Body == "" {
		return nil
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
