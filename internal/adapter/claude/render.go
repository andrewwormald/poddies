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
	fmt.Fprintf(&b, "You are %q, the chief-of-staff facilitator for the %q pod.\n", cos.ResolvedName(), pod.Name)
	b.WriteString("\nYour role is to keep the pod moving:\n")
	b.WriteString("- Route routing tie-breaks (when no member was @mentioned).\n")
	b.WriteString("- Post concise milestone summaries for the human lead.\n")
	b.WriteString("- Handle requests that don't clearly land on any pod member.\n")
	if len(roster) > 0 {
		b.WriteString("\nPod members:\n")
		for _, m := range roster {
			fmt.Fprintf(&b, "- %s: %s", m.Name, m.Title)
			if len(m.Skills) > 0 {
				fmt.Fprintf(&b, " [skills: %s]", strings.Join(m.Skills, ", "))
			}
			b.WriteString("\n")
		}
	}
	fmt.Fprintf(&b, "\nLead: %s\n", pod.Lead)
	b.WriteString("\nConventions:\n")
	b.WriteString("- Address a specific member with @name when that member clearly owns the request.\n")
	b.WriteString(concisenessBlock())
	b.WriteString(collaborationBlock())
	return b.String()
}

// RenderSystemPrompt builds the text passed to claude via
// --append-system-prompt. It combines role, persona, and pod context
// so the agent behaves consistently with its config.
func RenderSystemPrompt(member config.Member, pod config.Pod, roster []config.Member) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are %q, %s, in the %q pod.\n", member.Name, member.Title, pod.Name)
	fmt.Fprintf(&b, "Your domain is strictly %s. Do not attempt work outside this role — route it.\n", member.Title)
	if member.Persona != "" {
		fmt.Fprintf(&b, "\nPersona: %s\n", member.Persona)
	}
	if len(roster) > 0 {
		b.WriteString("\nPod members (route to the right specialist):\n")
		for _, m := range roster {
			you := " (you)"
			if m.Name != member.Name {
				you = ""
			}
			fmt.Fprintf(&b, "- %s: %s%s\n", m.Name, m.Title, you)
		}
	}
	fmt.Fprintf(&b, "\nLead: %s\n", pod.Lead)
	b.WriteString("\nConventions:\n")
	b.WriteString("- Stay in your lane. If a request belongs to another member's role, route it immediately with @name. Do not attempt it yourself.\n")
	b.WriteString("- Use @name to hand off to a specific pod member.\n")
	b.WriteString(concisenessBlock())
	b.WriteString(collaborationBlock())
	if member.SystemPromptExtra != "" {
		b.WriteString("\n")
		b.WriteString(member.SystemPromptExtra)
		b.WriteString("\n")
	}
	return b.String()
}

// concisenessBlock is the shared directive appended to every member
// and CoS system prompt. Central so that a future verbosity-aware
// pod config can swap the block without hunting through renderers.
//
// Every line an agent produces costs tokens (for it to output + for
// every subsequent turn to re-read). The directive pushes agents off
// their default "friendly assistant" register — preambles, echoed
// asks, ceremonial acknowledgements — and onto a tighter voice that
// respects the shared thread.
func concisenessBlock() string {
	return "- Be concise. No preamble, no 'great question!' openers, no restating the human's ask.\n" +
		"- Stay in your persona's voice. Don't narrate what you're about to do — just do it.\n" +
		"- Every line is shared with the human lead and re-processed on every subsequent turn. Make it count.\n"
}

// collaborationBlock is appended to every member system prompt to make
// multi-turn review/revise cycles explicit. Without this, agents assume
// they should produce a final answer in one shot and may not push back
// or request revisions from peers.
func collaborationBlock() string {
	return "\nCollaboration:\n" +
		"- Multi-turn cycles are normal: produce work, another agent may review it and push back, you then revise.\n" +
		"- When reviewing a peer's work, be specific about what needs to change and @mention them to request it.\n" +
		"- When you've addressed feedback, @mention the reviewer so they can verify.\n" +
		"- Don't self-declare work complete — the human lead approves final outcomes.\n"
}

// threadVerbatimN is the number of most-recent events rendered verbatim.
// Older events are compressed into a per-speaker "last position" summary.
const threadVerbatimN = 15

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
// RenderUserPrompt. Uses the CoS name in the call-to-action so the
// model doesn't get told "you are " (zero-value).
func RenderUserPromptForCoS(cos config.ChiefOfStaff, events []thread.Event) string {
	var b strings.Builder
	if len(events) == 0 {
		b.WriteString("The thread is empty.\n")
	} else {
		b.WriteString("Conversation so far:\n\n")
		b.WriteString(renderThreadContext(events))
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "You are %s, the chief-of-staff. Respond with your next message in the thread.\n", cos.ResolvedName())
	b.WriteString("If a specific pod member clearly owns the request, use @name to hand it to them. Otherwise answer directly and completely.\n")
	return b.String()
}

// RenderUserPrompt builds the text passed to claude via -p. It renders
// the thread as a flat transcript and ends with a call to action for
// the invoked member.
func RenderUserPrompt(member config.Member, events []thread.Event) string {
	var b strings.Builder
	if len(events) == 0 {
		b.WriteString("The thread is empty. Please start the conversation.\n")
	} else {
		b.WriteString("Conversation so far:\n\n")
		b.WriteString(renderThreadContext(events))
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "You are %s. Respond with your next message in the thread.\n", member.Name)
	b.WriteString("Use @name to address specific members. Do not repeat prior messages verbatim.\n")
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
	case thread.EventPermissionRequest:
		return fmt.Sprintf("[permission_request from %s] action=%s\n", e.From, e.Action)
	case thread.EventPermissionGrant:
		return fmt.Sprintf("[permission_grant by %s for %s]\n", e.From, e.RequestID)
	case thread.EventPermissionDeny:
		return fmt.Sprintf("[permission_deny by %s for %s]\n", e.From, e.RequestID)
	default:
		return fmt.Sprintf("[unknown:%s] %s\n", e.Type, truncBody(e.Body))
	}
}
