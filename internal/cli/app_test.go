package cli

import (
	"bytes"
	"strings"
	"testing"
)

// newTestApp returns an App wired to in-memory buffers and empty cwd/home.
// Tests that need real dirs pass their own tmp paths.
func newTestApp(cwd, home string) (*App, *bytes.Buffer, *bytes.Buffer) {
	var out, errBuf bytes.Buffer
	a := &App{
		Out:  &out,
		Err:  &errBuf,
		In:   strings.NewReader(""),
		Cwd:  cwd,
		Home: home,
	}
	return a, &out, &errBuf
}

func runCmd(t *testing.T, a *App, args ...string) error {
	t.Helper()
	cmd := a.NewRootCmd()
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestNewRootCmd_HasInitSubcommand(t *testing.T) {
	a, _, _ := newTestApp("", "")
	root := a.NewRootCmd()
	found := false
	for _, c := range root.Commands() {
		if c.Name() == "init" {
			found = true
			break
		}
	}
	if !found {
		t.Error("init subcommand missing")
	}
}

func TestNewRootCmd_VersionFlag(t *testing.T) {
	a, out, _ := newTestApp("", "")
	if err := runCmd(t, a, "--version"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), Version) {
		t.Errorf("want version in output, got %q", out.String())
	}
}

func TestNewRootCmd_RoutesStdoutToAppOut(t *testing.T) {
	a, out, _ := newTestApp("", "")
	if err := runCmd(t, a, "--help"); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		t.Error("expected --help output in a.Out")
	}
}
