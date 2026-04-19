package cli

import (
	"errors"
	"testing"

	"github.com/andrewwormald/poddies/internal/config"
)

func TestAddMember_CollidesWithCoSName_Errors(t *testing.T) {
	_, root := initLocalRoot(t)
	if _, err := CreatePod(root, "demo"); err != nil {
		t.Fatal(err)
	}
	// Enable CoS with name "sam" on the pod.
	p, err := config.LoadPod(PodDir(root, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	p.ChiefOfStaff = config.ChiefOfStaff{
		Enabled:  true,
		Name:     "sam",
		Adapter:  config.AdapterMock,
		Model:    "m",
		Triggers: []config.Trigger{config.TriggerMilestone},
	}
	if err := config.SavePod(PodDir(root, "demo"), p); err != nil {
		t.Fatal(err)
	}

	err = AddMember(root, "demo", config.Member{
		Name: "sam", Title: "T", Adapter: config.AdapterMock, Model: "m", Effort: config.EffortHigh,
	})
	if !errors.Is(err, ErrCoSNameCollision) {
		t.Errorf("want ErrCoSNameCollision, got %v", err)
	}
}

func TestAddMember_NoCoS_NoCollisionCheck(t *testing.T) {
	_, root := initLocalRoot(t)
	if _, err := CreatePod(root, "demo"); err != nil {
		t.Fatal(err)
	}
	// CoS disabled — "sam" should be accepted as a normal member name.
	err := AddMember(root, "demo", config.Member{
		Name: "sam", Title: "T", Adapter: config.AdapterMock, Model: "m", Effort: config.EffortHigh,
	})
	if err != nil {
		t.Errorf("want accepted, got %v", err)
	}
}
