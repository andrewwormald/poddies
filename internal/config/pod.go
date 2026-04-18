package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// PodFileName is the on-disk filename for a pod's root config.
const PodFileName = "pod.toml"

// Pod is the top-level configuration for a single pod. One Pod per directory
// under <root>/pods/<name>/.
type Pod struct {
	Name          string         `toml:"name"`
	Lead          string         `toml:"lead"`
	Cwd           string         `toml:"cwd,omitempty"`
	Hierarchy     [][]string     `toml:"hierarchy,omitempty"`
	ChiefOfStaff  ChiefOfStaff   `toml:"chief_of_staff,omitempty"`
}

// ChiefOfStaff configures the built-in facilitator agent. See
// memory/project_chief_of_staff.md for the visibility/trigger design.
type ChiefOfStaff struct {
	Enabled  bool      `toml:"enabled"`
	Name     string    `toml:"name,omitempty"`
	Adapter  Adapter   `toml:"adapter,omitempty"`
	Model    string    `toml:"model,omitempty"`
	Triggers []Trigger `toml:"triggers,omitempty"`
}

// DefaultChiefOfStaffName is used when the user leaves Name empty.
const DefaultChiefOfStaffName = "chief-of-staff"

// LoadPod reads <podDir>/pod.toml with strict (unknown-field) decoding.
func LoadPod(podDir string) (*Pod, error) {
	path := filepath.Join(podDir, PodFileName)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %q: %w", path, err)
	}
	defer f.Close()

	var p Pod
	dec := toml.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("decoding %q: %w", path, err)
	}
	return &p, nil
}

// SavePod writes <podDir>/pod.toml with 0o600 permissions. The parent
// directory must already exist; callers that created the dir are expected
// to have chmod'd it to 0o700.
func SavePod(podDir string, p *Pod) error {
	if p == nil {
		return fmt.Errorf("nil Pod")
	}
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.SetIndentTables(true)
	if err := enc.Encode(p); err != nil {
		return fmt.Errorf("encoding pod: %w", err)
	}
	path := filepath.Join(podDir, PodFileName)
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("writing %q: %w", path, err)
	}
	return nil
}

// Validate runs structural validation on the Pod. Cross-referential checks
// (e.g. hierarchy refers to a missing member) live in ValidateBundle.
func (p *Pod) Validate() error {
	if err := ValidateSlug(p.Name); err != nil {
		return fmt.Errorf("pod name: %w", err)
	}
	if p.Lead == "" {
		return fmt.Errorf("pod lead must not be empty (use \"human\" for human lead)")
	}
	if p.Lead != "human" {
		if err := ValidateSlug(p.Lead); err != nil {
			return fmt.Errorf("pod lead: %w", err)
		}
	}
	// hierarchy entries must be slugs or "human"; dup detection is bundle-level.
	for i, row := range p.Hierarchy {
		for j, n := range row {
			if n == "human" {
				continue
			}
			if err := ValidateSlug(n); err != nil {
				return fmt.Errorf("hierarchy[%d][%d]: %w", i, j, err)
			}
		}
	}
	if err := p.ChiefOfStaff.Validate(); err != nil {
		return fmt.Errorf("chief_of_staff: %w", err)
	}
	return nil
}

// Validate runs structural validation on the ChiefOfStaff config.
// When Enabled=false the other fields are allowed to be partially/fully
// populated (treated as disabled-but-configured); only enabled configs
// must fully validate.
func (c *ChiefOfStaff) Validate() error {
	if !c.Enabled {
		// best-effort: if fields are set, still validate enums so typos
		// are caught before the user enables the feature.
		if c.Adapter != "" {
			if err := c.Adapter.Validate(); err != nil {
				return err
			}
		}
		for i, t := range c.Triggers {
			if err := t.Validate(); err != nil {
				return fmt.Errorf("triggers[%d]: %w", i, err)
			}
		}
		return nil
	}
	if err := c.Adapter.Validate(); err != nil {
		return err
	}
	if c.Model == "" {
		return fmt.Errorf("model must be set when chief_of_staff is enabled")
	}
	if len(c.Triggers) == 0 {
		return fmt.Errorf("at least one trigger must be configured when enabled")
	}
	for i, t := range c.Triggers {
		if err := t.Validate(); err != nil {
			return fmt.Errorf("triggers[%d]: %w", i, err)
		}
	}
	if c.Name != "" {
		if err := ValidateSlug(c.Name); err != nil {
			return fmt.Errorf("name: %w", err)
		}
	}
	return nil
}

// ResolvedName returns c.Name, falling back to DefaultChiefOfStaffName.
func (c *ChiefOfStaff) ResolvedName() string {
	if c.Name == "" {
		return DefaultChiefOfStaffName
	}
	return c.Name
}
