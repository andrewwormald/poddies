package orchestrator

import (
	"context"
	"fmt"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

// DefaultBreakawayTurns caps how long agents can discuss before the
// breakaway ends. Each member response counts as one turn.
const DefaultBreakawayTurns = 10

// BreakawaySpec describes a breakaway conversation to start.
type BreakawaySpec struct {
	Members []string // participating agents (2+)
	Topic   string   // the CoS's instruction / discussion topic
}

// BreakawayResult is returned when a breakaway completes.
type BreakawayResult struct {
	Spec     BreakawaySpec
	Events   []thread.Event // all events from the breakaway
	Summary  string         // last speaker's final message (used as summary)
	TurnsRun int
}

// RunBreakaway executes a background agent-to-agent conversation.
// Members take turns responding to each other. The breakaway ends when
// max turns are reached or context is cancelled.
//
// onEvent is called for each event (for viz animation). It runs on
// the breakaway goroutine — the caller must handle thread safety.
func RunBreakaway(
	ctx context.Context,
	spec BreakawaySpec,
	log *thread.Log,
	pod *config.Pod,
	allMembers map[string]*config.Member,
	adapterLookup AdapterLookup,
	onEvent func(thread.Event),
) (BreakawayResult, error) {
	if len(spec.Members) < 2 {
		return BreakawayResult{}, fmt.Errorf("breakaway needs at least 2 members, got %d", len(spec.Members))
	}

	// Seed the breakaway thread with the topic.
	topicEvent, err := log.Append(thread.Event{
		Type: thread.EventSystem,
		Body: fmt.Sprintf("Breakaway discussion: %s", spec.Topic),
	})
	if err != nil {
		return BreakawayResult{}, fmt.Errorf("append topic: %w", err)
	}
	var events []thread.Event
	events = append(events, topicEvent)
	emit := func(e thread.Event) {
		events = append(events, e)
		if onEvent != nil {
			onEvent(e)
		}
	}

	meta := &thread.Meta{}
	var runUsage adapter.Usage
	var lastBody string

	for turn := 0; turn < DefaultBreakawayTurns; turn++ {
		if err := ctx.Err(); err != nil {
			break
		}

		// Round-robin: pick the next speaker.
		speaker := spec.Members[turn%len(spec.Members)]
		member, ok := allMembers[speaker]
		if !ok {
			return BreakawayResult{}, fmt.Errorf("unknown member %q", speaker)
		}

		a, err := adapterLookup(string(member.Adapter))
		if err != nil {
			return BreakawayResult{}, fmt.Errorf("adapter %q: %w", member.Adapter, err)
		}

		// Build a dispatch instruction that includes the topic and
		// tells the member they're in a breakaway discussion.
		instruction := fmt.Sprintf(
			"You're in a breakaway discussion with %s. Topic: %s. Respond to the latest message. Be direct and concise. When you've reached agreement, say DONE.",
			joinNames(spec.Members, speaker), spec.Topic,
		)

		// Use the breakaway thread as context (not the main thread).
		invokeThread := tailEvents(events, dispatchContextWindow)

		resp, err := a.Invoke(ctx, adapter.InvokeRequest{
			Role:                adapter.RoleMember,
			Member:              *member,
			Pod:                 *pod,
			Thread:              invokeThread,
			Effort:              member.Effort,
			DispatchInstruction: instruction,
		})
		if err != nil {
			return BreakawayResult{Events: events, TurnsRun: turn}, fmt.Errorf("invoke %s: %w", speaker, err)
		}
		meta.RecordTurn(speaker, resp.SessionID, resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.TotalCostUSD, resp.Usage.DurationMs)
		runUsage = runUsage.Add(resp.Usage)

		if resp.Body != "" {
			e, err := log.Append(thread.Event{
				Type: thread.EventMessage,
				From: speaker,
				Body: resp.Body,
			})
			if err != nil {
				return BreakawayResult{Events: events, TurnsRun: turn}, fmt.Errorf("append: %w", err)
			}
			emit(e)
			lastBody = resp.Body

			// Check for consensus signal.
			if containsDone(resp.Body) {
				break
			}
		}
	}

	return BreakawayResult{
		Spec:     spec,
		Events:   events,
		Summary:  lastBody,
		TurnsRun: len(events) - 1, // subtract the topic event
	}, nil
}

// joinNames returns "alice and bob" or "alice, bob, and carol" excluding self.
func joinNames(members []string, self string) string {
	var others []string
	for _, m := range members {
		if m != self {
			others = append(others, m)
		}
	}
	if len(others) == 1 {
		return others[0]
	}
	return fmt.Sprintf("%s", others)
}

// containsDone checks if the body contains a consensus signal.
func containsDone(body string) bool {
	for i := 0; i <= len(body)-4; i++ {
		if (body[i] == 'D' || body[i] == 'd') &&
			(body[i+1] == 'O' || body[i+1] == 'o') &&
			(body[i+2] == 'N' || body[i+2] == 'n') &&
			(body[i+3] == 'E' || body[i+3] == 'e') {
			return true
		}
	}
	return false
}
