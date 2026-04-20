# Pending features

Roadmap of work flagged but not yet landed. Grouped by theme; ordered
roughly by user impact.

## Token efficiency (new)

The conversation-through-a-thread model re-sends a lot of content on
every turn. Two work streams:

### A1. Audit for wasted repetition
- Measure: log per-turn input-token counts for representative runs;
  surface which chunks of the prompt are duplicated across turns
  (persona re-emitted, full transcript re-rendered, roster repeated).
- Quantify: baseline vs. post-refactor tokens/turn on a 5-turn mock
  conversation.
- Candidates to fix:
  - Prompt caching: Claude adapter already captures `cache_read_input_tokens`
    — verify caching actually fires for the stable prefix (system
    prompt + roster). Claude Code has `--cache-prompt` flags; may
    need to pass them. Similar for Gemini.
  - Delta-only resume: once session IDs persist (A2), stop
    re-rendering the whole thread into each Claude invocation and
    send only the delta since last turn.
  - Roster de-dupe: render the member roster once in the system
    prompt; don't also enumerate it in the user-side CTA.
  - System prompt minification: trim boilerplate; move conventions to
    the CoS prompt only.

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

### A3. Conciseness prompting
- Append a "be concise, stay in your persona, no preamble, no
  ceremonial acknowledgements" directive to every member system
  prompt. Members that habitually open with "Great question!" or
  repeat the user's ask back waste output tokens.
- Configurable per-pod: a `[prompt_style]` section with fields like
  `verbosity = "concise"` (default) vs `verbosity = "verbose"` for
  debug. Default to concise.
- CoS gets the same treatment — facilitator answers are summaries,
  not essays.
- Unit test: render the Claude system prompt, assert the conciseness
  directive is present; snapshot-compare against a stable fixture.

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

### P3. Per-user colors in transcript
- Palette landed (`internal/tui/colors.go`): deterministic hash of
  name → palette color; reserved colors for `human` and the CoS.
- **Not yet wired**: `renderEvent` in `view.go` still uses an
  unstyled `[name]` prefix. Swap in `styledName(e.From, cosName)`.

### P4. Text wrapping
- Helper landed (`internal/tui/wrap.go`): `wrapText(s, width)`
  word-wraps with hard-break fallback.
- **Not yet wired**: transcript event bodies and wizard questions
  still overflow long lines. Wrap at `m.viewport.Width` and
  modal-box width respectively.

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

### R2. Active thread tracking
- Cross-run: which thread was the user in last session? Store in a
  pod-level state file so `poddies` re-opens where they left off.

### R3. `/stats` view
- Full-screen view showing per-member + per-thread token burn,
  costs, turn counts over time. The footer counter is the
  at-a-glance version; `/stats` is the drill-down.

## Deferred

- **Pod / thread switching inside TUI**: right now switching pods or
  threads means quitting and relaunching with `POD=...` env or the
  scripting CLI. Plumb through a Backend interface so `:pods` and
  `:threads` views can switch in place.
- **CoS name @-mention UI affordance**: when typing `@` the CoS name
  should appear in the autocomplete list alongside members (logic
  supports this; rendering follows P1).
- **Agent tool-use event types**: current thread event schema is
  message / human / system / permission_{request,grant,deny}. When
  Claude Code internally runs tools (bash, edit), those don't surface
  in the poddies log. Worth a new event type + adapter hook so tool
  activity is visible.
- **Cross-machine resume**: `.meta.toml` is local. Adapters' session
  IDs are server-side; resume works cross-machine as long as the
  poddies root is synced (e.g. in Dropbox / git). Document this.

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
