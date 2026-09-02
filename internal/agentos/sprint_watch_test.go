package agentos

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/weave"
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

// THE ATTACHED WATCH IS THE HEARTBEAT, and it must beat with NO MAIL AT ALL.
//
// Before this, the only thing that refreshed a sprint lease was
// `sprint inbox-ack` — so a seat stayed live only while somebody kept sending
// its manager messages. A conductor working steadily and receiving nothing went
// STALE in thirty minutes with its mandated watch still attached. Measured on a
// live host: 2h08m of running watch, 1h19m of it reported as
// "STALE (no heartbeat — take it)".
func TestSprintWatchHeartbeatsItsLeaseWithoutAnyMail(t *testing.T) {
	var beats atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	rt := sprintWatchRuntime{
		ackEvery: time.Second, maxMisses: 3,
		poll:   sprintWatchTestPoll(func(string, int, bool) (inboxBatch, error) { return inboxBatch{}, nil }),
		ackSeq: func(int64, string) (int64, error) { return 0, nil },
		// Zero interval: every pass is due, so the schedule is not what is
		// under test here — that the watch beats at all, unprompted, is.
		beatEvery: 0,
		beat: func(id int64, owner string) error {
			if id != 98 || owner != "manager" {
				t.Errorf("beat(%d, %q), want the watched sprint and its owner", id, owner)
			}
			beats.Add(1)
			return nil
		},
	}
	if err := runSprintInboxWatch(ctx, &bytes.Buffer{}, &bytes.Buffer{}, 98, "manager", rt); err != nil {
		t.Fatal(err)
	}
	if beats.Load() == 0 {
		t.Fatal("an attached watch with no mail never refreshed the lease it holds open")
	}
}

// A refused heartbeat means the seat is no longer this owner's — somebody took
// it over. Detach and say so, rather than streaming mail addressed to a seat we
// have lost, and rather than fighting the new holder for the lease.
func TestSprintWatchDetachesWhenTheSeatIsTakenOver(t *testing.T) {
	rt := sprintWatchRuntime{
		ackEvery: time.Second, maxMisses: 3,
		poll:   sprintWatchTestPoll(func(string, int, bool) (inboxBatch, error) { return inboxBatch{}, nil }),
		ackSeq: func(int64, string) (int64, error) { return 0, nil },
		beat: func(int64, string) error {
			return errors.New("sprint #98 is not held by manager")
		},
	}
	err := runSprintInboxWatch(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, 98, "manager", rt)
	if err == nil || !strings.Contains(err.Error(), "no longer holds sprint #98") {
		t.Fatalf("takeover error = %v, want a detach naming the sprint", err)
	}
}

// The default runtime wires the real refresh and a schedule well inside the
// lease TTL. A heartbeat at or above the TTL cannot keep anything alive.
func TestDefaultSprintWatchRuntimeBeatsWellInsideTheLeaseTTL(t *testing.T) {
	rt := defaultSprintWatchRuntime()
	if rt.beat == nil {
		t.Fatal("the default watch does not refresh its lease at all")
	}
	if rt.beatEvery <= 0 || rt.beatEvery >= weave.SprintLeaseTTL {
		t.Fatalf("beatEvery = %s, want a positive interval inside the %s TTL", rt.beatEvery, weave.SprintLeaseTTL)
	}
	// Two consecutive misses must still leave the seat live: a heartbeat that
	// only just fits inside the TTL flaps on any scheduling hiccup.
	if rt.beatEvery*2 >= weave.SprintLeaseTTL {
		t.Fatalf("beatEvery = %s leaves no margin for a missed beat in %s", rt.beatEvery, weave.SprintLeaseTTL)
	}
}
