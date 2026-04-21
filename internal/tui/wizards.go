package tui

import (
	"fmt"
	"regexp"
)

// slugRe keeps wizard-side validation aligned with config.ValidateSlug
// without pulling the config package's heavier validation surface into
// the TUI. Mismatches will be caught again at config.SaveMember time
// anyway; this is just for fast inline feedback.
var slugRe = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?$`)

// validateSlug is a step.Validate function.
func validateSlug(s string) error {
	if !slugRe.MatchString(s) {
		return fmt.Errorf("use letters, digits, and '-' (no leading/trailing '-')")
	}
	if s == "human" {
		return fmt.Errorf("%q is reserved", s)
	}
	return nil
}

// addMemberWizard asks for the fields needed to create a new member.
// On completion it calls opts.OnAddMember with the collected values.
func addMemberWizard(opts Options) *Wizard {
	return &Wizard{
		Title: "add member",
		Steps: []WizardStep{
			{Question: "Name (slug, e.g. alice):", Validate: validateSlug},
			{Question: "Title (e.g. Staff Engineer):"},
			{
				Question: "Adapter:",
				Choices:  []string{"claude", "gemini", "mock"},
			},
			{
				Question: "Model (type a number or your own):",
				ChoicesFn: func(answers []string) []string {
					adapter := ""
					if len(answers) > 2 {
						adapter = answers[2]
					}
					switch adapter {
					case "gemini":
						return []string{"gemini-2.5-pro", "gemini-2.5-flash"}
					default: // claude, mock, or anything else
						return []string{"claude-opus-4-6", "claude-sonnet-4-6", "claude-haiku-4-5"}
					}
				},
				AllowCustom: true,
			},
			{
				Question: "Effort:",
				Choices:  []string{"low", "medium", "high"},
			},
			{Question: "Persona (optional):", Optional: true},
		},
		OnComplete: func(answers []string) error {
			if opts.OnAddMember == nil {
				return fmt.Errorf("/add is not wired in this TUI session")
			}
			return opts.OnAddMember(AddMemberSpec{
				Name:    answers[0],
				Title:   answers[1],
				Adapter: answers[2],
				Model:   answers[3],
				Effort:  answers[4],
				Persona: answers[5],
			})
		},
	}
}

// removeMemberWizard picks a member from a list and removes it.
// Returns nil if the roster is empty — the caller should surface a
// helpful message instead of entering an empty wizard.
func removeMemberWizard(opts Options) *Wizard {
	roster := listRoster(opts)
	if len(roster) == 0 {
		return nil
	}
	return &Wizard{
		Title: "remove member",
		Steps: []WizardStep{
			{Question: "Pick a member to remove:", Choices: roster},
			{Question: "Are you sure?", Choices: []string{"yes", "no"}},
		},
		OnComplete: func(answers []string) error {
			if answers[1] != "yes" {
				return fmt.Errorf("cancelled")
			}
			if opts.OnRemoveMember == nil {
				return fmt.Errorf("/remove is not wired in this TUI session")
			}
			return opts.OnRemoveMember(answers[0])
		},
	}
}

// editMemberWizard picks a member, then a field, then a new value.
func editMemberWizard(opts Options) *Wizard {
	roster := listRoster(opts)
	if len(roster) == 0 {
		return nil
	}
	return &Wizard{
		Title: "edit member",
		Steps: []WizardStep{
			{Question: "Pick a member to edit:", Choices: roster},
			{
				Question: "Which field?",
				Choices:  []string{"title", "adapter", "model", "effort", "persona"},
			},
			{Question: "New value (type 1..N or your own):", AllowCustom: true},
		},
		OnComplete: func(answers []string) error {
			if opts.OnEditMember == nil {
				return fmt.Errorf("/edit is not wired in this TUI session")
			}
			return opts.OnEditMember(answers[0], answers[1], answers[2])
		},
	}
}

// listRoster uses opts.OnListMembers if available, otherwise falls back
// to opts.Members (the display-only roster). Keeps the wizard usable
// even when the dynamic lookup isn't wired (tests, CLI disabled modes).
func listRoster(opts Options) []string {
	if opts.OnListMembers != nil {
		return opts.OnListMembers()
	}
	return opts.Members
}

// onboardingAddMemberWizard wraps addMemberWizard so its OnComplete can
// chain — asking "add another?" — until the user opts out. Each member
// still goes through the full addMemberWizard flow.
func onboardingAddMemberWizard(opts Options) *Wizard {
	base := addMemberWizard(opts)
	base.Title = "onboarding · add member"
	base.Preamble = "Welcome! Let's add your first agent.\n\nA pod is a team of AI agents — each has a name you @mention, a role, and a persona that shapes how it responds. Add 2–3 members for best results; you can /add more after."
	originalComplete := base.OnComplete
	base.OnComplete = func(answers []string) error {
		if err := originalComplete(answers); err != nil {
			return err
		}
		// Side effect only — the chaining wizard is installed by the
		// Model via a post-completion message. See Update's handling of
		// LoopDoneMsg-style transitions; for simplicity in v1 we stop
		// after the first member and surface a prompt in the status line
		// telling the user they can type /add to add more. Chaining via
		// an "add another?" step can be layered in later without
		// changing this signature.
		return nil
	}
	return base
}
