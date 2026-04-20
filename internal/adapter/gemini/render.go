package gemini

import (
	"fmt"
	"strings"

	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

// RenderChiefOfStaffPrompt builds the single prompt for a chief-of-staff
// invocation. Replaces the previous (broken) path that passed a
// zero-value Member into RenderPrompt — that left the CoS with no
// identity or role instructions at all.
func RenderChiefOfStaffPrompt(cos config.ChiefOfStaff, pod config.Pod, roster []config.Member, events []thread.Event) string {
	var b strings.Builder
	b.WriteString("---- SYSTEM ----\n")
	fmt.Fprintf(&b, "You are %q, the chief-of-staff facilitator for the %q pod.\n", cos.ResolvedName(), pod.Name)
	b.WriteString("Your role: route tie-breaks, milestone summaries, handle requests that don't clearly land on a pod member.\n")
	if len(roster) > 0 {
		b.WriteString("Pod members:\n")
		for _, m := range roster {
			fmt.Fprintf(&b, "- %s: %s", m.Name, m.Title)
			if len(m.Skills) > 0 {
				fmt.Fprintf(&b, " [skills: %s]", strings.Join(m.Skills, ", "))
			}
			b.WriteString("\n")
		}
	}
	fmt.Fprintf(&b, "Lead: %s\n", pod.Lead)
	b.WriteString("Conventions: use @name to address a member when they clearly own the request; otherwise answer directly.\n")
	b.WriteString(concisenessBlock())

	b.WriteString("\n---- THREAD ----\n")
	if len(events) == 0 {
		b.WriteString("(empty)\n")
	} else {
		for _, e := range events {
			b.WriteString(renderEvent(e))
		}
	}

	b.WriteString("\n---- YOUR TURN ----\n")
	fmt.Fprintf(&b, "You are %s, the chief-of-staff. Write your next message in the thread. If a specific member clearly owns the request, @mention them; otherwise answer directly and completely.\n", cos.ResolvedName())
	return b.String()
}

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
	fmt.Fprintf(&b, "Your domain is strictly %s. Do not attempt work outside this role — route it.\n", member.Title)
	if member.Persona != "" {
		fmt.Fprintf(&b, "Persona: %s\n", member.Persona)
	}
	if len(roster) > 0 {
		b.WriteString("Pod members (route to the right specialist):\n")
		for _, m := range roster {
			you := " (you)"
			if m.Name != member.Name {
				you = ""
			}
			fmt.Fprintf(&b, "- %s: %s%s\n", m.Name, m.Title, you)
		}
	}
	fmt.Fprintf(&b, "Lead: %s\n", pod.Lead)
	b.WriteString("Conventions: stay in your lane — if a request belongs to another member's role, route it immediately with @name; do not attempt it yourself.\n")
	b.WriteString(concisenessBlock())
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

// concisenessBlock is the shared directive appended to every member
// and CoS prompt. Same intent as claude.concisenessBlock (see that
// package for the longer rationale) — the text is duplicated here to
// keep the adapter packages independent.
func concisenessBlock() string {
	return "Be concise: no preamble, no 'great question!' openers, no restating the human's ask.\n" +
		"Stay in your persona's voice; don't narrate what you're about to do — just do it.\n" +
		"Every line is shared with the human lead and re-processed on every subsequent turn. Make it count.\n"
}

// maxBodyChars is the maximum characters rendered from any single event
// body — mirrors the constant in the claude renderer.
const maxBodyChars = 600

func truncBody(s string) string {
	if len(s) <= maxBodyChars {
		return s
	}
	return fmt.Sprintf("%s … [+%d chars]", s[:maxBodyChars], len(s)-maxBodyChars)
}

// renderEvent formats a single event into a transcript line. Shares
// the same vocabulary as the claude renderer for consistency but lives
// in the gemini package so it can drift if Gemini ever needs different
// formatting.
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
