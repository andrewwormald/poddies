package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/orchestrator"
	"github.com/andrewwormald/poddies/internal/thread"
	"github.com/andrewwormald/poddies/internal/tui"
)

// runStdout executes a single orchestrator.Loop in stdout streaming
// mode — used by both `poddies run` and `poddies thread resume` when
// --tui is not set.
func runStdout(ctx context.Context, a *App, root, pod string, log *thread.Log, firstMember, message string, maxTurns int, effort config.Effort) error {
	loop := &orchestrator.Loop{
		Root:           root,
		Pod:            pod,
		AdapterLookup:  a.adapterLookup(),
		Log:            log,
		HumanMessage:   message,
		MaxTurns:       maxTurns,
		EffortOverride: effort,
		FirstMember:    firstMember,
		OnEvent: func(e thread.Event) {
			switch e.Type {
			case thread.EventHuman:
				fmt.Fprintf(a.Out, "[human] %s\n", e.Body)
			case thread.EventMessage:
				fmt.Fprintf(a.Out, "[%s] %s\n", e.From, e.Body)
			case thread.EventSystem:
				fmt.Fprintf(a.Out, "[system] %s\n", e.Body)
			default:
				fmt.Fprintf(a.Out, "[%s] %s\n", e.Type, e.Body)
			}
		},
	}
	res, err := loop.Run(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "-- stopped: %s (turns=%d) --\n", res.StopReason, res.TurnsRun)
	return nil
}

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
		maxTurns                                         int
		useTUI                                           bool
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a pod. Loops across members via @mention routing until quiescence or --max-turns.",
		RunE: func(cmd *cobra.Command, args []string) error {
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

			if useTUI {
				return a.runTUI(cmd.Context(), root, pod, log, memberName, message, maxTurns, config.Effort(effort))
			}
			return runStdout(cmd.Context(), a, root, pod, log, memberName, message, maxTurns, config.Effort(effort))
		},
	}
	cmd.Flags().StringVar(&podName, "pod", "", "pod name (auto-selected when only one pod exists)")
	cmd.Flags().StringVar(&memberName, "member", "", "force the first turn for this member (subsequent turns use routing)")
	cmd.Flags().StringVar(&threadName, "thread", "", "thread filename under pods/<pod>/threads/ (default: default.jsonl)")
	cmd.Flags().StringVar(&message, "message", "", "optional human kickoff message appended before the loop")
	cmd.Flags().StringVar(&effort, "effort", "", "override every member's effort (low|medium|high)")
	cmd.Flags().IntVar(&maxTurns, "max-turns", 0, "cap on member invocations (0 = default, -1 = unlimited subject to safety cap)")
	cmd.Flags().BoolVar(&useTUI, "tui", false, "render the bubbletea TUI instead of stdout mode")
	return cmd
}

// runTUI wires an orchestrator.Loop behind the TUI's StartLoop callback.
// Each TUI submission starts a fresh Loop invocation, sharing the thread
// log so the conversation accumulates across prompts.
func (a *App) runTUI(ctx context.Context, root, pod string, log *thread.Log, firstMember, initialMessage string, maxTurns int, effort config.Effort) error {
	podCfg, err := config.LoadPod(PodDir(root, pod))
	if err != nil {
		return err
	}
	memberNames, _, err := listMemberNames(PodDir(root, pod))
	if err != nil {
		return err
	}

	start := func(lctx context.Context, kickoff string, onEvent func(thread.Event)) (orchestrator.LoopResult, error) {
		loop := &orchestrator.Loop{
			Root:           root,
			Pod:            pod,
			AdapterLookup:  a.adapterLookup(),
			Log:            log,
			HumanMessage:   kickoff,
			MaxTurns:       maxTurns,
			EffortOverride: effort,
			FirstMember:    firstMember,
			OnEvent:        onEvent,
		}
		firstMember = "" // only the first submission respects --member
		return loop.Run(lctx)
	}

	// Pending-permission callbacks: wire approve/deny through the
	// existing thread helpers so the TUI's keybindings mutate the log.
	pending := func() []thread.Event {
		events, err := log.Load()
		if err != nil {
			return nil
		}
		return thread.PendingPermissions(events)
	}
	approve := func(requestID string) error {
		events, err := log.Load()
		if err != nil {
			return err
		}
		_, err = AppendGrant(log, events, requestID, "human")
		return err
	}
	deny := func(requestID, reason string) error {
		events, err := log.Load()
		if err != nil {
			return err
		}
		_, err = AppendDeny(log, events, requestID, "human", reason)
		return err
	}

	// Slash-command callbacks: /add, /remove, /edit, /export, and the
	// dynamic roster lookup. Each delegates to the existing CLI-level
	// functions so business logic is centralized.
	addMember := func(spec tui.AddMemberSpec) error {
		return AddMember(root, pod, config.Member{
			Name:    spec.Name,
			Title:   spec.Title,
			Adapter: config.Adapter(spec.Adapter),
			Model:   spec.Model,
			Effort:  config.Effort(spec.Effort),
			Persona: spec.Persona,
		})
	}
	removeMember := func(name string) error {
		return RemoveMember(root, pod, name)
	}
	editMember := func(name, field, value string) error {
		patch := MemberPatch{}
		switch field {
		case "title":
			patch.Title = &value
		case "adapter":
			v := config.Adapter(value)
			patch.Adapter = &v
		case "model":
			patch.Model = &value
		case "effort":
			v := config.Effort(value)
			patch.Effort = &v
		case "persona":
			patch.Persona = &value
		default:
			return fmt.Errorf("unknown field %q", field)
		}
		_, err := EditMember(root, pod, name, patch)
		return err
	}
	listMembers := func() []string {
		names, _, err := listMemberNames(PodDir(root, pod))
		if err != nil {
			return nil
		}
		return names
	}
	exportPod := func() ([]byte, error) {
		return ExportPod(root, pod, "")
	}

	return tui.Run(ctx, tui.Options{
		PodName:        podCfg.Name,
		Members:        memberNames,
		Lead:           podCfg.Lead,
		StartLoop:      start,
		InitialKickoff: initialMessage,
		GetPending:     pending,
		OnApprove:      approve,
		OnDeny:         deny,
		OnAddMember:    addMember,
		OnRemoveMember: removeMember,
		OnEditMember:   editMember,
		OnListMembers:  listMembers,
		OnExportPod:    exportPod,
	}, a.In, a.Out)
}

// listMemberNames returns the member names under podDir, sorted. Shared
// with the orchestrator's roster loader but kept local to avoid an
// import cycle; it intentionally produces just names, not configs.
func listMemberNames(podDir string) ([]string, []string, error) {
	entries, err := osReadDir(filepath.Join(podDir, config.MembersDirName))
	if err != nil {
		return nil, nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if len(n) > 5 && n[len(n)-5:] == ".toml" {
			names = append(names, n[:len(n)-5])
		}
	}
	return names, names, nil
}

