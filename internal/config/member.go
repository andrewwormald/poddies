package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// MembersDirName is the subdirectory under a pod dir that holds member files.
const MembersDirName = "members"

// Member is the configuration for a single pod member (agent).
// One file per member at <podDir>/members/<name>.toml.
type Member struct {
	Name              string   `toml:"name"`
	Title             string   `toml:"title"`
	Adapter           Adapter  `toml:"adapter"`
	Model             string   `toml:"model"`
	Effort            Effort   `toml:"effort"`
	Persona           string   `toml:"persona,omitempty"`
	Skills            []string `toml:"skills,omitempty"`
	SystemPromptExtra string   `toml:"system_prompt_extra,omitempty"`
}

// MemberPath returns the canonical path for a member file.
func MemberPath(podDir, name string) string {
	return filepath.Join(podDir, MembersDirName, name+".toml")
}

// LoadMember reads a single member file with strict decoding.
func LoadMember(podDir, name string) (*Member, error) {
	path := MemberPath(podDir, name)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %q: %w", path, err)
	}
	defer f.Close()

	var m Member
	dec := toml.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("decoding %q: %w", path, err)
	}
	return &m, nil
}

// SaveMember writes a member file with 0o600 permissions. The members/
// directory must already exist.
func SaveMember(podDir string, m *Member) error {
	if m == nil {
		return fmt.Errorf("nil Member")
	}
	if err := m.Validate(); err != nil {
		return fmt.Errorf("validate: %w", err)
	}
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.SetIndentTables(true)
	if err := enc.Encode(m); err != nil {
		return fmt.Errorf("encoding member: %w", err)
	}
	path := MemberPath(podDir, m.Name)
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("writing %q: %w", path, err)
	}
	return nil
}

// Validate runs structural validation on a Member.
func (m *Member) Validate() error {
	if err := ValidateSlug(m.Name); err != nil {
		return fmt.Errorf("name: %w", err)
	}
	if IsReservedMemberName(m.Name) {
		return fmt.Errorf("name %q is reserved", m.Name)
	}
	if m.Title == "" {
		return fmt.Errorf("title must not be empty")
	}
	if err := m.Adapter.Validate(); err != nil {
		return err
	}
	if m.Model == "" {
		return fmt.Errorf("model must not be empty")
	}
	if err := m.Effort.Validate(); err != nil {
		return err
	}
	return nil
}
