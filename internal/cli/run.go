package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/orchestrator"
	"github.com/andrewwormald/poddies/internal/thread"
)

// DefaultThreadName is used when `poddies run` is called without --thread.
const DefaultThreadName = "default.jsonl"

// ThreadsDirName is the subdirectory under a pod dir that holds threads.
const ThreadsDirName = "threads"

// ThreadPath returns the canonical on-disk path for a named thread.
func ThreadPath(root, pod, threadName string) string {
	return filepath.Join(PodDir(root, pod), ThreadsDirName, threadName)
}

// resolvePod returns the requested pod name, or auto-selects when
// exactly one pod exists and none was specified.
func resolvePod(root, requested string) (string, error) {
	if requested != "" {
		if !PodExists(root, requested) {
			return "", fmt.Errorf("%w: %q", ErrPodNotFound, requested)
		}
		return requested, nil
	}
	pods, err := ListPods(root)
	if err != nil {
		return "", err
	}
	switch len(pods) {
	case 0:
		return "", errors.New("no pods; run `poddies pod create <name>`")
	case 1:
		return pods[0], nil
	default:
		return "", fmt.Errorf("multiple pods exist (%v); pass --pod to choose one", pods)
	}
}

func (a *App) newRunCmd() *cobra.Command {
	var (
		podName, memberName, threadName, message, effort string
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a single turn for a pod member against a thread.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if memberName == "" {
				return errors.New("--member is required")
			}
			root, err := a.rootFromApp()
			if err != nil {
				return err
			}
			pod, err := resolvePod(root, podName)
			if err != nil {
				return err
			}
			if threadName == "" {
				threadName = DefaultThreadName
			}

			log := thread.Open(ThreadPath(root, pod, threadName))
			if err := log.EnsureFile(); err != nil {
				return err
			}

			turn := &orchestrator.Turn{
				Root:           root,
				Pod:            pod,
				Member:         memberName,
				AdapterLookup:  a.adapterLookup(),
				Log:            log,
				HumanMessage:   message,
				EffortOverride: config.Effort(effort),
			}
			res, err := turn.Run(cmd.Context())
			if err != nil {
				return err
			}
			if res.HumanEvent.Type != "" {
				fmt.Fprintf(a.Out, "[human] %s\n", res.HumanEvent.Body)
			}
			fmt.Fprintf(a.Out, "[%s] %s\n", res.MemberEvent.From, res.MemberEvent.Body)
			return nil
		},
	}
	cmd.Flags().StringVar(&podName, "pod", "", "pod name (auto-selected when only one pod exists)")
	cmd.Flags().StringVar(&memberName, "member", "", "member to invoke (required)")
	cmd.Flags().StringVar(&threadName, "thread", "", "thread filename under pods/<pod>/threads/ (default: default.jsonl)")
	cmd.Flags().StringVar(&message, "message", "", "optional human kickoff message appended before the turn")
	cmd.Flags().StringVar(&effort, "effort", "", "override member effort (low|medium|high)")
	return cmd
}
