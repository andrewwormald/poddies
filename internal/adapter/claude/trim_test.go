package claude

import (
	"bytes"
	"testing"
)

func TestTrimToJSON_NoPreamble(t *testing.T) {
	got := trimToJSON([]byte(`{"type":"result"}`))
	if string(got) != `{"type":"result"}` {
		t.Errorf("got %q", got)
	}
}

func TestTrimToJSON_StripsTextPreamble(t *testing.T) {
	in := []byte("Update available: v1.2.3\n{\"type\":\"result\"}\n")
	got := trimToJSON(in)
	if !bytes.HasPrefix(got, []byte("{")) {
		t.Errorf("want starts with '{', got %q", got)
	}
}

func TestTrimToJSON_NoBrace_ReturnsInput(t *testing.T) {
	in := []byte("no json here")
	got := trimToJSON(in)
	if string(got) != "no json here" {
		t.Errorf("got %q", got)
	}
}

func TestTrimToJSON_Empty(t *testing.T) {
	if got := trimToJSON(nil); len(got) != 0 {
		t.Errorf("got %q", got)
	}
}
