package config

import (
	"fmt"
	"regexp"
)

// slugRe matches DNS-friendly identifiers: lowercase alphanumerics and
// '-', with no leading/trailing dash. Unicode is intentionally excluded
// because these identifiers become filenames (members/<name>.toml).
var slugRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

// MaxSlugLen caps identifier length (pod names, member names).
const MaxSlugLen = 64

// ReservedMemberNames are identifiers a user cannot assign to a member.
// "human" always refers to the pod lead. The chief-of-staff's configured
// name is reserved dynamically at bundle-load time (see ValidateBundle).
var ReservedMemberNames = map[string]struct{}{
	"human": {},
}

// ValidateSlug checks that s is a valid slug for a pod or member name.
// Returns a descriptive error on failure so it can be surfaced to the user.
func ValidateSlug(s string) error {
	if s == "" {
		return fmt.Errorf("name must not be empty")
	}
	if len(s) > MaxSlugLen {
		return fmt.Errorf("name %q is too long (max %d)", s, MaxSlugLen)
	}
	if !slugRe.MatchString(s) {
		return fmt.Errorf("name %q is not a valid slug (use lowercase letters, digits, and '-'; no leading/trailing '-')", s)
	}
	return nil
}

// IsReservedMemberName reports whether name is reserved (e.g., "human").
func IsReservedMemberName(name string) bool {
	_, ok := ReservedMemberNames[name]
	return ok
}
