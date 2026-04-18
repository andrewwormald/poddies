package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/andrewwormald/poddies/internal/config"
)

// ErrMemberNotFound is returned by member operations when the requested
// member file does not exist under the pod.
var ErrMemberNotFound = errors.New("member not found")

// AddMember validates m and writes it to <root>/pods/<pod>/members/<name>.toml.
// Errors if the pod does not exist or if a member with that name already exists.
func AddMember(root, pod string, m config.Member) error {
	if !PodExists(root, pod) {
		return fmt.Errorf("%w: %q", ErrPodNotFound, pod)
	}
	if err := m.Validate(); err != nil {
		return fmt.Errorf("validate: %w", err)
	}
	path := config.MemberPath(PodDir(root, pod), m.Name)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("member %q already exists in pod %q", m.Name, pod)
	}
	return config.SaveMember(PodDir(root, pod), &m)
}

// RemoveMember deletes <root>/pods/<pod>/members/<name>.toml. Errors if
// the file does not exist.
func RemoveMember(root, pod, name string) error {
	if !PodExists(root, pod) {
		return fmt.Errorf("%w: %q", ErrPodNotFound, pod)
	}
	path := config.MemberPath(PodDir(root, pod), name)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %q", ErrMemberNotFound, name)
		}
		return err
	}
	return os.Remove(path)
}

// MemberPatch is a partial update to a Member. Zero-valued fields are
// left untouched. Slices use nil to mean "unchanged" and empty non-nil
// (e.g. []string{}) to mean "clear".
type MemberPatch struct {
	Title             *string
	Adapter           *config.Adapter
	Model             *string
	Effort            *config.Effort
	Persona           *string
	Skills            []string
	SkillsExplicit    bool // true when Skills should overwrite (even to empty)
	SystemPromptExtra *string
}

// EditMember applies patch to the named member and writes it back.
// Returns the updated member. Errors if the member does not exist or if
// the resulting member fails validation.
func EditMember(root, pod, name string, patch MemberPatch) (*config.Member, error) {
	if !PodExists(root, pod) {
		return nil, fmt.Errorf("%w: %q", ErrPodNotFound, pod)
	}
	m, err := config.LoadMember(PodDir(root, pod), name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %q", ErrMemberNotFound, name)
		}
		return nil, err
	}
	if patch.Title != nil {
		m.Title = *patch.Title
	}
	if patch.Adapter != nil {
		m.Adapter = *patch.Adapter
	}
	if patch.Model != nil {
		m.Model = *patch.Model
	}
	if patch.Effort != nil {
		m.Effort = *patch.Effort
	}
	if patch.Persona != nil {
		m.Persona = *patch.Persona
	}
	if patch.SkillsExplicit {
		m.Skills = patch.Skills
	}
	if patch.SystemPromptExtra != nil {
		m.SystemPromptExtra = *patch.SystemPromptExtra
	}
	if err := config.SaveMember(PodDir(root, pod), m); err != nil {
		return nil, err
	}
	return m, nil
}

// --- cobra wiring ---

func (a *App) newMemberCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "member",
		Short: "Manage pod members (agents).",
	}
	cmd.AddCommand(a.newMemberAddCmd(), a.newMemberRemoveCmd(), a.newMemberEditCmd())
	return cmd
}

func (a *App) newMemberAddCmd() *cobra.Command {
	var (
		pod, name, title, adapterName, model, effort, persona, systemPromptExtra string
		skillsCSV                                                                string
	)
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a member (agent) to a pod.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pod == "" || name == "" || title == "" || adapterName == "" || model == "" || effort == "" {
				return errors.New("--pod, --name, --title, --adapter, --model, --effort are required")
			}
			root, err := a.rootFromApp()
			if err != nil {
				return err
			}
			m := config.Member{
				Name:              name,
				Title:             title,
				Adapter:           config.Adapter(adapterName),
				Model:             model,
				Effort:            config.Effort(effort),
				Persona:           persona,
				SystemPromptExtra: systemPromptExtra,
			}
			if skillsCSV != "" {
				m.Skills = splitCSV(skillsCSV)
			}
			if err := AddMember(root, pod, m); err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "added member %q to pod %q\n", name, pod)
			return nil
		},
	}
	cmd.Flags().StringVar(&pod, "pod", "", "pod name (required)")
	cmd.Flags().StringVar(&name, "name", "", "member name (slug)")
	cmd.Flags().StringVar(&title, "title", "", "member title (e.g. 'Staff Engineer')")
	cmd.Flags().StringVar(&adapterName, "adapter", "", "adapter name (claude|gemini|mock)")
	cmd.Flags().StringVar(&model, "model", "", "model identifier for the adapter")
	cmd.Flags().StringVar(&effort, "effort", "medium", "effort level (low|medium|high)")
	cmd.Flags().StringVar(&persona, "persona", "", "free-form persona/voice description")
	cmd.Flags().StringVar(&skillsCSV, "skills", "", "comma-separated skills tags")
	cmd.Flags().StringVar(&systemPromptExtra, "system-prompt-extra", "", "appended to the system prompt")
	return cmd
}

func (a *App) newMemberRemoveCmd() *cobra.Command {
	var pod, name string
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a member from a pod.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pod == "" || name == "" {
				return errors.New("--pod and --name are required")
			}
			root, err := a.rootFromApp()
			if err != nil {
				return err
			}
			if err := RemoveMember(root, pod, name); err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "removed member %q from pod %q\n", name, pod)
			return nil
		},
	}
	cmd.Flags().StringVar(&pod, "pod", "", "pod name")
	cmd.Flags().StringVar(&name, "name", "", "member name to remove")
	return cmd
}

func (a *App) newMemberEditCmd() *cobra.Command {
	var (
		pod, name, title, adapterName, model, effort, persona, systemPromptExtra string
		skillsCSV                                                                string
		skillsSet                                                                bool
	)
	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit a member's fields (partial update).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pod == "" || name == "" {
				return errors.New("--pod and --name are required")
			}
			root, err := a.rootFromApp()
			if err != nil {
				return err
			}
			patch := MemberPatch{}
			if cmd.Flags().Changed("title") {
				patch.Title = &title
			}
			if cmd.Flags().Changed("adapter") {
				v := config.Adapter(adapterName)
				patch.Adapter = &v
			}
			if cmd.Flags().Changed("model") {
				patch.Model = &model
			}
			if cmd.Flags().Changed("effort") {
				v := config.Effort(effort)
				patch.Effort = &v
			}
			if cmd.Flags().Changed("persona") {
				patch.Persona = &persona
			}
			if cmd.Flags().Changed("system-prompt-extra") {
				patch.SystemPromptExtra = &systemPromptExtra
			}
			if cmd.Flags().Changed("skills") {
				patch.SkillsExplicit = true
				if skillsCSV == "" {
					patch.Skills = []string{}
				} else {
					patch.Skills = splitCSV(skillsCSV)
				}
			}
			_ = skillsSet
			m, err := EditMember(root, pod, name, patch)
			if err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "updated member %q in pod %q\n", m.Name, pod)
			return nil
		},
	}
	cmd.Flags().StringVar(&pod, "pod", "", "pod name")
	cmd.Flags().StringVar(&name, "name", "", "member name to edit")
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&adapterName, "adapter", "", "new adapter")
	cmd.Flags().StringVar(&model, "model", "", "new model")
	cmd.Flags().StringVar(&effort, "effort", "", "new effort")
	cmd.Flags().StringVar(&persona, "persona", "", "new persona")
	cmd.Flags().StringVar(&skillsCSV, "skills", "", "new skills (comma-separated; pass empty to clear)")
	cmd.Flags().StringVar(&systemPromptExtra, "system-prompt-extra", "", "new extra system prompt")
	return cmd
}

// splitCSV parses a comma-separated list, trimming whitespace and
// dropping empty entries. "a, b,,c" -> ["a","b","c"].
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
