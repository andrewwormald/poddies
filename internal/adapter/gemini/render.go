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
	b.WriteString("Dispatch: @name task. Breakaway: +@name1+@name2 topic. No @mention = direct answer.\n")
	b.WriteString("No markdown. Plain @name only.\n")

	b.WriteString("---- THREAD ----\n")
	if len(events) > 0 {
		b.WriteString(renderThreadContext(events))
	}

	b.WriteString("---- DISPATCH ----\n")
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
func RenderPrompt(member config.Member, pod config.Pod, roster []config.Member, events []thread.Event, dispatchInstruction string) string {
	var b strings.Builder

	b.WriteString("---- SYSTEM ----\n")
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
	b.WriteString("Be concise. @name to route. Review cycles normal.\n")
	if member.SystemPromptExtra != "" {
		b.WriteString(member.SystemPromptExtra + "\n")
	}

	if dispatchInstruction != "" {
		b.WriteString("---- TASK ----\n")
		fmt.Fprintf(&b, "%s\n", dispatchInstruction)
		recent := events
		if len(recent) > 5 {
			recent = recent[len(recent)-5:]
		}
		if len(recent) > 0 {
			for _, e := range recent {
				b.WriteString(renderEvent(e))
			}
		}
	} else {
		b.WriteString("---- THREAD ----\n")
		if len(events) > 0 {
			b.WriteString(renderThreadContext(events))
		}
	}
	b.WriteString("---- GO ----\n")
	return b.String()
}

// threadVerbatimN is the number of most-recent events rendered verbatim.
const threadVerbatimN = 7

// renderThreadContext builds the thread body for the THREAD section.
// Events beyond threadVerbatimN are compressed into a per-speaker
// last-position summary.
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

// compressSpeakers extracts the last message per speaker from events.
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
	case thread.EventToolUse:
		return "" // tool calls are internal
	case thread.EventPermissionRequest, thread.EventPermissionGrant, thread.EventPermissionDeny:
		return "" // permission machinery is internal
	default:
		return fmt.Sprintf("[%s] %s\n", e.Type, truncBody(e.Body))
	}
}
