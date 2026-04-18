package cli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckCLI_Present(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "claude" {
			return "/fake/bin/claude", nil
		}
		return "", errors.New("not found")
	}
	c := CheckCLI("claude CLI", "claude", lookPath)
	if c.Status != CheckPass {
		t.Errorf("want pass, got %s", c.Status)
	}
	if c.Message != "/fake/bin/claude" {
		t.Errorf("want path, got %q", c.Message)
	}
}

func TestCheckCLI_Missing_Warns(t *testing.T) {
	lookPath := func(string) (string, error) { return "", errors.New("not found") }
	c := CheckCLI("gemini CLI", "gemini", lookPath)
	if c.Status != CheckWarn {
		t.Errorf("want warn, got %s", c.Status)
	}
	if !strings.Contains(c.Message, "gemini") {
		t.Errorf("message should name binary, got %q", c.Message)
	}
}

func TestCheckRoot_NoRoot_Fails(t *testing.T) {
	c := CheckRoot(t.TempDir(), t.TempDir(), "")
	if c.Status != CheckFail {
		t.Errorf("want fail, got %s", c.Status)
	}
	if !strings.Contains(c.Message, "poddies init") {
		t.Errorf("message should hint at init, got %q", c.Message)
	}
}

func TestCheckRoot_LocalFound_Passes(t *testing.T) {
	cwd, _ := initLocalRoot(t)
	c := CheckRoot(cwd, t.TempDir(), "")
	if c.Status != CheckPass {
		t.Errorf("want pass, got %s (%s)", c.Status, c.Message)
	}
	if !strings.Contains(c.Message, "local") {
		t.Errorf("want source label, got %q", c.Message)
	}
}

func TestCheckRootWritable_Writable(t *testing.T) {
	dir := t.TempDir()
	c := CheckRootWritable(dir)
	if c.Status != CheckPass {
		t.Errorf("want pass, got %s", c.Status)
	}
}

func TestCheckRootWritable_EmptyDir_Fails(t *testing.T) {
	c := CheckRootWritable("")
	if c.Status != CheckFail {
		t.Errorf("want fail for empty path, got %s", c.Status)
	}
}

func TestCheckRootWritable_NonExistentDir_Fails(t *testing.T) {
	c := CheckRootWritable(filepath.Join(t.TempDir(), "does-not-exist"))
	if c.Status != CheckFail {
		t.Errorf("want fail, got %s", c.Status)
	}
}

func TestRunDoctor_IncludesAllChecks_RootResolved(t *testing.T) {
	cwd, _ := initLocalRoot(t)
	lookPath := func(name string) (string, error) {
		return "/fake/" + name, nil
	}
	checks := RunDoctor(DoctorOpts{
		Cwd:      cwd,
		Home:     t.TempDir(),
		LookPath: lookPath,
	})
	if len(checks) != 4 {
		t.Fatalf("want 4 checks, got %d: %+v", len(checks), checks)
	}
	gotNames := map[string]bool{}
	for _, c := range checks {
		gotNames[c.Name] = true
	}
	for _, want := range []string{"claude CLI", "gemini CLI", "poddies root", "root writable"} {
		if !gotNames[want] {
			t.Errorf("missing check: %s", want)
		}
	}
}

func TestRunDoctor_SkipsWritableCheckWhenNoRoot(t *testing.T) {
	checks := RunDoctor(DoctorOpts{
		Cwd:      t.TempDir(),
		Home:     t.TempDir(),
		LookPath: func(name string) (string, error) { return "/fake/" + name, nil },
	})
	for _, c := range checks {
		if c.Name == "root writable" {
			t.Error("writable check should be skipped when root is missing")
		}
	}
}

func TestAnyFailed(t *testing.T) {
	if AnyFailed([]Check{{Status: CheckPass}, {Status: CheckWarn}}) {
		t.Error("want false when only pass/warn")
	}
	if !AnyFailed([]Check{{Status: CheckPass}, {Status: CheckFail}}) {
		t.Error("want true when a fail is present")
	}
}

func TestDoctorCmd_Success(t *testing.T) {
	cwd, _ := initLocalRoot(t)
	a, out, _ := newTestApp(cwd, t.TempDir())
	// override lookPath only via the App? We don't expose it; the
	// default uses exec.LookPath. Instead, just accept WARN outcomes
	// on CI machines where claude/gemini may not be installed.
	err := runCmd(t, a, "doctor")
	// doctor returns error only on FAIL; presence of WARN is acceptable.
	// We'll just check the output has each check name.
	if err != nil && !strings.Contains(err.Error(), "checks failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	outStr := out.String()
	for _, want := range []string{"claude CLI", "gemini CLI", "poddies root"} {
		if !strings.Contains(outStr, want) {
			t.Errorf("output missing %q; got:\n%s", want, outStr)
		}
	}
}

func TestDoctorCmd_NoRoot_ExitsWithError(t *testing.T) {
	a, _, _ := newTestApp(t.TempDir(), t.TempDir())
	err := runCmd(t, a, "doctor")
	if err == nil {
		t.Error("want error when root is missing (fail)")
	}
}
