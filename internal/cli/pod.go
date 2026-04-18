package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/andrewwormald/poddies/internal/config"
)

// CreatePod creates <root>/pods/<name>/ and writes a minimal pod.toml.
// The root must exist (run `poddies init` first). Errors if a pod with
// that name already exists or if name is not a valid slug.
func CreatePod(root, name string) (*config.Pod, error) {
	if err := config.ValidateSlug(name); err != nil {
		return nil, fmt.Errorf("pod name: %w", err)
	}
	if _, err := os.Stat(root); err != nil {
		return nil, fmt.Errorf("root %q: %w", root, err)
	}
	podDir := filepath.Join(root, PodsDirName, name)
	if _, err := os.Stat(podDir); err == nil {
		return nil, fmt.Errorf("pod %q already exists at %s", name, podDir)
	}
	if err := os.MkdirAll(filepath.Join(podDir, config.MembersDirName), 0o700); err != nil {
		return nil, fmt.Errorf("mkdir %q: %w", podDir, err)
	}
	p := &config.Pod{
		Name: name,
		Lead: "human",
	}
	if err := config.SavePod(podDir, p); err != nil {
		return nil, err
	}
	return p, nil
}

// ListPods returns the pod names (not paths) under <root>/pods, sorted.
// Missing pods dir returns an empty slice (not an error).
func ListPods(root string) ([]string, error) {
	podsDir := filepath.Join(root, PodsDirName)
	entries, err := os.ReadDir(podsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %q: %w", podsDir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// only include dirs that actually have a pod.toml (guards
		// against stray dirs left by the user).
		if _, err := os.Stat(filepath.Join(podsDir, e.Name(), config.PodFileName)); err != nil {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// PodDir returns <root>/pods/<name>.
func PodDir(root, name string) string {
	return filepath.Join(root, PodsDirName, name)
}

// PodExists reports whether <root>/pods/<name>/pod.toml exists.
func PodExists(root, name string) bool {
	_, err := os.Stat(filepath.Join(PodDir(root, name), config.PodFileName))
	return err == nil
}

// ErrPodNotFound is returned when an operation targets a pod that does
// not exist under the root.
var ErrPodNotFound = errors.New("pod not found")

// rootFromApp resolves which root the command should operate on, using
// the standard precedence (env > local > global). A command-scoped
// --global/--local override could be added later; for v1 we rely on the
// ambient filesystem state.
func (a *App) rootFromApp() (string, error) {
	res, err := config.ResolveRoot(config.ModeAuto, a.Cwd, a.Home, a.EnvRoot)
	if err != nil {
		return "", err
	}
	return res.Dir, nil
}

func (a *App) newPodCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pod",
		Short: "Manage pods.",
	}
	cmd.AddCommand(a.newPodCreateCmd(), a.newPodListCmd())
	return cmd
}

func (a *App) newPodCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new pod under the resolved root.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := a.rootFromApp()
			if err != nil {
				return err
			}
			p, err := CreatePod(root, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "created pod %q at %s\n", p.Name, PodDir(root, p.Name))
			return nil
		},
	}
}

func (a *App) newPodListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List pods under the resolved root.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := a.rootFromApp()
			if err != nil {
				return err
			}
			names, err := ListPods(root)
			if err != nil {
				return err
			}
			if len(names) == 0 {
				fmt.Fprintln(a.Out, "no pods")
				return nil
			}
			for _, n := range names {
				fmt.Fprintln(a.Out, n)
			}
			return nil
		},
	}
}
