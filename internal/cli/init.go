package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/andrewwormald/poddies/internal/config"
)

// ConfigFileName is the root config file written by `poddies init`.
// Its presence is the canonical "this directory is a poddies root" marker.
const ConfigFileName = "config.toml"

// PodsDirName is the subdirectory holding individual pods.
const PodsDirName = "pods"

// defaultConfig is what `init` writes into the new root's config.toml.
// Kept minimal for now; will grow as M1+ milestones add settings.
const defaultConfig = "# poddies root config\n# add global defaults here\n"

// InitResult describes what Init did — useful for tests and for the
// command's human-facing output.
type InitResult struct {
	Dir            string       // absolute path to the poddies root
	Source         config.Source // local or global
	AlreadyExisted bool          // true if root was already initialized (idempotent)
}

// Init creates (or recognizes) a poddies root at either the local
// (./poddies) or global (~/.poddies) location, per mode. The mode
// determines precedence; in ModeAuto the default is local.
//
// Behavior:
//   - if target exists with a config.toml → treat as already initialized
//     (idempotent, no changes made).
//   - if target exists but is non-empty without the marker → error unless
//     force is true.
//   - otherwise → create dir + pods/ subdir + config.toml.
func Init(cwd, home string, mode config.Mode, force bool) (InitResult, error) {
	var dir string
	var src config.Source

	switch mode {
	case config.ModeGlobal:
		if home == "" {
			return InitResult{}, errors.New("home directory is unset; cannot initialize global root")
		}
		dir = config.GlobalDir(home)
		src = config.SourceGlobal
	case config.ModeLocal, config.ModeAuto:
		dir = config.LocalDir(cwd)
		src = config.SourceLocal
	default:
		return InitResult{}, fmt.Errorf("unknown mode %v", mode)
	}

	info, err := os.Stat(dir)
	switch {
	case os.IsNotExist(err):
		// fresh dir — create.
	case err != nil:
		return InitResult{}, fmt.Errorf("stat %q: %w", dir, err)
	case !info.IsDir():
		return InitResult{}, fmt.Errorf("%q exists but is not a directory", dir)
	default:
		// dir exists; check for marker.
		if _, err := os.Stat(filepath.Join(dir, ConfigFileName)); err == nil {
			return InitResult{Dir: dir, Source: src, AlreadyExisted: true}, nil
		}
		if !force {
			empty, err := isEmptyDir(dir)
			if err != nil {
				return InitResult{}, err
			}
			if !empty {
				return InitResult{}, fmt.Errorf("%q exists and is not empty; rerun with --force to initialize anyway", dir)
			}
		}
	}

	if err := os.MkdirAll(filepath.Join(dir, PodsDirName), 0o700); err != nil {
		return InitResult{}, fmt.Errorf("mkdir %q: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return InitResult{}, fmt.Errorf("chmod %q: %w", dir, err)
	}
	cfgPath := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(cfgPath, []byte(defaultConfig), 0o600); err != nil {
		return InitResult{}, fmt.Errorf("write %q: %w", cfgPath, err)
	}
	return InitResult{Dir: dir, Source: src}, nil
}

// isEmptyDir returns true when dir contains no entries (including hidden).
func isEmptyDir(dir string) (bool, error) {
	f, err := os.Open(dir)
	if err != nil {
		return false, err
	}
	defer f.Close()
	names, err := f.Readdirnames(1)
	if err != nil && err.Error() != "EOF" {
		return false, err
	}
	return len(names) == 0, nil
}

// newInitCmd returns the `poddies init` cobra command. Flags:
//
//	--global  force ~/.poddies
//	--local   force ./poddies (default)
//	--force   initialize even if the target dir is non-empty
func (a *App) newInitCmd() *cobra.Command {
	var (
		useGlobal bool
		useLocal  bool
		force     bool
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a poddies root (global ~/.poddies or local ./poddies).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if useGlobal && useLocal {
				return errors.New("--global and --local are mutually exclusive")
			}
			mode := config.ModeLocal
			if useGlobal {
				mode = config.ModeGlobal
			}
			res, err := Init(a.Cwd, a.Home, mode, force)
			if err != nil {
				return err
			}
			if res.AlreadyExisted {
				fmt.Fprintf(a.Out, "already initialized at %s (%s)\n", res.Dir, res.Source)
				return nil
			}
			fmt.Fprintf(a.Out, "initialized poddies root at %s (%s)\n", res.Dir, res.Source)
			return nil
		},
	}
	cmd.Flags().BoolVar(&useGlobal, "global", false, "initialize the global (~/.poddies) root")
	cmd.Flags().BoolVar(&useLocal, "local", false, "initialize the local (./poddies) root (default)")
	cmd.Flags().BoolVar(&force, "force", false, "initialize even if the target dir is non-empty")
	return cmd
}
