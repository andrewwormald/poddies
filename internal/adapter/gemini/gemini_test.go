package gemini

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
	if a := New(); a.Name() != "gemini" {
		t.Errorf("want gemini, got %s", a.Name())
	}
}

func TestBuildArgs_IncludesModel(t *testing.T) {
	args := BuildArgs("gemini-2.5-pro")
	argstr := strings.Join(args, " ")
	if !strings.Contains(argstr, "--model") || !strings.Contains(argstr, "gemini-2.5-pro") {
		t.Errorf("args missing expected flags: %v", args)
	}
}

func TestInvoke_Success_ReturnsPlainStdout(t *testing.T) {
	r := &fakeRunner{out: []byte("hello @bob from gemini\n")}
	a := &Adapter{Binary: "gemini", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	got, err := a.Invoke(context.Background(), validMemberReq())
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "hello @bob from gemini" {
		t.Errorf("unexpected body: %q", got.Body)
	}
	if len(got.Mentions) != 1 || got.Mentions[0] != "bob" {
		t.Errorf("mentions: want [bob], got %v", got.Mentions)
	}
	if got.StopReason != adapter.StopDone {
		t.Errorf("want StopDone, got %s", got.StopReason)
	}
}

func TestInvoke_EmptyStdout_Errors(t *testing.T) {
	r := &fakeRunner{out: []byte("   \n   ")}
	a := &Adapter{Binary: "gemini", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	_, err := a.Invoke(context.Background(), validMemberReq())
	if err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Errorf("want empty-response error, got %v", err)
	}
}

func TestInvoke_PassesPromptOnStdin(t *testing.T) {
	r := &fakeRunner{out: []byte("ok")}
	a := &Adapter{Binary: "gemini", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	if _, err := a.Invoke(context.Background(), validMemberReq()); err != nil {
		t.Fatal(err)
	}
	stdin := string(r.stdin)
	for _, want := range []string{"---- SYSTEM ----", "---- THREAD ----", "---- GO ----"} {
		if !strings.Contains(stdin, want) {
			t.Errorf("stdin missing %q: got:\n%s", want, stdin)
		}
	}
}

func TestInvoke_ArgsContainModel(t *testing.T) {
	r := &fakeRunner{out: []byte("ok")}
	a := &Adapter{Binary: "gemini", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	if _, err := a.Invoke(context.Background(), validMemberReq()); err != nil {
		t.Fatal(err)
	}
	modelIdx := -1
	for i, v := range r.args {
		if v == "--model" {
			modelIdx = i
		}
	}
	if modelIdx < 0 || r.args[modelIdx+1] != "gemini-2.5-pro" {
		t.Errorf("model flag missing or wrong value: %v", r.args)
	}
}

func TestInvoke_RunnerError_WrappedWithStderr(t *testing.T) {
	r := &fakeRunner{err: errors.New("exit 5"), errOut: []byte("something broke")}
	a := &Adapter{Binary: "gemini", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	_, err := a.Invoke(context.Background(), validMemberReq())
	if err == nil || !strings.Contains(err.Error(), "something broke") {
		t.Errorf("want stderr in error, got %v", err)
	}
}

func TestInvoke_MissingModel_Errors(t *testing.T) {
	r := &fakeRunner{out: []byte("ok")}
	a := &Adapter{Binary: "gemini", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	req := validMemberReq()
	req.Member.Model = ""
	_, err := a.Invoke(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "model") {
		t.Errorf("want model-missing error, got %v", err)
	}
}

func TestInvoke_NilRunner_Errors(t *testing.T) {
	a := &Adapter{Binary: "gemini"}
	_, err := a.Invoke(context.Background(), validMemberReq())
	if err == nil {
		t.Fatal("want error for nil runner")
	}
}

func TestInvoke_InvalidRequest_Errors(t *testing.T) {
	a := &Adapter{Binary: "gemini", Runner: &fakeRunner{out: []byte("ok")}}
	_, err := a.Invoke(context.Background(), adapter.InvokeRequest{Role: adapter.RoleMember, Pod: demoPod()})
	if err == nil {
		t.Error("want error for invalid request")
	}
}

func TestInvoke_ChiefOfStaff_UsesCoSModel(t *testing.T) {
	r := &fakeRunner{out: []byte("summary")}
	a := &Adapter{Binary: "gemini", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	req := adapter.InvokeRequest{
		Role:         adapter.RoleChiefOfStaff,
		ChiefOfStaff: config.ChiefOfStaff{Enabled: true, Adapter: config.AdapterGemini, Model: "gemini-2.5-flash", Name: "sam", Triggers: []config.Trigger{config.TriggerMilestone}},
		Pod:          demoPod(),
	}
	if _, err := a.Invoke(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	modelIdx := -1
	for i, v := range r.args {
		if v == "--model" {
			modelIdx = i
		}
	}
	if modelIdx < 0 || r.args[modelIdx+1] != "gemini-2.5-flash" {
		t.Errorf("want gemini-2.5-flash model, got args %v", r.args)
	}
}

func TestInvoke_TrimsWhitespace(t *testing.T) {
	r := &fakeRunner{out: []byte("   \n\nhello  \n\n  ")}
	a := &Adapter{Binary: "gemini", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	got, err := a.Invoke(context.Background(), validMemberReq())
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "hello" {
		t.Errorf("want trimmed 'hello', got %q", got.Body)
	}
}

func TestInvoke_CallsRunnerWithExpectedBinary(t *testing.T) {
	r := &fakeRunner{out: []byte("ok")}
	a := &Adapter{Binary: "custom-gemini-path", Runner: r, Roster: func(config.Pod) ([]config.Member, error) { return nil, nil }}
	if _, err := a.Invoke(context.Background(), validMemberReq()); err != nil {
		t.Fatal(err)
	}
	if r.bin != "custom-gemini-path" {
		t.Errorf("want custom-gemini-path, got %s", r.bin)
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
