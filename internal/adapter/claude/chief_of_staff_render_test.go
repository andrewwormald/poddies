package claude

import (
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

func TestRenderChiefOfStaffSystemPrompt_IdentifiesCoS(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: true, Name: "sam"}
	got := RenderChiefOfStaffSystemPrompt(cos, demoPod(), nil)
	for _, want := range []string{"sam", "chief-of-staff", "demo", "Lead: human"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestRenderChiefOfStaffSystemPrompt_IncludesSkills(t *testing.T) {
	m := alice()
	m.Skills = []string{"go", "distributed-systems"}
	cos := config.ChiefOfStaff{Enabled: true, Name: "sam"}
	got := RenderChiefOfStaffSystemPrompt(cos, demoPod(), []config.Member{m})
	if !strings.Contains(got, "skills: go, distributed-systems") {
		t.Errorf("skills missing from roster:\n%s", got)
	}
}

func TestRenderChiefOfStaffSystemPrompt_DefaultNameWhenUnset(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: true}
	got := RenderChiefOfStaffSystemPrompt(cos, demoPod(), nil)
	if !strings.Contains(got, config.DefaultChiefOfStaffName) {
		t.Errorf("default name missing:\n%s", got)
	}
}

func TestRenderUserPromptForCoS_UsesCoSName(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: true, Name: "sam"}
	got := RenderUserPromptForCoS(cos, []thread.Event{
		{Type: thread.EventHuman, Body: "help?"},
	})
	if !strings.Contains(got, "You are sam, the chief-of-staff") {
		t.Errorf("CTA should name sam:\n%s", got)
	}
	if !strings.Contains(got, "answer directly") {
		t.Errorf("gray-area directive missing:\n%s", got)
	}
}
