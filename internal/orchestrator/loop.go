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
const DefaultMaxTurns = 16

// SafetyMaxTurns caps any loop even when MaxTurns is set to a large or
// negative (unlimited) value, to prevent runaway billing / CPU.
const SafetyMaxTurns = 1000

// DefaultContextWindow is the maximum number of thread events sent to a
// member per invocation in the non-dispatched (agent-to-agent) path.
// Capping at a fixed window makes per-turn cost O(1).
const DefaultContextWindow = 15

// cosContextWindow is a tighter window for the CoS dispatcher — it
// only needs recent messages to make routing decisions, not deep history.
const cosContextWindow = 10

// tailEvents returns the last n content events from s, filtering out
// internal events (tool_use, permission_*) that agents don't need.
func tailEvents(s []thread.Event, n int) []thread.Event {
	// Filter to content events only.
	var content []thread.Event
	for _, e := range s {
		switch e.Type {
		case thread.EventToolUse, thread.EventPermissionRequest,
			thread.EventPermissionGrant, thread.EventPermissionDeny:
			continue
		}
		content = append(content, e)
	}
	if len(content) <= n {
		return content
	}
	return content[len(content)-n:]
}

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

	// OnBreakaway, if non-nil, is called when the CoS requests a
	// breakaway conversation between agents. The callback should start
	// the breakaway in a background goroutine. If nil, breakaway
	// requests from the CoS are silently ignored.
	OnBreakaway func(spec BreakawaySpec)
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
	// Usage aggregates the adapter-reported token usage for this Run.
	// Each adapter invocation's Usage is Add'd; zero when adapters
	// don't report (e.g. gemini's plain-stdout path).
	Usage adapter.Usage
	// CumulativeMeta is a snapshot of the thread's meta sidecar after
	// this Run's turns have been recorded. Lets callers show lifetime
	// counters alongside the per-run numbers.
	CumulativeMeta *thread.Meta
}

// Run executes the loop. On context cancellation or adapter error the
// loop exits early; any events already appended are returned so callers
// can render a partial thread to the user.
func (l *Loop) Run(ctx context.Context) (result LoopResult, err error) {
	if l.Log == nil {
		return LoopResult{}, fmt.Errorf("orchestrator: Log must not be nil")
	}
	if l.AdapterLookup == nil {
		return LoopResult{}, fmt.Errorf("orchestrator: AdapterLookup must not be nil")
	}
	// Declared here so the deferred return-annotator below can see them.
	// Populated as the run progresses; captured at function exit into
	// result.Usage / result.CumulativeMeta.
	var (
		runUsageFromOuter adapter.Usage
		metaFromOuter     *thread.Meta
	)
	// Annotate every return with per-run usage and the cumulative meta
	// snapshot so callers (CLI stdout, TUI footer) can show burn rate
	// without re-reading the sidecar.
	defer func() {
		result.Usage = runUsageFromOuter
		if metaFromOuter != nil {
			// clone to avoid the TUI mutating our in-loop state
			m := *metaFromOuter
			if metaFromOuter.LastSessionIDs != nil {
				m.LastSessionIDs = make(map[string]string, len(metaFromOuter.LastSessionIDs))
				for k, v := range metaFromOuter.LastSessionIDs {
					m.LastSessionIDs[k] = v
				}
			}
			result.CumulativeMeta = &m
		}
	}()

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

	// Load per-thread metadata (session IDs + cumulative usage). Missing
	// is fine; LoadMeta returns an empty Meta. Any persist error after
	// a successful turn is logged but non-fatal — we never want meta
	// corruption to block the conversation.
	meta, err := thread.LoadMeta(l.Log.Path)
	if err != nil {
		return LoopResult{}, fmt.Errorf("load thread meta: %w", err)
	}
	metaFromOuter = meta
	var runUsage adapter.Usage
	persistMeta := func() {
		runUsageFromOuter = runUsage
		if err := thread.SaveMeta(l.Log.Path, meta); err != nil && l.OnEvent != nil {
			l.OnEvent(thread.Event{Type: thread.EventSystem, Body: "warning: meta save failed: " + err.Error()})
		}
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

		// --- CoS dispatcher: fires first on human messages ---
		// The CoS digests the human's intent and dispatches targeted
		// instructions to agents. Skipped when FirstMember is set (the
		// user explicitly chose who to invoke).
		if firstMember == "" && shouldFireGrayArea(existing, pod.ChiefOfStaff) {
			dispatches, err := l.invokeChiefOfStaff(ctx, pod, existing, emit, meta, &runUsage, memberSet)
			if err != nil {
				return LoopResult{
						Events:       appended,
						StopReason:   LoopError,
						TurnsRun:     turnsRun,
						LastDecision: lastDecision,
					},
					fmt.Errorf("chief_of_staff dispatch: %w", err)
			}
			cosRescued = true
			turnsSinceMilestone = 0

			// Start any breakaway conversations in the background.
			for _, bs := range dispatches.Breakaways {
				if l.OnBreakaway != nil {
					l.OnBreakaway(bs)
				}
			}

			// Execute individual dispatches immediately.
			if len(dispatches.Dispatches) > 0 {
				for _, d := range dispatches.Dispatches {
					if err := ctx.Err(); err != nil {
						break
					}
					n, err := l.invokeMemberDispatched(ctx, pod, members, d, existing, emit, meta, &runUsage)
					if err != nil {
						return LoopResult{
								Events:       appended,
								StopReason:   LoopError,
								TurnsRun:     turnsRun + n,
								LastDecision: lastDecision,
							},
							fmt.Errorf("dispatch %s: %w", d.Member, err)
					}
					turnsRun += n
					turnsSinceMilestone += n

					if thread.HasPendingPermissions(existing) {
						return LoopResult{
							Events:     appended,
							StopReason: LoopPendingPermission,
							TurnsRun:   turnsRun,
						}, nil
					}
				}
				continue
			}
			// No dispatches — CoS answered directly. Continue loop for
			// any @mentions in its response via normal routing.
			continue
		}

		// Milestone trigger fires before routing.
		if turnsRun > 0 && turnsSinceMilestone >= milestoneEvery && hasTrigger(pod.ChiefOfStaff, config.TriggerMilestone) {
			if _, err := l.invokeChiefOfStaff(ctx, pod, existing, emit, meta, &runUsage, memberSet); err != nil {
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
			firstMember = "" // consume
		} else {
			cosName := ""
			if pod.ChiefOfStaff.Enabled {
				cosName = pod.ChiefOfStaff.ResolvedName()
			}
			decision = Route(existing, memberSet, pod.Lead, cosName)
		}
		lastDecision = decision
		if decision.Action == ActionHalt {
			if !cosRescued && hasTrigger(pod.ChiefOfStaff, config.TriggerUnresolvedRouting) {
				cosRescued = true
				dispatches, err := l.invokeChiefOfStaff(ctx, pod, existing, emit, meta, &runUsage, memberSet)
				if err != nil {
					return LoopResult{
							Events:       appended,
							StopReason:   LoopError,
							TurnsRun:     turnsRun,
							LastDecision: decision,
						},
						fmt.Errorf("chief_of_staff rescue: %w", err)
				}
				for _, bs := range dispatches.Breakaways {
					if l.OnBreakaway != nil {
						l.OnBreakaway(bs)
					}
				}
				if len(dispatches.Dispatches) > 0 {
					for _, d := range dispatches.Dispatches {
						if err := ctx.Err(); err != nil {
							break
						}
						n, err := l.invokeMemberDispatched(ctx, pod, members, d, existing, emit, meta, &runUsage)
						if err != nil {
							return LoopResult{
									Events:       appended,
									StopReason:   LoopError,
									TurnsRun:     turnsRun + n,
									LastDecision: decision,
								},
								fmt.Errorf("dispatch rescue %s: %w", d.Member, err)
						}
						turnsRun += n
						turnsSinceMilestone += n
						if thread.HasPendingPermissions(existing) {
							return LoopResult{
								Events:     appended,
								StopReason: LoopPendingPermission,
								TurnsRun:   turnsRun,
							}, nil
						}
					}
				}
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

		// If Route picked the CoS (via @mention), detour through CoS.
		if pod.ChiefOfStaff.Enabled && decision.Member == pod.ChiefOfStaff.ResolvedName() {
			if _, err := l.invokeChiefOfStaff(ctx, pod, existing, emit, meta, &runUsage, memberSet); err != nil {
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

		// Sliding-window context: cap at DefaultContextWindow events so
		// per-turn cost stays O(1) regardless of conversation length.
		// We no longer use --resume / server-side session IDs for context
		// reconstruction — accumulated tool-call results in prior sessions
		// caused quadratic token growth.
		invokeThread := tailEvents(existing, DefaultContextWindow)

		resp, err := a.Invoke(ctx, adapter.InvokeRequest{
			Role:    adapter.RoleMember,
			Member:  *member,
			Pod:     *pod,
			Thread:  invokeThread,
			Effort:  effort,
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
		meta.RecordTurn(member.Name, resp.SessionID, resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.TotalCostUSD, resp.Usage.DurationMs)
		runUsage = runUsage.Add(resp.Usage)
		persistMeta()

		// Emit tool-use events before the member's response so the log
		// reflects the order of operations within the turn.
		for _, tc := range resp.ToolCalls {
			e, err := l.Log.Append(thread.Event{
				Type:   thread.EventToolUse,
				From:   member.Name,
				Action: tc.Name,
				Body:   tc.Input,
			})
			if err != nil {
				return LoopResult{
						Events:       appended,
						StopReason:   LoopError,
						TurnsRun:     turnsRun,
						LastDecision: decision,
					},
					fmt.Errorf("append tool_use: %w", err)
			}
			emit(e)
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

		// Record the delta index AFTER all emits so the next invocation
		// starts from just past this member's own response.
		meta.LastEventIdx[member.Name] = len(existing)
		persistMeta()

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
// path share identical semantics. Also records token usage + session
// ID into the thread metadata passed in.
// invokeChiefOfStaff invokes the CoS and returns any dispatch instructions
// parsed from its response. Returns nil dispatches when the CoS answered
// directly without routing.
func (l *Loop) invokeChiefOfStaff(ctx context.Context, pod *config.Pod, events []thread.Event, emit func(thread.Event), meta *thread.Meta, runUsage *adapter.Usage, memberSet map[string]struct{}) (DispatchResult, error) {
	cos := pod.ChiefOfStaff
	a, err := l.AdapterLookup(string(cos.Adapter))
	if err != nil {
		return DispatchResult{}, fmt.Errorf("resolve adapter %q: %w", cos.Adapter, err)
	}
	cosKey := cos.ResolvedName()

	// Tight context window for routing decisions — the CoS sees the
	// roster in its system prompt and only needs recent messages.
	invokeThread := tailEvents(events, cosContextWindow)

	var roster []string
	for name := range memberSet {
		roster = append(roster, name)
	}

	resp, err := a.Invoke(ctx, adapter.InvokeRequest{
		Role:         adapter.RoleChiefOfStaff,
		ChiefOfStaff: cos,
		Pod:          *pod,
		Thread:       invokeThread,
		Effort:       config.EffortLow,
		Roster:       roster,
	})
	if err != nil {
		return DispatchResult{}, fmt.Errorf("invoke: %w", err)
	}
	meta.RecordTurn(cosKey, resp.SessionID, resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.TotalCostUSD, resp.Usage.DurationMs)
	*runUsage = runUsage.Add(resp.Usage)
	if meta.LastEventIdx == nil {
		meta.LastEventIdx = map[string]int{}
	}
	meta.LastEventIdx[cosKey] = len(events)
	if saveErr := thread.SaveMeta(l.Log.Path, meta); saveErr != nil && l.OnEvent != nil {
		l.OnEvent(thread.Event{Type: thread.EventSystem, Body: "warning: meta save failed: " + saveErr.Error()})
	}
	for _, tc := range resp.ToolCalls {
		e, err := l.Log.Append(thread.Event{
			Type:   thread.EventToolUse,
			From:   cosKey,
			Action: tc.Name,
			Body:   tc.Input,
		})
		if err != nil {
			return DispatchResult{}, fmt.Errorf("append CoS tool_use: %w", err)
		}
		emit(e)
	}

	if resp.Body == "" {
		return DispatchResult{}, nil
	}
	e, err := l.Log.Append(thread.Event{
		Type: thread.EventMessage,
		From: cos.ResolvedName(),
		Body: resp.Body,
	})
	if err != nil {
		return DispatchResult{}, fmt.Errorf("append CoS event: %w", err)
	}
	emit(e)
	meta.LastEventIdx[cosKey]++
	if saveErr := thread.SaveMeta(l.Log.Path, meta); saveErr != nil && l.OnEvent != nil {
		l.OnEvent(thread.Event{Type: thread.EventSystem, Body: "warning: meta save failed: " + saveErr.Error()})
	}

	// Parse dispatch instructions from the CoS response.
	result := ParseDispatch(resp.Body, memberSet)
	return result, nil
}

// invokeMemberDispatched invokes a single member with a targeted dispatch
// instruction from the CoS. Returns the number of turns consumed (1 on
// success) and any error.
func (l *Loop) invokeMemberDispatched(
	ctx context.Context,
	pod *config.Pod,
	members map[string]*config.Member,
	d Dispatch,
	existing []thread.Event,
	emit func(thread.Event),
	meta *thread.Meta,
	runUsage *adapter.Usage,
) (int, error) {
	member, ok := members[d.Member]
	if !ok {
		return 0, fmt.Errorf("unknown member %q", d.Member)
	}
	a, err := l.AdapterLookup(string(member.Adapter))
	if err != nil {
		return 0, fmt.Errorf("resolve adapter %q: %w", member.Adapter, err)
	}

	effort := member.Effort
	if l.EffortOverride != "" {
		effort = l.EffortOverride
	}

	// Dispatched members get minimal context (last 5 events) since the
	// dispatch instruction is their primary task.
	invokeThread := tailEvents(existing, dispatchContextWindow)

	resp, err := a.Invoke(ctx, adapter.InvokeRequest{
		Role:                adapter.RoleMember,
		Member:              *member,
		Pod:                 *pod,
		Thread:              invokeThread,
		Effort:              effort,
		DispatchInstruction: d.Instruction,
	})
	if err != nil {
		return 0, fmt.Errorf("invoke: %w", err)
	}
	meta.RecordTurn(member.Name, resp.SessionID, resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.TotalCostUSD, resp.Usage.DurationMs)
	*runUsage = runUsage.Add(resp.Usage)

	for _, tc := range resp.ToolCalls {
		e, err := l.Log.Append(thread.Event{
			Type:   thread.EventToolUse,
			From:   member.Name,
			Action: tc.Name,
			Body:   tc.Input,
		})
		if err != nil {
			return 0, fmt.Errorf("append tool_use: %w", err)
		}
		emit(e)
	}

	// Append permission requests before the message body so the loop
	// halts with pending permissions on the next iteration.
	for _, pr := range resp.PermissionRequests {
		e, err := l.Log.Append(thread.Event{
			Type:    thread.EventPermissionRequest,
			From:    member.Name,
			Action:  pr.Action,
			Payload: pr.Payload,
		})
		if err != nil {
			return 0, fmt.Errorf("append permission_request: %w", err)
		}
		emit(e)
	}

	if resp.Body != "" {
		e, err := l.Log.Append(thread.Event{
			Type: thread.EventMessage,
			From: member.Name,
			Body: resp.Body,
		})
		if err != nil {
			return 0, fmt.Errorf("append message: %w", err)
		}
		emit(e)
	}

	if saveErr := thread.SaveMeta(l.Log.Path, meta); saveErr != nil && l.OnEvent != nil {
		l.OnEvent(thread.Event{Type: thread.EventSystem, Body: "warning: meta save failed: " + saveErr.Error()})
	}
	return 1, nil
}

// dispatchContextWindow is the number of recent events sent to a
// dispatched member. Kept small since the dispatch instruction is the
// primary task — context is just for continuity.
const dispatchContextWindow = 5

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
