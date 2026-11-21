package main

import (
	"time"
)

func sprintWatchTestPoll(snapshot func(string, int, bool) (inboxBatch, error)) inboxPollRuntime {
	return inboxPollRuntime{
		min: time.Millisecond, max: 2 * time.Millisecond, fullRescan: time.Millisecond,
		now: time.Now, wait: waitInboxPoll, snapshot: snapshot,
		fingerprint: func(string) (uint64, bool) { return 1, true },
	}
}
