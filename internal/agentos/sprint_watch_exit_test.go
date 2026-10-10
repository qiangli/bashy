package agentos

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// Sprint 413, todo 4a18c41ef997. Sprint 412's mandated watch —
// `bashy sprint start 412 --owner s412-manager --for 6h --watch`, backgrounded —
// "exited code 1 with no output", and the manager's next evidence was a board
// saying STALE. Two separate silences produced that: the host swallowed the
// error (sprint_dispatch_test.go), and the loop's NORMAL exits return nil, so
// even a clean detach said nothing about why the stream stopped or where the
// mail now was.
//
// An attached watch IS the seat. Ending it is a state change for the whole
// sprint, so every way out names its cause.

func sprintWatchExitRuntime(t *testing.T) sprintWatchRuntime {
	t.Helper()
	return sprintWatchRuntime{
		ackEvery:  time.Second,
		poll:      sprintWatchTestPoll(func(string, int, bool) (inboxBatch, error) { return inboxBatch{}, nil }),
		ackSeq:    func(int64, string) (int64, error) { return 0, nil },
		beatEvery: time.Minute,
		beat:      func(int64, string) error { return nil },
	}
}

func TestSprintWatchExplainsEveryExit(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*sprintWatchRuntime)
		cancel  bool
		wantErr bool
		want    []string
	}{{
		// The interrupt path, and the one that used to be a bare exit 0.
		name:   "interrupted",
		cancel: true,
		want:   []string{"detached from sprint #98 as manager", "the context was cancelled"},
	}, {
		name:    "seat taken over",
		mutate:  func(rt *sprintWatchRuntime) { rt.beat = func(int64, string) error { return errors.New("not held") } },
		wantErr: true,
		want:    []string{"the seat was taken over", "no longer held by this process"},
	}, {
		name: "store unreadable",
		mutate: func(rt *sprintWatchRuntime) {
			rt.poll = sprintWatchTestPoll(func(string, int, bool) (inboxBatch, error) {
				return inboxBatch{}, errors.New("queue.json is unreadable")
			})
		},
		wantErr: true,
		want:    []string{"a store read or write failed"},
	}, {
		name: "owning session gone",
		mutate: func(rt *sprintWatchRuntime) {
			rt.poll.ownerLive = func() error { return &inboxOwnerGoneError{agent: "manager", owner: 4242} }
		},
		wantErr: true,
		want:    []string{"no longer running"},
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sprintWatchIsolate(t)
			rt := sprintWatchExitRuntime(t)
			if tc.mutate != nil {
				tc.mutate(&rt)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
			defer cancel()
			if tc.cancel {
				cancel()
			}
			var out, errOut bytes.Buffer
			err := runSprintInboxWatch(ctx, &out, &errOut, 98, "manager", rt)
			if tc.wantErr && err == nil {
				t.Fatalf("%s should fail", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("%s should end cleanly, got %v", tc.name, err)
			}
			got := errOut.String()
			if strings.TrimSpace(got) == "" {
				t.Fatalf("%s ended with NOTHING on stderr (err=%v)", tc.name, err)
			}
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("%s detach note does not mention %q:\n%s", tc.name, want, got)
				}
			}
			// The question an agent always has when its stream ends.
			if !strings.Contains(got, "NO MAIL WAS LOST") {
				t.Errorf("%s detach note does not say where the mail now is:\n%s", tc.name, got)
			}
			if !strings.Contains(got, "bashy inbox --as manager") {
				t.Errorf("%s detach note does not name the read that recovers it:\n%s", tc.name, got)
			}
			// STDOUT carries NDJSON; prose there would break a parsing reader.
			if strings.Contains(out.String(), "detached from sprint") {
				t.Errorf("%s wrote the detach prose to stdout:\n%s", tc.name, out.String())
			}
		})
	}
}

// Nothing in the detach note may claim a cause it does not know. A loop that
// falls out without one says exactly that rather than naming the likeliest.
func TestSprintWatchDetachNoteNamesAnUnknownCauseHonestly(t *testing.T) {
	var buf bytes.Buffer
	writeSprintWatchDetach(&buf, 98, "manager", "", nil)
	if !strings.Contains(buf.String(), "without naming a cause") {
		t.Fatalf("an unnamed cause was papered over:\n%s", buf.String())
	}
}

// The positive lifecycle this must not weaken: an attached watch that is simply
// waiting on an unacknowledged batch keeps reminding and keeps its lease, and it
// does not detach. Guarded here as well as in sprint_watch_test.go because the
// exit-explanation work rewrote every return in that loop.
func TestSprintWatchWithUnackedMailStillBeatsAndDoesNotExit(t *testing.T) {
	sprintWatchIsolate(t)
	beats := 0
	batch := inboxBatch{
		events: []unifiedInboxEvent{{Schema: unifiedInboxSchema, Source: "mb", Seq: 3, Body: "answer me"}},
		acks:   []func() error{func() error { t.Error("unacknowledged input was consumed"); return nil }},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	rt := sprintWatchRuntime{
		ackEvery:  5 * time.Millisecond,
		poll:      sprintWatchTestPoll(func(string, int, bool) (inboxBatch, error) { return batch, nil }),
		ackSeq:    func(int64, string) (int64, error) { return 0, nil },
		beatEvery: 0, // every pass is due
		beat:      func(int64, string) error { beats++; return nil },
	}
	var out, errOut bytes.Buffer
	if err := runSprintInboxWatch(ctx, &out, &errOut, 98, "manager", rt); err != nil {
		t.Fatalf("the watch quit while mail sat unacknowledged: %v", err)
	}
	if beats == 0 {
		t.Fatal("a watch holding an unacknowledged batch stopped refreshing its lease — the STALE-while-attached bug")
	}
	if n := strings.Count(out.String(), `"type":"unacknowledged-inbox"`); n < 2 {
		t.Fatalf("reminders = %d, want it to keep asking:\n%s", n, out.String())
	}
	if !strings.Contains(errOut.String(), "the context was cancelled") {
		t.Fatalf("the only exit was the test's own deadline; it must still be named:\n%s", errOut.String())
	}
}
