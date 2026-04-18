package orchestrator

import "github.com/andrewwormald/poddies/internal/thread"

// RoutingAction is what Route concluded the loop should do next.
type RoutingAction string

const (
	// ActionInvoke means: invoke Member next.
	ActionInvoke RoutingAction = "invoke"
	// ActionHalt means: stop the loop cleanly.
	ActionHalt RoutingAction = "halt"
)

// RoutingDecision is the output of Route.
type RoutingDecision struct {
	Action RoutingAction
	Member string // set when Action == ActionInvoke
	Reason string // human-readable; surfaced in system events / logs
}

// Route picks the next speaker for a pod given the current thread,
// the set of member names, and the pod's lead.
//
// Policy (matches the M3 design in memory/project_chief_of_staff.md
// plus the A3 routing decision from the earlier planning conversation):
//  1. Walk backwards past system events to find the last real turn.
//  2. If that turn mentions a pod member (other than the speaker),
//     invoke the first such member.
//  3. Otherwise, if the last real turn is from the human and the pod
//     lead is a configured member (i.e. not "human"), route to the lead.
//  4. Otherwise halt — an agent producing no mentions signals quiescence.
//
// Route is a pure function: no I/O, no randomness, fully table-testable.
func Route(events []thread.Event, members map[string]struct{}, lead string) RoutingDecision {
	// find the last conversational event — system events and
	// permission_* events are meta and never drive routing.
	idx := -1
	for i := len(events) - 1; i >= 0; i-- {
		switch events[i].Type {
		case thread.EventSystem,
			thread.EventPermissionRequest,
			thread.EventPermissionGrant,
			thread.EventPermissionDeny:
			continue
		}
		idx = i
		break
	}
	if idx < 0 {
		return RoutingDecision{Action: ActionHalt, Reason: "empty thread or system events only"}
	}
	last := events[idx]

	// 1. actionable mention on the last turn
	for _, m := range last.Mentions {
		if m == last.From {
			continue
		}
		if _, ok := members[m]; ok {
			return RoutingDecision{Action: ActionInvoke, Member: m, Reason: "@mention of " + m}
		}
	}

	// 2. human with no actionable mention: route to lead if lead is a member
	if last.Type == thread.EventHuman && lead != "" && lead != "human" {
		if _, ok := members[lead]; ok {
			return RoutingDecision{Action: ActionInvoke, Member: lead, Reason: "human-no-mention: routing to lead " + lead}
		}
	}

	// 3. fall through: agent produced no actionable mention, or human
	//    addressed no one and no agent lead exists → quiescent.
	return RoutingDecision{Action: ActionHalt, Reason: "no actionable @mention"}
}

// MemberSet is a tiny helper to build the members map from a name slice.
func MemberSet(names []string) map[string]struct{} {
	m := make(map[string]struct{}, len(names))
	for _, n := range names {
		m[n] = struct{}{}
	}
	return m
}
