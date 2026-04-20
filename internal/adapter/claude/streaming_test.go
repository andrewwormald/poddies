package claude

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/config"
)

// fakeStreamingRunner implements cliproc.StreamingRunner for tests. It
// returns its lines field as a reader, and waitErr/waitStderr as the
// wait result.
type fakeStreamingRunner struct {
	lines     string
	waitErr   error
	waitStderr []byte
	// captured inputs
	bin   string
	args  []string
	stdin []byte
}

func (f *fakeStreamingRunner) Start(_ context.Context, bin string, args []string, stdin []byte) (io.Reader, func() ([]byte, error), error) {
	f.bin = bin
	f.args = append([]string(nil), args...)
	f.stdin = append([]byte(nil), stdin...)
	r := strings.NewReader(f.lines)
	wait := func() ([]byte, error) { return f.waitStderr, f.waitErr }
	return r, wait, nil
}

func streamingAdapter(lines string) (*Adapter, *fakeStreamingRunner) {
	sr := &fakeStreamingRunner{lines: lines}
	a := &Adapter{
		Binary:          "claude",
		Runner:          &fakeRunner{}, // non-nil; streaming path won't use it
		StreamingRunner: sr,
		Roster:          func(config.Pod) ([]config.Member, error) { return nil, nil },
	}
	return a, sr
}

// --- BuildStreamArgs ---

func TestBuildStreamArgs_IncludesStreamFormat(t *testing.T) {
	args := BuildStreamArgs("claude-opus-4-7", "system stuff")
	argstr := strings.Join(args, " ")
	for _, want := range []string{"-p", "-", "--output-format", "stream-json", "--model", "claude-opus-4-7", "--append-system-prompt", "system stuff"} {
		if !strings.Contains(argstr, want) {
			t.Errorf("args missing %q: %v", want, args)
		}
	}
}

func TestBuildStreamArgs_NoSystemPrompt_OmitsFlag(t *testing.T) {
	args := BuildStreamArgs("m", "")
	for _, a := range args {
		if a == "--append-system-prompt" {
			t.Errorf("system-prompt flag should be omitted when empty; got %v", args)
		}
	}
}

// --- happy path ---

func TestInvoke_Streaming_HappyPath(t *testing.T) {
	lines := `{"type":"assistant","content":[{"type":"text","text":"Hello "}]}
{"type":"assistant","content":[{"type":"text","text":"@bob!"}]}
{"type":"result","subtype":"success","result":"Hello @bob!","is_error":false}
`
	a, _ := streamingAdapter(lines)
	var tokens []string
	a.OnToken = func(d string) { tokens = append(tokens, d) }

	got, err := a.Invoke(context.Background(), validMemberReq())
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "Hello @bob!" {
		t.Errorf("want body 'Hello @bob!', got %q", got.Body)
	}
	if got.StopReason != adapter.StopDone {
		t.Errorf("want StopDone, got %s", got.StopReason)
	}
	if len(got.Mentions) != 1 || got.Mentions[0] != "bob" {
		t.Errorf("mentions: want [bob], got %v", got.Mentions)
	}
	if len(tokens) != 2 || tokens[0] != "Hello " || tokens[1] != "@bob!" {
		t.Errorf("tokens: want [Hello , @bob!], got %v", tokens)
	}
}

func TestInvoke_Streaming_TokensInOrder(t *testing.T) {
	lines := `{"type":"assistant","content":[{"type":"text","text":"one"}]}
{"type":"assistant","content":[{"type":"text","text":"two"}]}
{"type":"assistant","content":[{"type":"text","text":"three"}]}
{"type":"result","subtype":"success","result":"onetwothree","is_error":false}
`
	a, _ := streamingAdapter(lines)
	var tokens []string
	a.OnToken = func(d string) { tokens = append(tokens, d) }

	got, err := a.Invoke(context.Background(), validMemberReq())
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "onetwothree" {
		t.Errorf("want 'onetwothree', got %q", got.Body)
	}
	want := []string{"one", "two", "three"}
	if len(tokens) != len(want) {
		t.Fatalf("want %d tokens, got %d: %v", len(want), len(tokens), tokens)
	}
	for i, w := range want {
		if tokens[i] != w {
			t.Errorf("tokens[%d]: want %q, got %q", i, w, tokens[i])
		}
	}
}

func TestInvoke_Streaming_FinalBodyEqualsConcatenation(t *testing.T) {
	lines := `{"type":"assistant","content":[{"type":"text","text":"part1"}]}
{"type":"assistant","content":[{"type":"text","text":"part2"}]}
{"type":"result","subtype":"success","result":"","is_error":false}
`
	a, _ := streamingAdapter(lines)
	var body strings.Builder
	a.OnToken = func(d string) { body.WriteString(d) }

	got, err := a.Invoke(context.Background(), validMemberReq())
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != body.String() {
		t.Errorf("body %q != concatenated tokens %q", got.Body, body.String())
	}
}

// --- tool-use messages ---

// TestInvoke_Streaming_ToolUseIgnored verifies that a top-level type=tool_use
// message (not inside an assistant content block) is silently skipped — it's
// not a format the real Claude CLI emits but the parser should be robust.
func TestInvoke_Streaming_ToolUseIgnored(t *testing.T) {
	lines := `{"type":"tool_use","name":"bash","input":{"command":"ls"}}
{"type":"assistant","content":[{"type":"text","text":"done"}]}
{"type":"result","subtype":"success","result":"done","is_error":false}
`
	a, _ := streamingAdapter(lines)
	var tokens []string
	a.OnToken = func(d string) { tokens = append(tokens, d) }

	got, err := a.Invoke(context.Background(), validMemberReq())
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "done" {
		t.Errorf("want 'done', got %q", got.Body)
	}
	if len(tokens) != 1 || tokens[0] != "done" {
		t.Errorf("want only text tokens, got %v", tokens)
	}
}

// TestInvoke_Streaming_ToolUseContentBlock verifies that tool_use content
// blocks inside assistant messages are captured as ToolCalls in the response.
func TestInvoke_Streaming_ToolUseContentBlock_CapturedAsToolCall(t *testing.T) {
	lines := `{"type":"assistant","content":[{"type":"tool_use","name":"bash","input":{"command":"ls -la"}}]}
{"type":"assistant","content":[{"type":"tool_use","name":"read","input":{"file_path":"/tmp/x"}}]}
{"type":"assistant","content":[{"type":"text","text":"done"}]}
{"type":"result","subtype":"success","result":"done","is_error":false}
`
	a, _ := streamingAdapter(lines)
	a.OnToken = func(string) {}

	got, err := a.Invoke(context.Background(), validMemberReq())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ToolCalls) != 2 {
		t.Fatalf("want 2 tool calls, got %d: %v", len(got.ToolCalls), got.ToolCalls)
	}
	if got.ToolCalls[0].Name != "bash" {
		t.Errorf("tool[0].Name: want bash, got %q", got.ToolCalls[0].Name)
	}
	if got.ToolCalls[1].Name != "read" {
		t.Errorf("tool[1].Name: want read, got %q", got.ToolCalls[1].Name)
	}
}

func TestInvoke_Streaming_ToolUseContentBlock_InputTruncatedAt200(t *testing.T) {
	longInput := `{"command":"` + strings.Repeat("x", 300) + `"}`
	lines := `{"type":"assistant","content":[{"type":"tool_use","name":"bash","input":` + longInput + `}]}
{"type":"result","subtype":"success","result":"ok","is_error":false}
`
	a, _ := streamingAdapter(lines)
	a.OnToken = func(string) {}

	got, err := a.Invoke(context.Background(), validMemberReq())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("want 1 tool call, got %d", len(got.ToolCalls))
	}
	// 200 bytes + 3-byte UTF-8 ellipsis "…" = max 203 bytes
	if len(got.ToolCalls[0].Input) > 203 {
		t.Errorf("input should be truncated, len=%d", len(got.ToolCalls[0].Input))
	}
	if !strings.HasSuffix(got.ToolCalls[0].Input, "…") {
		t.Errorf("truncated input should end with ellipsis")
	}
}

func TestInvoke_Streaming_NoToolCalls_EmptySlice(t *testing.T) {
	lines := `{"type":"assistant","content":[{"type":"text","text":"hi"}]}
{"type":"result","subtype":"success","result":"hi","is_error":false}
`
	a, _ := streamingAdapter(lines)
	a.OnToken = func(string) {}

	got, err := a.Invoke(context.Background(), validMemberReq())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ToolCalls) != 0 {
		t.Errorf("want no tool calls, got %v", got.ToolCalls)
	}
}

func TestInvoke_Streaming_NonTextContentIgnored(t *testing.T) {
	lines := `{"type":"assistant","content":[{"type":"image","url":"http://x"},{"type":"text","text":"hi"}]}
{"type":"result","subtype":"success","result":"hi","is_error":false}
`
	a, _ := streamingAdapter(lines)
	var tokens []string
	a.OnToken = func(d string) { tokens = append(tokens, d) }

	got, err := a.Invoke(context.Background(), validMemberReq())
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 1 || tokens[0] != "hi" {
		t.Errorf("want only [hi], got %v", tokens)
	}
	if got.Body != "hi" {
		t.Errorf("want body 'hi', got %q", got.Body)
	}
}

// --- error cases ---

func TestInvoke_Streaming_IsErrorResult_ReturnsError(t *testing.T) {
	lines := `{"type":"result","subtype":"error_rate_limit","result":"rate limited","is_error":true}
`
	a, _ := streamingAdapter(lines)
	a.OnToken = func(string) {}

	_, err := a.Invoke(context.Background(), validMemberReq())
	if err == nil {
		t.Fatal("want error for is_error=true")
	}
	if !strings.Contains(err.Error(), "error_rate_limit") {
		t.Errorf("want subtype in error, got %v", err)
	}
}

func TestInvoke_Streaming_MalformedJSONLine_ReturnsError(t *testing.T) {
	lines := `{"type":"assistant","content":[{"type":"text","text":"hello"}]}
not valid json at all
{"type":"result","subtype":"success","result":"hello","is_error":false}
`
	a, _ := streamingAdapter(lines)
	a.OnToken = func(string) {}

	_, err := a.Invoke(context.Background(), validMemberReq())
	if err == nil {
		t.Fatal("want error for malformed JSONL")
	}
	if !strings.Contains(err.Error(), "malformed stream line") {
		t.Errorf("want 'malformed stream line' in error, got %v", err)
	}
}

func TestInvoke_Streaming_WaitError_WrapsStderr(t *testing.T) {
	lines := ""
	sr := &fakeStreamingRunner{
		lines:      lines,
		waitErr:    errors.New("exit 1"),
		waitStderr: []byte("something broke"),
	}
	a := &Adapter{
		Binary:          "claude",
		Runner:          &fakeRunner{},
		StreamingRunner: sr,
		Roster:          func(config.Pod) ([]config.Member, error) { return nil, nil },
		OnToken:         func(string) {},
	}

	_, err := a.Invoke(context.Background(), validMemberReq())
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "something broke") {
		t.Errorf("want stderr in error, got %v", err)
	}
}

func TestInvoke_Streaming_CtxCancellation(t *testing.T) {
	// Use a context already cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// The fakeStreamingRunner.Start returns a reader synchronously; the scanner
	// will see EOF immediately, then we check ctx.Err().
	lines := `{"type":"assistant","content":[{"type":"text","text":"hi"}]}
`
	sr := &fakeStreamingRunner{
		lines:   lines,
		waitErr: context.Canceled,
	}
	a := &Adapter{
		Binary:          "claude",
		Runner:          &fakeRunner{},
		StreamingRunner: sr,
		Roster:          func(config.Pod) ([]config.Member, error) { return nil, nil },
		OnToken:         func(string) {},
	}

	_, err := a.Invoke(ctx, validMemberReq())
	if err == nil {
		t.Fatal("want context error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("want context.Canceled, got %v", err)
	}
}

func TestInvoke_Streaming_NilStreamingRunner_Errors(t *testing.T) {
	a := &Adapter{
		Binary:  "claude",
		Runner:  &fakeRunner{},
		Roster:  func(config.Pod) ([]config.Member, error) { return nil, nil },
		OnToken: func(string) {},
	}

	_, err := a.Invoke(context.Background(), validMemberReq())
	if err == nil {
		t.Fatal("want error for nil streaming runner")
	}
	if !strings.Contains(err.Error(), "streaming runner not configured") {
		t.Errorf("want 'streaming runner not configured', got %v", err)
	}
}

// --- StreamingRunner uses stream-json format flag ---

func TestInvoke_Streaming_UsesStreamJsonFlag(t *testing.T) {
	lines := `{"type":"result","subtype":"success","result":"ok","is_error":false}
`
	a, sr := streamingAdapter(lines)
	a.OnToken = func(string) {}

	if _, err := a.Invoke(context.Background(), validMemberReq()); err != nil {
		t.Fatal(err)
	}
	argstr := strings.Join(sr.args, " ")
	if !strings.Contains(argstr, "stream-json") {
		t.Errorf("stream-json flag missing: %v", sr.args)
	}
	if strings.Contains(argstr, " json ") || strings.HasSuffix(argstr, " json") {
		t.Errorf("should not use plain json format in streaming mode: %v", sr.args)
	}
}

// --- fallback when no deltas but result has content ---

func TestInvoke_Streaming_FallbackToResultBody(t *testing.T) {
	lines := `{"type":"result","subtype":"success","result":"fallback body","is_error":false}
`
	a, _ := streamingAdapter(lines)
	a.OnToken = func(string) {}

	got, err := a.Invoke(context.Background(), validMemberReq())
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "fallback body" {
		t.Errorf("want 'fallback body', got %q", got.Body)
	}
}

// --- non-streaming path unaffected when OnToken is nil ---

func TestInvoke_NonStreaming_OnTokenNil_UsesRunner(t *testing.T) {
	r := &fakeRunner{out: []byte(`{"type":"result","result":"hello","is_error":false}`)}
	a := &Adapter{
		Binary:  "claude",
		Runner:  r,
		Roster:  func(config.Pod) ([]config.Member, error) { return nil, nil },
		OnToken: nil, // explicitly nil
	}

	got, err := a.Invoke(context.Background(), validMemberReq())
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "hello" {
		t.Errorf("want 'hello', got %q", got.Body)
	}
	if r.calls != 1 {
		t.Errorf("want 1 runner call, got %d", r.calls)
	}
}

// --- empty lines in JSONL are skipped ---

func TestInvoke_Streaming_EmptyLinesSkipped(t *testing.T) {
	lines := "\n\n" + `{"type":"assistant","content":[{"type":"text","text":"hi"}]}` + "\n\n" +
		`{"type":"result","subtype":"success","result":"hi","is_error":false}` + "\n"
	a, _ := streamingAdapter(lines)
	var tokens []string
	a.OnToken = func(d string) { tokens = append(tokens, d) }

	got, err := a.Invoke(context.Background(), validMemberReq())
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "hi" {
		t.Errorf("want 'hi', got %q", got.Body)
	}
	if len(tokens) != 1 {
		t.Errorf("want 1 token, got %v", tokens)
	}
}

// --- New() sets both runner and streaming runner ---

func TestNew_SetsStreamingRunner(t *testing.T) {
	a := New()
	if a.StreamingRunner == nil {
		t.Error("StreamingRunner must be set by New()")
	}
}

// --- ChiefOfStaff model used in streaming mode ---

func TestInvoke_Streaming_ChiefOfStaff_UsesCoSModel(t *testing.T) {
	lines := `{"type":"result","subtype":"success","result":"ok","is_error":false}
`
	sr := &fakeStreamingRunner{lines: lines}
	a := &Adapter{
		Binary:          "claude",
		Runner:          &fakeRunner{},
		StreamingRunner: sr,
		Roster:          func(config.Pod) ([]config.Member, error) { return nil, nil },
		OnToken:         func(string) {},
	}
	req := adapter.InvokeRequest{
		Role:         adapter.RoleChiefOfStaff,
		ChiefOfStaff: config.ChiefOfStaff{Enabled: true, Adapter: config.AdapterClaude, Model: "claude-haiku-4-5", Name: "sam", Triggers: []config.Trigger{config.TriggerMilestone}},
		Pod:          demoPod(),
	}
	if _, err := a.Invoke(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	var modelIdx = -1
	for i, v := range sr.args {
		if v == "--model" {
			modelIdx = i
			break
		}
	}
	if modelIdx < 0 || sr.args[modelIdx+1] != "claude-haiku-4-5" {
		t.Errorf("want claude-haiku-4-5 model, got args %v", sr.args)
	}
}
