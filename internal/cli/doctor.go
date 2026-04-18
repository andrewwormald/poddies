package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/andrewwormald/poddies/internal/config"
)

// CheckStatus is the result of a single doctor check.
type CheckStatus string

const (
	// CheckPass means the check succeeded with no issues.
	CheckPass CheckStatus = "pass"
	// CheckWarn means the check found a non-blocking issue (e.g. an
	// optional adapter CLI is missing).
	CheckWarn CheckStatus = "warn"
	// CheckFail means the check found a blocking issue (e.g. the
	// poddies root is not writable).
	CheckFail CheckStatus = "fail"
)

// Check is a single diagnostic result.
type Check struct {
	Name    string
	Status  CheckStatus
	Message string
}

// LookPathFunc abstracts exec.LookPath so tests can inject fakes.
type LookPathFunc func(name string) (string, error)

// CheckCLI reports whether the named binary is on PATH. It is a WARN
// (not FAIL) on absence because adapters are optional — a user may
// only intend to use Gemini, and should still be able to run poddies.
func CheckCLI(displayName, bin string, lookPath LookPathFunc) Check {
	path, err := lookPath(bin)
	if err != nil {
		return Check{
			Name:    displayName,
			Status:  CheckWarn,
			Message: fmt.Sprintf("%q not found on PATH (install it if you plan to use this adapter)", bin),
		}
	}
	return Check{
		Name:    displayName,
		Status:  CheckPass,
		Message: path,
	}
}

// CheckRoot reports on the resolved poddies root.
func CheckRoot(cwd, home, envRoot string) Check {
	res, err := config.ResolveRoot(config.ModeAuto, cwd, home, envRoot)
	if err != nil {
		if errors.Is(err, config.ErrNoRoot) {
			return Check{
				Name:    "poddies root",
				Status:  CheckFail,
				Message: "no root found; run `poddies init` (or `poddies init --global`)",
			}
		}
		return Check{Name: "poddies root", Status: CheckFail, Message: err.Error()}
	}
	return Check{
		Name:    "poddies root",
		Status:  CheckPass,
		Message: fmt.Sprintf("%s (%s)", res.Dir, res.Source),
	}
}

// CheckRootWritable attempts a touch-and-delete in rootDir to confirm
// the process can write there. Returns CheckFail on any issue.
func CheckRootWritable(rootDir string) Check {
	if rootDir == "" {
		return Check{Name: "root writable", Status: CheckFail, Message: "no root directory to check"}
	}
	probe := filepath.Join(rootDir, ".poddies-doctor-probe")
	if err := os.WriteFile(probe, []byte("probe"), 0o600); err != nil {
		return Check{Name: "root writable", Status: CheckFail, Message: err.Error()}
	}
	if err := os.Remove(probe); err != nil {
		return Check{Name: "root writable", Status: CheckFail, Message: err.Error()}
	}
	return Check{Name: "root writable", Status: CheckPass, Message: rootDir}
}

// DoctorOpts bundles the inputs needed to run the aggregate doctor.
type DoctorOpts struct {
	Cwd, Home, EnvRoot string
	LookPath           LookPathFunc
}

// RunDoctor runs the full suite of checks and returns them in order.
// Adapters are probed independently so a missing one doesn't shortcut
// the remaining checks.
func RunDoctor(opts DoctorOpts) []Check {
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	checks := []Check{
		CheckCLI("claude CLI", "claude", lookPath),
		CheckCLI("gemini CLI", "gemini", lookPath),
	}
	rootCheck := CheckRoot(opts.Cwd, opts.Home, opts.EnvRoot)
	checks = append(checks, rootCheck)
	if rootCheck.Status == CheckPass {
		// parse dir out of "dir (source)"
		dir := rootCheck.Message
		if idx := indexByte(dir, ' '); idx > 0 {
			dir = dir[:idx]
		}
		checks = append(checks, CheckRootWritable(dir))
	}
	return checks
}

// indexByte mirrors strings.IndexByte without the strings import.
// Kept local to avoid pulling another import into the doctor file.
func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// AnyFailed reports whether any check ended in CheckFail.
func AnyFailed(checks []Check) bool {
	for _, c := range checks {
		if c.Status == CheckFail {
			return true
		}
	}
	return false
}

// --- cobra ---

func (a *App) newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Preflight check: adapters installed, root configured, etc.",
		RunE: func(cmd *cobra.Command, args []string) error {
			checks := RunDoctor(DoctorOpts{
				Cwd:     a.Cwd,
				Home:    a.Home,
				EnvRoot: a.EnvRoot,
			})
			for _, c := range checks {
				fmt.Fprintf(a.Out, "[%s] %s: %s\n", c.Status, c.Name, c.Message)
			}
			if AnyFailed(checks) {
				return errors.New("doctor: one or more checks failed")
			}
			return nil
		},
	}
}
