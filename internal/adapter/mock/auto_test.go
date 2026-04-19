package mock

import (
	"context"
	"strings"
	"testing"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

func TestInvoke_AutoMode_ReturnsCannedResponse(t *testing.T) {
	a := New(WithAuto(true))
	r := adapter.InvokeRequest{
		Role:   adapter.RoleMember,
		Member: config.Member{Name: "alice"},
		Pod:    config.Pod{Name: "demo"},
		Thread: []thread.Event{{Type: thread.EventHuman, Body: "investigate the auth bug please"}},
	}
	got, err := a.Invoke(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Body, "alice") {
		t.Errorf("canned response should name the member: %q", got.Body)
	}
	if !strings.Contains(got.Body, "investigate the auth bug") {
		t.Errorf("canned response should echo human snippet: %q", got.Body)
	}
	if got.StopReason != adapter.StopDone {
		t.Errorf("want StopDone, got %s", got.StopReason)
	}
}

func TestInvoke_AutoMode_NoEvents_StillResponds(t *testing.T) {
	a := New(WithAuto(true))
	r := adapter.InvokeRequest{
		Role:   adapter.RoleMember,
		Member: config.Member{Name: "alice"},
		Pod:    config.Pod{Name: "demo"},
	}
	got, err := a.Invoke(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if got.Body == "" {
		t.Error("want non-empty canned body")
	}
}

func TestInvoke_AutoOff_StillErrorsOnExhaustedScript(t *testing.T) {
	a := New() // Auto=false
	r := adapter.InvokeRequest{
		Role:   adapter.RoleMember,
		Member: config.Member{Name: "alice"},
		Pod:    config.Pod{Name: "demo"},
	}
	_, err := a.Invoke(context.Background(), r)
	if err == nil || !strings.Contains(err.Error(), "exhausted") {
		t.Errorf("want exhausted error with Auto=false, got %v", err)
	}
}

func TestInvoke_AutoMode_TruncatesLongSnippet(t *testing.T) {
	a := New(WithAuto(true))
	long := strings.Repeat("x", 300)
	r := adapter.InvokeRequest{
		Role:   adapter.RoleMember,
		Member: config.Member{Name: "alice"},
		Pod:    config.Pod{Name: "demo"},
		Thread: []thread.Event{{Type: thread.EventHuman, Body: long}},
	}
	got, err := a.Invoke(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Body, "...") {
		t.Errorf("long snippet should be truncated with ellipsis: %q", got.Body)
	}
}
