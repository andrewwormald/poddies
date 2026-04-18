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
// per-test basis via adapter.Register.
type Adapter struct {
	mu     sync.Mutex
	name   string
	script []ScriptedResponse
	cursor int
	calls  []Call
	strict bool
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
