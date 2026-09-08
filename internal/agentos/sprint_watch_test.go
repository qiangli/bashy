package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/room"
	"github.com/qiangli/coreutils/pkg/weave"
)

func sprintWatchTestPoll(snapshot func(string, int, bool) (inboxBatch, error)) inboxPollRuntime {
	return inboxPollRuntime{
		min: time.Millisecond, max: 2 * time.Millisecond, fullRescan: time.Millisecond,
		now: time.Now, wait: waitInboxPoll, snapshot: snapshot,
		fingerprint: func(string) (uint64, bool) { return 1, true },
	}
}

// THE GUARD IS THE CURSOR, NOT THE EXIT.
//
// Unacknowledged input must never advance a source cursor — that is what stops
// a message being lost. The watch used to ALSO quit after three reminders,
// which protected nothing further (the mail was already safe) while destroying
// the seat's delivery path over an unread message; a conductor that was merely
// busy came back to a dead watch and a board reporting UNREACHABLE, measured
// twice in one session. So it reminds for as long as the mail is unread, and
// keeps running.
func TestSprintWatchRemindsWithoutQuittingAndNeverConsumesUnackedInput(t *testing.T) {
	sprintWatchIsolate(t)
	var committed atomic.Bool
	batch := inboxBatch{
		events: []unifiedInboxEvent{{Schema: unifiedInboxSchema, Source: "mb", Seq: 7, Body: "please respond"}},
		acks:   []func() error{func() error { committed.Store(true); return nil }},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	rt := sprintWatchRuntime{
		ackEvery: 5 * time.Millisecond,
		poll:     sprintWatchTestPoll(func(string, int, bool) (inboxBatch, error) { return batch, nil }),
		ackSeq:   func(int64, string) (int64, error) { return 0, nil },
	}
	var out bytes.Buffer
	// Ends only because the context expires — never of its own accord.
	if err := runSprintInboxWatch(ctx, &out, &out, 98, "manager", rt); err != nil {
		t.Fatalf("the watch quit on unacknowledged input: %v", err)
	}
	if n := strings.Count(out.String(), `"type":"unacknowledged-inbox"`); n < 4 {
		t.Fatalf("reminders = %d; it must keep reminding past the old three-strike limit:\n%s", n, out.String())
	}
	if committed.Load() {
		t.Fatal("unacknowledged output advanced its source cursor — the guard that actually prevents loss")
	}
}

func TestSprintWatchAcknowledgementCommitsExactBatch(t *testing.T) {
	sprintWatchIsolate(t)
	var committed atomic.Bool
	var ackReads atomic.Int32
	batch := inboxBatch{
		events: []unifiedInboxEvent{{Schema: unifiedInboxSchema, Source: "mb", Seq: 8, Body: "read me"}},
		acks:   []func() error{func() error { committed.Store(true); return nil }},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	rt := sprintWatchRuntime{
		ackEvery: time.Second,
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
	sprintWatchIsolate(t)
	var beats atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	rt := sprintWatchRuntime{
		ackEvery: time.Second,
		poll:     sprintWatchTestPoll(func(string, int, bool) (inboxBatch, error) { return inboxBatch{}, nil }),
		ackSeq:   func(int64, string) (int64, error) { return 0, nil },
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
	sprintWatchIsolate(t)
	rt := sprintWatchRuntime{
		ackEvery: time.Second,
		poll:     sprintWatchTestPoll(func(string, int, bool) (inboxBatch, error) { return inboxBatch{}, nil }),
		ackSeq:   func(int64, string) (int64, error) { return 0, nil },
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
	sprintWatchIsolate(t)
	rt := defaultSprintWatchRuntime()
	defer rt.poll.close()
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

// TestSprintWatchStandsTheSeatDownOnDetach is the other half of the ghost fix.
//
// The watch is documented as holding the seat for as long as it runs, but
// ending it used to write nothing at all — the last beat simply stayed on the
// lease, and `bashy agents` went on reporting a healthy conductor for the rest
// of the TTL. Every exit is covered, not just the tidy one: a takeover, a
// store error and a cancelled context all end the same evidence.
func TestSprintWatchStandsTheSeatDownOnDetach(t *testing.T) {
	sprintWatchIsolate(t)
	cases := []struct {
		name string
		rt   func(*sprintWatchRuntime)
	}{
		{"context cancelled", func(rt *sprintWatchRuntime) {}},
		{"seat taken over", func(rt *sprintWatchRuntime) {
			rt.beat = func(int64, string) error { return errors.New("sprint #98 is not held by manager") }
		}},
		{"store read failed", func(rt *sprintWatchRuntime) {
			rt.poll = sprintWatchTestPoll(func(string, int, bool) (inboxBatch, error) {
				return inboxBatch{}, errors.New("queue.json is unreadable")
			})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var released atomic.Int64
			rt := sprintWatchRuntime{
				ackEvery:  time.Second,
				poll:      sprintWatchTestPoll(func(string, int, bool) (inboxBatch, error) { return inboxBatch{}, nil }),
				ackSeq:    func(int64, string) (int64, error) { return 0, nil },
				beatEvery: time.Minute,
				beat:      func(int64, string) error { return nil },
				release: func(id int64, owner string) error {
					if id != 98 || owner != "manager" {
						t.Errorf("release(%d, %q), want the seat this watch was holding", id, owner)
					}
					released.Add(1)
					return nil
				},
			}
			tc.rt(&rt)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_ = runSprintInboxWatch(ctx, &bytes.Buffer{}, &bytes.Buffer{}, 98, "manager", rt)
			if released.Load() != 1 {
				t.Fatalf("released %d times, want exactly 1: a detached watch that leaves its beat "+
					"standing keeps a dead conductor on the board for a full TTL", released.Load())
			}
		})
	}
}

// The wiring test. A release hook that exists but is never installed is the
// same bug with an extra layer, and it looks finished from the inside.
func TestDefaultSprintWatchRuntimeReleasesTheSeat(t *testing.T) {
	sprintWatchIsolate(t)
	rt := defaultSprintWatchRuntime()
	defer rt.poll.close()
	if rt.release == nil {
		t.Fatal("the default watch never stands its seat down; detaching would be invisible to `bashy agents`")
	}
}

// Even runtime-wiring tests create native filesystem watches: keep every source
// rooted in this test's HOME rather than inheriting the operator's stores.
func sprintWatchIsolate(t testing.TB) string {
	t.Helper()
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "BASHY_") {
			t.Setenv(key, "")
		}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, store := range []string{"ROOM", "MB", "MEET", "SPRINT", "FLEET"} {
		path := filepath.Join(home, strings.ToLower(store))
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("BASHY_"+store+"_DIR", path)
	}
	return filepath.Join(room.Dir(), "timeline.jsonl")
}
func sprintWatchWriteEvents(t testing.TB, path string, events ...room.Event) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, event := range events {
		if err := json.NewEncoder(f).Encode(event); err != nil {
			t.Fatal(err)
		}
	}
}
func TestSprintWatchIncrementalAckExactFilters(t *testing.T) {
	path := sprintWatchIsolate(t)
	good := room.Event{Type: room.EventAck, Topic: sprintWatchTopic(138), Actor: "MANAGER", Target: room.AgentClaimID("manager")}
	events := []room.Event{good, good, good, good, good}
	events[0].Type = room.EventNote
	events[1].Topic = sprintWatchTopic(139)
	events[2].Actor = "other"
	events[3].Target = room.AgentClaimID("other")
	events[4].Target = strings.ToUpper(good.Target)
	sprintWatchWriteEvents(t, path, events...)
	r := newSprintWatchAckReader()
	p, err := r.latest(context.Background(), 138, "manager")
	if err != nil || p.Seq != 0 {
		t.Fatalf("unrelated event accepted: %+v %v", p, err)
	}
	sprintWatchWriteEvents(t, path, good)
	p, err = r.latest(context.Background(), 138, "manager")
	if err != nil || p.Seq != 6 {
		t.Fatalf("exact ack missing: %+v %v", p, err)
	}
	before := r.reader.Stats()
	r.latest(context.Background(), 138, "manager")
	if got := r.reader.Stats(); got != before {
		t.Fatalf("unchanged ack read did work: %+v -> %+v", before, got)
	}
}
func TestSprintWatchResetRequiresFreshExplicitAck(t *testing.T) {
	path := sprintWatchIsolate(t)
	good := room.Event{Type: room.EventAck, Topic: sprintWatchTopic(138), Actor: "manager", Target: room.AgentClaimID("manager")}
	sprintWatchWriteEvents(t, path, good)
	committed := 0
	reads := 0
	r := newSprintWatchAckReader()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rt := sprintWatchRuntime{ackEvery: time.Hour, poll: sprintWatchTestPoll(func(string, int, bool) (inboxBatch, error) {
		return inboxBatch{events: []unifiedInboxEvent{{Schema: unifiedInboxSchema, Body: "pending"}}, acks: []func() error{func() error { committed++; cancel(); return nil }}}, nil
	})}
	rt.ackPosition = func(ctx context.Context, id int64, owner string) (room.TimelinePosition, error) {
		reads++
		if reads == 2 {
			sprintWatchWriteEvents(t, path+".new", good, good)
			if err := os.Rename(path+".new", path); err != nil {
				t.Fatal(err)
			}
		}
		if reads == 3 {
			if committed != 0 {
				t.Fatal("historical ack in replacement consumed pending batch")
			}
			sprintWatchWriteEvents(t, path, good)
		}
		return r.latest(ctx, id, owner)
	}
	if err := runSprintInboxWatch(ctx, &bytes.Buffer{}, &bytes.Buffer{}, 138, "manager", rt); err != nil {
		t.Fatal(err)
	}
	if committed != 1 || reads != 3 {
		t.Fatalf("committed=%d reads=%d; want one exact batch after fresh ack", committed, reads)
	}
}
func TestSprintWatchBoundsUnrelatedWakeups(t *testing.T) {
	sprintWatchIsolate(t)
	reads := 0
	rt := sprintWatchRuntime{ackEvery: time.Hour, poll: sprintWatchTestPoll(func(string, int, bool) (inboxBatch, error) {
		return inboxBatch{events: []unifiedInboxEvent{{Body: "pending"}}}, nil
	}), ackPosition: func(context.Context, int64, string) (room.TimelinePosition, error) {
		reads++
		return room.TimelinePosition{Generation: 1}, nil
	}}
	rt.poll.min = 10 * time.Millisecond
	rt.poll.wait = func(context.Context, time.Duration) error { return nil }
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Millisecond)
	defer cancel()
	if err := runSprintInboxWatch(ctx, &bytes.Buffer{}, &bytes.Buffer{}, 138, "manager", rt); err != nil {
		t.Fatal(err)
	}
	if reads > 7 {
		t.Fatalf("unrelated wakes caused %d ack polls in 55ms", reads)
	}
}
