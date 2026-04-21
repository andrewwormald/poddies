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

// ExportPod loads the pod at <root>/pods/<name> into a Bundle and writes TOML
// to the given path. If outPath is empty the encoded bundle is returned as a
// byte slice without touching the filesystem.
func ExportPod(root, name, outPath string) ([]byte, error) {
	if !PodExists(root, name) {
		return nil, fmt.Errorf("%w: %q", ErrPodNotFound, name)
	}
	b, err := config.NewBundleFromPodDir(PodDir(root, name))
	if err != nil {
		return nil, fmt.Errorf("build bundle: %w", err)
	}
	if outPath == "" {
		var buf []byte
		w := &byteWriter{b: &buf}
		if err := config.SaveBundle(w, b); err != nil {
			return nil, err
		}
		return buf, nil
	}
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", outPath, err)
	}
	defer f.Close()
	if err := config.SaveBundle(f, b); err != nil {
		return nil, err
	}
	return nil, nil
}

// byteWriter is a minimal io.Writer that appends into a []byte slice.
type byteWriter struct{ b *[]byte }

func (w *byteWriter) Write(p []byte) (int, error) {
	*w.b = append(*w.b, p...)
	return len(p), nil
}

// ImportPod reads a bundle from bundlePath, optionally renames it with asName,
// and writes the pod to <root>/pods/<name>. overwrite controls whether an
// existing pod dir is replaced.
func ImportPod(root, bundlePath, asName string, overwrite bool) (*config.Bundle, error) {
	f, err := os.Open(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("open bundle %q: %w", bundlePath, err)
	}
	defer f.Close()

	b, err := config.LoadBundle(f)
	if err != nil {
		return nil, err
	}

	if asName != "" {
		if err := config.ValidateSlug(asName); err != nil {
			return nil, fmt.Errorf("--as name: %w", err)
		}
		b.Pod.Name = asName
	}

	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("validate bundle: %w", err)
	}

	podDir := PodDir(root, b.Pod.Name)
	if err := config.WriteBundleToPodDir(podDir, b, overwrite); err != nil {
		return nil, err
	}
	return b, nil
}

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
		ChiefOfStaff: config.ChiefOfStaff{
			Enabled:  true,
			Adapter:  config.AdapterClaude,
			Model:    "claude-sonnet-4-6",
			Triggers: []config.Trigger{config.TriggerGrayArea, config.TriggerMilestone, config.TriggerUnresolvedRouting},
		},
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
	cmd.AddCommand(a.newPodCreateCmd(), a.newPodListCmd(), a.newPodExportCmd(), a.newPodImportCmd())
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

func (a *App) newPodExportCmd() *cobra.Command {
	var outPath string
	cmd := &cobra.Command{
		Use:   "export <name>",
		Short: "Export a pod as a portable TOML bundle.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := a.rootFromApp()
			if err != nil {
				return err
			}
			data, err := ExportPod(root, args[0], outPath)
			if err != nil {
				return err
			}
			if outPath == "" {
				_, err = a.Out.Write(data)
				return err
			}
			fmt.Fprintf(a.Out, "exported pod %q to %s\n", args[0], outPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&outPath, "out", "", "write bundle to file instead of stdout")
	return cmd
}

func (a *App) newPodImportCmd() *cobra.Command {
	var asName string
	var overwrite bool
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import a pod bundle, creating the pod under the resolved root.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := a.rootFromApp()
			if err != nil {
				return err
			}
			b, err := ImportPod(root, args[0], asName, overwrite)
			if err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "imported pod %q to %s\n", b.Pod.Name, PodDir(root, b.Pod.Name))
			return nil
		},
	}
	cmd.Flags().StringVar(&asName, "as", "", "rename the pod on import")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "replace an existing pod of the same name")
	return cmd
}
