// Package claude is the adapter that drives the `claude` CLI (Claude Code)
// as a subprocess in one-shot JSON mode. One process per turn; all
// conversation state lives in poddies' thread log and is re-rendered
// into each invocation's prompt.
package claude

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

// DefaultBinary is the executable name resolved via PATH when the
// adapter is used without an explicit override.
const DefaultBinary = "claude"

// Adapter is the Claude Code CLI adapter.
type Adapter struct {
	Binary string // overrideable for tests or sidecar installs
	Runner Runner // defaults to ExecRunner
	Roster RosterFn
}

// RosterFn returns the full member list for a pod. The adapter calls
// this to populate the system prompt with the member roster. Tests
// inject a deterministic roster; production wires it to a config loader
// in the orchestrator.
type RosterFn func(pod config.Pod) ([]config.Member, error)

// New constructs an Adapter with production defaults.
func New() *Adapter {
	return &Adapter{
		Binary: DefaultBinary,
		Runner: NewExecRunner(),
		Roster: func(config.Pod) ([]config.Member, error) { return nil, nil },
	}
}

// Name implements adapter.Adapter.
func (a *Adapter) Name() string { return string(config.AdapterClaude) }

// ClaudeResult mirrors the JSON object emitted by
// `claude -p ... --output-format json`.
type ClaudeResult struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
	IsError   bool   `json:"is_error"`
	NumTurns  int    `json:"num_turns"`
	DurationMs int    `json:"duration_ms"`
}

// Invoke implements adapter.Adapter. It renders the thread into a
// claude-flavored prompt, runs one shot, parses the JSON result, and
// returns the text.
func (a *Adapter) Invoke(ctx context.Context, req adapter.InvokeRequest) (adapter.InvokeResponse, error) {
	if err := adapter.ValidateRequest(req); err != nil {
		return adapter.InvokeResponse{}, err
	}
	if a.Runner == nil {
		return adapter.InvokeResponse{}, fmt.Errorf("claude: runner not configured")
	}

	roster, _ := a.Roster(req.Pod) // best-effort; renderer handles nil

	model := req.Member.Model
	if req.Role == adapter.RoleChiefOfStaff {
		model = req.ChiefOfStaff.Model
	}
	if model == "" {
		return adapter.InvokeResponse{}, fmt.Errorf("claude: model must be set on member or chief_of_staff")
	}

	systemPrompt := RenderSystemPrompt(req.Member, req.Pod, roster)
	userPrompt := RenderUserPrompt(req.Member, req.Thread)

	args := BuildArgs(model, systemPrompt)

	stdout, stderr, err := a.Runner.Run(ctx, a.Binary, args, []byte(userPrompt))
	if err != nil {
		return adapter.InvokeResponse{}, fmt.Errorf("claude: run failed: %w (stderr: %s)", err, truncate(stderr, 512))
	}

	var res ClaudeResult
	if err := json.Unmarshal(stdout, &res); err != nil {
		return adapter.InvokeResponse{}, fmt.Errorf("claude: parse output: %w (raw: %s)", err, truncate(stdout, 512))
	}
	if res.IsError {
		return adapter.InvokeResponse{}, fmt.Errorf("claude: returned error (%s): %s", res.Subtype, res.Result)
	}

	return adapter.InvokeResponse{
		Body:       res.Result,
		Mentions:   thread.ParseMentions(res.Result),
		StopReason: adapter.StopDone,
	}, nil
}

// BuildArgs assembles the argv passed to `claude`. Exported so tests
// can assert the exact flag list without running anything.
//
// Note: the user prompt is sent via stdin (not -p) because -p's argv
// size is limited and a large thread transcript would blow it out.
// `claude -p -` reads the prompt from stdin.
func BuildArgs(model, systemPrompt string) []string {
	args := []string{
		"-p", "-", // read user prompt from stdin
		"--output-format", "json",
		"--model", model,
	}
	if systemPrompt != "" {
		args = append(args, "--append-system-prompt", systemPrompt)
	}
	return args
}

// truncate keeps the first n bytes of b, useful for error messages.
func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "...(truncated)"
}

// init auto-registers the production adapter in the global registry,
// but only when the binary path has not been overridden. Tests
// constructing their own Adapter instance bypass the registry entirely
// by passing it directly to orchestrators.
func init() {
	adapter.Register(New())
}
