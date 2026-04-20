// Package claude is the adapter that drives the `claude` CLI (Claude Code)
// as a subprocess in one-shot JSON mode. One process per turn; all
// conversation state lives in poddies' thread log and is re-rendered
// into each invocation's prompt.
package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/adapter/cliproc"
	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

// DefaultBinary is the executable name resolved via PATH when the
// adapter is used without an explicit override.
const DefaultBinary = "claude"

// Adapter is the Claude Code CLI adapter.
type Adapter struct {
	Binary          string                // overrideable for tests or sidecar installs
	Runner          cliproc.Runner        // defaults to cliproc.NewExecRunner()
	StreamingRunner cliproc.StreamingRunner // used when OnToken is set; defaults to cliproc.NewExecRunner()
	Roster          RosterFn
	// OnToken, when non-nil, switches the adapter to streaming mode. Each
	// partial text delta from the assistant is passed to OnToken as it
	// arrives. The final InvokeResponse.Body equals the concatenation of
	// all deltas.
	OnToken func(delta string)
}

// RosterFn returns the full member list for a pod. The adapter calls
// this to populate the system prompt with the member roster. Tests
// inject a deterministic roster; production wires it to a config loader
// in the orchestrator.
type RosterFn func(pod config.Pod) ([]config.Member, error)

// New constructs an Adapter with production defaults.
func New() *Adapter {
	exec := cliproc.NewExecRunner()
	return &Adapter{
		Binary:          DefaultBinary,
		Runner:          exec,
		StreamingRunner: exec,
		Roster:          func(config.Pod) ([]config.Member, error) { return nil, nil },
	}
}

// Name implements adapter.Adapter.
func (a *Adapter) Name() string { return string(config.AdapterClaude) }

// ClaudeResult mirrors the JSON object emitted by
// `claude -p ... --output-format json`.
type ClaudeResult struct {
	Type         string       `json:"type"`
	Subtype      string       `json:"subtype"`
	Result       string       `json:"result"`
	SessionID    string       `json:"session_id"`
	IsError      bool         `json:"is_error"`
	NumTurns     int          `json:"num_turns"`
	DurationMs   int          `json:"duration_ms"`
	TotalCostUSD float64      `json:"total_cost_usd"`
	Usage        ClaudeUsage  `json:"usage"`
}

// ClaudeUsage is the per-turn token + cache accounting in the result JSON.
type ClaudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// streamMessage is one line of JSONL from --output-format stream-json.
type streamMessage struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	// For type=="assistant" messages, content holds text deltas.
	Content []streamContent `json:"content"`
	// For type=="result" messages.
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
}

type streamContent struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	// tool_use fields
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// Invoke implements adapter.Adapter. It renders the thread into a
// claude-flavored prompt, runs one shot, parses the JSON result, and
// returns the text. When OnToken is set it switches to streaming mode.
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

	var systemPrompt string
	if req.Role == adapter.RoleChiefOfStaff {
		systemPrompt = RenderChiefOfStaffSystemPrompt(req.ChiefOfStaff, req.Pod, roster)
	} else {
		systemPrompt = RenderSystemPrompt(req.Member, req.Pod, roster)
	}
	userPrompt := RenderUserPrompt(req.Member, req.Thread)
	if req.Role == adapter.RoleChiefOfStaff {
		// Give the CoS a call-to-action addressed to itself rather than
		// to a zero-value member name.
		userPrompt = RenderUserPromptForCoS(req.ChiefOfStaff, req.Thread)
	}

	if a.OnToken != nil {
		return a.invokeStreaming(ctx, model, systemPrompt, userPrompt)
	}

	args := BuildArgs(model, systemPrompt)
	// No --resume: each invocation is a fresh session. Server-side
	// session accumulation was the primary cause of quadratic token growth
	// (tool results from prior turns reload on every subsequent turn).
	// Context is reconstructed from the thread window in the user prompt.

	stdout, stderr, err := a.Runner.Run(ctx, a.Binary, args, []byte(userPrompt))
	if err != nil {
		return adapter.InvokeResponse{}, fmt.Errorf("claude: run failed: %w (stderr: %s)", err, cliproc.Truncate(stderr, 512))
	}

	// Some claude CLI versions emit a preamble line (auth warning, update
	// notice) before the JSON object. Strip any leading non-JSON bytes
	// so a noisy CLI doesn't break the parse.
	payload := trimToJSON(stdout)
	var res ClaudeResult
	if err := json.Unmarshal(payload, &res); err != nil {
		return adapter.InvokeResponse{}, fmt.Errorf("claude: parse output: %w (raw: %s)", err, cliproc.Truncate(stdout, 512))
	}
	if res.IsError {
		return adapter.InvokeResponse{}, fmt.Errorf("claude: returned error (%s): %s", res.Subtype, res.Result)
	}

	return adapter.InvokeResponse{
		Body:       res.Result,
		Mentions:   thread.ParseMentions(res.Result),
		StopReason: adapter.StopDone,
		SessionID:  res.SessionID,
		Usage: adapter.Usage{
			InputTokens:  res.Usage.InputTokens + res.Usage.CacheCreationInputTokens + res.Usage.CacheReadInputTokens,
			OutputTokens: res.Usage.OutputTokens,
			TotalCostUSD: res.TotalCostUSD,
			DurationMs:   res.DurationMs,
		},
	}, nil
}

// invokeStreaming runs the claude CLI with --output-format stream-json,
// reads JSONL lines as they arrive, calls OnToken for each text delta,
// and returns when the result message is received.
func (a *Adapter) invokeStreaming(ctx context.Context, model, systemPrompt, userPrompt string) (adapter.InvokeResponse, error) {
	if a.StreamingRunner == nil {
		return adapter.InvokeResponse{}, fmt.Errorf("claude: streaming runner not configured")
	}
	args := BuildStreamArgs(model, systemPrompt)
	stdout, wait, err := a.StreamingRunner.Start(ctx, a.Binary, args, []byte(userPrompt))
	if err != nil {
		return adapter.InvokeResponse{}, fmt.Errorf("claude: stream start: %w", err)
	}

	var body strings.Builder
	var toolCalls []adapter.ToolCall
	scanner := bufio.NewScanner(stdout)
	var parseErr error
	var resultMsg *streamMessage
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg streamMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			// skip malformed lines; record first error
			if parseErr == nil {
				parseErr = fmt.Errorf("claude: malformed stream line: %w (line: %s)", err, cliproc.Truncate([]byte(line), 256))
			}
			continue
		}
		switch msg.Type {
		case "assistant":
			for _, c := range msg.Content {
				switch c.Type {
				case "text":
					if c.Text != "" {
						a.OnToken(c.Text)
						body.WriteString(c.Text)
					}
				case "tool_use":
					if c.Name != "" {
						input := strings.TrimSpace(string(c.Input))
						if len(input) > 200 {
							input = input[:200] + "…"
						}
						toolCalls = append(toolCalls, adapter.ToolCall{Name: c.Name, Input: input})
					}
				}
			}
		case "result":
			resultMsg = &msg
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return adapter.InvokeResponse{}, fmt.Errorf("claude: stream read: %w", err)
	}

	stderr, waitErr := wait()
	if waitErr != nil && ctx.Err() == nil {
		return adapter.InvokeResponse{}, fmt.Errorf("claude: stream wait: %w (stderr: %s)", waitErr, cliproc.Truncate(stderr, 512))
	}
	if ctx.Err() != nil {
		return adapter.InvokeResponse{}, ctx.Err()
	}
	if parseErr != nil {
		return adapter.InvokeResponse{}, parseErr
	}
	if resultMsg != nil && resultMsg.IsError {
		return adapter.InvokeResponse{}, fmt.Errorf("claude: returned error (%s): %s", resultMsg.Subtype, resultMsg.Result)
	}

	text := body.String()
	// If no assistant deltas arrived but the result message has content, fall back to it.
	if text == "" && resultMsg != nil {
		text = resultMsg.Result
	}
	return adapter.InvokeResponse{
		Body:       text,
		Mentions:   thread.ParseMentions(text),
		StopReason: adapter.StopDone,
		ToolCalls:  toolCalls,
	}, nil
}

// trimToJSON returns b from the first '{' byte onward. Lets the
// non-streaming Claude parser tolerate a preamble line (auth warning,
// update notice, etc.) emitted before the JSON object. If no '{' is
// present the original bytes are returned unchanged so json.Unmarshal
// surfaces its normal error message.
func trimToJSON(b []byte) []byte {
	for i, c := range b {
		if c == '{' {
			return b[i:]
		}
	}
	return b
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

// BuildStreamArgs assembles argv for streaming mode (--output-format stream-json).
// Exported so tests can assert the exact flag list.
func BuildStreamArgs(model, systemPrompt string) []string {
	args := []string{
		"-p", "-",
		"--output-format", "stream-json",
		"--model", model,
	}
	if systemPrompt != "" {
		args = append(args, "--append-system-prompt", systemPrompt)
	}
	return args
}

// init auto-registers the production adapter in the global registry,
// but only when the binary path has not been overridden. Tests
// constructing their own Adapter instance bypass the registry entirely
// by passing it directly to orchestrators.
func init() {
	adapter.Register(New())
}
