package gemini

import (
	"fmt"
	"strings"

	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

// RenderPrompt builds the single prompt sent to the Gemini CLI via
// stdin. Unlike Claude, the Gemini CLI does not expose a separate
// system-prompt flag, so we inline the role/pod context at the top
// followed by the thread transcript and call-to-action.
//
// Structure:
//
//	---- SYSTEM ----
//	<role, persona, roster, conventions>
//	---- THREAD ----
//	[human] ...
//	[alice] ...
//	---- YOUR TURN ----
//	<CTA to the invoked member>
//
// The explicit dividers help weaker models keep the sections separate;
// Gemini 2.5 typically doesn't need them but they're low-cost insurance.
func RenderPrompt(member config.Member, pod config.Pod, roster []config.Member, events []thread.Event) string {
	var b strings.Builder

	b.WriteString("---- SYSTEM ----\n")
	fmt.Fprintf(&b, "You are %q, %s, in the %q pod.\n", member.Name, member.Title, pod.Name)
	if member.Persona != "" {
		fmt.Fprintf(&b, "Persona: %s\n", member.Persona)
	}
	if len(roster) > 0 {
		b.WriteString("Pod members:\n")
		for _, m := range roster {
			you := ""
			if m.Name == member.Name {
				you = " (you)"
			}
			fmt.Fprintf(&b, "- %s: %s%s\n", m.Name, m.Title, you)
		}
	}
	fmt.Fprintf(&b, "Lead: %s\n", pod.Lead)
	b.WriteString("Conventions: use @name to address members. Keep replies concise.\n")
	if member.SystemPromptExtra != "" {
		b.WriteString(member.SystemPromptExtra)
		b.WriteString("\n")
	}

	b.WriteString("\n---- THREAD ----\n")
	if len(events) == 0 {
		b.WriteString("(empty)\n")
	} else {
		for _, e := range events {
			b.WriteString(renderEvent(e))
		}
	}

	b.WriteString("\n---- YOUR TURN ----\n")
	fmt.Fprintf(&b, "You are %s. Write your next message in the thread. Use @name to address specific members. Do not repeat prior messages verbatim.\n", member.Name)
	return b.String()
}

// renderEvent formats a single event into a transcript line. Shares
// the same vocabulary as the claude renderer for consistency but lives
// in the gemini package so it can drift if Gemini ever needs different
// formatting.
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
