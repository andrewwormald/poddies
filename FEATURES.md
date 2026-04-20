# Pending features

Roadmap of work flagged but not yet landed. Grouped by theme; ordered
roughly by user impact.

## Token efficiency (new)

The conversation-through-a-thread model re-sends a lot of content on
every turn. Two work streams:

### A1. Audit for wasted repetition (partially done)
- `poddies dump-prompt <pod> <member>` lands in `internal/cli/run.go`
  — renders the full system prompt so repetition is visible without a
  live run. Primary audit tool.
- Delta-only resume shipped as A2 ✓.
- Prompt caching: Claude Code CLI manages cache headers internally;
  `cache_read_input_tokens` captured in `ClaudeUsage` so hit rate is
  observable. No further wiring needed from poddies side.
- Roster de-dupe: investigated — roster appears only in system prompt,
  not in the user-side CTA. No duplication.
- Still open:
  - System prompt minification: trim boilerplate; move conventions to
    CoS prompt only. Low-priority; measure first with dump-prompt.

### A2. Delta-resume via session IDs ✓
- `thread.Meta` gains `LastEventIdx map[string]int` (TOML-persisted).
  Tracks the exclusive-end event index at the time each member/CoS
  last responded — the start of their next delta.
- Orchestrator loop: before each member/CoS invocation, if
  `LastSessionIDs[name] != ""`, passes `existing[LastEventIdx[name]:]`
  instead of the full thread. Falls back to full thread when no prior
  session (first run, or adapter doesn't return a SessionID).
- After all emits for a turn, `LastEventIdx[name] = len(existing)` is
  persisted so the next run picks up from the right place.
- `mock.Call` gains `PriorSessionID` field so tests can assert the
  session ID is threaded through correctly.
- 2 orchestrator tests: delta shrinks thread on run 2 (with session ID);
  full thread sent when no session ID (Gemini plain-stdout compat).
- 2 thread/meta tests: LastEventIdx round-trips through TOML; LoadMeta
  initialises the map when the sidecar is missing.

### A3. Conciseness prompting ✓
- Conciseness directive appended to every member + CoS system prompt
  in both Claude and Gemini adapters (`render.go` in each).
- `internal/adapter/claude/conciseness_test.go` and
  `internal/adapter/gemini/conciseness_test.go` assert the directive
  is present; golden snapshots (`testdata/golden/render_full.txt`)
  capture the full rendered prompt.
- `poddies dump-prompt` CLI command (`internal/cli/run.go`) surfaces
  the rendered prompt for a given pod member — primary A1 audit tool.

## TUI polish (carried over)

### P1. Ghost-text @mention autocomplete ✓
- Logic: `internal/tui/autocomplete.go` — detects `@xxx` prefix,
  computes suggestion, `applySuggestion` accepts it.
- Rendering: `renderInputLine()` in `view.go` appends the ghost suffix
  (faint style) after `m.input.View()`, which places it right after
  the cursor. No custom renderer needed — the textinput cursor sits
  naturally between typed text and ghost.
- Tab key: intercepted in `onKey` before the input update path; calls
  `applySuggestion` when a suggestion is active, otherwise falls through.

### P2. Modal wizard rendering ✓
- `renderWizardModal()` in `view.go` builds a bordered box
  (lipgloss.NormalBorder, header-blue border foreground) with inner
  content: title · step N/M · question · choices · input · hint.
- `renderThreadView()` now `return`s the modal directly when
  StatePrompting, bypassing the normal header+body+footer layout.
- `lipgloss.Place(width, height, Center, Center, box)` centers the box
  on the full terminal — no character-level overlay needed.

### P3. Per-user colors in transcript ✓
- `internal/tui/colors.go`: deterministic hash of name → palette color;
  reserved colors for `human` and the CoS.
- `renderEvent` in `view.go` uses `styledName(e.From, cosName)` to
  colour speaker prefixes; human events use `styledName("human", cosName)`.

### P4. Text wrapping ✓
- `internal/tui/wrap.go`: `wrapText(s, width)` word-wraps with
  hard-break fallback.
- `renderEvent` wraps event bodies at `bodyWidth` (viewport width
  minus name prefix); `renderWizardModal()` wraps wizard questions
  at the inner box width.

## Resume UX (partially done)

### R1. `/resume` slash command ✓
- `/resume` with no arg renders a numbered session list as a system
  event in the transcript; user picks with `/resume <n>` or
  `/resume <id-prefix>`.
- `/resume <n>` resolves by 1-based list position. `/resume <id>`
  does ID or prefix match. Both paths call `OnResumeSession` and
  `tea.Quit`; the launch wrapper restarts bound to that session.
- `doResume()` helper deduplicates the quit/callback path.
- 7 new unit tests cover: not-wired, no sessions, list display
  (numbered), pick by number, pick by ID, out-of-range, bad ID.

### R2. Active thread tracking ✓
- `session.SaveLastSession(root, pod, id)` / `LoadLastSession` write
  and read `<root>/state.toml` (`[last_session]` table keyed by pod
  name). Atomic tmp+rename writes; missing file returns `("", nil)`.
- `launchTUI` in `launch.go`: on first iteration, calls
  `LoadLastSession` and tries `session.Find`; falls back silently to a
  fresh session if the record is stale or the state file is absent.
  After every TUI close, calls `SaveLastSession` so the next launch
  picks up where the user left off.
- 5 session unit tests: missing → empty, round-trip, update, multi-pod,
  StatePath location.

### R3. `/stats` view ✓
- `/stats` slash command and `:stats` palette entry switch to `ViewStats`.
- `renderStatsView()` in `views.go`: thread totals block (input tokens,
  output tokens, cost USD, turn count) from `OnUsageSnapshot`; per-member
  message counts and human-message count derived from in-session events.
- Graceful when `OnUsageSnapshot` is nil — shows a note instead of
  crashing.
- 5 unit tests in `stats_test.go` cover: view switch, totals rendering,
  member-name rendering, human message count, not-wired fallback.

## Deferred (now shipped)

### Pod / thread switching inside TUI ✓
- `:pods` and `:threads` views gain cursor navigation (↑↓ arrow keys)
  and Enter to switch. Cursor state lives in `Model.cursorPos`; reset
  to 0 on each view change.
- `:pods` Enter: `selectCurrentPod()` → `doSwitchPod(name)` records
  `switchPodTarget` and quits. `launchTUI` detects the pod name,
  resets `pod` and resumes from the last-active session for the new pod.
- `:threads` Enter: `selectCurrentThread()` → `doResume(sessionID)` —
  reuses the existing resume machinery; no new callbacks needed.
- `launchTUIWithSession` returns a `launchResult` struct instead of a
  plain string, distinguishing resume vs. pod-switch vs. quit.
- `Options.OnSwitchPod func(name string)` wired from `launchTUIWithSession`.
- 12 unit tests in `switch_test.go`.

### CoS name @-mention UI affordance ✓
- Already fully wired in the prior P1 commit.
  `findMentionSuggestion` / `applySuggestion` both accept `cosName`
  and call `mentionCandidates(members, cosName)` — CoS appears in the
  autocomplete list alongside regular members. `view.go` and `update.go`
  both pass `m.opts.CoSName`.

### Agent tool-use event types ✓
- `EventToolUse EventType = "tool_use"` added to `thread/event.go`;
  `Action` = tool name, `Body` = input summary, `From` = member name.
- Claude streaming adapter captures `tool_use` content blocks from
  `assistant` messages; truncates input to 200 bytes + "…".
- `InvokeResponse.ToolCalls []ToolCall` carries them to the orchestrator.
- Orchestrator emits `EventToolUse` events before `EventMessage` for
  both member and CoS turns.
- TUI renders tool-use events as `[tool:name] input` in metaStyle.
- 4 streaming tests + 2 event validation tests.

### Cross-machine resume ✓
- Documented in `README.md` (Sessions and /resume → Cross-machine
  resume section). Thread logs + `.meta.toml` travel with the `.poddies/`
  sync; Claude server-side sessions are addressed by ID and work from
  any machine.

## Already shipped recently (for reference)

- Default-to-TUI (`poddies` with no args), hidden scripting CLI
- Command palette (`:` k9s-style), multi-view architecture
- `gray_area` trigger + `@<cos>` addressability
- Permission approve/deny keybindings
- Pod export/import with TOML bundle
- CoS identity / system prompt fix
- Claude streaming path via `OnToken`
- Bundle overwrite cleans stale members
- FirstMember consume-after-use (prevents unbounded loop with CoS
  name collision)
