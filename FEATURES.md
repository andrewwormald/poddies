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

### A2. Delta-resume via session IDs
- Plumbing landed in this commit: `thread.Meta` sidecar stores
  per-member session IDs, Claude adapter passes `--resume`, loop
  persists + loads on every run.
- **Not yet**: render only the incremental events (since last turn
  from this member) instead of the full thread. Needs care around
  permission grants that rewrite state.
- Add tests with a mock adapter that asserts the prompt size shrinks
  after the first turn.

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

### P1. Ghost-text @mention autocomplete
- Logic landed (`internal/tui/autocomplete.go` + tests): detects `@xxx`
  prefix, computes suggestion, `applySuggestion` accepts it.
- **Not yet wired**: Rendering the suggestion as faint/0.5-opacity
  ghost text inline in the input. Needs a custom input renderer
  because `bubbles/textinput` owns its own View(). Tab key should
  accept the suggestion.

### P2. Modal wizard rendering
- Wizards currently replace the footer pane. User wants them rendered
  as a **centered bordered box** over a dimmed background (think
  Claude Code's setup prompts).
- Implementation: `lipgloss.Place` + `lipgloss.NormalBorder`; build
  the box contents (title · step N/M · question · choices · input ·
  hint), center over the thread transcript (dimmed).

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

### R1. `/resume` slash command
- Backend plumbing: session IDs persist in `<thread>.meta.toml`;
  Claude adapter consumes `PriorSessionID`. Adapter-side resume is
  live this commit.
- **Not yet**: in-TUI `/resume` slash command that re-opens the last
  active thread (or lets the user pick from a list). For now, just
  re-running `poddies` picks up the `default` thread with prior
  session IDs intact — the token-saving side effect is real even
  without the command.

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
