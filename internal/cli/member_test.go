package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/config"
)

// setupPod creates a local root with one pod named "demo" and returns cwd/root.
func setupPod(t *testing.T, podName string) (cwd, root string) {
	t.Helper()
	cwd, root = initLocalRoot(t)
	if _, err := CreatePod(root, podName); err != nil {
		t.Fatal(err)
	}
	return cwd, root
}

func aliceMember() config.Member {
	return config.Member{
		Name:    "alice",
		Title:   "Staff Engineer",
		Adapter: config.AdapterClaude,
		Model:   "claude-opus-4-7",
		Effort:  config.EffortHigh,
	}
}

func TestAddMember_Succeeds(t *testing.T) {
	_, root := setupPod(t, "demo")
	if err := AddMember(root, "demo", aliceMember()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(config.MemberPath(PodDir(root, "demo"), "alice")); err != nil {
		t.Errorf("member file missing: %v", err)
	}
}

func TestAddMember_NoPod_Errors(t *testing.T) {
	_, root := initLocalRoot(t)
	err := AddMember(root, "ghost", aliceMember())
	if !errors.Is(err, ErrPodNotFound) {
		t.Errorf("want ErrPodNotFound, got %v", err)
	}
}

func TestAddMember_Duplicate_Errors(t *testing.T) {
	_, root := setupPod(t, "demo")
	if err := AddMember(root, "demo", aliceMember()); err != nil {
		t.Fatal(err)
	}
	err := AddMember(root, "demo", aliceMember())
	if err == nil {
		t.Error("want error on duplicate")
	}
}

func TestAddMember_InvalidMember_Errors(t *testing.T) {
	_, root := setupPod(t, "demo")
	m := aliceMember()
	m.Name = "" // invalid
	err := AddMember(root, "demo", m)
	if err == nil {
		t.Error("want error for invalid member")
	}
}

func TestAddMember_ReservedName_Errors(t *testing.T) {
	_, root := setupPod(t, "demo")
	m := aliceMember()
	m.Name = "human"
	err := AddMember(root, "demo", m)
	if err == nil {
		t.Error("want error for reserved name")
	}
}

func TestRemoveMember_Succeeds(t *testing.T) {
	_, root := setupPod(t, "demo")
	if err := AddMember(root, "demo", aliceMember()); err != nil {
		t.Fatal(err)
	}
	if err := RemoveMember(root, "demo", "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(config.MemberPath(PodDir(root, "demo"), "alice")); !os.IsNotExist(err) {
		t.Errorf("want not-exist, got %v", err)
	}
}

func TestRemoveMember_NotFound_Errors(t *testing.T) {
	_, root := setupPod(t, "demo")
	err := RemoveMember(root, "demo", "ghost")
	if !errors.Is(err, ErrMemberNotFound) {
		t.Errorf("want ErrMemberNotFound, got %v", err)
	}
}

func TestRemoveMember_NoPod_Errors(t *testing.T) {
	_, root := initLocalRoot(t)
	err := RemoveMember(root, "ghost", "alice")
	if !errors.Is(err, ErrPodNotFound) {
		t.Errorf("want ErrPodNotFound, got %v", err)
	}
}

func TestEditMember_PartialUpdate_Title(t *testing.T) {
	_, root := setupPod(t, "demo")
	if err := AddMember(root, "demo", aliceMember()); err != nil {
		t.Fatal(err)
	}
	newTitle := "Principal Engineer"
	m, err := EditMember(root, "demo", "alice", MemberPatch{Title: &newTitle})
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != newTitle {
		t.Errorf("want %s, got %s", newTitle, m.Title)
	}
	// untouched field
	if m.Model != "claude-opus-4-7" {
		t.Errorf("model unexpectedly changed to %s", m.Model)
	}
}

func TestEditMember_UpdateSkills_Overwrites(t *testing.T) {
	_, root := setupPod(t, "demo")
	m := aliceMember()
	m.Skills = []string{"go", "cli"}
	if err := AddMember(root, "demo", m); err != nil {
		t.Fatal(err)
	}
	updated, err := EditMember(root, "demo", "alice", MemberPatch{
		SkillsExplicit: true,
		Skills:         []string{"rust"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updated.Skills, []string{"rust"}) {
		t.Errorf("want [rust], got %v", updated.Skills)
	}
}

func TestEditMember_ClearSkills(t *testing.T) {
	_, root := setupPod(t, "demo")
	m := aliceMember()
	m.Skills = []string{"go"}
	if err := AddMember(root, "demo", m); err != nil {
		t.Fatal(err)
	}
	updated, err := EditMember(root, "demo", "alice", MemberPatch{
		SkillsExplicit: true,
		Skills:         []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Skills) != 0 {
		t.Errorf("want empty, got %v", updated.Skills)
	}
}

func TestEditMember_NotFound_Errors(t *testing.T) {
	_, root := setupPod(t, "demo")
	_, err := EditMember(root, "demo", "ghost", MemberPatch{})
	if !errors.Is(err, ErrMemberNotFound) {
		t.Errorf("want ErrMemberNotFound, got %v", err)
	}
}

func TestEditMember_NoPod_Errors(t *testing.T) {
	_, root := initLocalRoot(t)
	_, err := EditMember(root, "ghost", "alice", MemberPatch{})
	if !errors.Is(err, ErrPodNotFound) {
		t.Errorf("want ErrPodNotFound, got %v", err)
	}
}

func TestEditMember_ValidationOnSave(t *testing.T) {
	_, root := setupPod(t, "demo")
	if err := AddMember(root, "demo", aliceMember()); err != nil {
		t.Fatal(err)
	}
	bad := config.Effort("extreme")
	_, err := EditMember(root, "demo", "alice", MemberPatch{Effort: &bad})
	if err == nil {
		t.Error("want validation error on bad effort")
	}
}

func TestSplitCSV(t *testing.T) {
	cases := map[string][]string{
		"":             nil,
		"a":            {"a"},
		"a,b,c":        {"a", "b", "c"},
		" a , b ,, c ": {"a", "b", "c"},
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			got := splitCSV(in)
			if len(got) == 0 && len(want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("want %v, got %v", want, got)
			}
		})
	}
}

// --- cobra ---

func TestMemberAddCmd_Succeeds(t *testing.T) {
	cwd, _ := setupPod(t, "demo")
	a, out, _ := newTestApp(cwd, t.TempDir())
	err := runCmd(t, a, "member", "add",
		"--pod", "demo",
		"--name", "alice",
		"--title", "Staff",
		"--adapter", "claude",
		"--model", "claude-opus-4-7",
		"--effort", "high",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "added member") {
		t.Errorf("want added-member output, got %q", out.String())
	}
}

func TestMemberAddCmd_MissingFlags_Errors(t *testing.T) {
	cwd, _ := setupPod(t, "demo")
	a, _, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "member", "add", "--pod", "demo", "--name", "alice"); err == nil {
		t.Error("want error for missing required flags")
	}
}

func TestMemberAddCmd_SkillsCSV(t *testing.T) {
	cwd, root := setupPod(t, "demo")
	a, _, _ := newTestApp(cwd, t.TempDir())
	err := runCmd(t, a, "member", "add",
		"--pod", "demo",
		"--name", "alice",
		"--title", "x",
		"--adapter", "claude",
		"--model", "m",
		"--effort", "low",
		"--skills", "go, cli, testing",
	)
	if err != nil {
		t.Fatal(err)
	}
	m, err := config.LoadMember(PodDir(root, "demo"), "alice")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"go", "cli", "testing"}
	if !reflect.DeepEqual(m.Skills, want) {
		t.Errorf("want %v, got %v", want, m.Skills)
	}
}

func TestMemberRemoveCmd(t *testing.T) {
	cwd, root := setupPod(t, "demo")
	if err := AddMember(root, "demo", aliceMember()); err != nil {
		t.Fatal(err)
	}
	a, out, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "member", "remove", "--pod", "demo", "--name", "alice"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "removed member") {
		t.Errorf("got %q", out.String())
	}
	if _, err := os.Stat(config.MemberPath(PodDir(root, "demo"), "alice")); !os.IsNotExist(err) {
		t.Errorf("want file removed, got %v", err)
	}
}

func TestMemberEditCmd_UpdatesOnlyFlaggedFields(t *testing.T) {
	cwd, root := setupPod(t, "demo")
	orig := aliceMember()
	orig.Persona = "original persona"
	if err := AddMember(root, "demo", orig); err != nil {
		t.Fatal(err)
	}
	a, _, _ := newTestApp(cwd, t.TempDir())
	if err := runCmd(t, a, "member", "edit",
		"--pod", "demo",
		"--name", "alice",
		"--title", "Principal",
	); err != nil {
		t.Fatal(err)
	}
	m, err := config.LoadMember(PodDir(root, "demo"), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "Principal" {
		t.Errorf("title: want Principal, got %s", m.Title)
	}
	if m.Persona != "original persona" {
		t.Errorf("persona should be unchanged, got %s", m.Persona)
	}
}

// Avoid unused import if strings not otherwise referenced.
var _ = filepath.Join
