package orchestrator

import (
	"context"
	"testing"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/adapter/mock"
	"github.com/andrewwormald/poddies/internal/config"
	"github.com/andrewwormald/poddies/internal/thread"
)

func TestShouldFireGrayArea_TriggerNotSet_False(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: true, Name: "sam", Triggers: []config.Trigger{config.TriggerMilestone}}
	events := []thread.Event{{Type: thread.EventHuman, Body: "hi"}}
	if shouldFireGrayArea(events, cos) {
		t.Error("want false when trigger not configured")
	}
}

func TestShouldFireGrayArea_CoSDisabled_False(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: false, Triggers: []config.Trigger{config.TriggerGrayArea}}
	events := []thread.Event{{Type: thread.EventHuman, Body: "hi"}}
	if shouldFireGrayArea(events, cos) {
		t.Error("want false when CoS disabled")
	}
}

func TestShouldFireGrayArea_HumanIsLast_True(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: true, Name: "sam", Triggers: []config.Trigger{config.TriggerGrayArea}}
	events := []thread.Event{{Type: thread.EventHuman, Body: "help"}}
	if !shouldFireGrayArea(events, cos) {
		t.Error("want true when human is last real event")
	}
}

func TestShouldFireGrayArea_MemberResponded_False(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: true, Name: "sam", Triggers: []config.Trigger{config.TriggerGrayArea}}
	events := []thread.Event{
		{Type: thread.EventHuman, Body: "help"},
		{Type: thread.EventMessage, From: "alice", Body: "on it"},
	}
	if shouldFireGrayArea(events, cos) {
		t.Error("want false after member has responded")
	}
}

func TestShouldFireGrayArea_CoSAlreadySpoke_False(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: true, Name: "sam", Triggers: []config.Trigger{config.TriggerGrayArea}}
	events := []thread.Event{
		{Type: thread.EventHuman, Body: "help"},
		{Type: thread.EventMessage, From: "sam", Body: "let me see"},
	}
	if shouldFireGrayArea(events, cos) {
		t.Error("want false after CoS already responded — prevents double-fire")
	}
}

func TestShouldFireGrayArea_SkipsMetaEvents(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: true, Name: "sam", Triggers: []config.Trigger{config.TriggerGrayArea}}
	events := []thread.Event{
		{Type: thread.EventHuman, Body: "help"},
		{Type: thread.EventSystem, Body: "note"},
		{Type: thread.EventPermissionGrant, From: "human", RequestID: "r1"},
	}
	if !shouldFireGrayArea(events, cos) {
		t.Error("should ignore system/permission events when looking back")
	}
}

func TestShouldFireGrayArea_EmptyThread_False(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: true, Triggers: []config.Trigger{config.TriggerGrayArea}}
	if shouldFireGrayArea(nil, cos) {
		t.Error("empty thread should not fire")
	}
}

func TestShouldFireGrayArea_HumanWithMention_False(t *testing.T) {
	cos := config.ChiefOfStaff{Enabled: true, Name: "sam", Triggers: []config.Trigger{config.TriggerGrayArea}}
	events := []thread.Event{{Type: thread.EventHuman, Body: "@alice fix it", Mentions: []string{"alice"}}}
	if shouldFireGrayArea(events, cos) {
		t.Error("explicit @mention should suppress gray_area — defer to human's intent")
	}
}

// TestLoop_GrayArea_AnswersDirectly covers the product use case the
// user called out: a request with no clear owner, no @mention. CoS
// answers directly; loop halts cleanly with a useful message for the
// human instead of silently halting turns=0.
func TestLoop_GrayArea_AnswersDirectly(t *testing.T) {
	root := scaffoldWithCoS(t, "human", "sam",
		[]config.Trigger{config.TriggerGrayArea},
		"alice", "bob",
	)
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{
			ForMember: "sam",
			Response:  adapter.InvokeResponse{Body: "That's outside alice and bob's specialties; here's what I'd do instead…"},
		},
	))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "any thoughts on containerizing this?",
		MaxTurns:      5,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.StopReason != LoopQuiescent {
		t.Errorf("want quiescent, got %s", res.StopReason)
	}
	// human + sam answer = 2 events. No member turn.
	if len(res.Events) != 2 {
		for i, e := range res.Events {
			t.Logf("event[%d] %s from=%s body=%q", i, e.Type, e.From, e.Body)
		}
		t.Errorf("want 2 events (human + sam), got %d", len(res.Events))
	}
	if res.Events[1].From != "sam" {
		t.Errorf("want sam to respond, got from=%s", res.Events[1].From)
	}
}

// TestLoop_GrayArea_RoutesToMember covers the other arm: the CoS
// decides alice owns it after all and @mentions her.
func TestLoop_GrayArea_RoutesToMember(t *testing.T) {
	root := scaffoldWithCoS(t, "human", "sam",
		[]config.Trigger{config.TriggerGrayArea},
		"alice", "bob",
	)
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "sam", Response: adapter.InvokeResponse{Body: "@alice this looks like your area"}},
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "on it"}},
	))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "fix the auth bug",
		MaxTurns:      5,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != LoopQuiescent {
		t.Errorf("want quiescent, got %s", res.StopReason)
	}
	// human + sam (gray-area) + alice = 3 events
	if len(res.Events) != 3 {
		t.Errorf("want 3 events, got %d", len(res.Events))
	}
}

// TestLoop_CoS_AtMention_Routes covers the @-mentionable CoS: a member
// (or the human) can address @sam directly and the CoS responds.
func TestLoop_CoS_AtMention_Routes(t *testing.T) {
	root := scaffoldWithCoS(t, "alice", "sam",
		// don't enable gray_area — isolate the @mention routing
		[]config.Trigger{config.TriggerMilestone},
		"alice",
	)
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@sam want your take"}},
		mock.ScriptedResponse{ForMember: "sam", Response: adapter.InvokeResponse{Body: "my take: ship it"}},
	))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "deploy?",
		MaxTurns:      5,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// human + alice + sam; the @sam mention routed to CoS.
	if len(res.Events) != 3 {
		for i, e := range res.Events {
			t.Logf("event[%d] from=%s body=%q", i, e.From, e.Body)
		}
		t.Fatalf("want 3 events, got %d", len(res.Events))
	}
	if res.Events[2].From != "sam" {
		t.Errorf("want sam to respond to @mention, got %s", res.Events[2].From)
	}
}

// TestLoop_CoS_AtMention_OnlyWhenEnabled guards that @sam doesn't
// route to a CoS that's disabled — it should be treated as a
// non-member mention (halt if it's the only target).
func TestLoop_CoS_AtMention_OnlyWhenEnabled(t *testing.T) {
	// Build a pod with CoS disabled.
	root := scaffoldWithMembers(t, "alice", "alice")
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "alice", Response: adapter.InvokeResponse{Body: "@sam thoughts?"}},
	))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		HumanMessage:  "go",
		MaxTurns:      5,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// alice's @sam is a non-member mention → halt. Only human + alice.
	if len(res.Events) != 2 {
		t.Errorf("want 2 events with disabled CoS, got %d", len(res.Events))
	}
}

// TestRoute_CoS_AtMention_WhenCosNameProvided ensures the pure Route
// function surfaces the CoS as the routing target.
func TestRoute_CoS_AtMention_WhenCosNameProvided(t *testing.T) {
	events := []thread.Event{
		{Type: thread.EventMessage, From: "alice", Body: "@sam help", Mentions: []string{"sam"}},
	}
	got := Route(events, abMembers, "alice", "sam")
	if got.Action != ActionInvoke || got.Member != "sam" {
		t.Errorf("want invoke sam, got %+v", got)
	}
}

func TestRoute_CoS_NotMentioned_NormalRouting(t *testing.T) {
	events := []thread.Event{
		{Type: thread.EventMessage, From: "alice", Body: "@bob please", Mentions: []string{"bob"}},
	}
	got := Route(events, abMembers, "alice", "sam")
	if got.Action != ActionInvoke || got.Member != "bob" {
		t.Errorf("want invoke bob, got %+v", got)
	}
}
