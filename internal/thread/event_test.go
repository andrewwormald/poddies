package thread

import (
	"strings"
	"testing"
)

func TestEventType_IsKnown(t *testing.T) {
	for _, k := range KnownEventTypes {
		if !k.IsKnown() {
			t.Errorf("%q should be known", k)
		}
	}
	if EventType("bogus").IsKnown() {
		t.Error("bogus should not be known")
	}
}

func TestEvent_Validate_EmptyType(t *testing.T) {
	e := Event{}
	if err := e.Validate(); err == nil {
		t.Error("want error for empty type")
	}
}

func TestEvent_Validate_Message_RequiresFrom(t *testing.T) {
	if err := (&Event{Type: EventMessage, Body: "hi"}).Validate(); err == nil {
		t.Error("want error for message with no from")
	}
	if err := (&Event{Type: EventMessage, From: "alice", Body: "hi"}).Validate(); err != nil {
		t.Errorf("want ok, got %v", err)
	}
}

func TestEvent_Validate_Human_DefaultsFrom(t *testing.T) {
	e := &Event{Type: EventHuman, Body: "hi"}
	if err := e.Validate(); err != nil {
		t.Fatalf("want ok, got %v", err)
	}
	if e.From != "human" {
		t.Errorf("want from=human, got %q", e.From)
	}
}

func TestEvent_Validate_Human_ForeignFrom_Errors(t *testing.T) {
	e := &Event{Type: EventHuman, From: "alice"}
	if err := e.Validate(); err == nil {
		t.Error("want error for human event with non-human from")
	}
}

func TestEvent_Validate_System_OK(t *testing.T) {
	if err := (&Event{Type: EventSystem, Body: "routed to alice"}).Validate(); err != nil {
		t.Errorf("want ok, got %v", err)
	}
}

func TestEvent_Validate_PermissionRequest_RequiresFromAndAction(t *testing.T) {
	if err := (&Event{Type: EventPermissionRequest}).Validate(); err == nil {
		t.Error("want error for missing from")
	}
	if err := (&Event{Type: EventPermissionRequest, From: "alice"}).Validate(); err == nil {
		t.Error("want error for missing action")
	}
	if err := (&Event{Type: EventPermissionRequest, From: "alice", Action: "run"}).Validate(); err != nil {
		t.Errorf("want ok, got %v", err)
	}
}

func TestEvent_Validate_PermissionGrantDeny_RequireRequestID(t *testing.T) {
	for _, tp := range []EventType{EventPermissionGrant, EventPermissionDeny} {
		if err := (&Event{Type: tp}).Validate(); err == nil {
			t.Errorf("%s: want error for missing request_id", tp)
		}
		if err := (&Event{Type: tp, RequestID: "req-1"}).Validate(); err != nil {
			t.Errorf("%s: want ok, got %v", tp, err)
		}
	}
}

func TestEvent_Validate_UnknownType_AcceptedForForwardCompat(t *testing.T) {
	if err := (&Event{Type: "future_type"}).Validate(); err != nil {
		t.Errorf("unknown type should not error on validate, got %v", err)
	}
}

func TestNewID_Uniqueness(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := NewID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id %q at i=%d", id, i)
		}
		seen[id] = struct{}{}
	}
}

func TestNewID_Length(t *testing.T) {
	if got := len(NewID()); got != 32 {
		t.Errorf("want 32 hex chars, got %d", got)
	}
}

func TestParseMentions_SingleMention(t *testing.T) {
	got := ParseMentions("hey @alice can you check this?")
	want := []string{"alice"}
	if !eqStrSlice(got, want) {
		t.Errorf("want %v, got %v", want, got)
	}
}

func TestParseMentions_AtStartOfString(t *testing.T) {
	got := ParseMentions("@alice hi")
	if !eqStrSlice(got, []string{"alice"}) {
		t.Errorf("got %v", got)
	}
}

func TestParseMentions_Multiple(t *testing.T) {
	got := ParseMentions("@alice and @bob and @carol")
	if !eqStrSlice(got, []string{"alice", "bob", "carol"}) {
		t.Errorf("got %v", got)
	}
}

func TestParseMentions_Dedup(t *testing.T) {
	got := ParseMentions("@alice @alice @alice")
	if !eqStrSlice(got, []string{"alice"}) {
		t.Errorf("got %v", got)
	}
}

func TestParseMentions_EmailNotMatched(t *testing.T) {
	got := ParseMentions("email me at andrew@example.com")
	if len(got) != 0 {
		t.Errorf("want no mentions in email, got %v", got)
	}
}

func TestParseMentions_UppercaseNotMatched(t *testing.T) {
	got := ParseMentions("hey @Alice")
	if len(got) != 0 {
		t.Errorf("want no mentions for uppercase @Alice, got %v", got)
	}
}

func TestParseMentions_PunctuationStripped(t *testing.T) {
	got := ParseMentions("@alice, @bob. @carol! @dave?")
	if !eqStrSlice(got, []string{"alice", "bob", "carol", "dave"}) {
		t.Errorf("got %v", got)
	}
}

func TestParseMentions_Multiline(t *testing.T) {
	body := "hi @alice\nthink about this\n@bob can confirm"
	got := ParseMentions(body)
	if !eqStrSlice(got, []string{"alice", "bob"}) {
		t.Errorf("got %v", got)
	}
}

func TestParseMentions_Empty(t *testing.T) {
	if got := ParseMentions(""); len(got) != 0 {
		t.Errorf("want nil, got %v", got)
	}
}

func TestParseMentions_NoMentions(t *testing.T) {
	if got := ParseMentions("a normal sentence with no mentions"); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestParseMentions_AtAloneIgnored(t *testing.T) {
	if got := ParseMentions("lone @ and @."); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestParseMentions_ParenthesizedMention(t *testing.T) {
	got := ParseMentions("(cc @alice)")
	if !eqStrSlice(got, []string{"alice"}) {
		t.Errorf("got %v", got)
	}
}

func eqStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sanity check that the regex does not match things that look like
// slugs but live inside other identifiers.
func TestParseMentions_InsideIdentifier(t *testing.T) {
	if got := ParseMentions("foo@bar@baz"); len(got) != 0 {
		// "foo@bar" — 'o' precedes '@bar' so rejected
		// "@baz" — 'r' precedes so rejected
		t.Errorf("got %v", got)
	}
	if got := ParseMentions("foo_@bar"); len(got) != 0 {
		// underscore before '@' is still alphanumeric-ish — we include _
		// in the disallowed set.
		t.Errorf("underscore-prefix should reject, got %v", got)
	}
}

func TestParseMentions_TrailingHyphenDropped(t *testing.T) {
	// "@alice-" → mention is "alice-"? Regex permits '-' in slug body.
	// slug validator in config rejects trailing '-' but we don't validate
	// here (parser is deliberately liberal; validator is strict).
	got := ParseMentions("@alice-")
	if !eqStrSlice(got, []string{"alice-"}) {
		t.Errorf("parser should be liberal and keep trailing dash; got %v", got)
	}
	if strings.HasPrefix(got[0], "-") {
		t.Errorf("unexpected leading dash in %v", got)
	}
}
