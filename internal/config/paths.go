// Package config handles loading, saving, and resolving poddies
// configuration (pod.toml, member toml files, and the chief-of-staff
// section) along with the root-directory precedence rules.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Mode controls how ResolveRoot picks between local and global roots.
type Mode int

const (
	// ModeAuto picks local if ./poddies exists, else global.
	ModeAuto Mode = iota
	// ModeGlobal forces the global (~/.poddies) root.
	ModeGlobal
	// ModeLocal forces the local (./poddies) root.
	ModeLocal
)

// Source describes which root was picked.
type Source string

const (
	SourceLocal  Source = "local"
	SourceGlobal Source = "global"
	SourceEnv    Source = "env"
)

// Resolved is the result of ResolveRoot.
type Resolved struct {
	Dir    string
	Source Source
}

// ErrNoRoot is returned when no poddies root directory can be found.
var ErrNoRoot = errors.New("no poddies root found")

// LocalDir returns the local (project-scoped) poddies directory for cwd.
func LocalDir(cwd string) string {
	return filepath.Join(cwd, "poddies")
}

// GlobalDir returns the global (user-scoped) poddies directory for home.
// Returns "" when home is empty, letting callers detect and error out.
func GlobalDir(home string) string {
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".poddies")
}

// ResolveRoot picks the poddies root directory based on the mode and the
// existence of local / global dirs. Precedence:
//
//	envRoot (non-empty) > mode override (Local/Global) > local > global
//
// When ModeAuto is used and neither local nor global exist, ErrNoRoot is
// returned so callers can prompt the user to run `poddies init`.
func ResolveRoot(mode Mode, cwd, home, envRoot string) (Resolved, error) {
	if envRoot != "" {
		info, err := os.Stat(envRoot)
		if err != nil {
			return Resolved{}, fmt.Errorf("POD_ROOT %q: %w", envRoot, err)
		}
		if !info.IsDir() {
			return Resolved{}, fmt.Errorf("POD_ROOT %q is not a directory", envRoot)
		}
		return Resolved{Dir: envRoot, Source: SourceEnv}, nil
	}

	local := LocalDir(cwd)
	global := GlobalDir(home)

	localOK, localErr := isDir(local)
	if localErr != nil {
		return Resolved{}, fmt.Errorf("checking local root %q: %w", local, localErr)
	}

	globalOK := false
	if global != "" {
		var err error
		globalOK, err = isDir(global)
		if err != nil {
			return Resolved{}, fmt.Errorf("checking global root %q: %w", global, err)
		}
	}

	switch mode {
	case ModeLocal:
		if !localOK {
			return Resolved{}, fmt.Errorf("%w: local root %q does not exist", ErrNoRoot, local)
		}
		return Resolved{Dir: local, Source: SourceLocal}, nil
	case ModeGlobal:
		if global == "" {
			return Resolved{}, fmt.Errorf("home directory is unset; cannot resolve global root")
		}
		if !globalOK {
			return Resolved{}, fmt.Errorf("%w: global root %q does not exist", ErrNoRoot, global)
		}
		return Resolved{Dir: global, Source: SourceGlobal}, nil
	case ModeAuto:
		if localOK {
			return Resolved{Dir: local, Source: SourceLocal}, nil
		}
		if globalOK {
			return Resolved{Dir: global, Source: SourceGlobal}, nil
		}
		return Resolved{}, ErrNoRoot
	default:
		return Resolved{}, fmt.Errorf("unknown mode %v", mode)
	}
}

// isDir returns (exists, err). Non-existence is (false, nil);
// an existing non-directory returns an error.
func isDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("path %q exists but is not a directory", path)
	}
	return true, nil
}
