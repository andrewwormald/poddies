# poddies

A CLI for running a "pod" of AI agents as a shared, Slack-thread-style
conversation. The human user is the pod lead / CEO; agents (Claude Code,
Gemini CLI, etc.) are spawned as subprocesses, address each other by
`@mention`, and respect a configurable hierarchy of roles.

Status: early development.

## Install

```sh
go install github.com/andrewwormald/poddies/cmd/poddies@latest
```

Verify:

```sh
poddies --version
poddies doctor
```

`doctor` checks for `claude` and `gemini` on PATH and verifies your
poddies root is writable. Adapters are optional — a missing CLI is a
warning, not an error, so you can use only the backends you have.

## Quickstart

```sh
# 1. Create a local poddies root under the current directory.
poddies init --local

# 2. Create a pod.
poddies pod create demo

# 3. Add members (agents). The adapter must be one of claude, gemini, mock.
poddies member add \
  --pod demo --name alice --title "Staff Engineer" \
  --adapter claude --model claude-opus-4-7 --effort high \
  --persona "Pragmatic, terse. Pushes back on over-engineering."

poddies member add \
  --pod demo --name bob --title "Senior Engineer" \
  --adapter gemini --model gemini-2.5-pro --effort medium

# 4. (Optional) Set alice as the lead so the pod routes to her when the
# human doesn't @mention anyone.
# Edit poddies/pods/demo/pod.toml and set `lead = "alice"`.

# 5. Run the pod. The conversation loops across members via @mention
# routing until quiescence or --max-turns is hit.
poddies run --pod demo --message "investigate the auth bug"

# Same thing but with the bubbletea TUI:
poddies run --pod demo --tui

# 6. Inspect threads.
poddies thread list
poddies thread show default
poddies thread resume default --message "any update?"
```

## Architecture

- **Config** (`internal/config`): `pod.toml`, per-member TOML files, strict
  unknown-field rejection, slug/reserved-name validation.
- **Thread log** (`internal/thread`): append-only JSONL event log with
  forward-compatible unknown event types, @mention parser, deterministic
  test hooks.
- **Adapter interface** (`internal/adapter`): one `Invoke(ctx, req)` per
  turn; backend picks how to render the thread into its prompt format.
  Built-in adapters: `claude`, `gemini`, `mock` (tests). Shared subprocess
  plumbing lives in `internal/adapter/cliproc`.
- **Orchestrator** (`internal/orchestrator`): pure `Route` next-speaker
  policy (@mention + human-routes-to-lead + halt); `Loop` ties it
  together with `milestone` + `unresolved_routing` triggers for the
  chief-of-staff facilitator.
- **TUI** (`internal/tui`): bubbletea three-pane view (header, thread,
  input) that streams loop events via a channel and supports repeated
  kickoffs in one session.
- **CLI** (`internal/cli`): cobra wiring for everything above, with
  injectable I/O and adapter lookup so the full stack is testable
  without touching the user's filesystem or real CLIs.

## Chief of staff

Each pod can enable a built-in facilitator agent. See
`pod.toml` `[chief_of_staff]`:

```toml
[chief_of_staff]
enabled = true
name = "sam"            # visible as [sam] in the thread
adapter = "claude"
model = "claude-haiku-4-5"
triggers = ["milestone", "unresolved_routing"]
```

- `milestone` fires every N member turns (default 3) and posts a
  summary.
- `unresolved_routing` fires once per `poddies run` when routing halts,
  giving the facilitator a single chance to propose a next speaker.
- `gray_area` fires whenever the human posts a message with no
  `@mention`. The facilitator then either routes via `@name` to a
  member who owns the request, or answers it directly when no one does.
  Explicit human `@mentions` suppress this trigger — the facilitator
  stays out of the way when you've told it who you want.

The chief-of-staff is addressable via `@<name>` from members and the
human when enabled — e.g. `@sam what do you think?` routes to the
facilitator.

## Testing

Every function has direct unit tests. Golden-file tests cover config
round-trip, JSONL log format, renderer output, and full E2E scenarios.
Run the suite:

```sh
go test ./... -race -count=1
```

## License

MIT — see [LICENSE](./LICENSE).
