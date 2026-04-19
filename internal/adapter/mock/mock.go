// Package mock provides a deterministic Adapter implementation used by
// unit and end-to-end tests. It replays a scripted queue of responses
// and can optionally assert that each invocation's thread includes
// certain substrings, so tests can pin down prompt rendering.
package mock

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/thread"
)

// ScriptedResponse is one scripted turn.
type ScriptedResponse struct {
	// ForMember, if non-empty, requires that the invocation be for a
	// member with this name. The mock returns an error if the actual
	// member differs — catching routing bugs fast.
	ForMember string

	// WantContains, if non-empty, requires that the serialized thread
	// (concatenation of event bodies) contains each of these strings.
	// Useful for asserting prompt rendering without tying tests to a
	// particular format.
	WantContains []string

	// Response is the envelope returned to the caller on match.
	Response adapter.InvokeResponse
}

// Call records a single invocation for test-side assertion.
type Call struct {
	MemberName   string
	Role         adapter.Role
	ThreadLength int
	Effort       string
}

// Adapter is the mock adapter. It is NOT registered globally by default;
// tests pass an instance directly to orchestrators or register it on a
// per-test basis via adapter.Register. A production-flavored instance
// (with Auto=true) is registered at startup from cmd/poddies so that
// users reaching for `--adapter mock` during onboarding / demos get a
// working pod without installing any real CLI.
type Adapter struct {
	mu     sync.Mutex
	name   string
	script []ScriptedResponse
	cursor int
	calls  []Call
	strict bool
	// Auto, when true, produces a canned acknowledgement response when
	// the scripted queue is exhausted instead of returning an error.
	// Used by the production registration so the mock is actually
	// usable end-to-end; tests leave this off (default false).
	Auto bool
}

// Option configures a new mock Adapter.
type Option func(*Adapter)

// WithName overrides the adapter's name (default: "mock").
func WithName(n string) Option { return func(a *Adapter) { a.name = n } }

// WithScript sets the initial scripted responses.
func WithScript(s ...ScriptedResponse) Option { return func(a *Adapter) { a.script = s } }

// WithStrict, when true, makes any unmet WantContains assertion a hard
// error rather than a soft mismatch. Default true.
func WithStrict(strict bool) Option { return func(a *Adapter) { a.strict = strict } }

// WithAuto enables auto-canned responses when the scripted queue is
// exhausted. Intended for production registration, not tests.
func WithAuto(auto bool) Option { return func(a *Adapter) { a.Auto = auto } }

// New constructs a mock Adapter. Default name is "mock".
func New(opts ...Option) *Adapter {
	a := &Adapter{name: "mock", strict: true}
	for _, o := range opts {
		o(a)
	}
	return a
}

// Name implements adapter.Adapter.
func (a *Adapter) Name() string { return a.name }

// Queue appends additional scripted responses.
func (a *Adapter) Queue(s ...ScriptedResponse) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.script = append(a.script, s...)
}

// Calls returns a snapshot of calls made so far.
func (a *Adapter) Calls() []Call {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Call, len(a.calls))
	copy(out, a.calls)
	return out
}

// Remaining reports how many scripted responses have not been consumed.
func (a *Adapter) Remaining() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.script) - a.cursor
}

// Invoke implements adapter.Adapter.
func (a *Adapter) Invoke(ctx context.Context, req adapter.InvokeRequest) (adapter.InvokeResponse, error) {
	if err := ctx.Err(); err != nil {
		return adapter.InvokeResponse{}, err
	}
	if err := adapter.ValidateRequest(req); err != nil {
		return adapter.InvokeResponse{}, fmt.Errorf("mock: invalid request: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cursor >= len(a.script) {
		if a.Auto {
			memberName := req.Member.Name
			if req.Role == adapter.RoleChiefOfStaff {
				memberName = req.ChiefOfStaff.ResolvedName()
			}
			a.calls = append(a.calls, Call{
				MemberName:   memberName,
				Role:         req.Role,
				ThreadLength: len(req.Thread),
				Effort:       string(req.Effort),
			})
			body := autoResponse(memberName, req.Thread)
			// Synthesize plausible-looking token counts so the TUI's
			// burn-rate display doesn't read as zeros against the mock.
			in := 4 * len(renderThreadForAssert(req.Thread)) / 3
			out := 4 * len(body) / 3
			return adapter.InvokeResponse{
				Body:       body,
				StopReason: adapter.StopDone,
				SessionID:  "mock-" + memberName,
				Usage: adapter.Usage{
					InputTokens:  in,
					OutputTokens: out,
				},
			}, nil
		}
		return adapter.InvokeResponse{}, fmt.Errorf("mock: script exhausted at call #%d (member=%q)", a.cursor+1, req.Member.Name)
	}
	s := a.script[a.cursor]
	a.cursor++

	memberName := req.Member.Name
	if req.Role == adapter.RoleChiefOfStaff {
		memberName = req.ChiefOfStaff.ResolvedName()
	}

	if s.ForMember != "" && s.ForMember != memberName {
		return adapter.InvokeResponse{}, fmt.Errorf("mock: scripted turn #%d expected member %q, got %q", a.cursor, s.ForMember, memberName)
	}

	if a.strict && len(s.WantContains) > 0 {
		joined := renderThreadForAssert(req.Thread)
		for _, sub := range s.WantContains {
			if !strings.Contains(joined, sub) {
				return adapter.InvokeResponse{}, fmt.Errorf("mock: scripted turn #%d: thread does not contain %q", a.cursor, sub)
			}
		}
	}

	a.calls = append(a.calls, Call{
		MemberName:   memberName,
		Role:         req.Role,
		ThreadLength: len(req.Thread),
		Effort:       string(req.Effort),
	})

	resp := s.Response
	if resp.StopReason == "" {
		if len(resp.PermissionRequests) > 0 {
			resp.StopReason = adapter.StopNeedsPermission
		} else {
			resp.StopReason = adapter.StopDone
		}
	}
	return resp, nil
}

// renderThreadForAssert concatenates event bodies with newline separators
// so WantContains can match substrings in any body. Production adapters
// render differently; this is only for test assertions.
func renderThreadForAssert(events []thread.Event) string {
	var b strings.Builder
	for _, e := range events {
		b.WriteString(e.Body)
		b.WriteByte('\n')
	}
	return b.String()
}

// autoResponse builds a canned reply for Auto-mode invocations. It
// surfaces enough context (member name + snippet of the triggering
// human message, if present) that the thread still reads sensibly
// without any real LLM behind it — good enough for demos, onboarding,
// and CI smoke tests.
func autoResponse(memberName string, events []thread.Event) string {
	last := ""
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.Type == thread.EventHuman || e.Type == thread.EventMessage {
			last = e.Body
			break
		}
	}
	if last == "" {
		return "(mock) " + memberName + " here — ready when you are."
	}
	snippet := last
	if len(snippet) > 120 {
		snippet = snippet[:117] + "..."
	}
	return "(mock) " + memberName + " acknowledged: " + snippet
}

// init registers a production-flavored mock adapter (Auto=true) so
// `--adapter mock` works end-to-end without the user needing claude or
// gemini installed. Tests that touch the global registry reset it
// before each test (see internal/adapter), so this registration is
// invisible to them.
func init() {
	adapter.Register(New(WithAuto(true)))
}
