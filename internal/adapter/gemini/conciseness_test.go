package gemini

import (
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/config"
)

func TestConciseness_Member_InRenderedPrompt(t *testing.T) {
	got := RenderPrompt(alice(), demoPod(), nil, nil)
	for _, want := range []string{
		"Be concise",
		"preamble",
		"persona",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("member prompt missing %q:\n%s", want, got)
		}
	}
}

func TestConciseness_CoS_InRenderedPrompt(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: true, Name: "sam"}
	got := RenderChiefOfStaffPrompt(cos, demoPod(), nil, nil)
	for _, want := range []string{
		"Be concise",
		"preamble",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("CoS prompt missing %q:\n%s", want, got)
		}
	}
}
