package agentos

import (
	"bytes"
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func sprintWatchTestPoll(snapshot func(string, int, bool) (inboxBatch, error)) inboxPollRuntime {
	return inboxPollRuntime{
		min: time.Millisecond, max: 2 * time.Millisecond, fullRescan: time.Millisecond,
		now: time.Now, wait: waitInboxPoll, snapshot: snapshot,
		fingerprint: func(string) (uint64, bool) { return 1, true },
	}
}

func TestSprintWatchFailsAfterThreeUnacknowledgedReminders(t *testing.T) {
	var committed atomic.Bool
	batch := inboxBatch{
		events: []unifiedInboxEvent{{Schema: unifiedInboxSchema, Source: "mb", Seq: 7, Body: "please respond"}},
		acks:   []func() error{func() error { committed.Store(true); return nil }},
	}
	rt := sprintWatchRuntime{
		ackEvery: 5 * time.Millisecond, maxMisses: 3,
		poll:   sprintWatchTestPoll(func(string, int, bool) (inboxBatch, error) { return batch, nil }),
		ackSeq: func(int64, string) (int64, error) { return 0, nil },
	}
	var out bytes.Buffer
	err := runSprintInboxWatch(context.Background(), &out, &out, 98, "manager", rt)
	if err == nil || !strings.Contains(err.Error(), "monitoring ENDED") || !strings.Contains(err.Error(), "messages remain unread") {
		t.Fatalf("fuse error = %v", err)
	}
	if got := strings.Count(out.String(), `"type":"unacknowledged-inbox"`); got != 3 {
		t.Fatalf("reminders = %d, want 3:\n%s", got, out.String())
	}
	if committed.Load() {
		t.Fatal("unacknowledged output advanced its source cursor")
	}
}

func TestSprintWatchAcknowledgementCommitsExactBatch(t *testing.T) {
	var committed atomic.Bool
	var ackReads atomic.Int32
	batch := inboxBatch{
		events: []unifiedInboxEvent{{Schema: unifiedInboxSchema, Source: "mb", Seq: 8, Body: "read me"}},
		acks:   []func() error{func() error { committed.Store(true); return nil }},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	rt := sprintWatchRuntime{
		ackEvery: time.Second, maxMisses: 3,
		poll: sprintWatchTestPoll(func(string, int, bool) (inboxBatch, error) {
			if committed.Load() {
				return inboxBatch{}, nil
			}
			return batch, nil
		}),
		ackSeq: func(int64, string) (int64, error) {
			if ackReads.Add(1) > 1 {
				return 1, nil
			}
			return 0, nil
		},
	}
	if err := runSprintInboxWatch(ctx, &bytes.Buffer{}, &bytes.Buffer{}, 98, "manager", rt); err != nil {
		t.Fatal(err)
	}
	if !committed.Load() {
		t.Fatal("explicit acknowledgement did not advance the rendered batch")
	}
}
