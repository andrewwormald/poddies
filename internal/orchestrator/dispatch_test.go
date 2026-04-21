package orchestrator

import (
	"testing"
)

func TestParseDispatch_SingleMember(t *testing.T) {
	members := MemberSet([]string{"alice", "bob"})
	body := "@alice Build the calculator in Go."
	got := ParseDispatch(body, members)
	if len(got.Dispatches) != 1 {
		t.Fatalf("want 1 dispatch, got %d", len(got.Dispatches))
	}
	if got.Dispatches[0].Member != "alice" || got.Dispatches[0].Instruction != "Build the calculator in Go." {
		t.Errorf("got %+v", got.Dispatches[0])
	}
}

func TestParseDispatch_MultipleMembers(t *testing.T) {
	members := MemberSet([]string{"alice", "bob"})
	body := "@alice Build the calculator.\n@bob Review alice's code."
	got := ParseDispatch(body, members)
	if len(got.Dispatches) != 2 {
		t.Fatalf("want 2 dispatches, got %d", len(got.Dispatches))
	}
}

func TestParseDispatch_IgnoresNonMemberMentions(t *testing.T) {
	members := MemberSet([]string{"alice"})
	body := "@alice Build it.\n@unknown Do something."
	got := ParseDispatch(body, members)
	if len(got.Dispatches) != 1 {
		t.Fatalf("want 1 dispatch, got %d", len(got.Dispatches))
	}
}

func TestParseDispatch_NoDispatches(t *testing.T) {
	members := MemberSet([]string{"alice"})
	body := "I'll handle this directly — no agent needed."
	got := ParseDispatch(body, members)
	if len(got.Dispatches) != 0 {
		t.Fatalf("want 0 dispatches, got %d", len(got.Dispatches))
	}
}

func TestParseDispatch_MixedContent(t *testing.T) {
	members := MemberSet([]string{"alice", "bob"})
	body := "Let me route this:\n@alice Implement the feature.\nSome comment.\n@bob Test it."
	got := ParseDispatch(body, members)
	if len(got.Dispatches) != 2 {
		t.Fatalf("want 2 dispatches, got %d", len(got.Dispatches))
	}
}

func TestParseDispatch_TrailingPunctuation(t *testing.T) {
	members := MemberSet([]string{"alice"})
	body := "@alice, please build the calculator."
	got := ParseDispatch(body, members)
	if len(got.Dispatches) != 1 {
		t.Fatalf("want 1 dispatch, got %d", len(got.Dispatches))
	}
	if got.Dispatches[0].Member != "alice" {
		t.Errorf("want alice, got %s", got.Dispatches[0].Member)
	}
}

func TestParseDispatch_Breakaway(t *testing.T) {
	members := MemberSet([]string{"alice", "bob"})
	body := "+@alice+@bob Discuss whether the auth bug is a race condition."
	got := ParseDispatch(body, members)
	if len(got.Breakaways) != 1 {
		t.Fatalf("want 1 breakaway, got %d", len(got.Breakaways))
	}
	ba := got.Breakaways[0]
	if len(ba.Members) != 2 {
		t.Fatalf("want 2 members, got %d", len(ba.Members))
	}
	if ba.Members[0] != "alice" || ba.Members[1] != "bob" {
		t.Errorf("want [alice bob], got %v", ba.Members)
	}
	if ba.Topic == "" {
		t.Error("topic should not be empty")
	}
}

func TestParseDispatch_MixedDispatchAndBreakaway(t *testing.T) {
	members := MemberSet([]string{"alice", "bob", "carol"})
	body := "@carol Write the tests.\n+@alice+@bob Debate the architecture approach."
	got := ParseDispatch(body, members)
	if len(got.Dispatches) != 1 {
		t.Fatalf("want 1 dispatch, got %d", len(got.Dispatches))
	}
	if len(got.Breakaways) != 1 {
		t.Fatalf("want 1 breakaway, got %d", len(got.Breakaways))
	}
}
