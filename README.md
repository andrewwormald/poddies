# poddies

A CLI for running a "pod" of AI agents as a shared, Slack-thread-style conversation.
The human user acts as pod lead / CEO; agents (Claude Code, Gemini CLI, etc.) are
spawned as subprocesses, address each other by `@mention`, and respect a configurable
hierarchy of roles.

Status: early development. See milestones in the repo issues / project.

## License

MIT — see [LICENSE](./LICENSE).
