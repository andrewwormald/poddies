# AGENTS.md

Context for AI agents (Claude Code, Codex, Cursor, etc.) working on
this repo. Read this **before** making changes. The ground rules here
were set early by the maintainer and are not optional.

---

## What this repo is

`poddies` is a terminal-first CLI that orchestrates a pod of AI agents
as a shared, Slack-thread-style conversation. Think `k9s` for
agent-team orchestration. Members (Claude Code, Gemini CLI, mock) are
spawned as subprocesses and address each other by `@mention`. A
built-in "chief-of-staff" facilitator fills gray-area ownership.

See [README.md](./README.md) for the user-facing pitch and
[FEATURES.md](./FEATURES.md) for the roadmap.

---

## Ground rules

These are non-negotiable. They were established at the start of the
project and exist because the maintainer has been burned by the
absence of each one.

### 1. Testing discipline: TDD, every function, E2E with mock

- **Every exported function gets a direct unit test**, in the same
  commit as the function. "Coverage exists somewhere else in the
  package" is not sufficient — reviewer should be able to grep the
  function name and find its test.
- **Write tests first.** Enumerate edge cases (happy path, error
  paths, boundary values, concurrency, partial failure, unusual
  inputs, forward-compat) before writing implementation. That's the
  step that catches design flaws.
- **Spend more time on tests than on code.** If the test file is
  shorter than the implementation file for non-trivial code, add more
  cases. Good code follows from good tests.
- **Golden-file tests for structured output.** Anything that produces
  a deterministic serialized artifact (TOML configs, JSONL event
  logs, render output) gets a golden fixture in `testdata/` + a
  normalizing diff harness with an `-update` flag.
- **End-to-end tests use the `mock` adapter.** It's in-process,
  deterministic, no auth, no network. Never write an E2E that shells
  out to a real `claude` / `gemini` binary — that belongs in a
  build-tagged integration test that's skipped by default.
- `go test ./... -race -count=1` must be green before any commit. The
  race flag is load-bearing: we have goroutines in the TUI and
  concurrent appends to the thread log, and dropping `-race` has
  masked real bugs in this repo before.

### 2. Push back when the framing is off

Do not rubber-stamp. If the maintainer's request would introduce a
regression, conflict with a prior decision, or miss an obvious
tradeoff, say so. "I'd push back on X because Y" with a recommended
alternative is always the right move over silent compliance.
Examples from the project history:

- When asked to build a "System Operator" that would route all actions
  through a central executor, the right move was to flag the token
  cost (N+1 LLM calls per turn) and ask whether the user meant a
  deterministic policy engine or another LLM. User revised the ask
  after the pushback.
- When asked to rename to `wrangler`, the right move was to flag the
  Cloudflare Wrangler collision and recommend against it. User kept
  `poddies`.

### 3. Code style: WHY, not WHAT

- Default to writing **no comments**. Well-named identifiers carry
  intent; tests carry the contract.
- When you do write a comment, explain **why** — the non-obvious
  constraint, the invariant, the past bug the workaround prevents,
  the design fork chosen.
- Never comment **what** the code does. Never reference the current
  task or PR. Never add "this is used by X" or "added for Y" — those
  belong in commit messages and rot in the code.
- No helper abstractions for hypothetical future needs. Three similar
  lines beats a premature abstraction.

### 4. Commits: Co-Authored-By + why-focused messages

- Every commit is co-authored:
  ```
  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  ```
- Commit message body: focused on **why**, not a diff recap. The diff
  is self-evident; the motivation isn't.
- Reference bug IDs from QA runs (e.g. "B1 fix") when relevant so a
  future reader can trace from QA note → commit.
- Never `git push` without explicit user approval for that push.
- Never `--no-verify`, `--no-gpg-sign`, `--amend` without explicit
  ask.

---

## Architecture principles

Each principle earned its place. Don't rearrange them without
reading the commit history that got them there.

### Subprocess adapters, not direct API

Agents run as their native CLIs (`claude`, `gemini`). Reasons:

- Users already authenticate once via the CLI (no key management in
  poddies).
- Agents inherit their CLI's tool ecosystem (MCP, slash commands,
  skills).
- Cost attribution is the user's existing plan.
- We track CLI flag stability, not SDK churn.

Never add a direct-API adapter without the maintainer's sign-off —
it's a design-level choice, not an implementation detail.

### Thread as single source of truth

The per-session `thread.jsonl` is append-only, forward-compatible
(unknown event types are preserved), and the canonical conversation
state. Adapters re-render it into their own prompt format per turn.
Don't add out-of-band state without extending the event schema.

### Pure `Route` function

`orchestrator.Route(events, members, lead, cosName)` is deliberately
pure — no I/O, no time, no randomness. This is why it's trivial to
table-test every routing edge case. Keep it that way.

### Adapter contract stability

`adapter.InvokeRequest` and `InvokeResponse` are a stable contract.
Adding optional fields is OK; renaming or removing fields is a big
deal that ripples through every adapter, test, and mock.

### Session-per-launch, not shared default thread

Each `poddies` invocation starts a fresh session to avoid the "noise
trap" — long-lived shared threads accumulate off-topic chatter that
every future turn pays to re-process. `/resume` brings back priors.
30-day auto-cleanup. Do not add a "default shared thread" mode.

### TUI is the product; CLI is for scripts

`poddies` with no args launches the TUI. Subcommands (`init`, `pod`,
`member`, `run`, `thread`) are hidden from `--help` by default and
exist for CI / automation only. Any new user-facing capability goes
in the TUI first.

---

## Package map

```
cmd/poddies/            binary entrypoint; blank-imports register adapters
internal/
  adapter/              Adapter interface + registry
    claude/             claude CLI adapter (one-shot + streaming)
    gemini/             gemini CLI adapter
    mock/               deterministic mock for tests + Auto mode for demos
    cliproc/            shared subprocess runner (Runner, ExecRunner)
  config/               pod.toml, member TOMLs, bundle format, validation
  thread/               JSONL event log, permission helpers, meta sidecar
  orchestrator/         Route (pure) + Loop (CoS triggers, permissions)
  session/              per-launch sessions, cleanup, legacy migration
  tui/                  bubbletea Model/Update/View, palette, wizards, views
  cli/                  cobra wiring; hidden scripting commands
e2e/                    end-to-end scenarios using mock adapter only
```

Files whose names start with `view_` / `slash` / `wizard` / `palette`
in `internal/tui/` exist to keep the Update/View functions short.
Split the next TUI feature the same way — don't grow `update.go`.

---

## Common patterns to follow

### Adding a new adapter

1. Create `internal/adapter/<name>/` with an `Adapter` struct, a
   `New()` constructor, `Name()`, and `Invoke()`.
2. Use `cliproc.Runner` / `cliproc.StreamingRunner` for the subprocess
   plumbing — don't reimplement.
3. Handle `req.PriorSessionID` for resume support if the CLI supports
   it (`--resume` / `--session-id`).
4. Populate `resp.Usage` (token counts + cost) if the CLI reports
   them; zero is fine otherwise.
5. Populate `resp.SessionID` so session-level resume works.
6. Register via `init()` calling `adapter.Register(New())`.
7. Blank-import the package from `cmd/poddies/main.go`.
8. Add `adapter.<Name>` to `config.ValidAdapters`.
9. Tests with a fake `cliproc.Runner`; cover happy path, preamble
   tolerance, stderr wrapping, context cancel, empty output, CoS
   role, model flag construction, streaming path.

### Adding a new TUI view

1. New token in `internal/tui/palette.go` `paletteCommands` map.
2. New `View` constant in `internal/tui/model.go`.
3. New `render<Name>View` method in `internal/tui/views.go`.
4. Add dispatch in `renderActiveView`.
5. If the view needs new Options callbacks, add them to
   `tui.Options` (additive) and wire in `internal/cli/launch.go`.
6. Unit tests: exercise Update for state transitions; View for
   expected content substrings (not full-string matches — terminfo
   escapes vary).

### Adding a new slash command

1. Case in `dispatchSlashCommand` in `internal/tui/slash.go`.
2. Handler method on `Model`.
3. Unit tests in `internal/tui/slash_test.go` — cover activation,
   happy path, missing callback error, status line.
4. Update the `/help` line in the dispatcher.
5. Document in README's slash-commands table.

### Adding a new CoS trigger

1. Constant in `internal/config/enums.go`, add to `ValidTriggers`.
2. Firing logic in `internal/orchestrator/loop.go` — remember to
   share the one-rescue-per-run budget with other CoS triggers
   unless the trigger is genuinely orthogonal (milestone is
   orthogonal; gray_area and unresolved_routing share).
3. Tests covering fires-when-configured, does-not-fire-when-absent,
   does-not-double-fire, interaction with other triggers.
4. Update README's CoS section + `pod.toml` example.

---

## Pitfalls I've hit and you should avoid

- **Test hang on `poddies` with no args.** The binary launches the
  TUI on stdin/stdout if they're TTYs. In tests they're not, so the
  TTY detection in `internal/cli/app.go` falls back to printing help.
  If you ever remove that fallback, `TestBinary_*` will hang for 10+
  minutes.
- **`listMemberNames` used to return duplicate slices.** QA caught it.
  Don't reintroduce dead return values.
- **Empty-body events poison routing.** `invokeChiefOfStaff` guards
  against appending a `message` event with `Body == ""`. A stray
  empty event becomes the "last real turn" for `Route`, which has no
  mentions, so the loop halts forever. If you add a new adapter
  invocation site, keep the guard.
- **Shared rescue budget.** `gray_area` + `unresolved_routing` +
  `@<cos>`-mention all consume the same `cosRescued` flag. Forgetting
  to set it lets the CoS fire twice on one halt.
- **`FirstMember` must be consumed after first use.** Bug: without
  consumption, the CoS-detour path (which doesn't bump `turnsRun`)
  re-fires FirstMember indefinitely. Hit the `SafetyMaxTurns` cap of
  1000 before I fixed it.
- **Don't commit `pods/` or `.poddies/`.** They're user runtime
  state. `.gitignore` excludes them; don't second-guess it.
- **Don't add dependencies casually.** We use `go-toml/v2`, `cobra`,
  `bubbletea` + `bubbles` + `lipgloss`, and stdlib. A new dep needs
  explicit sign-off — it becomes a supply-chain and vendoring
  concern.
- **Don't skip the `race` flag.** See above.

---

## When in doubt

- Read the commit log for the file you're touching. Every design
  decision has a commit that explains it.
- Look for a matching test; if none exists, that's the first thing
  to write.
- Ask. "I see pattern X, is this intentional?" is better than a PR
  that undoes a subtle invariant.
