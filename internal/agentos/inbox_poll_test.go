package agentos

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/meet"
	"github.com/qiangli/coreutils/pkg/weave"
)

func TestInboxWatchIdleHasBoundedPollsAndOneFullRead(t *testing.T) {
	now := time.Unix(100, 0)
	var waits []time.Duration
	fullReads, samples := 0, 0
	runtime := inboxPollRuntime{
		min: 10 * time.Millisecond, max: 40 * time.Millisecond, fullRescan: time.Hour,
		now: func() time.Time { return now },
		wait: func(_ context.Context, d time.Duration) error {
			waits = append(waits, d)
			now = now.Add(d)
			if len(waits) == 12 {
				return context.Canceled
			}
			return nil
		},
		snapshot: func(string, int, bool) (inboxBatch, error) {
			fullReads++
			return inboxBatch{}, nil
		},
		fingerprint: func(string) (uint64, bool) {
			samples++
			return 1, true
		},
	}

	var out, errOut bytes.Buffer
	if err := runUnifiedInboxWithPoll(context.Background(), &out, &errOut, "alice", 0, false, false, true, 0, runtime); err != nil {
		t.Fatal(err)
	}
	if fullReads != 1 {
		t.Fatalf("idle watch ran %d full fan-in reads across %d metadata polls, want 1", fullReads, samples)
	}
	want := []time.Duration{10, 20, 40, 40}
	if len(waits) < len(want) {
		t.Fatalf("watch stopped after %d waits, want at least %d", len(waits), len(want))
	}
	for i, duration := range want {
		if waits[i] != duration*time.Millisecond {
			t.Fatalf("wait[%d] = %v, want %v", i, waits[i], duration*time.Millisecond)
		}
	}
	for _, duration := range waits {
		if duration > runtime.max {
			t.Fatalf("idle poll pause %v exceeded delivery bound %v", duration, runtime.max)
		}
	}
	if out.Len() != 0 || errOut.Len() != 0 {
		t.Fatalf("idle watch emitted stdout=%q stderr=%q", out.String(), errOut.String())
	}
}

func TestInboxWatchDeliversChangeWithinBackoffCeiling(t *testing.T) {
	now := time.Unix(200, 0)
	var arrival, delivered time.Time
	var sum uint64 = 1
	waits, fullReads := 0, 0
	var out, errOut bytes.Buffer
	runtime := inboxPollRuntime{
		min: 10 * time.Millisecond, max: 40 * time.Millisecond, fullRescan: time.Hour,
		now: func() time.Time { return now },
		wait: func(_ context.Context, d time.Duration) error {
			waits++
			if waits == 3 {
				// The event lands immediately after a poll at the maximum backoff.
				arrival = now.Add(time.Millisecond)
				sum = 2
			}
			now = now.Add(d)
			if out.Len() > 0 {
				return context.Canceled
			}
			return nil
		},
		snapshot: func(string, int, bool) (inboxBatch, error) {
			fullReads++
			if sum == 1 {
				return inboxBatch{}, nil
			}
			delivered = now
			return inboxBatch{events: []unifiedInboxEvent{{Schema: unifiedInboxSchema, Source: "mb", Seq: 1, Body: "after backoff"}}}, nil
		},
		fingerprint: func(string) (uint64, bool) { return sum, true },
	}

	if err := runUnifiedInboxWithPoll(context.Background(), &out, &errOut, "alice", 0, false, false, true, 0, runtime); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "after backoff") {
		t.Fatalf("changed inbox was not rendered: %q", out.String())
	}
	if latency := delivered.Sub(arrival); latency < 0 || latency > runtime.max {
		t.Fatalf("delivery latency = %v, want within %v", latency, runtime.max)
	}
	if fullReads != 2 {
		t.Fatalf("full reads = %d, want initial read plus changed read", fullReads)
	}
}

func TestInboxPollGateSeesEventThatLandsDuringRead(t *testing.T) {
	var sum uint64 = 1
	gate := inboxPollGate{
		reader: "alice", fullRescan: time.Hour,
		fingerprint: func(string) (uint64, bool) { return sum, true },
	}
	now := time.Unix(300, 0)
	read, _, sampled, ok := gate.due(now)
	if !read || !ok {
		t.Fatal("initial full read was not requested")
	}
	sum = 2 // append after the pre-read sample, before the read commits
	gate.commit(sampled, ok, now)
	read, changed, _, _ := gate.due(now.Add(time.Millisecond))
	if !read || !changed {
		t.Fatal("event appended during a full read was hidden by its commit")
	}
}

func TestInboxPollGateFailsOpenOnFingerprintError(t *testing.T) {
	gate := inboxPollGate{
		reader: "alice", fullRescan: time.Hour,
		fingerprint: func(string) (uint64, bool) { return 0, false },
		sampled:     true, sum: 1, lastFull: time.Now(),
	}
	read, _, _, ok := gate.due(time.Now())
	if !read || ok {
		t.Fatal("an incomplete metadata sample suppressed the safe full read")
	}

	f := sourceFingerprinter{h: fnv.New64a(), ok: true}
	f.file("invalid\x00path")
	if f.ok {
		t.Fatal("a stat error other than not-exist was treated as stable absence")
	}
}

func TestInboxSourceFingerprintMovesForEverySource(t *testing.T) {
	isolateUnifiedInbox(t)
	reader := inboxTestReader
	priorRoles := bus.HostRoles
	holder := "someone-else"
	bus.HostRoles = func() []bus.HostRole {
		return []bus.HostRole{{Label: "conductor:1", Topic: "conductor.1", Holder: holder}}
	}
	t.Cleanup(func() { bus.HostRoles = priorRoles })
	st, err := meet.Create(meet.CreateOptions{Topic: "channel", Board: true, Participants: []string{reader}, Human: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	base, ok := inboxSourcesFingerprint(reader)
	if !ok {
		t.Fatal("fingerprint unavailable on an isolated store")
	}

	for _, step := range []struct {
		name string
		emit func() error
	}{
		{"message board post", func() error {
			return bus.PostMessage(bus.Post{From: "bob", To: reader, Body: "board"})
		}},
		{"bus notification", func() error {
			return bus.Publish(bus.Notification{Principal: "scheduler", To: reader, Body: "bus"})
		}},
		{"meet board record", func() error {
			return meet.AppendEvent(st.ID, meet.Event{Speaker: reader, To: reader, Kind: "status", Text: "meet"})
		}},
		{"legacy role holder", func() error {
			holder = reader
			return nil
		}},
	} {
		if err := step.emit(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		next, ok := inboxSourcesFingerprint(reader)
		if !ok {
			t.Fatalf("%s: fingerprint unavailable", step.name)
		}
		if next == base {
			t.Fatalf("%s did not move the source fingerprint", step.name)
		}
		base = next
	}
}

func TestInboxChangeNotifierSeesNestedMeetWrite(t *testing.T) {
	mbDir, roomDir, meetDir := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("BASHY_MB_DIR", mbDir)
	t.Setenv("BASHY_ROOM_DIR", roomDir)
	t.Setenv("BASHY_MEET_DIR", meetDir)
	roomPath := filepath.Join(meetDir, "standing-room")
	if err := os.Mkdir(roomPath, 0o700); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(roomPath, "transcript.jsonl")
	if err := os.WriteFile(transcript, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	changes := newInboxChangeNotifier()
	t.Cleanup(changes.close)
	before, _ := changes.fingerprint("")
	if err := os.WriteFile(transcript, []byte("new durable event\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForInboxGeneration(t, changes, before)
}

func TestInboxChangeNotifierArmsStoreCreatedAfterStart(t *testing.T) {
	parent := t.TempDir()
	t.Setenv("BASHY_MB_DIR", filepath.Join(parent, "mb"))
	t.Setenv("BASHY_ROOM_DIR", filepath.Join(parent, "room"))
	t.Setenv("BASHY_MEET_DIR", filepath.Join(parent, "meet"))
	changes := newInboxChangeNotifier()
	t.Cleanup(changes.close)
	before, _ := changes.fingerprint("")

	roomPath := filepath.Join(parent, "meet", "new-room")
	if err := os.MkdirAll(roomPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roomPath, "transcript.jsonl"), []byte("first event\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForInboxGeneration(t, changes, before)
}

func waitForInboxGeneration(t *testing.T, changes *inboxChangeNotifier, before uint64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		_ = changes.wait(ctx, 250*time.Millisecond)
		cancel()
		after, _ := changes.fingerprint("")
		if after != before {
			return
		}
	}
	t.Fatal("filesystem change did not advance inbox notification generation")
}

func TestInboxWatchWaitIsCanceledPromptly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := waitInboxPoll(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v, want context canceled", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("canceled wait took %v", elapsed)
	}
}

func BenchmarkInboxSourcesFingerprint(b *testing.B) {
	b.Setenv("BASHY_MB_DIR", b.TempDir())
	b.Setenv("BASHY_ROOM_DIR", b.TempDir())
	b.Setenv("BASHY_FLEET_DIR", b.TempDir())
	b.Setenv("BASHY_MEET_DIR", b.TempDir())
	for i := 0; i < b.N; i++ {
		if _, ok := inboxSourcesFingerprint("alice"); !ok {
			b.Fatal("fingerprint unavailable")
		}
	}
}

func BenchmarkInboxIdleFingerprintLargeMeetStore(b *testing.B) {
	mbDir, roomDir, meetDir := b.TempDir(), b.TempDir(), b.TempDir()
	b.Setenv("BASHY_MB_DIR", mbDir)
	b.Setenv("BASHY_ROOM_DIR", roomDir)
	b.Setenv("BASHY_MEET_DIR", meetDir)
	// Store-shaped fixture measured on the affected host: 92 Meet rooms / 559
	// files plus roughly 110 room routing files. History bytes do not dominate
	// the metadata walk; retained directory entries do.
	for _, subdir := range []string{"pending", "subs"} {
		path := filepath.Join(roomDir, subdir)
		if err := os.Mkdir(path, 0o700); err != nil {
			b.Fatal(err)
		}
		for i := 0; i < 55; i++ {
			if err := os.WriteFile(filepath.Join(path, fmt.Sprintf("route-%03d", i)), nil, 0o600); err != nil {
				b.Fatal(err)
			}
		}
	}
	for i := 0; i < 92; i++ {
		roomPath := filepath.Join(meetDir, fmt.Sprintf("room-%05d", i))
		if err := os.Mkdir(roomPath, 0o700); err != nil {
			b.Fatal(err)
		}
		for _, name := range []string{"state.json", "transcript.jsonl", "lease.json", "summary.json", "members.json", "notes.jsonl"} {
			if err := os.WriteFile(filepath.Join(roomPath, name), nil, 0o600); err != nil {
				b.Fatal(err)
			}
		}
	}

	b.Run("legacy-recursive-stat", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, ok := inboxSourcesFingerprint("alice"); !ok {
				b.Fatal("fingerprint unavailable")
			}
		}
	})
	b.Run("notification-generation", func(b *testing.B) {
		changes := newInboxChangeNotifier()
		b.Cleanup(changes.close)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, ok := changes.fingerprint("alice"); !ok {
				b.Fatal("fingerprint unavailable")
			}
		}
	})
}

func BenchmarkInboxFullSnapshot(b *testing.B) {
	b.Setenv("BASHY_MB_DIR", b.TempDir())
	b.Setenv("BASHY_ROOM_DIR", b.TempDir())
	b.Setenv("BASHY_FLEET_DIR", b.TempDir())
	b.Setenv("BASHY_MEET_DIR", b.TempDir())
	for i := 0; i < b.N; i++ {
		if _, err := snapshotUnifiedInbox("alice", 0, true); err != nil {
			b.Fatal(err)
		}
	}
}

// A LONG WATCH MUST KEEP THE SEAT LIVE, not refresh once and drift.
//
// Measured on sprint #111's own seat: a watch running 1h18m showed its lease
// STALE for 43m — the age of the last BOUNDED command, not of the watch. So an
// agent doing exactly what the tool prints ("`--watch` to stay attached") still
// went stale. That instruction is what replaced the reverted transport gate —
// "reading your inbox is what keeps the seat live" — so a watch that does not
// refresh does not hold up the thing it replaced.
func TestInboxWatchKeepsRefreshingTheSeat(t *testing.T) {
	beatEvery := weave.SprintLeaseTTL / 3
	now := time.Unix(100, 0)
	var beats []time.Time
	prev := refreshSprintOwnerActivity
	refreshSprintOwnerActivity = func(string) { beats = append(beats, now) }
	t.Cleanup(func() { refreshSprintOwnerActivity = prev })

	// Step the clock a third of the beat each tick, so it takes several ticks
	// to earn one beat: a beat per tick would hammer the sprint store, and a
	// beat that never comes is the bug.
	step := beatEvery / 3
	ticks := 0
	runtime := inboxPollRuntime{
		min: step, max: step, fullRescan: time.Hour,
		now: func() time.Time { return now },
		wait: func(_ context.Context, d time.Duration) error {
			ticks++
			now = now.Add(step)
			if ticks == 10 {
				return context.Canceled
			}
			return nil
		},
		snapshot:    func(string, int, bool) (inboxBatch, error) { return inboxBatch{}, nil },
		fingerprint: func(string) (uint64, bool) { return 1, true },
	}

	var out, errOut bytes.Buffer
	if err := runUnifiedInboxWithPoll(context.Background(), &out, &errOut, "alice", 0, false, false, true, 0, runtime); err != nil {
		t.Fatal(err)
	}

	// Ten ticks of a third of a beat is a bit over three beats' worth of time.
	// The exact count is not the property; "more than once" is.
	if len(beats) < 2 {
		t.Fatalf("a watch spanning %v refreshed the seat %d time(s); it must keep "+
			"refreshing or the conductor goes stale while demonstrably attending",
			time.Duration(10)*step, len(beats))
	}
	// And it must NOT beat on every tick: the poll interval can be a second and
	// the sprint store is not something to rewrite at that rate.
	if len(beats) >= ticks {
		t.Fatalf("refreshed %d times in %d ticks — the rate limit is not holding",
			len(beats), ticks)
	}
	for i := 1; i < len(beats); i++ {
		if gap := beats[i].Sub(beats[i-1]); gap < beatEvery {
			t.Fatalf("beats %d and %d are %v apart, want at least %v", i-1, i, gap, beatEvery)
		}
	}
}
