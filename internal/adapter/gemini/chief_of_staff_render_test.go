package gemini

import (
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

func TestRenderChiefOfStaffPrompt_IdentifiesCoS(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: true, Name: "sam"}
	got := RenderChiefOfStaffPrompt(cos, demoPod(), nil, nil)
	for _, want := range []string{"sam", "chief-of-staff", "demo", "Lead: human", "---- YOUR TURN ----"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestRenderChiefOfStaffPrompt_IncludesThread(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: true, Name: "sam"}
	events := []thread.Event{{Type: thread.EventHuman, Body: "deploy the new auth"}}
	got := RenderChiefOfStaffPrompt(cos, demoPod(), nil, events)
	if !strings.Contains(got, "deploy the new auth") {
		t.Errorf("thread not rendered:\n%s", got)
	}
}

func TestRenderChiefOfStaffPrompt_IncludesSkills(t *testing.T) {
	m := alice()
	m.Skills = []string{"go", "cli"}
	cos := config.ChiefOfStaff{Enabled: true, Name: "sam"}
	got := RenderChiefOfStaffPrompt(cos, demoPod(), []config.Member{m}, nil)
	if !strings.Contains(got, "skills: go, cli") {
		t.Errorf("skills missing:\n%s", got)
	}
}
