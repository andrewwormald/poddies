package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/orchestrator"
	"github.com/andrewwormald/poddies/internal/thread"
	"github.com/andrewwormald/poddies/internal/tui"
)

// launchTUI is the poddies-with-no-args entrypoint. It handles the
// cold-launch bootstrap flow (init if no root, pod-create if no pods)
// and then hands off to the bubbletea TUI with an active pod + thread
// session wired up.
func (a *App) launchTUI(ctx context.Context) error {
	root, err := a.resolveOrInit()
	if err != nil {
		return err
	}

	pod, err := a.pickOrCreatePod(root)
	if err != nil {
		return err
	}

	threadName := DefaultThreadName
	log := thread.Open(ThreadPath(root, pod, threadName))
	if err := log.EnsureFile(); err != nil {
		return err
	}

	return a.launchTUIWithSession(ctx, root, pod, log)
}

// resolveOrInit returns the poddies root directory, prompting to
// create one when absent. ModeAuto prefers local; we honour that here
// by defaulting to ./poddies when nothing exists.
func (a *App) resolveOrInit() (string, error) {
	res, err := config.ResolveRoot(config.ModeAuto, a.Cwd, a.Home, a.EnvRoot)
	if err == nil {
		return res.Dir, nil
	}
	if !errors.Is(err, config.ErrNoRoot) {
		return "", err
	}
	// No root anywhere. Auto-create a local root silently — matches how
	// k9s assumes ~/.kube/config is there and just works. Users who
	// want global can still use `poddies init --global` via the hidden
	// scripting surface.
	result, err := Init(a.Cwd, a.Home, config.ModeLocal, false)
	if err != nil {
		return "", fmt.Errorf("bootstrap local poddies root: %w", err)
	}
	fmt.Fprintf(a.Out, "created poddies root at %s\n", result.Dir)
	return result.Dir, nil
}

// pickOrCreatePod returns the name of the pod to open. If no pods
// exist a default "default" pod is scaffolded; multiple pods resolve
// via environment-variable POD (or first alphabetical for v1).
func (a *App) pickOrCreatePod(root string) (string, error) {
	names, err := ListPods(root)
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		p, err := CreatePod(root, "default")
		if err != nil {
			return "", fmt.Errorf("bootstrap default pod: %w", err)
		}
		fmt.Fprintf(a.Out, "created default pod %q\n", p.Name)
		return p.Name, nil
	}
	if preferred := os.Getenv("POD"); preferred != "" {
		for _, n := range names {
			if n == preferred {
				return n, nil
			}
		}
	}
	sort.Strings(names)
	return names[0], nil
}

// launchTUIWithSession wires a TUI session for the given pod + log.
// Extracted so tests / future multi-pod switching can reuse it.
func (a *App) launchTUIWithSession(ctx context.Context, root, pod string, log *thread.Log) error {
	podCfg, err := config.LoadPod(PodDir(root, pod))
	if err != nil {
		return err
	}
	memberNames, err := listMemberNames(PodDir(root, pod))
	if err != nil {
		return err
	}

	start := func(lctx context.Context, kickoff string, onEvent func(thread.Event)) (orchestrator.LoopResult, error) {
		loop := &orchestrator.Loop{
			Root:          root,
			Pod:           pod,
			AdapterLookup: a.adapterLookup(),
			Log:           log,
			HumanMessage:  kickoff,
			OnEvent:       onEvent,
		}
		return loop.Run(lctx)
	}

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
	removeMember := func(name string) error { return RemoveMember(root, pod, name) }
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
		names, err := listMemberNames(PodDir(root, pod))
		if err != nil {
			return nil
		}
		return names
	}
	listPods := func() []string {
		names, _ := ListPods(root)
		return names
	}
	listThreads := func() []tui.ThreadSummary {
		infos, err := ListThreads(root, pod)
		if err != nil {
			return nil
		}
		out := make([]tui.ThreadSummary, 0, len(infos))
		for _, t := range infos {
			out = append(out, tui.ThreadSummary{
				Name:       t.Name,
				Events:     t.Events,
				LastFrom:   t.LastFrom,
				ModifiedAt: t.ModifiedAt.Format(time.RFC3339),
				Corrupt:    t.Corrupt,
			})
		}
		return out
	}
	exportPod := func() ([]byte, error) { return ExportPod(root, pod, "") }
	runDoctor := func() []tui.DoctorCheck {
		checks := RunDoctor(DoctorOpts{Cwd: a.Cwd, Home: a.Home, EnvRoot: a.EnvRoot})
		out := make([]tui.DoctorCheck, 0, len(checks))
		for _, c := range checks {
			out = append(out, tui.DoctorCheck{Name: c.Name, Status: string(c.Status), Message: c.Message})
		}
		return out
	}

	return tui.Run(ctx, tui.Options{
		PodName:        podCfg.Name,
		Members:        memberNames,
		Lead:           podCfg.Lead,
		StartLoop:      start,
		GetPending:     pending,
		OnApprove:      approve,
		OnDeny:         deny,
		OnAddMember:    addMember,
		OnRemoveMember: removeMember,
		OnEditMember:   editMember,
		OnListMembers:  listMembers,
		OnListPods:     listPods,
		OnListThreads:  listThreads,
		OnExportPod:    exportPod,
		OnDoctor:       runDoctor,
	}, a.In, a.Out)
}

// Silence unused imports if some branches are compiled out in tests.
var _ = filepath.Join
var _ = fs.ErrNotExist
