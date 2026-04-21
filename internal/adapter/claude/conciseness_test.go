package claude

import (
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/config"
)

func TestConciseness_Member_InSystemPrompt(t *testing.T) {
	got := RenderSystemPrompt(alice(), demoPod(), nil)
	if !strings.Contains(got, "concise") {
		t.Errorf("member system prompt missing conciseness directive:\n%s", got)
	}
}

func TestConciseness_CoS_InSystemPrompt(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: true, Name: "sam"}
	got := RenderChiefOfStaffSystemPrompt(cos, demoPod(), nil)
	if !strings.Contains(got, "specific") {
		t.Errorf("CoS system prompt missing specificity directive:\n%s", got)
	}
}
