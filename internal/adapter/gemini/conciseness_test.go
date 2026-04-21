package gemini

import (
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/config"
)

func TestConciseness_Member_InRenderedPrompt(t *testing.T) {
	got := RenderPrompt(alice(), demoPod(), nil, nil, "")
	if !strings.Contains(got, "concise") {
		t.Errorf("member prompt missing conciseness directive:\n%s", got)
	}
}

func TestConciseness_CoS_InRenderedPrompt(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: true, Name: "sam"}
	got := RenderChiefOfStaffPrompt(cos, demoPod(), nil, nil)
	if !strings.Contains(got, "Dispatch") {
		t.Errorf("CoS prompt missing dispatch directive:\n%s", got)
	}
}
