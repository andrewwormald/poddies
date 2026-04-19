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
	"github.com/andrewwormald/poddies/internal/session"
	"github.com/andrewwormald/poddies/internal/thread"
	"github.com/andrewwormald/poddies/internal/tui"
)

// launchTUI is the poddies-with-no-args entrypoint. Cold-launch flow:
// auto-migrate the legacy ./poddies/ directory if present, bootstrap
// the root (.poddies) if missing, pick or create a pod, create a
// fresh session for this launch, kick off an async cleanup goroutine,
// hand off to the TUI. If the TUI returns with a resume target, loop
// with that session instead of a fresh one — lets `/resume` work
// without the OS having to re-exec the binary.
func (a *App) launchTUI(ctx context.Context) error {
	// Legacy migration: if ./poddies/ is a directory and ./.poddies/
	// isn't there, move it. One-time per machine; subsequent launches
	// are no-ops.
	if migrated, err := session.MigrateLegacyRoot(a.Cwd); err != nil {
		return fmt.Errorf("migrate legacy root: %w", err)
	} else if migrated {
		fmt.Fprintln(a.Out, "migrated legacy ./poddies/ → ./.poddies/ (hidden)")
	}

	root, err := a.resolveOrInit()
	if err != nil {
		return err
	}

	pod, err := a.pickOrCreatePod(root)
	if err != nil {
		return err
	}

	// Kick off background cleanup. Async so the TUI doesn't wait on
	// disk scans before opening; 1-hour timeout per user spec.
	a.startCleanupGoroutine(root)

	// First iteration: always a fresh session. Resume loop iterates
	// into whichever session the TUI asks for.
	var resumeTo string
	for {
		var s session.Session
		if resumeTo != "" {
			if s, err = session.Find(root, resumeTo); err != nil {
				return fmt.Errorf("resume: %w", err)
			}
			resumeTo = ""
		} else {
			if s, err = session.Create(root, pod); err != nil {
				return fmt.Errorf("create session: %w", err)
			}
		}
		log := thread.Open(session.ThreadPath(root, s.ID))
		if err := log.EnsureFile(); err != nil {
			return err
		}
		next, err := a.launchTUIWithSession(ctx, root, pod, log, s.ID)
		if err != nil {
			return err
		}
		if next == "" {
			return nil
		}
		resumeTo = next
	}
}

// startCleanupGoroutine runs session.CleanupStale once, with a
// 1-hour context cap. Errors are surfaced to stderr but never block
// the TUI. Goroutine exits when the scan completes or the context
// times out — whichever comes first.
func (a *App) startCleanupGoroutine(root string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		maxAge := time.Duration(a.cleanupDays()) * 24 * time.Hour
		removed, err := session.CleanupStale(ctx, root, maxAge)
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintf(a.Err, "cleanup: %v\n", err)
		}
		if removed > 0 {
			fmt.Fprintf(a.Err, "cleanup: removed %d stale session(s)\n", removed)
		}
	}()
}

// cleanupDays reads the config cleanup window, defaulting to 30.
// Configurable per root via config.toml in a future commit; for now
// returns the default constant.
func (a *App) cleanupDays() int {
	return session.DefaultCleanupDays
}

// resolveOrInit returns the poddies root directory, prompting to
// create one when absent. ModeAuto prefers local; we honour that here
// by defaulting to ./poddies when nothing exists.
func (a *App) resolveOrInit() (string, error) {
	// If the current directory has a file (not a directory) at
	// ./poddies, ResolveRoot errors — surface a clean explanation
	// rather than the raw stat message. Most common trigger: the user
	// ran `go build` in the repo and got a binary named `poddies`
	// sitting next to where the config directory would go.
	localPath := config.LocalDir(a.Cwd)
	if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
		return "", fmt.Errorf(
			"%s exists but is a file, not a directory — remove it (e.g. a stray `go build` binary) or run poddies from a different directory",
			localPath,
		)
	}

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
// Returns the ID of a session the user wants to resume (from /resume),
// or empty string on a normal quit. Caller loops accordingly.
func (a *App) launchTUIWithSession(ctx context.Context, root, pod string, log *thread.Log, sessionID string) (string, error) {
	podCfg, err := config.LoadPod(PodDir(root, pod))
	if err != nil {
		return "", err
	}
	memberNames, err := listMemberNames(PodDir(root, pod))
	if err != nil {
		return "", err
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
	usageSnapshot := func() tui.UsageSnapshot {
		m, err := thread.LoadMeta(log.Path)
		if err != nil || m == nil {
			return tui.UsageSnapshot{}
		}
		return tui.UsageSnapshot{
			InputTokens:  m.InputTokens,
			OutputTokens: m.OutputTokens,
			CostUSD:      m.CostUSD,
			TurnCount:    m.TurnCount,
		}
	}
	runDoctor := func() []tui.DoctorCheck {
		checks := RunDoctor(DoctorOpts{Cwd: a.Cwd, Home: a.Home, EnvRoot: a.EnvRoot})
		out := make([]tui.DoctorCheck, 0, len(checks))
		for _, c := range checks {
			out = append(out, tui.DoctorCheck{Name: c.Name, Status: string(c.Status), Message: c.Message})
		}
		return out
	}

	listSessions := func() []tui.SessionSummary {
		list, err := session.ListRecent(root)
		if err != nil {
			return nil
		}
		out := make([]tui.SessionSummary, 0, len(list))
		for _, s := range list {
			out = append(out, tui.SessionSummary{
				ID:           s.ID,
				Pod:          s.Pod,
				TurnCount:    s.TurnCount,
				LastSpeaker:  s.LastSpeaker,
				LastEditedAt: s.LastEditedAt.Format(time.RFC3339),
				IsCurrent:    s.ID == sessionID,
			})
		}
		return out
	}

	// Resume output channel: the TUI sets this via OnResumeSession when
	// the user picks a session from the palette. After the TUI quits we
	// pick this up and loop the outer launchTUI into the new session.
	var resumeTarget string
	onResumeSession := func(id string) { resumeTarget = id }

	err = tui.Run(ctx, tui.Options{
		PodName:         podCfg.Name,
		SessionID:       sessionID,
		Members:         memberNames,
		Lead:            podCfg.Lead,
		StartLoop:       start,
		GetPending:      pending,
		OnApprove:       approve,
		OnDeny:          deny,
		OnAddMember:     addMember,
		OnRemoveMember:  removeMember,
		OnEditMember:    editMember,
		OnListMembers:   listMembers,
		OnListPods:      listPods,
		OnListThreads:   listThreads,
		OnListSessions:  listSessions,
		OnResumeSession: onResumeSession,
		OnExportPod:     exportPod,
		OnDoctor:        runDoctor,
		OnUsageSnapshot: usageSnapshot,
	}, a.In, a.Out)
	return resumeTarget, err
}

// Silence unused imports if some branches are compiled out in tests.
var _ = filepath.Join
var _ = fs.ErrNotExist
