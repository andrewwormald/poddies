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
// the set of member names, the pod's lead, and (optionally) the
// chief-of-staff's resolved name so the CoS can be @-mentioned.
//
// Policy:
//  1. Walk backwards past system/permission events to the last real turn.
//  2. If that turn mentions the CoS name (when set) or a pod member
//     (other than the speaker), invoke the first such mention. The CoS
//     is recognized by name match — callers wire an empty cosName when
//     the CoS is disabled so @mentions to it don't accidentally route.
//  3. Otherwise, if the last real turn is from the human and the pod
//     lead is a configured member (i.e. not "human"), route to the lead.
//  4. Otherwise halt — an agent producing no mentions signals quiescence.
//
// Route is a pure function: no I/O, no randomness, fully table-testable.
func Route(events []thread.Event, members map[string]struct{}, lead, cosName string) RoutingDecision {
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
		if cosName != "" && m == cosName {
			return RoutingDecision{Action: ActionInvoke, Member: cosName, Reason: "@mention of chief-of-staff " + cosName}
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
