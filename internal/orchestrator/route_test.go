package orchestrator

import (
	"testing"

	"github.com/andrewwormald/poddies/internal/thread"
)

var abMembers = MemberSet([]string{"alice", "bob"})

func TestRoute_Empty_Halts(t *testing.T) {
	got := Route(nil, abMembers, "human")
	if got.Action != ActionHalt {
		t.Errorf("want halt, got %+v", got)
	}
}

func TestRoute_OnlySystem_Halts(t *testing.T) {
	events := []thread.Event{
		{Type: thread.EventSystem, Body: "pod started"},
		{Type: thread.EventSystem, Body: "routed"},
	}
	got := Route(events, abMembers, "human")
	if got.Action != ActionHalt {
		t.Errorf("want halt, got %+v", got)
	}
}

func TestRoute_LastMentionsMember_Invokes(t *testing.T) {
	events := []thread.Event{
		{Type: thread.EventHuman, From: "human", Body: "start", Mentions: nil},
		{Type: thread.EventMessage, From: "alice", Body: "@bob over to you", Mentions: []string{"bob"}},
	}
	got := Route(events, abMembers, "human")
	if got.Action != ActionInvoke || got.Member != "bob" {
		t.Errorf("want invoke bob, got %+v", got)
	}
}

func TestRoute_SkipsTrailingSystem_ToReachMentionEvent(t *testing.T) {
	events := []thread.Event{
		{Type: thread.EventMessage, From: "alice", Body: "@bob go", Mentions: []string{"bob"}},
		{Type: thread.EventSystem, Body: "facilitator note"},
	}
	got := Route(events, abMembers, "human")
	if got.Action != ActionInvoke || got.Member != "bob" {
		t.Errorf("want invoke bob after skipping system, got %+v", got)
	}
}

func TestRoute_LastMentionsNonMember_Halts(t *testing.T) {
	events := []thread.Event{
		{Type: thread.EventMessage, From: "alice", Body: "@carol?", Mentions: []string{"carol"}},
	}
	got := Route(events, abMembers, "human")
	if got.Action != ActionHalt {
		t.Errorf("want halt (carol not a member), got %+v", got)
	}
}

func TestRoute_SelfMentionSkipped(t *testing.T) {
	events := []thread.Event{
		{Type: thread.EventMessage, From: "alice", Body: "@alice hmm", Mentions: []string{"alice"}},
	}
	got := Route(events, abMembers, "human")
	if got.Action != ActionHalt {
		t.Errorf("want halt on self-mention, got %+v", got)
	}
}

func TestRoute_MultipleMentions_FirstActionableWins(t *testing.T) {
	events := []thread.Event{
		{Type: thread.EventMessage, From: "alice", Body: "@carol and @bob", Mentions: []string{"carol", "bob"}},
	}
	got := Route(events, abMembers, "human")
	if got.Action != ActionInvoke || got.Member != "bob" {
		t.Errorf("want invoke bob (carol is not a member), got %+v", got)
	}
}

func TestRoute_HumanNoMention_LeadIsMember_RoutesToLead(t *testing.T) {
	events := []thread.Event{
		{Type: thread.EventHuman, From: "human", Body: "what should we do?"},
	}
	got := Route(events, abMembers, "alice")
	if got.Action != ActionInvoke || got.Member != "alice" {
		t.Errorf("want invoke alice, got %+v", got)
	}
}

func TestRoute_HumanNoMention_LeadIsHuman_Halts(t *testing.T) {
	events := []thread.Event{
		{Type: thread.EventHuman, From: "human", Body: "what do we do?"},
	}
	got := Route(events, abMembers, "human")
	if got.Action != ActionHalt {
		t.Errorf("want halt when lead is human, got %+v", got)
	}
}

func TestRoute_HumanNoMention_LeadNotAMember_Halts(t *testing.T) {
	events := []thread.Event{
		{Type: thread.EventHuman, From: "human", Body: "what?"},
	}
	got := Route(events, abMembers, "ghost")
	if got.Action != ActionHalt {
		t.Errorf("want halt when lead isn't a member, got %+v", got)
	}
}

func TestRoute_AgentNoMention_Halts(t *testing.T) {
	events := []thread.Event{
		{Type: thread.EventMessage, From: "alice", Body: "done"},
	}
	got := Route(events, abMembers, "alice")
	if got.Action != ActionHalt {
		t.Errorf("want halt when agent produces no mentions, got %+v", got)
	}
}

func TestRoute_MentionedMemberEqualsFrom_TreatedAsSelfMention(t *testing.T) {
	// Edge: an event's author mentioned their own name in text. e.Mentions
	// will contain them; we skip and fall through.
	events := []thread.Event{
		{Type: thread.EventMessage, From: "alice", Body: "alice thinks @alice", Mentions: []string{"alice"}},
	}
	got := Route(events, abMembers, "alice")
	if got.Action != ActionHalt {
		t.Errorf("want halt, got %+v", got)
	}
}

func TestMemberSet_Basic(t *testing.T) {
	s := MemberSet([]string{"alice", "bob"})
	if _, ok := s["alice"]; !ok {
		t.Error("alice missing")
	}
	if _, ok := s["bob"]; !ok {
		t.Error("bob missing")
	}
	if _, ok := s["carol"]; ok {
		t.Error("carol should not be present")
	}
}

func TestMemberSet_Empty(t *testing.T) {
	if s := MemberSet(nil); len(s) != 0 {
		t.Errorf("want empty, got %v", s)
	}
}
