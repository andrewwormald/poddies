package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

// ErrThreadNotFound is returned by thread commands when the named
// thread file does not exist under the resolved pod.
var ErrThreadNotFound = errors.New("thread not found")

// ThreadInfo is one row of `poddies thread list` output.
type ThreadInfo struct {
	Name       string    // without .jsonl suffix
	Path       string    // absolute
	Events     int       // count of events in the file
	LastFrom   string    // from-field of the last non-system event (or "")
	ModifiedAt time.Time // file mtime
	// Corrupt is true when the underlying JSONL file failed to parse.
	// Callers should render it distinctly so a broken file is never
	// confused with a legitimately empty new thread.
	Corrupt bool
}

// ListThreads walks <root>/pods/<pod>/threads/ and returns one ThreadInfo
// per .jsonl file, sorted by most-recently-modified first.
func ListThreads(root, pod string) ([]ThreadInfo, error) {
	dir := filepath.Join(PodDir(root, pod), ThreadsDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %q: %w", dir, err)
	}
	out := make([]ThreadInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		fullPath := filepath.Join(dir, name)
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		events, err := thread.Open(fullPath).Load()
		if err != nil {
			// don't block listing on one corrupt file, but flag it so
			// the CLI can render a distinct marker rather than silently
			// showing events=0.
			out = append(out, ThreadInfo{
				Name:       strings.TrimSuffix(name, ".jsonl"),
				Path:       fullPath,
				ModifiedAt: info.ModTime(),
				Corrupt:    true,
			})
			continue
		}
		last := ""
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].Type == thread.EventSystem {
				continue
			}
			last = events[i].From
			break
		}
		out = append(out, ThreadInfo{
			Name:       strings.TrimSuffix(name, ".jsonl"),
			Path:       fullPath,
			Events:     len(events),
			LastFrom:   last,
			ModifiedAt: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ModifiedAt.After(out[j].ModifiedAt)
	})
	return out, nil
}

// normalizeThreadName converts "default" → "default.jsonl", leaves a
// already-suffixed name alone. Lets the user type short IDs.
func normalizeThreadName(name string) string {
	if strings.HasSuffix(name, ".jsonl") {
		return name
	}
	return name + ".jsonl"
}

// LoadThread loads the events for the named thread. Returns
// ErrThreadNotFound when the file is absent.
func LoadThread(root, pod, name string) ([]thread.Event, error) {
	path := filepath.Join(PodDir(root, pod), ThreadsDirName, normalizeThreadName(name))
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %q", ErrThreadNotFound, name)
		}
		return nil, err
	}
	return thread.Open(path).Load()
}

// --- cobra wiring ---

func (a *App) newThreadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "thread",
		Short: "Inspect and resume threads under a pod.",
	}
	cmd.AddCommand(
		a.newThreadListCmd(),
		a.newThreadShowCmd(),
		a.newThreadResumeCmd(),
		a.newThreadPermissionsCmd(),
		a.newThreadApproveCmd(),
		a.newThreadDenyCmd(),
	)
	return cmd
}

func (a *App) newThreadListCmd() *cobra.Command {
	var podName string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List threads under a pod, newest first.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := a.rootFromApp()
			if err != nil {
				return err
			}
			pod, err := resolvePod(root, podName)
			if err != nil {
				return err
			}
			infos, err := ListThreads(root, pod)
			if err != nil {
				return err
			}
			if len(infos) == 0 {
				fmt.Fprintln(a.Out, "no threads")
				return nil
			}
			for _, t := range infos {
				if t.Corrupt {
					fmt.Fprintf(a.Out, "%-24s CORRUPT (failed to parse)  %s\n",
						t.Name, t.ModifiedAt.Format(time.RFC3339))
					continue
				}
				last := t.LastFrom
				if last == "" {
					last = "-"
				}
				fmt.Fprintf(a.Out, "%-24s events=%-4d last=%-12s  %s\n",
					t.Name, t.Events, last, t.ModifiedAt.Format(time.RFC3339))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&podName, "pod", "", "pod name (auto-selected when only one pod exists)")
	return cmd
}

func (a *App) newThreadShowCmd() *cobra.Command {
	var podName string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show <thread>",
		Short: "Print the events of a thread.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := a.rootFromApp()
			if err != nil {
				return err
			}
			pod, err := resolvePod(root, podName)
			if err != nil {
				return err
			}
			events, err := LoadThread(root, pod, args[0])
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(a.Out)
				for _, e := range events {
					if err := enc.Encode(e); err != nil {
						return err
					}
				}
				return nil
			}
			for _, e := range events {
				switch e.Type {
				case thread.EventHuman:
					fmt.Fprintf(a.Out, "[human] %s\n", e.Body)
				case thread.EventMessage:
					fmt.Fprintf(a.Out, "[%s] %s\n", e.From, e.Body)
				case thread.EventSystem:
					fmt.Fprintf(a.Out, "[system] %s\n", e.Body)
				case thread.EventPermissionRequest:
					fmt.Fprintf(a.Out, "[permission_request from %s] action=%s\n", e.From, e.Action)
				case thread.EventPermissionGrant:
					fmt.Fprintf(a.Out, "[permission_grant by %s for %s]\n", e.From, e.RequestID)
				case thread.EventPermissionDeny:
					fmt.Fprintf(a.Out, "[permission_deny by %s for %s]\n", e.From, e.RequestID)
				default:
					fmt.Fprintf(a.Out, "[%s] %s\n", e.Type, e.Body)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&podName, "pod", "", "pod name (auto-selected when only one pod exists)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit raw JSONL instead of human-readable transcript")
	return cmd
}

func (a *App) newThreadResumeCmd() *cobra.Command {
	var (
		podName, memberName, message, effort string
		maxTurns                             int
		useTUI                               bool
	)
	cmd := &cobra.Command{
		Use:   "resume <thread>",
		Short: "Resume an existing thread (shorthand for `run --thread <name>`).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := a.rootFromApp()
			if err != nil {
				return err
			}
			pod, err := resolvePod(root, podName)
			if err != nil {
				return err
			}
			threadName := normalizeThreadName(args[0])
			// confirm thread exists before starting a loop
			if _, err := os.Stat(ThreadPath(root, pod, threadName)); err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return fmt.Errorf("%w: %q", ErrThreadNotFound, args[0])
				}
				return err
			}
			log := thread.Open(ThreadPath(root, pod, threadName))
			if err := log.EnsureFile(); err != nil {
				return err
			}
			if useTUI {
				return a.runTUI(cmd.Context(), root, pod, log, memberName, message, maxTurns, config.Effort(effort))
			}
			return runStdout(cmd.Context(), a, root, pod, log, memberName, message, maxTurns, config.Effort(effort))
		},
	}
	cmd.Flags().StringVar(&podName, "pod", "", "pod name")
	cmd.Flags().StringVar(&memberName, "member", "", "force first turn for this member")
	cmd.Flags().StringVar(&message, "message", "", "optional human message appended before the loop")
	cmd.Flags().StringVar(&effort, "effort", "", "override every member's effort")
	cmd.Flags().IntVar(&maxTurns, "max-turns", 0, "cap on member invocations")
	cmd.Flags().BoolVar(&useTUI, "tui", false, "render the bubbletea TUI")
	return cmd
}
