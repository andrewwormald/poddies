package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func fullPod() *Pod {
	return &Pod{
		Name: "platform-pod",
		Lead: "human",
		Cwd:  ".",
		Hierarchy: [][]string{
			{"human"},
			{"alice"},
			{"bob", "carol"},
		},
		ChiefOfStaff: ChiefOfStaff{
			Enabled:  true,
			Name:     "sam",
			Adapter:  AdapterClaude,
			Model:    "claude-haiku-4-5",
			Triggers: []Trigger{TriggerUnresolvedRouting, TriggerMilestone, TriggerGrayArea},
		},
	}
}

func TestSavePod_ProducesGolden(t *testing.T) {
	p := fullPod()
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.SetIndentTables(true)
	if err := enc.Encode(p); err != nil {
		t.Fatalf("encode: %v", err)
	}
	goldenCompare(t, "testdata/golden/pod_full.toml", buf.Bytes())
}

func TestLoadPod_Golden_RoundTrips(t *testing.T) {
	tmp := t.TempDir()
	src, err := os.ReadFile("testdata/golden/pod_full.toml")
	if err != nil {
		t.Skipf("golden missing (run -update first): %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, PodFileName), src, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPod(tmp)
	if err != nil {
		t.Fatalf("LoadPod: %v", err)
	}
	if !reflect.DeepEqual(got, fullPod()) {
		t.Errorf("round-trip mismatch\nwant %#v\ngot  %#v", fullPod(), got)
	}
}

func TestSavePod_ThenLoadPod_Equivalent(t *testing.T) {
	tmp := t.TempDir()
	want := fullPod()
	if err := SavePod(tmp, want); err != nil {
		t.Fatalf("SavePod: %v", err)
	}
	got, err := LoadPod(tmp)
	if err != nil {
		t.Fatalf("LoadPod: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mismatch\nwant %#v\ngot  %#v", want, got)
	}
}

func TestSavePod_NilErrors(t *testing.T) {
	if err := SavePod(t.TempDir(), nil); err == nil {
		t.Error("want error for nil pod")
	}
}

func TestSavePod_FilePermissions(t *testing.T) {
	tmp := t.TempDir()
	if err := SavePod(tmp, fullPod()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(tmp, PodFileName))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("want 0600, got %o", got)
	}
}

func TestLoadPod_MissingFile_Errors(t *testing.T) {
	if _, err := LoadPod(t.TempDir()); err == nil {
		t.Error("want error for missing pod.toml")
	}
}

func TestLoadPod_MalformedTOML_Errors(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, PodFileName), []byte("name = \nbroken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPod(tmp); err == nil {
		t.Error("want error for malformed TOML")
	}
}

func TestLoadPod_UnknownField_Errors(t *testing.T) {
	tmp := t.TempDir()
	content := `name = "p"
lead = "human"
bogus_field = "oops"
`
	if err := os.WriteFile(filepath.Join(tmp, PodFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadPod(tmp)
	if err == nil {
		t.Fatal("want error for unknown field")
	}
	if !strings.Contains(err.Error(), "strict") {
		t.Errorf("error should indicate strict-mode failure, got %v", err)
	}
}

func TestPod_Validate_OK(t *testing.T) {
	if err := fullPod().Validate(); err != nil {
		t.Errorf("expected valid, got %v", err)
	}
}

func TestPod_Validate_EmptyName(t *testing.T) {
	p := fullPod()
	p.Name = ""
	if err := p.Validate(); err == nil {
		t.Error("want error for empty name")
	}
}

func TestPod_Validate_BadSlugName(t *testing.T) {
	p := fullPod()
	p.Name = "Bad Name"
	if err := p.Validate(); err == nil {
		t.Error("want error for invalid slug")
	}
}

func TestPod_Validate_EmptyLead(t *testing.T) {
	p := fullPod()
	p.Lead = ""
	if err := p.Validate(); err == nil {
		t.Error("want error for empty lead")
	}
}

func TestPod_Validate_HumanLead_OK(t *testing.T) {
	p := fullPod()
	p.Lead = "human"
	if err := p.Validate(); err != nil {
		t.Errorf("human lead should be valid, got %v", err)
	}
}

func TestPod_Validate_NonHumanLead_SlugValidated(t *testing.T) {
	p := fullPod()
	p.Lead = "NotASlug"
	if err := p.Validate(); err == nil {
		t.Error("want error for non-slug lead")
	}
}

func TestPod_Validate_HierarchyInvalidSlug(t *testing.T) {
	p := fullPod()
	p.Hierarchy = [][]string{{"Bad Slug"}}
	if err := p.Validate(); err == nil {
		t.Error("want error for invalid hierarchy slug")
	}
}

func TestPod_Validate_HierarchyHumanAllowed(t *testing.T) {
	p := fullPod()
	p.Hierarchy = [][]string{{"human"}, {"alice"}}
	if err := p.Validate(); err != nil {
		t.Errorf("human in hierarchy should be valid, got %v", err)
	}
}

func TestChiefOfStaff_Validate_Disabled_PartialOK(t *testing.T) {
	c := ChiefOfStaff{Enabled: false}
	if err := c.Validate(); err != nil {
		t.Errorf("disabled empty should be valid, got %v", err)
	}
}

func TestChiefOfStaff_Validate_Disabled_BadAdapter_Errors(t *testing.T) {
	c := ChiefOfStaff{Enabled: false, Adapter: Adapter("bogus")}
	if err := c.Validate(); err == nil {
		t.Error("want error for typo in adapter even when disabled")
	}
}

func TestChiefOfStaff_Validate_Disabled_BadTrigger_Errors(t *testing.T) {
	c := ChiefOfStaff{Enabled: false, Triggers: []Trigger{"bogus"}}
	if err := c.Validate(); err == nil {
		t.Error("want error for typo in trigger even when disabled")
	}
}

func TestChiefOfStaff_Validate_Enabled_MissingAdapter(t *testing.T) {
	c := ChiefOfStaff{Enabled: true, Model: "m", Triggers: []Trigger{TriggerMilestone}}
	if err := c.Validate(); err == nil {
		t.Error("want error for missing adapter")
	}
}

func TestChiefOfStaff_Validate_Enabled_MissingModel(t *testing.T) {
	c := ChiefOfStaff{Enabled: true, Adapter: AdapterClaude, Triggers: []Trigger{TriggerMilestone}}
	if err := c.Validate(); err == nil {
		t.Error("want error for missing model")
	}
}

func TestChiefOfStaff_Validate_Enabled_NoTriggers(t *testing.T) {
	c := ChiefOfStaff{Enabled: true, Adapter: AdapterClaude, Model: "m"}
	if err := c.Validate(); err == nil {
		t.Error("want error for empty triggers")
	}
}

func TestChiefOfStaff_Validate_Enabled_BadTrigger(t *testing.T) {
	c := ChiefOfStaff{Enabled: true, Adapter: AdapterClaude, Model: "m", Triggers: []Trigger{"bogus"}}
	if err := c.Validate(); err == nil {
		t.Error("want error for bad trigger")
	}
}

func TestChiefOfStaff_Validate_Enabled_BadName(t *testing.T) {
	c := ChiefOfStaff{
		Enabled:  true,
		Adapter:  AdapterClaude,
		Model:    "m",
		Triggers: []Trigger{TriggerMilestone},
		Name:     "Bad Name",
	}
	if err := c.Validate(); err == nil {
		t.Error("want error for bad slug name")
	}
}

func TestChiefOfStaff_Validate_Enabled_OK(t *testing.T) {
	c := ChiefOfStaff{
		Enabled:  true,
		Adapter:  AdapterClaude,
		Model:    "claude-haiku-4-5",
		Triggers: []Trigger{TriggerMilestone},
	}
	if err := c.Validate(); err != nil {
		t.Errorf("want ok, got %v", err)
	}
}

func TestChiefOfStaff_ResolvedName_Default(t *testing.T) {
	c := ChiefOfStaff{}
	if got := c.ResolvedName(); got != DefaultChiefOfStaffName {
		t.Errorf("want %q, got %q", DefaultChiefOfStaffName, got)
	}
}

func TestChiefOfStaff_ResolvedName_Custom(t *testing.T) {
	c := ChiefOfStaff{Name: "sam"}
	if got := c.ResolvedName(); got != "sam" {
		t.Errorf("want sam, got %q", got)
	}
}
