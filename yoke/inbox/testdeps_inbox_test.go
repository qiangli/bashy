package main

import (
	"context"
	"io"
	"time"

	"github.com/qiangli/yoke/pkg/room"
)

func registerSprintInboxWatcher(reader string) (inboxWatcherClaim, error) {
	return registerInboxWatcherAs(reader, "sprint-inbox", "managing sprint with attached inbox stream",
		[]string{room.CapInboxStream})
}

func runUnifiedInbox(ctx context.Context, out, errOut io.Writer, reader string, limit int, peek, jsonOut, watch bool, bound time.Duration) error {
	// READING YOUR MAIL IS THE HEARTBEAT. An agent that reads its inbox is
	// demonstrably running and demonstrably attending to this channel, which is
	// what a seat needs to be true — and it is something an agent already does
	// at a turn boundary rather than a process it has to hold open. Best-effort
	// and silent: bookkeeping the caller did not ask for may never fail a read.
	refreshSprintOwnerActivity(reader)
	return runUnifiedInboxWithPoll(ctx, out, errOut, reader, limit, peek, jsonOut, watch, bound, defaultInboxPollRuntime(watch || bound > 0))
}

func runUnifiedInboxWithPoll(ctx context.Context, out, errOut io.Writer, reader string, limit int, peek, jsonOut, watch bool, bound time.Duration, poll inboxPollRuntime) error {
	return runUnifiedInboxWithOptions(ctx, out, errOut, reader, limit, 0, peek, jsonOut, watch, bound, poll)
}
