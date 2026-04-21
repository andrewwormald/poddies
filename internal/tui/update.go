package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/andrewwormald/poddies/internal/orchestrator"
	"github.com/andrewwormald/poddies/internal/thread"
)

// Init implements tea.Model. It arms the initial subscription Cmd.
// When the pod has no members yet, Init queues an onboarding-trigger
// message that opens the addMemberWizard on first tick. Otherwise, if
// an InitialKickoff was configured, it auto-submits that instead.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{waitForSubMsg(m.sub), waitForSubMsg(m.breakawaySub)}
	if m.needsOnboarding() {
		cmds = append(cmds, func() tea.Msg { return startOnboardingMsg{} })
	} else if m.opts.InitialKickoff != "" {
		cmds = append(cmds, func() tea.Msg {
			return autoSubmitMsg{text: m.opts.InitialKickoff}
		})
	}
	return tea.Batch(cmds...)
}

// needsOnboarding reports whether the TUI should open an addMember
// wizard on first tick — true when the pod has no members yet.
func (m Model) needsOnboarding() bool {
	if m.opts.OnAddMember == nil {
		// we can't add members via the TUI in this session, so there's
		// nothing useful onboarding can do. Skip.
		return false
	}
	if m.opts.OnListMembers != nil {
		return len(m.opts.OnListMembers()) == 0
	}
	return len(m.opts.Members) == 0
}

// startOnboardingMsg is an internal signal handled by Update to kick
// off the onboarding wizard on the first tick.
type startOnboardingMsg struct{}

// autoSubmitMsg is an internal signal used by Init to trigger a
// first-tick kickoff without the user having to press Enter. Exported
// via the message interface for clarity in Update.
type autoSubmitMsg struct{ text string }

// waitForSubMsg returns a Cmd that reads one message from the Model's
// subscription channel. After every channel-driven Update, we re-arm
// this Cmd so the channel keeps being drained. The channel is a
// reference type, so the value-semantics Model doesn't lose the
// subscription across Update calls.
func waitForSubMsg(sub <-chan any) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-sub
		if !ok {
			return nil
		}
		return msg
	}
}

// activeBreakaway tracks a running background conversation.
type activeBreakaway struct {
	Members []string
	Topic   string
}

// BreakawayEventMsg streams a single event from a breakaway conversation.
type BreakawayEventMsg struct {
	Event   thread.Event
	Members []string // participants in this breakaway
}

// BreakawayDoneMsg signals a breakaway completed.
type BreakawayDoneMsg struct {
	Summary string
	Members []string
	LogPath string // path to the breakaway thread log for debugging
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onResize(msg)
	case BreakawayEventMsg:
		return m.onBreakawayEvent(msg)
	case BreakawayDoneMsg:
		return m.onBreakawayDone(msg)
	case tea.KeyMsg:
		return m.onKey(msg)
	case autoSubmitMsg:
		return m.submit(msg.text)
	case startOnboardingMsg:
		m.statusLine = "welcome — let's add your first member"
		m = m.activateWizard(onboardingAddMemberWizard(m.opts))
		return m, waitForSubMsg(m.sub)
	case animTickMsg:
		return m.onAnimTick(msg)
	case EventMsg:
		return m.onEvent(msg)
	case LoopDoneMsg:
		return m.onLoopDone(msg)
	case errorMsg:
		m.lastErr = msg.err
		m.statusLine = "error: " + msg.err.Error()
		return m, waitForSubMsg(m.sub)
	case tea.MouseMsg:
		// Forward mouse events (scroll wheel) to the viewport.
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

// autoScrollBottom scrolls to the bottom only if the user was already
// at the bottom (within 2 lines). This prevents new events from yanking
// the user back down while they're reading history.
func (m *Model) autoScrollBottom() {
	atBottom := m.viewport.AtBottom() || m.viewport.TotalLineCount()-m.viewport.YOffset <= m.viewport.Height+2
	if atBottom {
		m.viewport.GotoBottom()
	}
}

// chatWidth returns the width available to the chat viewport,
// accounting for the viz panel when it is open.
func (m Model) chatWidth() int {
	if m.vizOpen {
		w := m.width - vizPanelW - 1 // 1 for divider column
		if w < 20 {
			return 20
		}
		return w
	}
	return m.width
}

// recalcViewport resizes the viewport and re-renders the transcript to
// match the current terminal width and viz-panel state. Call after
// toggling vizOpen or when a WindowSizeMsg arrives.
func (m Model) recalcViewport() Model {
	w := m.chatWidth()
	m.viewport.Width = w
	m.input.Width = w - 4
	m.viewport.SetContent(m.viewportContent(w))
	return m
}

// viewportContent builds the transcript text plus an optional typing
// indicator when a loop is running.
func (m Model) viewportContent(w int) string {
	content := renderTranscript(m.events, m.opts.CoSName, w, m.avatarSize, m.debug)
	if m.state == StateRunning {
		content += m.renderTypingIndicator()
	}
	return content
}

// Spinner frames for the typing indicator.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// renderTypingIndicator returns a coloured "is typing…" line with spinner.
func (m Model) renderTypingIndicator() string {
	frame := spinnerFrames[m.typingTick%len(spinnerFrames)]
	who := m.typingWho
	if who == "" {
		return metaStyle.Render(frame+" thinking…") + "\n"
	}
	av := AvatarFor(who)
	style := lipgloss.NewStyle().Foreground(av.Color)
	return style.Render(frame+" "+who+" is typing…") + "\n"
}

// Width thresholds for responsive defaults.
const (
	smallWindowW = 100 // below this: hide viz, small avatars
	largeWindowW = 140 // above this: show viz, large avatars
)

// onResize updates the viewport and input widths, caching dimensions
// so View can lay out cleanly. On the first resize (when we learn the
// actual terminal size), applies size-based defaults for viz and avatar
// unless the user has persisted preferences.
func (m Model) onResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	headerH := 3
	footerH := 3
	vpH := msg.Height - headerH - footerH
	if vpH < 3 {
		vpH = 3
	}
	m.viewport.Height = vpH

	// Apply size-based defaults once (first resize = actual terminal size).
	if !m.prefsApplied {
		m.prefsApplied = true
		if m.prefs.VizOpen == nil {
			m.vizOpen = msg.Width >= largeWindowW
		}
		if m.prefs.AvatarSize == nil {
			if msg.Width < smallWindowW {
				m.avatarSize = AvatarOff
			} else if msg.Width >= largeWindowW {
				m.avatarSize = AvatarLarge
			} else {
				m.avatarSize = AvatarSmall
			}
		}
	}

	m.ready = true
	m = m.recalcViewport()
	m.viewport.GotoBottom()
	var cmds []tea.Cmd
	if m.vizOpen {
		cmds = append(cmds, animTick())
	}
	return m, tea.Batch(cmds...)
}

// onKey dispatches keyboard input based on state.
func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Palette capture takes precedence over everything except Ctrl-C.
	if m.state == StatePalette {
		return m.onPaletteKey(msg)
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		// always quits, cancelling any in-flight loop
		if m.cancelLoop != nil {
			m.cancelLoop()
		}
		m.state = StateQuit
		return m, tea.Quit
	case tea.KeyEsc:
		if m.state == StatePrompting {
			m = m.cancelWizard()
			return m, waitForSubMsg(m.sub)
		}
		// From any non-thread view, Esc returns home.
		if m.view != ViewThread {
			m.view = ViewThread
			m.statusLine = ""
			return m, waitForSubMsg(m.sub)
		}
	case tea.KeyUp:
		if (m.state == StateIdle || m.state == StateRunning) && (m.view == ViewPods || m.view == ViewThreads || m.view == ViewSessions) {
			if m.cursorPos > 0 {
				m.cursorPos--
			}
			return m, waitForSubMsg(m.sub)
		}
	case tea.KeyDown:
		if (m.state == StateIdle || m.state == StateRunning) && (m.view == ViewPods || m.view == ViewThreads || m.view == ViewSessions) {
			m.cursorPos = m.cursorPos + 1 // clamped in view render
			return m, waitForSubMsg(m.sub)
		}
	case tea.KeyTab:
		if (m.state == StateIdle || m.state == StateRunning) && m.view == ViewThread {
			// Try slash command completion first, then @mention.
			if _, ok := findSlashSuggestion(m.input.Value()); ok {
				m.input.SetValue(applySlashSuggestion(m.input.Value()))
				return m, waitForSubMsg(m.sub)
			}
			if _, ok := findMentionSuggestion(m.input.Value(), m.currentRoster(), m.opts.CoSName); ok {
				m.input.SetValue(applySuggestion(m.input.Value(), m.currentRoster(), m.opts.CoSName))
				return m, waitForSubMsg(m.sub)
			}
		}
	case tea.KeyEnter:
		// In :pods view, Enter switches to the highlighted pod.
		if m.state == StateIdle && m.view == ViewPods {
			return m.selectCurrentPod()
		}
		// In :threads view, Enter resumes the highlighted thread (session).
		if m.state == StateIdle && m.view == ViewThreads {
			return m.selectCurrentThread()
		}
		// In :sessions view, Enter resumes the highlighted session.
		if m.state == StateIdle && m.view == ViewSessions {
			return m.selectCurrentSession()
		}
		if m.state == StatePrompting {
			text := m.input.Value()
			m.input.SetValue("")
			return m.wizardSubmit(text)
		}
		if m.state == StateIdle && m.view == ViewThread {
			text := m.input.Value()
			if text == "" {
				return m, waitForSubMsg(m.sub)
			}
			m.input.SetValue("")
			return m.submit(text)
		}
	}

	// Global key shortcuts — available in Idle and Running states so
	// the user can change views, toggle viz/avatars, and compose the
	// next message while agents are working. Only message submission
	// (Enter) is blocked during Running.
	if (m.state == StateIdle || m.state == StateRunning) && msg.Type == tea.KeyRunes {
		switch string(msg.Runes) {
		case "v":
			// Toggle the pod-visualization panel.
			if m.view == ViewThread && m.input.Value() == "" {
				m.vizOpen = !m.vizOpen
				m = m.recalcViewport()
				m.viewport.GotoBottom()
				m.savePrefs()
				var tickCmd tea.Cmd
				if m.vizOpen {
					tickCmd = animTick()
				}
				return m, tea.Batch(waitForSubMsg(m.sub), tickCmd)
			}
		case "p":
			// Cycle avatar size: Small → Large → Off → Small …
			if m.view == ViewThread && m.input.Value() == "" {
				switch m.avatarSize {
				case AvatarSmall:
					m.avatarSize = AvatarLarge
				case AvatarLarge:
					m.avatarSize = AvatarOff
				default:
					m.avatarSize = AvatarSmall
				}
				m = m.recalcViewport()
				m.viewport.GotoBottom()
				m.savePrefs()
				labels := map[AvatarSize]string{AvatarOff: "off", AvatarSmall: "small", AvatarLarge: "large"}
				m.statusLine = "avatars: " + labels[m.avatarSize]
				return m, waitForSubMsg(m.sub)
			}
		case ":":
			if m.view != ViewThread || m.input.Value() == "" {
				m = m.openPalette()
				return m, waitForSubMsg(m.sub)
			}
		case "?":
			if m.view != ViewThread || m.input.Value() == "" {
				m.view = ViewHelp
				return m, waitForSubMsg(m.sub)
			}
		}
	}

	if m.state == StateIdle && len(m.pendingRequests) > 0 {
		switch msg.Type {
		case tea.KeyRunes:
			switch string(msg.Runes) {
			case "a":
				return m.onApprove()
			case "d":
				return m.onDeny()
			case "A":
				return m.onApproveAll()
			case "D":
				return m.onDenyAll()
			}
		}
	}

	// Scroll keys always go to the viewport (not the input field).
	switch msg.Type {
	case tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	if (m.state == StateIdle || m.state == StateRunning) && m.view == ViewThread {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	if m.state == StateIdle || m.state == StatePrompting {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// submit appends a human event to the local view (the real event
// lands in the log via the loop's HumanMessage kickoff) and launches
// the loop in a goroutine. If the input starts with "/", it is routed
// to the slash-command dispatcher instead.
func (m Model) submit(text string) (tea.Model, tea.Cmd) {
	if len(text) > 0 && text[0] == '/' {
		return m.dispatchSlashCommand(text)
	}
	if m.state == StateRunning {
		// ignore double-submits (e.g. from maybeResume firing while a
		// previous loop's goroutine is still draining into sub).
		return m, waitForSubMsg(m.sub)
	}
	if m.opts.StartLoop == nil {
		m.lastErr = fmt.Errorf("start-loop not configured")
		m.statusLine = "error: start-loop not configured"
		return m, waitForSubMsg(m.sub)
	}
	m.state = StateRunning
	m.statusLine = "running…"
	m.typingWho = ""
	m.typingTick = 0
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelLoop = cancel
	start := m.opts.StartLoop
	sub := m.sub
	go func() {
		result, err := start(ctx, text, func(e thread.Event) {
			sub <- EventMsg{Event: e}
		})
		sub <- LoopDoneMsg{Result: result, Err: err}
	}()
	return m, tea.Batch(waitForSubMsg(m.sub), animTick())
}

// onAnimTick advances the viz panel animation. Re-arms the tick while
// the panel is open; prunes fully-expired links to bound slice growth.
func (m Model) onAnimTick(msg animTickMsg) (tea.Model, tea.Cmd) {
	needTick := false

	// Advance typing spinner when running.
	if m.state == StateRunning {
		m.typingTick++
		if m.ready {
			m.viewport.SetContent(m.viewportContent(m.chatWidth()))
			m.autoScrollBottom()
		}
		needTick = true
	}

	// Prune expired viz links.
	if m.vizOpen {
		now := msg.t
		live := m.activeLinks[:0]
		for _, l := range m.activeLinks {
			if !l.expired(now) {
				live = append(live, l)
			}
		}
		m.activeLinks = live
		needTick = true
	}

	if !needTick {
		return m, nil
	}
	return m, animTick()
}

// onEvent appends an event to the transcript, records a viz link for
// the new speaker, and re-arms the subscription.
func (m Model) onEvent(msg EventMsg) (tea.Model, tea.Cmd) {
	e := msg.Event
	m.events = append(m.events, e)

	// Track who spoke for viz animations + typing indicator.
	switch e.Type {
	case "message":
		if e.From != "" && e.From != m.lastSpeaker {
			m.activeLinks = append(m.activeLinks, vizLink{
				from:    e.From,
				to:      m.lastSpeaker, // "" = human, or previous member
				startAt: time.Now(),
			})
		}
		if e.From != "" {
			m.lastSpeaker = e.From
		}
		// This speaker just responded — set typing to whoever they @mention next.
		if len(e.Mentions) > 0 {
			m.typingWho = e.Mentions[0]
		} else {
			m.typingWho = ""
		}
	case "human":
		if m.lastSpeaker != "" {
			m.activeLinks = append(m.activeLinks, vizLink{
				from:    "",
				to:      m.lastSpeaker,
				startAt: time.Now(),
			})
		}
		m.lastSpeaker = ""
		// Human just spoke — set typing to whoever they @mention.
		if len(e.Mentions) > 0 {
			m.typingWho = e.Mentions[0]
		} else {
			m.typingWho = ""
		}
	}

	// Keep the slice bounded; old links are pruned by onAnimTick when
	// viz is open, but we clip here too so the model stays lean.
	if len(m.activeLinks) > 32 {
		m.activeLinks = m.activeLinks[len(m.activeLinks)-32:]
	}

	if m.ready {
		m.viewport.SetContent(m.viewportContent(m.viewport.Width))
		m.autoScrollBottom()
	}
	return m, waitForSubMsg(m.sub)
}

// onBreakawayEvent handles a single event from a background breakaway.
// Updates the viz panel but does NOT add to the main transcript.
func (m Model) onBreakawayEvent(msg BreakawayEventMsg) (tea.Model, tea.Cmd) {
	e := msg.Event
	// Track as active breakaway if not already.
	if !m.hasBreakaway(msg.Members) {
		m.breakaways = append(m.breakaways, activeBreakaway{
			Members: msg.Members,
		})
	}
	// Create viz links for the breakaway conversation.
	if e.Type == thread.EventMessage && e.From != "" {
		for _, other := range msg.Members {
			if other != e.From {
				m.activeLinks = append(m.activeLinks, vizLink{
					from:    e.From,
					to:      other,
					startAt: time.Now(),
				})
			}
		}
	}

	// In debug mode, show breakaway events inline in the transcript.
	if m.debug && e.Type == thread.EventMessage {
		names := strings.Join(msg.Members, "+")
		m.events = append(m.events, thread.Event{
			Type: thread.EventSystem,
			Body: fmt.Sprintf("[breakaway:%s] [%s] %s", names, e.From, e.Body),
		})
		if m.ready {
			m.viewport.SetContent(m.viewportContent(m.chatWidth()))
			m.autoScrollBottom()
		}
	}
	return m, waitForSubMsg(m.breakawaySub)
}

// onBreakawayDone handles a completed breakaway — posts the summary
// to the main transcript.
func (m Model) onBreakawayDone(msg BreakawayDoneMsg) (tea.Model, tea.Cmd) {
	// Remove from active breakaways.
	var remaining []activeBreakaway
	for _, ba := range m.breakaways {
		if !sameMembers(ba.Members, msg.Members) {
			remaining = append(remaining, ba)
		}
	}
	m.breakaways = remaining

	// Post summary to transcript.
	if msg.Summary != "" {
		names := strings.Join(msg.Members, " & ")
		body := fmt.Sprintf("[breakaway] %s concluded: %s", names, msg.Summary)
		if m.debug && msg.LogPath != "" {
			body += fmt.Sprintf("\n  log: %s", msg.LogPath)
		}
		m.events = append(m.events, thread.Event{
			Type: thread.EventSystem,
			Body: body,
		})
		if m.ready {
			m.viewport.SetContent(m.viewportContent(m.chatWidth()))
			m.autoScrollBottom()
		}
	}
	return m, waitForSubMsg(m.breakawaySub)
}

func (m Model) hasBreakaway(members []string) bool {
	for _, ba := range m.breakaways {
		if sameMembers(ba.Members, members) {
			return true
		}
	}
	return false
}

func sameMembers(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// onLoopDone transitions back to Idle and summarizes the run.
// When the loop stopped for pending permissions, populates pendingRequests.
func (m Model) onLoopDone(msg LoopDoneMsg) (tea.Model, tea.Cmd) {
	m.state = StateIdle
	m.cancelLoop = nil
	m.typingWho = ""
	if msg.Err != nil {
		m.lastErr = msg.Err
		m.statusLine = fmt.Sprintf("error: %v", msg.Err)
	} else {
		m.lastStop = msg.Result.StopReason
		m.turnsRun = msg.Result.TurnsRun
		m.statusLine = fmt.Sprintf("stopped: %s (turns=%d)", msg.Result.StopReason, msg.Result.TurnsRun)
		if msg.Result.StopReason == orchestrator.LoopPendingPermission && m.opts.GetPending != nil {
			m.pendingRequests = m.opts.GetPending()
		}
	}
	return m, waitForSubMsg(m.sub)
}

// onApprove approves the oldest pending request.
func (m Model) onApprove() (tea.Model, tea.Cmd) {
	if len(m.pendingRequests) == 0 || m.opts.OnApprove == nil {
		return m, waitForSubMsg(m.sub)
	}
	req := m.pendingRequests[0]
	m.pendingRequests = m.pendingRequests[1:]
	if err := m.opts.OnApprove(req.ID); err != nil {
		m.lastErr = err
		m.statusLine = "error: " + err.Error()
		return m, waitForSubMsg(m.sub)
	}
	return m.maybeResume()
}

// onDeny denies the oldest pending request.
func (m Model) onDeny() (tea.Model, tea.Cmd) {
	if len(m.pendingRequests) == 0 || m.opts.OnDeny == nil {
		return m, waitForSubMsg(m.sub)
	}
	req := m.pendingRequests[0]
	m.pendingRequests = m.pendingRequests[1:]
	if err := m.opts.OnDeny(req.ID, ""); err != nil {
		m.lastErr = err
		m.statusLine = "error: " + err.Error()
		return m, waitForSubMsg(m.sub)
	}
	return m.maybeResume()
}

// onApproveAll approves all pending requests.
func (m Model) onApproveAll() (tea.Model, tea.Cmd) {
	if len(m.pendingRequests) == 0 || m.opts.OnApprove == nil {
		return m, waitForSubMsg(m.sub)
	}
	for _, req := range m.pendingRequests {
		if err := m.opts.OnApprove(req.ID); err != nil {
			m.lastErr = err
			m.statusLine = "error: " + err.Error()
			m.pendingRequests = nil
			return m, waitForSubMsg(m.sub)
		}
	}
	m.pendingRequests = nil
	return m.maybeResume()
}

// onDenyAll denies all pending requests.
func (m Model) onDenyAll() (tea.Model, tea.Cmd) {
	if len(m.pendingRequests) == 0 || m.opts.OnDeny == nil {
		return m, waitForSubMsg(m.sub)
	}
	for _, req := range m.pendingRequests {
		if err := m.opts.OnDeny(req.ID, ""); err != nil {
			m.lastErr = err
			m.statusLine = "error: " + err.Error()
			m.pendingRequests = nil
			return m, waitForSubMsg(m.sub)
		}
	}
	m.pendingRequests = nil
	return m.maybeResume()
}

// maybeResume re-kicks the loop with an empty message when all pending
// requests have been handled, so agents can continue automatically.
// Guards against re-entry while a loop is already running — without
// this, rapid-fire approve/deny presses (or programmatic sequences)
// can start a second goroutine that races with the first, drops
// messages, and leaks ctx cancellation handles.
func (m Model) maybeResume() (tea.Model, tea.Cmd) {
	if len(m.pendingRequests) > 0 {
		return m, waitForSubMsg(m.sub)
	}
	if m.state == StateRunning {
		return m, waitForSubMsg(m.sub)
	}
	return m.submit("")
}
