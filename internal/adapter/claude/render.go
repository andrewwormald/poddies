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

// RenderUserPromptForCoS is the CoS-flavored counterpart of
// RenderUserPrompt. Uses the CoS name in the call-to-action so the
// model doesn't get told "you are " (zero-value).
func RenderUserPromptForCoS(cos config.ChiefOfStaff, events []thread.Event) string {
	var b strings.Builder
	if len(events) == 0 {
		b.WriteString("The thread is empty.\n")
	} else {
		b.WriteString("Conversation so far:\n\n")
		for _, e := range events {
			b.WriteString(renderEvent(e))
		}
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
		for _, e := range events {
			b.WriteString(renderEvent(e))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "You are %s. Respond with your next message in the thread.\n", member.Name)
	b.WriteString("Use @name to address specific members. Do not repeat prior messages verbatim.\n")
	return b.String()
}

// renderEvent formats a single event into a transcript line. Unknown
// event types are rendered as "[unknown:<type>]" so nothing is silently
// dropped on the way into the prompt.
func renderEvent(e thread.Event) string {
	switch e.Type {
	case thread.EventHuman:
		return fmt.Sprintf("[human] %s\n", e.Body)
	case thread.EventMessage:
		from := e.From
		if from == "" {
			from = "?"
		}
		return fmt.Sprintf("[%s] %s\n", from, e.Body)
	case thread.EventSystem:
		return fmt.Sprintf("[system] %s\n", e.Body)
	case thread.EventPermissionRequest:
		return fmt.Sprintf("[permission_request from %s] action=%s\n", e.From, e.Action)
	case thread.EventPermissionGrant:
		return fmt.Sprintf("[permission_grant by %s for %s]\n", e.From, e.RequestID)
	case thread.EventPermissionDeny:
		return fmt.Sprintf("[permission_deny by %s for %s]\n", e.From, e.RequestID)
	default:
		return fmt.Sprintf("[unknown:%s] %s\n", e.Type, e.Body)
	}
}
