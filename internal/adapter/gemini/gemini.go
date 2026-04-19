// Package gemini is the adapter that drives the `gemini` CLI (Gemini
// CLI from the google-gemini/gemini-cli project) as a subprocess in
// one-shot mode. One process per turn; the thread log is the single
// source of conversation state and is re-rendered into each invocation.
//
// Design choices vs. the claude adapter:
//   - Prompt is passed on stdin (argv has size limits, long transcripts
//     blow it out).
//   - Gemini CLI does not expose a separate system-prompt flag, so the
//     role/persona/roster is inlined at the top of the stdin payload
//     (see render.go).
//   - Stdout is treated as plain text: it is the model's response
//     body. No JSON envelope is parsed, since the Gemini CLI's
//     structured-output mode is not uniformly available across versions.
package gemini

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/adapter/cliproc"
	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

// DefaultBinary is the executable name resolved via PATH when the
// adapter is used without an explicit override.
const DefaultBinary = "gemini"

// Adapter is the Gemini CLI adapter.
type Adapter struct {
	Binary string
	Runner cliproc.Runner
	Roster RosterFn
}

// RosterFn returns the full member list for a pod. See also the
// equivalent type in the claude adapter.
type RosterFn func(config.Pod) ([]config.Member, error)

// New constructs an Adapter with production defaults.
func New() *Adapter {
	return &Adapter{
		Binary: DefaultBinary,
		Runner: cliproc.NewExecRunner(),
		Roster: func(config.Pod) ([]config.Member, error) { return nil, nil },
	}
}

// Name implements adapter.Adapter.
func (a *Adapter) Name() string { return string(config.AdapterGemini) }

// BuildArgs assembles the argv passed to `gemini`. Exported so tests
// can assert the exact flag list without running anything.
//
// Note: Gemini CLI reads the prompt from stdin when none is passed on
// the command line.
func BuildArgs(model string) []string {
	return []string{"--model", model}
}

// Invoke implements adapter.Adapter.
func (a *Adapter) Invoke(ctx context.Context, req adapter.InvokeRequest) (adapter.InvokeResponse, error) {
	if err := adapter.ValidateRequest(req); err != nil {
		return adapter.InvokeResponse{}, err
	}
	if a.Runner == nil {
		return adapter.InvokeResponse{}, fmt.Errorf("gemini: runner not configured")
	}

	roster, _ := a.Roster(req.Pod)

	model := req.Member.Model
	if req.Role == adapter.RoleChiefOfStaff {
		model = req.ChiefOfStaff.Model
	}
	if model == "" {
		return adapter.InvokeResponse{}, fmt.Errorf("gemini: model must be set on member or chief_of_staff")
	}

	var prompt string
	if req.Role == adapter.RoleChiefOfStaff {
		prompt = RenderChiefOfStaffPrompt(req.ChiefOfStaff, req.Pod, roster, req.Thread)
	} else {
		prompt = RenderPrompt(req.Member, req.Pod, roster, req.Thread)
	}
	args := BuildArgs(model)

	stdout, stderr, err := a.Runner.Run(ctx, a.Binary, args, []byte(prompt))
	if err != nil {
		return adapter.InvokeResponse{}, fmt.Errorf("gemini: run failed: %w (stderr: %s)", err, cliproc.Truncate(stderr, 512))
	}

	body := strings.TrimRight(string(bytes.TrimLeft(stdout, " \t\r\n")), " \t\r\n")
	if body == "" {
		return adapter.InvokeResponse{}, fmt.Errorf("gemini: empty response (stderr: %s)", cliproc.Truncate(stderr, 512))
	}

	return adapter.InvokeResponse{
		Body:       body,
		Mentions:   thread.ParseMentions(body),
		StopReason: adapter.StopDone,
	}, nil
}

func init() {
	adapter.Register(New())
}
