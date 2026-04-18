package e2e

import (
	"strings"
	"testing"
)

func TestNormalize_ReplacesTimestamps(t *testing.T) {
	in := []byte(`{"ts":"2026-04-18T12:00:00Z"} and {"ts":"2026-04-18T12:00:00.123456789Z"}`)
	got := string(Normalize(in, ""))
	if strings.Contains(got, "2026") {
		t.Errorf("timestamp leaked: %s", got)
	}
	if !strings.Contains(got, "<TS>") {
		t.Errorf("want <TS>, got %s", got)
	}
}

func TestNormalize_ReplacesHexIDs(t *testing.T) {
	in := []byte(`{"id":"0123456789abcdef0123456789abcdef"}`)
	got := string(Normalize(in, ""))
	if strings.Contains(got, "0123456789abcdef") {
		t.Errorf("hex id leaked: %s", got)
	}
	if !strings.Contains(got, "<ID>") {
		t.Errorf("want <ID>, got %s", got)
	}
}

func TestNormalize_PreservesDeterministicIDs(t *testing.T) {
	in := []byte(`{"id":"evt-001"}`)
	got := string(Normalize(in, ""))
	if !strings.Contains(got, "evt-001") {
		t.Errorf("deterministic id should remain, got %s", got)
	}
}

func TestNormalize_ReplacesTmpRoot(t *testing.T) {
	in := []byte(`path=/tmp/TestFoo/001/poddies/pods/demo`)
	got := string(Normalize(in, "/tmp/TestFoo/001"))
	if strings.Contains(got, "TestFoo") {
		t.Errorf("tmp root leaked: %s", got)
	}
	if !strings.Contains(got, "<ROOT>") {
		t.Errorf("want <ROOT>, got %s", got)
	}
}
