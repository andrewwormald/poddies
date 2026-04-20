package tui

import (
	"fmt"
	"strconv"
)

// Wizard is a linear multi-step prompt flow used for onboarding and
// slash-command interactions inside the TUI. A Wizard is data-driven:
// each WizardStep specifies a question and optional numbered choices;
// the Wizard accumulates answers and calls OnComplete when done.
//
// Wizards are created, advanced one Step at a time via Next, and
// rendered by the Model's View. They are not goroutine-safe — the TUI
// runs single-threaded through bubbletea's message loop.
type Wizard struct {
	Title string
	// Preamble is shown above the first step question to orient the user.
	// Leave empty for non-onboarding wizards where context is obvious.
	Preamble string
	Steps []WizardStep
	// OnComplete runs when the user finishes the last step. answers[i]
	// is the resolved answer for Steps[i] (after choice-number
	// expansion).
	OnComplete func(answers []string) error
	// OnCancel, if non-nil, runs when the user cancels (e.g. /cancel).
	OnCancel func()

	current int
	answers []string
}

// WizardStep is one question in a Wizard.
type WizardStep struct {
	// Question is the prompt shown to the user.
	Question string
	// Choices, if non-empty, renders as "1. X  2. Y  3. …". The user can
	// type a number to pick one, or type free text to provide a custom
	// answer. When Choices is empty the step is free-text only.
	Choices []string
	// ChoicesFn, if set, overrides Choices with a dynamic list computed
	// from answers collected so far. Useful when a step's options depend
	// on a previous answer (e.g. model list filtered by adapter).
	ChoicesFn func(answers []string) []string
	// AllowCustom controls whether typed text that doesn't match a
	// choice number is accepted. Ignored when Choices is empty (free
	// text is always allowed). Default false — only numbered picks.
	AllowCustom bool
	// Validate, if set, runs against the resolved answer. Any error is
	// surfaced to the user and the step is re-asked.
	Validate func(string) error
	// Optional makes the step skippable with an empty answer.
	Optional bool
}

// CurrentStepChoices returns the resolved choices for the current step,
// calling ChoicesFn with answers collected so far when present.
func (w *Wizard) CurrentStepChoices() []string {
	step := w.CurrentStep()
	if step == nil {
		return nil
	}
	if step.ChoicesFn != nil {
		return step.ChoicesFn(w.answers)
	}
	return step.Choices
}

// CurrentStep returns the step the wizard is waiting on. Returns nil
// when the wizard has completed.
func (w *Wizard) CurrentStep() *WizardStep {
	if w.current >= len(w.Steps) {
		return nil
	}
	return &w.Steps[w.current]
}

// Progress returns (currentIndex, totalSteps) for rendering "step 2/5".
func (w *Wizard) Progress() (int, int) {
	return w.current + 1, len(w.Steps)
}

// Answers returns the answers captured so far (read-only view).
func (w *Wizard) Answers() []string {
	out := make([]string, len(w.answers))
	copy(out, w.answers)
	return out
}

// Next resolves raw user input against the current step, records the
// answer, and advances. Returns done=true when the final step was just
// answered (caller should then invoke OnComplete). If validation or
// choice-matching fails, returns an error describing the issue — the
// wizard stays on the same step.
func (w *Wizard) Next(raw string) (done bool, err error) {
	step := w.CurrentStep()
	if step == nil {
		return true, nil
	}
	// Build a copy with dynamic choices resolved so resolveAnswer sees
	// the correct list regardless of whether ChoicesFn is set.
	resolved := *step
	if resolved.ChoicesFn != nil {
		resolved.Choices = resolved.ChoicesFn(w.answers)
	}
	answer, err := resolveAnswer(resolved, raw)
	if err != nil {
		return false, err
	}
	if step.Validate != nil {
		if verr := step.Validate(answer); verr != nil {
			return false, verr
		}
	}
	w.answers = append(w.answers, answer)
	w.current++
	return w.current >= len(w.Steps), nil
}

// Cancel marks the wizard as done and invokes OnCancel if set. The
// Model clears its wizard pointer after calling this.
func (w *Wizard) Cancel() {
	if w.OnCancel != nil {
		w.OnCancel()
	}
	w.current = len(w.Steps)
}

// Complete is invoked by the Model once Next returns done=true. It runs
// OnComplete with the captured answers. Returned error (if any) should
// surface in the status line.
func (w *Wizard) Complete() error {
	if w.OnComplete == nil {
		return nil
	}
	return w.OnComplete(w.answers)
}

// resolveAnswer interprets raw input for a step:
//   - empty input on Optional step → ""
//   - empty input on required step → error
//   - input that parses as 1..N and Choices non-empty → choice[n-1]
//   - input matching a choice text exactly → that choice
//   - any other input with Choices set but AllowCustom=false → error
//   - otherwise the raw string (trimmed) is the answer
func resolveAnswer(step WizardStep, raw string) (string, error) {
	trim := trimSpace(raw)
	if trim == "" {
		if step.Optional {
			return "", nil
		}
		return "", fmt.Errorf("an answer is required")
	}
	if len(step.Choices) == 0 {
		return trim, nil
	}
	// try number pick
	if n, err := strconv.Atoi(trim); err == nil {
		if n >= 1 && n <= len(step.Choices) {
			return step.Choices[n-1], nil
		}
		return "", fmt.Errorf("choice %d is out of range 1..%d", n, len(step.Choices))
	}
	// try literal match against a choice string
	for _, c := range step.Choices {
		if c == trim {
			return c, nil
		}
	}
	if !step.AllowCustom {
		return "", fmt.Errorf("not a valid choice; type 1..%d", len(step.Choices))
	}
	return trim, nil
}

// trimSpace is a small wrapper so the tui package doesn't pull in
// strings for a single call on the hot path. Keeps behavior obvious.
func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end {
		c := s[start]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			start++
			continue
		}
		break
	}
	for end > start {
		c := s[end-1]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			end--
			continue
		}
		break
	}
	return s[start:end]
}
