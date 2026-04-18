package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/andrewwormald/poddies/internal/thread"
)

// ErrPermissionAlreadyResolved is returned when an approve/deny
// targets a request that's already been answered.
var ErrPermissionAlreadyResolved = errors.New("permission request already resolved")

// ErrPermissionNotFound is returned when a request id doesn't exist
// in the thread.
var ErrPermissionNotFound = errors.New("permission request not found")

// AppendGrant appends a permission_grant event for requestID.
// Errors if the request doesn't exist or has already been resolved.
func AppendGrant(log *thread.Log, events []thread.Event, requestID, grantedBy string) (thread.Event, error) {
	if _, ok := thread.FindRequest(events, requestID); !ok {
		return thread.Event{}, fmt.Errorf("%w: %q", ErrPermissionNotFound, requestID)
	}
	if thread.IsResolved(events, requestID) {
		return thread.Event{}, fmt.Errorf("%w: %q", ErrPermissionAlreadyResolved, requestID)
	}
	if grantedBy == "" {
		grantedBy = "human"
	}
	return log.Append(thread.Event{
		Type:      thread.EventPermissionGrant,
		From:      grantedBy,
		RequestID: requestID,
	})
}

// AppendDeny appends a permission_deny event for requestID.
// Errors if the request doesn't exist or has already been resolved.
func AppendDeny(log *thread.Log, events []thread.Event, requestID, deniedBy, reason string) (thread.Event, error) {
	if _, ok := thread.FindRequest(events, requestID); !ok {
		return thread.Event{}, fmt.Errorf("%w: %q", ErrPermissionNotFound, requestID)
	}
	if thread.IsResolved(events, requestID) {
		return thread.Event{}, fmt.Errorf("%w: %q", ErrPermissionAlreadyResolved, requestID)
	}
	if deniedBy == "" {
		deniedBy = "human"
	}
	return log.Append(thread.Event{
		Type:      thread.EventPermissionDeny,
		From:      deniedBy,
		RequestID: requestID,
		Body:      reason,
	})
}

// --- cobra ---

func (a *App) newThreadPermissionsCmd() *cobra.Command {
	var podName string
	cmd := &cobra.Command{
		Use:   "permissions <thread>",
		Short: "List unresolved permission requests in a thread.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := a.rootFromApp()
			if err != nil {
				return err
			}
			pod, err := resolvePod(root, podName)
			if err != nil {
				return err
			}
			events, err := LoadThread(root, pod, args[0])
			if err != nil {
				return err
			}
			pending := thread.PendingPermissions(events)
			if len(pending) == 0 {
				fmt.Fprintln(a.Out, "no pending permission requests")
				return nil
			}
			for _, e := range pending {
				fmt.Fprintf(a.Out, "%s  from=%s  action=%s\n", e.ID, e.From, e.Action)
				if len(e.Payload) > 0 {
					fmt.Fprintf(a.Out, "    payload=%s\n", string(e.Payload))
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&podName, "pod", "", "pod name")
	return cmd
}

func (a *App) newThreadApproveCmd() *cobra.Command {
	var podName string
	cmd := &cobra.Command{
		Use:   "approve <thread> <request-id>",
		Short: "Approve a pending permission request.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.resolvePermission(args[0], args[1], podName, true, "")
		},
	}
	cmd.Flags().StringVar(&podName, "pod", "", "pod name")
	return cmd
}

func (a *App) newThreadDenyCmd() *cobra.Command {
	var podName, reason string
	cmd := &cobra.Command{
		Use:   "deny <thread> <request-id>",
		Short: "Deny a pending permission request.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.resolvePermission(args[0], args[1], podName, false, reason)
		},
	}
	cmd.Flags().StringVar(&podName, "pod", "", "pod name")
	cmd.Flags().StringVar(&reason, "reason", "", "optional reason (stored in body)")
	return cmd
}

// resolvePermission is shared by the approve and deny commands.
func (a *App) resolvePermission(threadName, requestID, podName string, grant bool, reason string) error {
	root, err := a.rootFromApp()
	if err != nil {
		return err
	}
	pod, err := resolvePod(root, podName)
	if err != nil {
		return err
	}
	path := ThreadPath(root, pod, normalizeThreadName(threadName))
	log := thread.Open(path)
	events, err := log.Load()
	if err != nil {
		return err
	}
	var out thread.Event
	if grant {
		out, err = AppendGrant(log, events, requestID, "human")
		if err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "granted %s\n", out.RequestID)
	} else {
		out, err = AppendDeny(log, events, requestID, "human", reason)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "denied %s\n", out.RequestID)
	}
	return nil
}
