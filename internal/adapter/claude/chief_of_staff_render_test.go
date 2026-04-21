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
	for _, want := range []string{"sam", "dispatcher", "demo", "Lead: human"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestRenderChiefOfStaffSystemPrompt_IncludesRoster(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: true, Name: "sam"}
	got := RenderChiefOfStaffSystemPrompt(cos, demoPod(), []config.Member{alice()})
	if !strings.Contains(got, "alice(") {
		t.Errorf("roster missing alice:\n%s", got)
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
	if !strings.Contains(got, "Dispatch") {
		t.Errorf("dispatch CTA missing:\n%s", got)
	}
}
