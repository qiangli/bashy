package agentos

import (
	"bytes"
	"context"
	"errors"
	"hash/fnv"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/meet"
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
	reader := "codex-gpt5.6-sol"
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
			_, err := meet.PostAs(st.ID, reader, reader, "meet")
			return err
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
