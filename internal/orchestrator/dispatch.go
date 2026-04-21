package orchestrator

import (
	"strings"
)

// Dispatch is a single targeted instruction from the CoS to a member.
type Dispatch struct {
	Member      string // member name (from @mention)
	Instruction string // what the CoS wants this member to do
}

// DispatchResult holds both individual dispatches and breakaway groups.
type DispatchResult struct {
	Dispatches []Dispatch
	Breakaways []BreakawaySpec
}

// ParseDispatch extracts dispatches and breakaways from a CoS response.
//
// Individual dispatch: `@alice Build the calculator.`
// Breakaway:           `+@alice+@bob Discuss the auth bug approach.`
//
// Lines starting with `+@` are parsed as breakaways (multiple agents
// discussing together). Lines starting with `@` are individual dispatches.
func ParseDispatch(body string, members map[string]struct{}) DispatchResult {
	var result DispatchResult
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Strip markdown bold formatting (CoS may use **@name**).
		clean := strings.ReplaceAll(line, "**", "")
		clean = strings.ReplaceAll(clean, "*", "")
		clean = strings.TrimSpace(clean)
		if clean == "" {
			continue
		}

		// Breakaway: +@alice+@bob topic
		if strings.HasPrefix(clean, "+@") {
			if spec, ok := parseBreakaway(clean, members); ok {
				result.Breakaways = append(result.Breakaways, spec)
			}
			continue
		}

		// Individual dispatch: @name instruction
		if clean[0] != '@' {
			continue
		}
		name, instruction, _ := strings.Cut(clean[1:], " ")
		name = strings.TrimRight(name, ",:;.!?")
		if _, ok := members[name]; !ok {
			continue
		}
		instruction = strings.TrimSpace(instruction)
		if instruction == "" {
			continue
		}
		result.Dispatches = append(result.Dispatches, Dispatch{
			Member:      name,
			Instruction: instruction,
		})
	}
	return result
}

// parseBreakaway parses "+@alice+@bob topic" into a BreakawaySpec.
func parseBreakaway(line string, members map[string]struct{}) (BreakawaySpec, bool) {
	// Strip leading "+" and split on spaces to find the @name+@name part and the topic.
	parts := strings.SplitN(line, " ", 2)
	namePart := parts[0] // "+@alice+@bob"
	topic := ""
	if len(parts) > 1 {
		topic = strings.TrimSpace(parts[1])
	}
	if topic == "" {
		return BreakawaySpec{}, false
	}

	// Parse member names from "+@alice+@bob"
	var names []string
	for _, seg := range strings.Split(namePart, "+") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		if seg[0] == '@' {
			seg = seg[1:]
		}
		seg = strings.TrimRight(seg, ",:;.!?")
		if _, ok := members[seg]; ok {
			names = append(names, seg)
		}
	}
	if len(names) < 2 {
		return BreakawaySpec{}, false
	}
	return BreakawaySpec{Members: names, Topic: topic}, true
}
