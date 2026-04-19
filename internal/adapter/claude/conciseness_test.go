package claude

import (
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/config"
)

// TestConciseness_Member_InSystemPrompt guards A3: the directive must
// be present in every member's system prompt so agents stop wasting
// output tokens on preambles.
func TestConciseness_Member_InSystemPrompt(t *testing.T) {
	got := RenderSystemPrompt(alice(), demoPod(), nil)
	for _, want := range []string{
		"Be concise",
		"No preamble",
		"Stay in your persona",
		"Every line",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("member system prompt missing %q:\n%s", want, got)
		}
	}
}

// TestConciseness_CoS_InSystemPrompt — same for the chief-of-staff.
// The CoS runs often (every human message with gray_area, every Nth
// turn with milestone) so verbose default register is especially
// expensive here.
func TestConciseness_CoS_InSystemPrompt(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: true, Name: "sam"}
	got := RenderChiefOfStaffSystemPrompt(cos, demoPod(), nil)
	for _, want := range []string{
		"Be concise",
		"No preamble",
		"Stay in your persona",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("CoS system prompt missing %q:\n%s", want, got)
		}
	}
}

// TestConciseness_Block_AvoidsEchoingTheHumanAsk is defensive: if a
// future edit softens the directive, the test still catches the
// specific anti-pattern we want to stamp out — "great question!"
// openers.
func TestConciseness_Block_NamesTheAntipatterns(t *testing.T) {
	block := concisenessBlock()
	for _, want := range []string{"great question", "restating", "narrate"} {
		if !strings.Contains(strings.ToLower(block), want) {
			t.Errorf("directive should name %q as an anti-pattern; got:\n%s", want, block)
		}
	}
}
