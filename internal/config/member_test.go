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

func fullMember() *Member {
	return &Member{
		Name:              "alice",
		Title:             "Staff Engineer",
		Adapter:           AdapterClaude,
		Model:             "claude-opus-4-7",
		Effort:            EffortHigh,
		Persona:           "Pragmatic, terse. Pushes back on over-engineering.",
		Skills:            []string{"go", "cli-design", "distributed-systems"},
		SystemPromptExtra: "",
	}
}

func TestSaveMember_ProducesGolden(t *testing.T) {
	m := fullMember()
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.SetIndentTables(true)
	if err := enc.Encode(m); err != nil {
		t.Fatalf("encode: %v", err)
	}
	goldenCompare(t, "testdata/golden/member_full.toml", buf.Bytes())
}

func writeMember(t *testing.T, podDir, name string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(podDir, MembersDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(MemberPath(podDir, name), body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMember_Golden_RoundTrips(t *testing.T) {
	src, err := os.ReadFile("testdata/golden/member_full.toml")
	if err != nil {
		t.Skipf("golden missing (run -update first): %v", err)
	}
	pod := t.TempDir()
	writeMember(t, pod, "alice", src)

	got, err := LoadMember(pod, "alice")
	if err != nil {
		t.Fatalf("LoadMember: %v", err)
	}
	if !reflect.DeepEqual(got, fullMember()) {
		t.Errorf("round-trip mismatch\nwant %#v\ngot  %#v", fullMember(), got)
	}
}

func TestSaveMember_ThenLoadMember_Equivalent(t *testing.T) {
	pod := t.TempDir()
	if err := os.MkdirAll(filepath.Join(pod, MembersDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	want := fullMember()
	if err := SaveMember(pod, want); err != nil {
		t.Fatalf("SaveMember: %v", err)
	}
	got, err := LoadMember(pod, "alice")
	if err != nil {
		t.Fatalf("LoadMember: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mismatch\nwant %#v\ngot  %#v", want, got)
	}
}

func TestSaveMember_NilErrors(t *testing.T) {
	if err := SaveMember(t.TempDir(), nil); err == nil {
		t.Error("want error for nil member")
	}
}

func TestSaveMember_FilePermissions(t *testing.T) {
	pod := t.TempDir()
	if err := os.MkdirAll(filepath.Join(pod, MembersDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := SaveMember(pod, fullMember()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(MemberPath(pod, "alice"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("want 0600, got %o", got)
	}
}

func TestLoadMember_MissingFile_Errors(t *testing.T) {
	if _, err := LoadMember(t.TempDir(), "alice"); err == nil {
		t.Error("want error for missing member file")
	}
}

func TestLoadMember_UnknownField_Errors(t *testing.T) {
	pod := t.TempDir()
	body := `name = "alice"
title = "x"
adapter = "claude"
model = "m"
effort = "low"
bogus_field = "oops"
`
	writeMember(t, pod, "alice", []byte(body))
	_, err := LoadMember(pod, "alice")
	if err == nil {
		t.Fatal("want error for unknown field")
	}
	if !strings.Contains(err.Error(), "strict") {
		t.Errorf("want error to indicate strict-mode failure, got %v", err)
	}
}

func TestMember_Validate_OK(t *testing.T) {
	if err := fullMember().Validate(); err != nil {
		t.Errorf("want ok, got %v", err)
	}
}

func TestMember_Validate_EmptyName(t *testing.T) {
	m := fullMember()
	m.Name = ""
	if err := m.Validate(); err == nil {
		t.Error("want error for empty name")
	}
}

func TestMember_Validate_ReservedName_Human(t *testing.T) {
	m := fullMember()
	m.Name = "human"
	if err := m.Validate(); err == nil {
		t.Error("want error for reserved name human")
	}
}

func TestMember_Validate_BadSlugName(t *testing.T) {
	m := fullMember()
	m.Name = "Alice"
	if err := m.Validate(); err == nil {
		t.Error("want error for uppercase name")
	}
}

func TestMember_Validate_EmptyTitle(t *testing.T) {
	m := fullMember()
	m.Title = ""
	if err := m.Validate(); err == nil {
		t.Error("want error for empty title")
	}
}

func TestMember_Validate_BadAdapter(t *testing.T) {
	m := fullMember()
	m.Adapter = Adapter("nonsense")
	if err := m.Validate(); err == nil {
		t.Error("want error for bad adapter")
	}
}

func TestMember_Validate_EmptyModel(t *testing.T) {
	m := fullMember()
	m.Model = ""
	if err := m.Validate(); err == nil {
		t.Error("want error for empty model")
	}
}

func TestMember_Validate_BadEffort(t *testing.T) {
	m := fullMember()
	m.Effort = Effort("extreme")
	if err := m.Validate(); err == nil {
		t.Error("want error for bad effort")
	}
}

func TestMember_Validate_PathTraversalName_Errors(t *testing.T) {
	m := fullMember()
	m.Name = "../../evil"
	if err := m.Validate(); err == nil {
		t.Error("want error for path-traversal in name")
	}
}

func TestMemberPath_Shape(t *testing.T) {
	got := MemberPath("/pods/x", "alice")
	want := "/pods/x/members/alice.toml"
	if got != want {
		t.Errorf("want %s, got %s", want, got)
	}
}
