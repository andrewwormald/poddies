package tui

import (
	"errors"
	"strings"
	"testing"
)

func twoStep() *Wizard {
	return &Wizard{
		Title: "test",
		Steps: []WizardStep{
			{Question: "name?"},
			{Question: "pick", Choices: []string{"a", "b", "c"}},
		},
	}
}

func TestWizard_CurrentStep_InitialAndEnd(t *testing.T) {
	w := twoStep()
	if s := w.CurrentStep(); s == nil || s.Question != "name?" {
		t.Fatalf("want first step, got %+v", s)
	}
	_, _ = w.Next("alice")
	_, _ = w.Next("1")
	if s := w.CurrentStep(); s != nil {
		t.Errorf("want nil after all steps, got %+v", s)
	}
}

func TestWizard_Progress(t *testing.T) {
	w := twoStep()
	cur, total := w.Progress()
	if cur != 1 || total != 2 {
		t.Errorf("initial 1/2, got %d/%d", cur, total)
	}
	_, _ = w.Next("alice")
	cur, _ = w.Progress()
	if cur != 2 {
		t.Errorf("want step 2 after one answer, got %d", cur)
	}
}

func TestWizard_Next_FreeTextStep_AcceptsTrimmed(t *testing.T) {
	w := twoStep()
	done, err := w.Next("  alice  ")
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Error("should not be done after step 1 of 2")
	}
	if w.Answers()[0] != "alice" {
		t.Errorf("want trimmed 'alice', got %q", w.Answers()[0])
	}
}

func TestWizard_Next_ChoiceStep_PickByNumber(t *testing.T) {
	w := twoStep()
	_, _ = w.Next("alice")
	done, err := w.Next("2")
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Error("want done after final step")
	}
	if w.Answers()[1] != "b" {
		t.Errorf("want 'b', got %q", w.Answers()[1])
	}
}

func TestWizard_Next_ChoiceStep_PickByLiteral(t *testing.T) {
	w := twoStep()
	_, _ = w.Next("alice")
	_, err := w.Next("c")
	if err != nil {
		t.Fatal(err)
	}
	if w.Answers()[1] != "c" {
		t.Errorf("want 'c', got %q", w.Answers()[1])
	}
}

func TestWizard_Next_ChoiceStep_OutOfRange_Errors(t *testing.T) {
	w := twoStep()
	_, _ = w.Next("alice")
	_, err := w.Next("9")
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("want out-of-range error, got %v", err)
	}
	// wizard should NOT advance on error
	if cur, _ := w.Progress(); cur != 2 {
		t.Errorf("want still on step 2, got %d", cur)
	}
}

func TestWizard_Next_ChoiceStep_CustomRejectedByDefault(t *testing.T) {
	w := twoStep()
	_, _ = w.Next("alice")
	_, err := w.Next("custom-value")
	if err == nil {
		t.Error("want error when custom input used on non-AllowCustom choice step")
	}
}

func TestWizard_Next_ChoiceStep_CustomAcceptedWhenAllowed(t *testing.T) {
	w := &Wizard{
		Steps: []WizardStep{{
			Question:    "adapter",
			Choices:     []string{"claude", "gemini"},
			AllowCustom: true,
		}},
	}
	done, err := w.Next("my-local-llama")
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Error("should be done")
	}
	if w.Answers()[0] != "my-local-llama" {
		t.Errorf("want custom value, got %q", w.Answers()[0])
	}
}

func TestWizard_Next_EmptyAnswer_RequiredErrors(t *testing.T) {
	w := twoStep()
	_, err := w.Next("")
	if err == nil {
		t.Error("want error on empty required step")
	}
}

func TestWizard_Next_EmptyAnswer_OptionalAccepted(t *testing.T) {
	w := &Wizard{Steps: []WizardStep{{Question: "persona?", Optional: true}}}
	done, err := w.Next("")
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Error("should be done")
	}
	if w.Answers()[0] != "" {
		t.Errorf("want empty answer, got %q", w.Answers()[0])
	}
}

func TestWizard_Next_Validate_Errors(t *testing.T) {
	w := &Wizard{Steps: []WizardStep{{
		Question: "slug",
		Validate: func(s string) error {
			if strings.Contains(s, " ") {
				return errors.New("no spaces")
			}
			return nil
		},
	}}}
	_, err := w.Next("has spaces")
	if err == nil {
		t.Fatal("want validation error")
	}
	// didn't advance
	if cur, _ := w.Progress(); cur != 1 {
		t.Errorf("want still on step 1, got %d", cur)
	}
}

func TestWizard_Next_Validate_Passes(t *testing.T) {
	w := &Wizard{Steps: []WizardStep{{
		Question: "slug",
		Validate: func(s string) error { return nil },
	}}}
	done, err := w.Next("alice")
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Error("should be done")
	}
}

func TestWizard_Next_PastLastStep_ReturnsDone(t *testing.T) {
	w := &Wizard{Steps: []WizardStep{{Question: "q"}}}
	_, _ = w.Next("a")
	done, err := w.Next("extra")
	if err != nil {
		t.Error("no error expected once done")
	}
	if !done {
		t.Error("want done")
	}
}

func TestWizard_Cancel_InvokesHook(t *testing.T) {
	called := false
	w := &Wizard{
		Steps:    []WizardStep{{Question: "q"}},
		OnCancel: func() { called = true },
	}
	w.Cancel()
	if !called {
		t.Error("OnCancel not invoked")
	}
	if w.CurrentStep() != nil {
		t.Error("Cancel should move past last step")
	}
}

func TestWizard_Complete_RunsHook(t *testing.T) {
	var got []string
	w := &Wizard{
		Steps:      []WizardStep{{Question: "q"}},
		OnComplete: func(answers []string) error { got = answers; return nil },
	}
	_, _ = w.Next("alice")
	if err := w.Complete(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "alice" {
		t.Errorf("want [alice], got %v", got)
	}
}

func TestWizard_Complete_PropagatesError(t *testing.T) {
	w := &Wizard{
		Steps:      []WizardStep{{Question: "q"}},
		OnComplete: func([]string) error { return errors.New("boom") },
	}
	_, _ = w.Next("a")
	if err := w.Complete(); err == nil || err.Error() != "boom" {
		t.Errorf("want 'boom', got %v", err)
	}
}

func TestResolveAnswer_TrimsWhitespace(t *testing.T) {
	s, err := resolveAnswer(WizardStep{Question: "x"}, "   hi   ")
	if err != nil {
		t.Fatal(err)
	}
	if s != "hi" {
		t.Errorf("want hi, got %q", s)
	}
}

func TestTrimSpace_EdgeCases(t *testing.T) {
	cases := map[string]string{
		"":              "",
		"hi":            "hi",
		"  hi  ":        "hi",
		"\n\t hi \t\n ": "hi",
		"   ":           "",
	}
	for in, want := range cases {
		if got := trimSpace(in); got != want {
			t.Errorf("trimSpace(%q): want %q, got %q", in, want, got)
		}
	}
}
