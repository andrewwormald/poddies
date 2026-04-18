package orchestrator

import (
	"fmt"

	"github.com/andrewwormald/poddies/internal/thread"
)

// eventTypes (real impl) returns a compact string slice of "<type>:<from>"
// for each event, used in test failure messages. The stub in
// chief_of_staff_test.go has a generic signature we override here with a
// concrete slice of thread.Event for readability.
func eventTypesConcrete(events []thread.Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = fmt.Sprintf("%s:%s", e.Type, e.From)
	}
	return out
}
