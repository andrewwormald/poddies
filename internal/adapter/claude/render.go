package claude

import (
	"fmt"
	"strings"

	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

// RenderChiefOfStaffSystemPrompt builds the system prompt for a
// chief-of-staff invocation. Before this existed the adapter was
// passing a zero-value Member into RenderSystemPrompt, producing a
// system prompt that identified the CoS as a nameless member with no
// role — effectively giving it no instructions at all. This prompt
// tells the CoS who it is, what it's there to do, and lists the pod
// roster so it can route via @mention.
func RenderChiefOfStaffSystemPrompt(cos config.ChiefOfStaff, pod config.Pod, roster []config.Member) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: dispatcher, pod %q. Lead: %s\n", cos.ResolvedName(), pod.Name, pod.Lead)
	if len(roster) > 0 {
		b.WriteString("Team: ")
		for i, m := range roster {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s(%s)", m.Name, m.Title)
		}
		b.WriteString("\n")
	}
	b.WriteString("Dispatch: @name task (one per line). No @mention = direct answer.\n")
	b.WriteString("Breakaway: +@name1+@name2 topic — agents discuss privately, you get the summary.\n")
	b.WriteString("Agents see only your instruction. Be specific. Greetings → dispatch all.\n")
	b.WriteString("No markdown. Plain @name only.\n")
	return b.String()
}

// RenderSystemPrompt builds the text passed to claude via
// --append-system-prompt. It combines role, persona, and pod context
// so the agent behaves consistently with its config.
func RenderSystemPrompt(member config.Member, pod config.Pod, roster []config.Member) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s, pod %q. Domain: %s only — @route otherwise.\n", member.Name, member.Title, pod.Name, member.Title)
	if member.Persona != "" {
		fmt.Fprintf(&b, "Persona: %s\n", member.Persona)
	}
	if len(roster) > 0 {
		b.WriteString("Team: ")
		for i, m := range roster {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s(%s)", m.Name, m.Title)
		}
		b.WriteString("\n")
	}
	b.WriteString("Be concise. No preamble. @name to route. Review cycles normal — don't self-declare done.\n")
	if member.SystemPromptExtra != "" {
		b.WriteString(member.SystemPromptExtra)
		b.WriteString("\n")
	}
	return b.String()
}

// threadVerbatimN is the number of most-recent events rendered verbatim.
// Older events are compressed into a per-speaker "last position" summary.
const threadVerbatimN = 7

// renderThreadContext builds the conversation body for a user prompt.
// When the event list is longer than threadVerbatimN it compresses the
// older half into a "last position per speaker" summary so the model
// always has full situational awareness without ballooning the prompt.
func renderThreadContext(events []thread.Event) string {
	if len(events) == 0 {
		return ""
	}
	if len(events) <= threadVerbatimN {
		var b strings.Builder
		for _, e := range events {
			b.WriteString(renderEvent(e))
		}
		return b.String()
	}

	older := events[:len(events)-threadVerbatimN]
	recent := events[len(events)-threadVerbatimN:]

	var b strings.Builder
	b.WriteString("[Earlier conversation — most recent position of each participant]\n")
	b.WriteString(compressSpeakers(older))
	b.WriteString("\n[Recent conversation]\n")
	for _, e := range recent {
		b.WriteString(renderEvent(e))
	}
	return b.String()
}

// compressSpeakers extracts the last message per speaker from events and
// returns them in chronological order of last appearance. This gives
// any agent reading the context a clear "where everyone stands" snapshot.
func compressSpeakers(events []thread.Event) string {
	type pos struct {
		key  string
		body string
		idx  int
	}
	seen := map[string]*pos{}
	var order []string

	for i, e := range events {
		var key, label, body string
		switch e.Type {
		case thread.EventHuman:
			key, label, body = "human", "human", e.Body
		case thread.EventMessage:
			if e.From == "" {
				continue
			}
			key, label, body = e.From, e.From, e.Body
		default:
			continue
		}
		if _, ok := seen[key]; !ok {
			order = append(order, key)
		}
		seen[key] = &pos{key: label, body: truncBody(body), idx: i}
	}

	var b strings.Builder
	for _, k := range order {
		p := seen[k]
		fmt.Fprintf(&b, "[%s] %s\n", p.key, p.body)
	}
	return b.String()
}

// RenderUserPromptForCoS is the CoS-flavored counterpart of
// RenderUserPrompt. The CoS sees the full thread context so it can
// make informed dispatch decisions.
func RenderUserPromptForCoS(cos config.ChiefOfStaff, events []thread.Event) string {
	var b strings.Builder
	if len(events) > 0 {
		b.WriteString(renderThreadContext(events))
	}
	b.WriteString("Dispatch now.\n")
	return b.String()
}

// dispatchContextN is the number of recent events shown to a dispatched
// member for continuity. Kept small since the dispatch instruction is
// the primary task.
const dispatchContextN = 5

// RenderUserPrompt builds the text passed to claude via -p. When
// dispatchInstruction is non-empty, the member gets a targeted task
// from the CoS with only minimal recent context. Otherwise falls back
// to full thread rendering for agent-to-agent routing.
func RenderUserPrompt(member config.Member, events []thread.Event, dispatchInstruction string) string {
	var b strings.Builder

	if dispatchInstruction != "" {
		fmt.Fprintf(&b, "Task: %s\n", dispatchInstruction)
		recent := events
		if len(recent) > dispatchContextN {
			recent = recent[len(recent)-dispatchContextN:]
		}
		if len(recent) > 0 {
			b.WriteString("Context:\n")
			for _, e := range recent {
				b.WriteString(renderEvent(e))
			}
		}
		return b.String()
	}

	if len(events) == 0 {
		b.WriteString("Thread empty.\n")
	} else {
		b.WriteString(renderThreadContext(events))
	}
	fmt.Fprintf(&b, "Respond as %s. @name to route.\n", member.Name)
	return b.String()
}

// maxBodyChars is the maximum number of characters rendered from any
// single event body. Long tool outputs and messages are truncated to
// keep the prompt bounded; the tail note tells the model what happened.
const maxBodyChars = 600

// truncBody caps s at maxBodyChars, appending a byte count note when cut.
func truncBody(s string) string {
	if len(s) <= maxBodyChars {
		return s
	}
	return fmt.Sprintf("%s … [+%d chars]", s[:maxBodyChars], len(s)-maxBodyChars)
}

// renderEvent formats a single event into a transcript line. Unknown
// event types are rendered as "[unknown:<type>]" so nothing is silently
// dropped on the way into the prompt.
func renderEvent(e thread.Event) string {
	switch e.Type {
	case thread.EventHuman:
		return fmt.Sprintf("[human] %s\n", truncBody(e.Body))
	case thread.EventMessage:
		from := e.From
		if from == "" {
			from = "?"
		}
		return fmt.Sprintf("[%s] %s\n", from, truncBody(e.Body))
	case thread.EventSystem:
		return fmt.Sprintf("[system] %s\n", truncBody(e.Body))
	case thread.EventToolUse:
		return "" // tool calls are internal — agents don't need to see them
	case thread.EventPermissionRequest, thread.EventPermissionGrant, thread.EventPermissionDeny:
		return "" // permission machinery is internal
	default:
		return fmt.Sprintf("[%s] %s\n", e.Type, truncBody(e.Body))
	}
}
