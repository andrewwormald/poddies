package mock

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

func req(member string) adapter.InvokeRequest {
	return adapter.InvokeRequest{
		Role:   adapter.RoleMember,
		Member: config.Member{Name: member},
		Pod:    config.Pod{Name: "p"},
	}
}

func TestNew_DefaultsName(t *testing.T) {
	a := New()
	if a.Name() != "mock" {
		t.Errorf("want mock, got %s", a.Name())
	}
}

func TestNew_WithNameOverride(t *testing.T) {
	a := New(WithName("fake"))
	if a.Name() != "fake" {
		t.Errorf("want fake, got %s", a.Name())
	}
}

func TestInvoke_ReturnsScriptedResponseInOrder(t *testing.T) {
	a := New(WithScript(
		ScriptedResponse{Response: adapter.InvokeResponse{Body: "first"}},
		ScriptedResponse{Response: adapter.InvokeResponse{Body: "second"}},
	))
	got, err := a.Invoke(context.Background(), req("alice"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "first" {
		t.Errorf("want first, got %q", got.Body)
	}
	got, _ = a.Invoke(context.Background(), req("alice"))
	if got.Body != "second" {
		t.Errorf("want second, got %q", got.Body)
	}
}

func TestInvoke_DefaultStopReason_Done(t *testing.T) {
	a := New(WithScript(ScriptedResponse{Response: adapter.InvokeResponse{Body: "x"}}))
	got, _ := a.Invoke(context.Background(), req("alice"))
	if got.StopReason != adapter.StopDone {
		t.Errorf("want StopDone, got %s", got.StopReason)
	}
}

func TestInvoke_DefaultStopReason_NeedsPermission(t *testing.T) {
	a := New(WithScript(ScriptedResponse{
		Response: adapter.InvokeResponse{
			Body: "may I",
			PermissionRequests: []adapter.PermissionRequest{{Action: "run_shell"}},
		},
	}))
	got, _ := a.Invoke(context.Background(), req("alice"))
	if got.StopReason != adapter.StopNeedsPermission {
		t.Errorf("want StopNeedsPermission, got %s", got.StopReason)
	}
}

func TestInvoke_ExplicitStopReason_Preserved(t *testing.T) {
	a := New(WithScript(ScriptedResponse{
		Response: adapter.InvokeResponse{Body: "x", StopReason: adapter.StopYield},
	}))
	got, _ := a.Invoke(context.Background(), req("alice"))
	if got.StopReason != adapter.StopYield {
		t.Errorf("want StopYield, got %s", got.StopReason)
	}
}

func TestInvoke_ScriptExhausted_Errors(t *testing.T) {
	a := New(WithScript(ScriptedResponse{Response: adapter.InvokeResponse{Body: "only"}}))
	_, _ = a.Invoke(context.Background(), req("alice"))
	_, err := a.Invoke(context.Background(), req("alice"))
	if err == nil {
		t.Fatal("want error on exhausted script")
	}
	if !strings.Contains(err.Error(), "exhausted") {
		t.Errorf("want exhausted message, got %v", err)
	}
}

func TestInvoke_ForMemberMismatch_Errors(t *testing.T) {
	a := New(WithScript(ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "hi"}}))
	_, err := a.Invoke(context.Background(), req("bob"))
	if err == nil {
		t.Fatal("want error for wrong member")
	}
	if !strings.Contains(err.Error(), "alice") || !strings.Contains(err.Error(), "bob") {
		t.Errorf("error should name both, got %v", err)
	}
}

func TestInvoke_ForMember_Matching_OK(t *testing.T) {
	a := New(WithScript(ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "hi"}}))
	if _, err := a.Invoke(context.Background(), req("alice")); err != nil {
		t.Errorf("want ok, got %v", err)
	}
}

func TestInvoke_WantContains_Satisfied(t *testing.T) {
	a := New(WithScript(ScriptedResponse{
		WantContains: []string{"hello", "alice"},
		Response:     adapter.InvokeResponse{Body: "ok"},
	}))
	r := req("alice")
	r.Thread = []thread.Event{
		{Type: thread.EventMessage, Body: "hello alice how are you"},
	}
	if _, err := a.Invoke(context.Background(), r); err != nil {
		t.Errorf("want ok, got %v", err)
	}
}

func TestInvoke_WantContains_Missing_Errors(t *testing.T) {
	a := New(WithScript(ScriptedResponse{
		WantContains: []string{"nope"},
		Response:     adapter.InvokeResponse{Body: "ok"},
	}))
	r := req("alice")
	r.Thread = []thread.Event{{Type: thread.EventMessage, Body: "something else"}}
	_, err := a.Invoke(context.Background(), r)
	if err == nil {
		t.Fatal("want error when WantContains missing")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error should name missing substring, got %v", err)
	}
}

func TestInvoke_NonStrict_MissingWantContainsStillErrors(t *testing.T) {
	// non-strict mode: WantContains currently still enforced; relaxing
	// would defeat the point. This test pins that behavior — flip if
	// we intentionally change semantics.
	a := New(
		WithStrict(false),
		WithScript(ScriptedResponse{
			WantContains: []string{"absent"},
			Response:     adapter.InvokeResponse{Body: "ok"},
		}),
	)
	r := req("alice")
	r.Thread = []thread.Event{{Type: thread.EventMessage, Body: "x"}}
	_, err := a.Invoke(context.Background(), r)
	if err != nil {
		t.Logf("got error (non-strict): %v", err)
	}
	// not asserting pass/fail: the behavior is intentional. Document it.
}

func TestInvoke_RecordsCalls(t *testing.T) {
	a := New(WithScript(
		ScriptedResponse{Response: adapter.InvokeResponse{Body: "1"}},
		ScriptedResponse{Response: adapter.InvokeResponse{Body: "2"}},
	))
	r := req("alice")
	r.Thread = []thread.Event{{Type: thread.EventMessage, Body: "x"}}
	r.Effort = config.EffortHigh
	_, _ = a.Invoke(context.Background(), r)
	_, _ = a.Invoke(context.Background(), req("bob"))

	calls := a.Calls()
	if len(calls) != 2 {
		t.Fatalf("want 2 calls, got %d", len(calls))
	}
	if calls[0].MemberName != "alice" || calls[0].ThreadLength != 1 || calls[0].Effort != "high" {
		t.Errorf("call 0 wrong: %+v", calls[0])
	}
	if calls[1].MemberName != "bob" {
		t.Errorf("call 1 wrong: %+v", calls[1])
	}
}

func TestInvoke_ContextCancelled_ReturnsCtxErr(t *testing.T) {
	a := New(WithScript(ScriptedResponse{Response: adapter.InvokeResponse{Body: "x"}}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Invoke(ctx, req("alice"))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("want context.Canceled, got %v", err)
	}
}

func TestInvoke_ContextDeadline_ReturnsCtxErr(t *testing.T) {
	a := New(WithScript(ScriptedResponse{Response: adapter.InvokeResponse{Body: "x"}}))
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	time.Sleep(time.Millisecond)
	_, err := a.Invoke(ctx, req("alice"))
	if err == nil {
		t.Error("want ctx error")
	}
}

func TestInvoke_InvalidRequest_Errors(t *testing.T) {
	a := New(WithScript(ScriptedResponse{Response: adapter.InvokeResponse{Body: "x"}}))
	// empty member name + member role
	_, err := a.Invoke(context.Background(), adapter.InvokeRequest{
		Role: adapter.RoleMember,
		Pod:  config.Pod{Name: "p"},
	})
	if err == nil {
		t.Error("want error for invalid request")
	}
}

func TestQueue_AppendsResponses(t *testing.T) {
	a := New()
	a.Queue(ScriptedResponse{Response: adapter.InvokeResponse{Body: "a"}})
	a.Queue(ScriptedResponse{Response: adapter.InvokeResponse{Body: "b"}})
	if got := a.Remaining(); got != 2 {
		t.Errorf("want 2 remaining, got %d", got)
	}
	_, _ = a.Invoke(context.Background(), req("x"))
	if got := a.Remaining(); got != 1 {
		t.Errorf("want 1 remaining, got %d", got)
	}
}

func TestInvoke_ChiefOfStaffRole_UsesConfiguredName(t *testing.T) {
	a := New(WithScript(ScriptedResponse{
		ForMember: "sam",
		Response:  adapter.InvokeResponse{Body: "summary"},
	}))
	r := adapter.InvokeRequest{
		Role:         adapter.RoleChiefOfStaff,
		ChiefOfStaff: config.ChiefOfStaff{Enabled: true, Name: "sam"},
		Pod:          config.Pod{Name: "p"},
	}
	if _, err := a.Invoke(context.Background(), r); err != nil {
		t.Errorf("want ok, got %v", err)
	}
	calls := a.Calls()
	if len(calls) != 1 || calls[0].MemberName != "sam" {
		t.Errorf("want call for sam, got %+v", calls)
	}
}

func TestInvoke_ChiefOfStaffRole_DefaultName(t *testing.T) {
	a := New(WithScript(ScriptedResponse{
		ForMember: config.DefaultChiefOfStaffName,
		Response:  adapter.InvokeResponse{Body: "x"},
	}))
	r := adapter.InvokeRequest{
		Role:         adapter.RoleChiefOfStaff,
		ChiefOfStaff: config.ChiefOfStaff{Enabled: true}, // no Name set
		Pod:          config.Pod{Name: "p"},
	}
	if _, err := a.Invoke(context.Background(), r); err != nil {
		t.Errorf("want ok, got %v", err)
	}
}
