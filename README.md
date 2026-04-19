# poddies

A terminal UI for running a "pod" of AI agents as a shared,
Slack-thread-style conversation. The human user is the pod lead /
CEO; agents (Claude Code, Gemini CLI, etc.) are spawned as
subprocesses, address each other by `@mention`, and respect a
configurable hierarchy of roles.

poddies is **TUI-first** — think k9s for AI-agent pods. Launch the
binary, the interface opens; pod / member / thread management all
happens inside. There's a hidden scripting surface underneath (for CI,
automation, test harnesses); see the bottom of this file.

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

## Run it

```sh
poddies
```

That's it. On a fresh machine:

1. A local `./poddies/` root is created.
2. A default pod is scaffolded.
3. The onboarding wizard fires so you can add your first member (name,
   title, adapter, model, effort, persona — all via numbered choices
   or free-text).
4. You land in the chat view. Type a message, Enter to send.

## Inside the TUI

**Views** (k9s-style — press `:` then type a command):

- `:thread` (default) — the chat conversation
- `:members` — pod member roster
- `:pods` — all pods under the root
- `:threads` — all threads under the current pod
- `:perms` — pending permission requests
- `:doctor` — adapter + root health check
- `:help` — keybindings + command list
- `:quit` — exit

**Global keybindings:**

- `:` open the command palette
- `?` open the help view
- `Esc` back to `:thread` (or cancel a wizard)
- `Ctrl-C` exit (cancels any in-flight loop)

**In the chat view** (slash commands):

- `/add` run the member-add wizard
- `/remove` pick a member and remove
- `/edit` edit a member's field (title / adapter / model / effort / persona)
- `/export` dump the pod as a portable TOML bundle into the transcript
- `/help` · `/quit`

**When a loop halts with pending permission requests**, the chat view
shows a yellow pane listing them with keybindings:

- `a` approve the oldest pending request
- `d` deny the oldest
- `A` / `D` approve / deny all pending at once

## Chief of staff

Each pod can enable a built-in facilitator agent via
`[chief_of_staff]` in `pod.toml`:

```toml
[chief_of_staff]
enabled = true
name = "sam"
adapter = "claude"
model = "claude-haiku-4-5"
triggers = ["milestone", "unresolved_routing", "gray_area"]
```

- `milestone` fires every N member turns (default 3) with a summary.
- `unresolved_routing` fires once per run when routing halts, giving
  the facilitator a chance to propose a next speaker.
- `gray_area` fires when the human posts a message with no `@mention`.
  The facilitator either routes via `@name` to a member who owns the
  request, or answers it directly when no one does. Explicit human
  `@mentions` suppress this — the facilitator stays out of the way
  when you've told it who you want.

The chief-of-staff is addressable via `@<name>` from members and the
human when enabled — e.g. `@sam what do you think?`.

## Scripting surface (hidden)

The same CRUD the TUI drives is available as subcommands for
automation. They're hidden from `--help` by default; pass
`--help-scripting` to see them:

```sh
poddies --help-scripting
```

Everything you'd expect:

```sh
poddies init --local
poddies pod create demo
poddies pod export demo --out bundle.toml
poddies pod import bundle.toml --as team-beta --overwrite
poddies member add --pod demo --name alice --title "Staff" \
  --adapter claude --model claude-opus-4-7 --effort high
poddies member edit --pod demo --name alice --effort medium
poddies member remove --pod demo --name alice
poddies thread list --pod demo
poddies thread show --pod demo default
poddies thread show --pod demo default --json
poddies thread resume default --message "follow-up"
poddies thread permissions default
poddies thread approve default <request-id>
poddies thread deny default <request-id> --reason "..."
poddies run --pod demo --message "@alice ship it"
```

These are useful for CI pipelines and scripted bootstraps. The TUI
remains the intended day-to-day surface.

## Architecture

- **Config** (`internal/config`): `pod.toml`, per-member TOML files,
  portable bundle format, strict unknown-field rejection,
  slug/reserved-name validation.
- **Thread log** (`internal/thread`): append-only JSONL event log with
  forward-compatible unknown event types, `@mention` parser,
  deterministic test hooks, permission bookkeeping helpers.
- **Adapter interface** (`internal/adapter`): one `Invoke(ctx, req)`
  per turn; backend picks how to render the thread into its prompt
  format. Built-in adapters: `claude` (one-shot + streaming),
  `gemini`, `mock` (tests + demos). Shared subprocess plumbing lives
  in `internal/adapter/cliproc`.
- **Orchestrator** (`internal/orchestrator`): pure `Route`
  next-speaker policy (`@mention` + human-routes-to-lead + halt);
  `Loop` ties it together with milestone / unresolved_routing /
  gray_area triggers for the chief-of-staff facilitator.
- **TUI** (`internal/tui`): bubbletea app with multi-view command
  palette, wizard abstraction for in-chat CRUD, streaming event
  subscription via a self-re-arming `tea.Cmd`.
- **CLI** (`internal/cli`): cobra wiring for everything above, with
  injectable I/O and adapter lookup so the full stack is testable
  without touching the user's filesystem or real CLIs.

## Testing

Every function has direct unit tests. Golden-file tests cover config
round-trip, JSONL log format, renderer output, and full E2E scenarios.
Run the suite:

```sh
go test ./... -race -count=1
```

## License

MIT — see [LICENSE](./LICENSE).
