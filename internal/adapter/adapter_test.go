package adapter

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/andrewwormald/poddies/internal/config"
)

// fakeAdapter is a minimal Adapter used only by registry tests.
type fakeAdapter struct{ name string }

func (f *fakeAdapter) Name() string { return f.name }
func (f *fakeAdapter) Invoke(_ context.Context, _ InvokeRequest) (InvokeResponse, error) {
	return InvokeResponse{}, nil
}

func TestRegister_AddsToRegistry(t *testing.T) {
	t.Cleanup(reset)
	reset()
	Register(&fakeAdapter{name: "x"})
	got, err := Get("x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name() != "x" {
		t.Errorf("want name x, got %s", got.Name())
	}
}

func TestRegister_NilPanics(t *testing.T) {
	t.Cleanup(reset)
	reset()
	defer func() {
		if recover() == nil {
			t.Error("want panic for nil adapter")
		}
	}()
	Register(nil)
}

func TestRegister_EmptyNamePanics(t *testing.T) {
	t.Cleanup(reset)
	reset()
	defer func() {
		if recover() == nil {
			t.Error("want panic for empty name")
		}
	}()
	Register(&fakeAdapter{name: ""})
}

func TestRegister_DuplicatePanics(t *testing.T) {
	t.Cleanup(reset)
	reset()
	Register(&fakeAdapter{name: "x"})
	defer func() {
		if recover() == nil {
			t.Error("want panic for duplicate name")
		}
	}()
	Register(&fakeAdapter{name: "x"})
}

func TestGet_UnknownErrors(t *testing.T) {
	t.Cleanup(reset)
	reset()
	_, err := Get("nope")
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Errorf("want ErrUnknownAdapter, got %v", err)
	}
}

func TestRegistered_ReturnsAllNames(t *testing.T) {
	t.Cleanup(reset)
	reset()
	Register(&fakeAdapter{name: "a"})
	Register(&fakeAdapter{name: "b"})
	Register(&fakeAdapter{name: "c"})
	got := Registered()
	sort.Strings(got)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] want %s, got %s", i, want[i], got[i])
		}
	}
}

func TestRegistered_EmptyRegistry(t *testing.T) {
	t.Cleanup(reset)
	reset()
	if got := Registered(); len(got) != 0 {
		t.Errorf("want empty, got %v", got)
	}
}

func TestValidateRequest_Member_OK(t *testing.T) {
	req := InvokeRequest{
		Role:   RoleMember,
		Member: config.Member{Name: "alice"},
		Pod:    config.Pod{Name: "p"},
	}
	if err := ValidateRequest(req); err != nil {
		t.Errorf("want ok, got %v", err)
	}
}

func TestValidateRequest_DefaultRoleTreatedAsMember(t *testing.T) {
	req := InvokeRequest{
		Member: config.Member{Name: "alice"},
		Pod:    config.Pod{Name: "p"},
	}
	if err := ValidateRequest(req); err != nil {
		t.Errorf("want ok for default role, got %v", err)
	}
}

func TestValidateRequest_Member_MissingName(t *testing.T) {
	req := InvokeRequest{Role: RoleMember, Pod: config.Pod{Name: "p"}}
	if err := ValidateRequest(req); err == nil {
		t.Error("want error for missing member name")
	}
}

func TestValidateRequest_MissingPodName(t *testing.T) {
	req := InvokeRequest{Role: RoleMember, Member: config.Member{Name: "alice"}}
	if err := ValidateRequest(req); err == nil {
		t.Error("want error for missing pod name")
	}
}

func TestValidateRequest_ChiefOfStaff_Disabled(t *testing.T) {
	req := InvokeRequest{
		Role: RoleChiefOfStaff,
		Pod:  config.Pod{Name: "p"},
	}
	if err := ValidateRequest(req); err == nil {
		t.Error("want error when chief-of-staff disabled")
	}
}

func TestValidateRequest_ChiefOfStaff_Enabled_OK(t *testing.T) {
	req := InvokeRequest{
		Role:         RoleChiefOfStaff,
		ChiefOfStaff: config.ChiefOfStaff{Enabled: true},
		Pod:          config.Pod{Name: "p"},
	}
	if err := ValidateRequest(req); err != nil {
		t.Errorf("want ok, got %v", err)
	}
}

func TestValidateRequest_UnknownRole(t *testing.T) {
	req := InvokeRequest{Role: "weird", Pod: config.Pod{Name: "p"}}
	if err := ValidateRequest(req); err == nil {
		t.Error("want error for unknown role")
	}
}
