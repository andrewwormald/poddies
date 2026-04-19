package orchestrator

import (
	"context"
	"testing"

	"github.com/andrewwormald/poddies/internal/adapter"
	"github.com/andrewwormald/poddies/internal/adapter/mock"
	"github.com/andrewwormald/poddies/internal/config"
)

// TestLoop_FirstMember_ConsumedAfterFirstIter guards against a subtle
// bug where FirstMember would re-fire on every iteration whenever the
// preceding iteration did not increment turnsRun. Concretely: if
// FirstMember names the CoS (or a CoS-shadowed member), Loop detours
// to the CoS invocation path which intentionally does not bump
// turnsRun — the naive "while turnsRun == 0, use FirstMember" check
// then fires again, producing an unbounded loop until SafetyMaxTurns.
//
// The fix consumes a local firstMember variable after the first
// decision, regardless of which path handles it.
func TestLoop_FirstMember_ConsumedAfterFirstIter(t *testing.T) {
	root := scaffoldWithCoS(t, "human", "sam",
		[]config.Trigger{config.TriggerUnresolvedRouting},
		"alice",
	)
	m := mock.New(mock.WithScript(
		mock.ScriptedResponse{ForMember: "sam", Response: adapter.InvokeResponse{Body: "handled"}},
	))
	log := newDeterministicLog(t, t.TempDir())

	loop := &Loop{
		Root:          root,
		Pod:           "demo",
		AdapterLookup: MapLookup(map[string]adapter.Adapter{"mock": m}),
		Log:           log,
		FirstMember:   "sam", // the CoS name
		HumanMessage:  "start",
		MaxTurns:      5,
	}
	res, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Expect: human kickoff + sam (once via FirstMember→CoS detour) → halt.
	if res.StopReason != LoopQuiescent {
		t.Errorf("want quiescent, got %s", res.StopReason)
	}
	if len(res.Events) != 2 {
		for i, e := range res.Events {
			t.Logf("event[%d] from=%s", i, e.From)
		}
		t.Errorf("want 2 events, got %d — FirstMember likely re-fired", len(res.Events))
	}
	// Mock script had one response; Remaining should be zero (consumed once).
	if m.Remaining() != 0 {
		t.Errorf("want script consumed, got %d remaining", m.Remaining())
	}
}
