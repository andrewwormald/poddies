package claude

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/config"
)

// fakeRunner captures Run() inputs and returns fixed outputs.
type fakeRunner struct {
	bin    string
	args   []string
	stdin  []byte
	out    []byte
	errOut []byte
	err    error
	calls  int
}

func (f *fakeRunner) Run(_ context.Context, bin string, args []string, stdin []byte) ([]byte, []byte, error) {
	f.calls++
	f.bin = bin
	f.args = append([]string(nil), args...)
	f.stdin = append([]byte(nil), stdin...)
	return f.out, f.errOut, f.err
}

func validMemberReq() adapter.InvokeRequest {
	return adapter.InvokeRequest{
		Role:   adapter.RoleMember,
		Member: alice(),
		Pod:    demoPod(),
	}
}

func TestName(t *testing.T) {
	a := New()
	if a.Name() != "claude" {
		t.Errorf("want claude, got %s", a.Name())
	}
}

func TestBuildArgs_IncludesAllExpected(t *testing.T) {
	args := BuildArgs("claude-opus-4-7", "system stuff")
	argstr := strings.Join(args, " ")
	for _, want := range []string{"-p", "-", "--output-format", "json", "--model", "claude-opus-4-7", "--append-system-prompt", "system stuff"} {
		if !strings.Contains(argstr, want) {
			t.Errorf("args missing %q: %v", want, args)
		}
	}
}

func TestBuildArgs_NoSystemPrompt_OmitsFlag(t *testing.T) {
	args := BuildArgs("m", "")
	for i, a := range args {
		if a == "--append-system-prompt" {
			t.Errorf("system-prompt flag should be omitted when empty; found at %d", i)
		}
	}
}

func TestInvoke_Success_ParsesResult(t *testing.T) {
	r := &fakeRunner{out: []byte(`{"type":"result","subtype":"success","result":"hello @bob","session_id":"s","is_error":false}`)}
	a := &Adapter{Binary: "claude", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	got, err := a.Invoke(context.Background(), validMemberReq())
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "hello @bob" {
		t.Errorf("want body hello @bob, got %q", got.Body)
	}
	if len(got.Mentions) != 1 || got.Mentions[0] != "bob" {
		t.Errorf("mentions: want [bob], got %v", got.Mentions)
	}
	if got.StopReason != adapter.StopDone {
		t.Errorf("want StopDone, got %s", got.StopReason)
	}
}

func TestInvoke_PassesUserPromptOnStdin(t *testing.T) {
	r := &fakeRunner{out: []byte(`{"type":"result","result":"ok"}`)}
	a := &Adapter{Binary: "claude", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	if _, err := a.Invoke(context.Background(), validMemberReq()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(r.stdin), "Respond as alice") {
		t.Errorf("stdin missing user prompt CTA; got %q", r.stdin)
	}
}

func TestInvoke_PassesSystemPromptAsFlag(t *testing.T) {
	r := &fakeRunner{out: []byte(`{"type":"result","result":"ok"}`)}
	a := &Adapter{Binary: "claude", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	if _, err := a.Invoke(context.Background(), validMemberReq()); err != nil {
		t.Fatal(err)
	}
	// find --append-system-prompt and verify its value mentions the member.
	var sysIdx int = -1
	for i, a := range r.args {
		if a == "--append-system-prompt" {
			sysIdx = i
			break
		}
	}
	if sysIdx < 0 || sysIdx+1 >= len(r.args) {
		t.Fatalf("append-system-prompt flag missing: %v", r.args)
	}
	if !strings.Contains(r.args[sysIdx+1], "alice") {
		t.Errorf("system prompt should name alice, got %q", r.args[sysIdx+1])
	}
}

func TestInvoke_RunnerError_WrappedWithStderr(t *testing.T) {
	r := &fakeRunner{err: errors.New("exit 7"), errOut: []byte("something broke")}
	a := &Adapter{Binary: "claude", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	_, err := a.Invoke(context.Background(), validMemberReq())
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "something broke") {
		t.Errorf("want stderr in error, got %v", err)
	}
}

func TestInvoke_MalformedJSON_ReturnsError(t *testing.T) {
	r := &fakeRunner{out: []byte("not json")}
	a := &Adapter{Binary: "claude", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	_, err := a.Invoke(context.Background(), validMemberReq())
	if err == nil {
		t.Fatal("want parse error")
	}
	if !strings.Contains(err.Error(), "parse output") {
		t.Errorf("want 'parse output' in err, got %v", err)
	}
}

func TestInvoke_IsErrorResult_ReturnsError(t *testing.T) {
	r := &fakeRunner{out: []byte(`{"type":"result","subtype":"error_rate_limit","is_error":true,"result":"rate limited"}`)}
	a := &Adapter{Binary: "claude", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	_, err := a.Invoke(context.Background(), validMemberReq())
	if err == nil {
		t.Fatal("want error for is_error=true")
	}
	if !strings.Contains(err.Error(), "error_rate_limit") {
		t.Errorf("want subtype in error, got %v", err)
	}
}

func TestInvoke_MissingModel_Errors(t *testing.T) {
	r := &fakeRunner{out: []byte(`{"type":"result"}`)}
	a := &Adapter{Binary: "claude", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	req := validMemberReq()
	req.Member.Model = ""
	_, err := a.Invoke(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "model") {
		t.Errorf("want model-missing error, got %v", err)
	}
}

func TestInvoke_NilRunner_Errors(t *testing.T) {
	a := &Adapter{Binary: "claude"}
	_, err := a.Invoke(context.Background(), validMemberReq())
	if err == nil {
		t.Fatal("want error for nil runner")
	}
}

func TestInvoke_InvalidRequest_Errors(t *testing.T) {
	a := &Adapter{Binary: "claude", Runner: &fakeRunner{out: []byte(`{}`)}}
	_, err := a.Invoke(context.Background(), adapter.InvokeRequest{Role: adapter.RoleMember, Pod: demoPod()})
	if err == nil {
		t.Error("want error for invalid request")
	}
}

func TestInvoke_ChiefOfStaff_UsesCoSModel(t *testing.T) {
	r := &fakeRunner{out: []byte(`{"type":"result","result":"summary"}`)}
	a := &Adapter{Binary: "claude", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	req := adapter.InvokeRequest{
		Role:         adapter.RoleChiefOfStaff,
		ChiefOfStaff: config.ChiefOfStaff{Enabled: true, Adapter: config.AdapterClaude, Model: "claude-haiku-4-5", Name: "sam", Triggers: []config.Trigger{config.TriggerMilestone}},
		Pod:          demoPod(),
	}
	if _, err := a.Invoke(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	// assert model flag value
	var modelIdx = -1
	for i, v := range r.args {
		if v == "--model" {
			modelIdx = i
			break
		}
	}
	if modelIdx < 0 || r.args[modelIdx+1] != "claude-haiku-4-5" {
		t.Errorf("want claude-haiku-4-5 model, got args %v", r.args)
	}
}

func TestInvoke_CallsRunnerWithExpectedBinary(t *testing.T) {
	r := &fakeRunner{out: []byte(`{"type":"result","result":"ok"}`)}
	a := &Adapter{Binary: "custom-claude-path", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	if _, err := a.Invoke(context.Background(), validMemberReq()); err != nil {
		t.Fatal(err)
	}
	if r.bin != "custom-claude-path" {
		t.Errorf("want custom-claude-path, got %s", r.bin)
	}
}

func TestNew_Defaults(t *testing.T) {
	a := New()
	if a.Binary != DefaultBinary {
		t.Errorf("want %s, got %s", DefaultBinary, a.Binary)
	}
	if a.Runner == nil {
		t.Error("runner must be set")
	}
	if a.Roster == nil {
		t.Error("roster must be set")
	}
}
