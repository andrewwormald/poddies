package thread

// PendingPermissions returns the permission_request events in events
// that are not yet answered by a matching permission_grant or
// permission_deny (by RequestID). Order is preserved: the earliest
// unresolved request appears first.
func PendingPermissions(events []Event) []Event {
	resolved := make(map[string]struct{})
	for _, e := range events {
		if e.Type == EventPermissionGrant || e.Type == EventPermissionDeny {
			if e.RequestID != "" {
				resolved[e.RequestID] = struct{}{}
			}
		}
	}
	var pending []Event
	for _, e := range events {
		if e.Type != EventPermissionRequest {
			continue
		}
		if _, ok := resolved[e.ID]; ok {
			continue
		}
		pending = append(pending, e)
	}
	return pending
}

// HasPendingPermissions reports whether any permission_request in
// events remains unresolved.
func HasPendingPermissions(events []Event) bool {
	return len(PendingPermissions(events)) > 0
}

// FindRequest returns the permission_request event with the given ID,
// and a boolean indicating whether it was found.
func FindRequest(events []Event, id string) (Event, bool) {
	for _, e := range events {
		if e.Type == EventPermissionRequest && e.ID == id {
			return e, true
		}
	}
	return Event{}, false
}

// IsResolved reports whether the permission_request with requestID
// already has a matching grant or deny in events.
func IsResolved(events []Event, requestID string) bool {
	for _, e := range events {
		if (e.Type == EventPermissionGrant || e.Type == EventPermissionDeny) && e.RequestID == requestID {
			return true
		}
	}
	return false
}
