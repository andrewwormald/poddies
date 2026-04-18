package claude

import (
	"fmt"
	"strings"

	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

// RenderSystemPrompt builds the text passed to claude via
// --append-system-prompt. It combines role, persona, and pod context
// so the agent behaves consistently with its config.
func RenderSystemPrompt(member config.Member, pod config.Pod, roster []config.Member) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are %q, %s, in the %q pod.\n", member.Name, member.Title, pod.Name)
	if member.Persona != "" {
		fmt.Fprintf(&b, "\nPersona: %s\n", member.Persona)
	}
	if len(roster) > 0 {
		b.WriteString("\nPod members:\n")
		for _, m := range roster {
			you := ""
			if m.Name == member.Name {
				you = " (you)"
			}
			fmt.Fprintf(&b, "- %s: %s%s\n", m.Name, m.Title, you)
		}
	}
	fmt.Fprintf(&b, "\nLead: %s\n", pod.Lead)
	b.WriteString("\nConventions:\n")
	b.WriteString("- Use @name to address another pod member.\n")
	b.WriteString("- Keep replies concise; the thread is shared with the human lead.\n")
	if member.SystemPromptExtra != "" {
		b.WriteString("\n")
		b.WriteString(member.SystemPromptExtra)
		b.WriteString("\n")
	}
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
