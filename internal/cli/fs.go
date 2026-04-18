package cli

import "os"

// osReadDir is a tiny indirection so tests can swap in a fake filesystem
// if we ever need to. Not exported; kept here so fs-related helpers
// stay out of command files.
func osReadDir(dir string) ([]os.DirEntry, error) {
	return os.ReadDir(dir)
}
