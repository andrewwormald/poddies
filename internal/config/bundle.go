package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const currentSchemaVersion = 1

// Bundle is a portable snapshot of a pod: its Pod config, all Member configs,
// and the embedded ChiefOfStaff config (part of Pod). A single TOML file
// encodes everything needed to recreate the pod directory tree on another
// machine.
type Bundle struct {
	SchemaVersion int      `toml:"schema_version"`
	Pod           Pod      `toml:"pod"`
	Members       []Member `toml:"members"`
}

// Validate checks the bundle is self-consistent.
func (b *Bundle) Validate() error {
	if b.SchemaVersion != currentSchemaVersion {
		return fmt.Errorf("unsupported schema_version %d (want %d)", b.SchemaVersion, currentSchemaVersion)
	}
	if err := b.Pod.Validate(); err != nil {
		return fmt.Errorf("pod: %w", err)
	}
	seen := make(map[string]struct{}, len(b.Members))
	cosName := ""
	if b.Pod.ChiefOfStaff.Enabled {
		cosName = b.Pod.ChiefOfStaff.ResolvedName()
	}
	for i, m := range b.Members {
		if err := m.Validate(); err != nil {
			return fmt.Errorf("members[%d]: %w", i, err)
		}
		if _, dup := seen[m.Name]; dup {
			return fmt.Errorf("duplicate member name %q in bundle", m.Name)
		}
		if cosName != "" && m.Name == cosName {
			return fmt.Errorf("member %q collides with chief_of_staff name", m.Name)
		}
		seen[m.Name] = struct{}{}
	}
	return nil
}

// LoadBundle decodes a Bundle from r with strict unknown-field rejection.
func LoadBundle(r io.Reader) (*Bundle, error) {
	var b Bundle
	dec := toml.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return nil, fmt.Errorf("decoding bundle: %w", err)
	}
	return &b, nil
}

// SaveBundle encodes b to w as TOML.
func SaveBundle(w io.Writer, b *Bundle) error {
	enc := toml.NewEncoder(w)
	enc.SetIndentTables(true)
	if err := enc.Encode(b); err != nil {
		return fmt.Errorf("encoding bundle: %w", err)
	}
	return nil
}

// NewBundleFromPodDir reads <podDir>/pod.toml and all <podDir>/members/*.toml,
// assembling them into a Bundle with SchemaVersion set.
func NewBundleFromPodDir(podDir string) (*Bundle, error) {
	pod, err := LoadPod(podDir)
	if err != nil {
		return nil, fmt.Errorf("load pod: %w", err)
	}

	membersDir := filepath.Join(podDir, MembersDirName)
	entries, err := os.ReadDir(membersDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read members dir: %w", err)
	}

	var members []Member
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".toml" {
			continue
		}
		name := e.Name()[:len(e.Name())-len(".toml")]
		m, err := LoadMember(podDir, name)
		if err != nil {
			return nil, fmt.Errorf("load member %q: %w", name, err)
		}
		members = append(members, *m)
	}

	return &Bundle{
		SchemaVersion: currentSchemaVersion,
		Pod:           *pod,
		Members:       members,
	}, nil
}

// WriteBundleToPodDir creates the pod directory tree under podDir and writes
// pod.toml and members/*.toml from b. If overwrite is false and pod.toml
// already exists the call fails. podDir is the directory for the pod itself
// (i.e. <root>/pods/<name>), not the root.
//
// On --overwrite, any existing *.toml files in the members directory that
// are not named by the bundle are removed. Without this, an overwrite would
// produce a hybrid pod (bundle members plus stale members from the previous
// pod) — confusing and not what "overwrite" implies.
func WriteBundleToPodDir(podDir string, b *Bundle, overwrite bool) error {
	podFile := filepath.Join(podDir, PodFileName)
	if _, err := os.Stat(podFile); err == nil && !overwrite {
		return fmt.Errorf("pod.toml already exists at %q (use --overwrite to replace)", podFile)
	}

	membersDir := filepath.Join(podDir, MembersDirName)
	if err := os.MkdirAll(membersDir, 0o700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	if overwrite {
		wanted := make(map[string]struct{}, len(b.Members))
		for _, m := range b.Members {
			wanted[m.Name+".toml"] = struct{}{}
		}
		entries, err := os.ReadDir(membersDir)
		if err != nil {
			return fmt.Errorf("read members dir: %w", err)
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".toml" {
				continue
			}
			if _, keep := wanted[e.Name()]; keep {
				continue
			}
			if err := os.Remove(filepath.Join(membersDir, e.Name())); err != nil {
				return fmt.Errorf("remove stale member %q: %w", e.Name(), err)
			}
		}
	}

	if err := SavePod(podDir, &b.Pod); err != nil {
		return fmt.Errorf("save pod: %w", err)
	}

	for i := range b.Members {
		if err := SaveMember(podDir, &b.Members[i]); err != nil {
			return fmt.Errorf("save member %q: %w", b.Members[i].Name, err)
		}
	}

	return nil
}
