package main

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/bus"
)

func TestUnifiedTurnAdmissionAcknowledgesOnlyRenderedRoleAndBusRecords(t *testing.T) {
	// These tests cover the byte-budget admission and provenance, not the
	// per-turn item cap (yoke bus turn_cap_test.go); lift the cap so every
	// admitted record is rendered (default: 1 item + a counter, yoke f4a1c2f2).
	t.Setenv("BASHY_CHAT_INBOX_MAX", "100")
	isolateUnifiedInbox(t)
	wireMessageBoard()
	const roleTopic = "conductor.408"
	prior := bus.HostRoles
	bus.HostRoles = func() []bus.HostRole {
		return []bus.HostRole{{Label: "conductor:408", Topic: roleTopic, Holder: inboxTestReader}}
	}
	t.Cleanup(func() { bus.HostRoles = prior })
	for i := 0; i < 12; i++ {
		if err := bus.Publish(bus.Notification{Principal: "human", To: roleTopic, Body: fmt.Sprintf("role message %02d %s", i, strings.Repeat("x", 90))}); err != nil {
			t.Fatal(err)
		}
	}
	if err := bus.Publish(bus.Notification{Principal: "human", To: inboxTestReader, Body: "direct bus message"}); err != nil {
		t.Fatal(err)
	}
	initial, err := snapshotUnifiedInbox(inboxTestReader, 0, true)
	if err != nil || len(initial.events) != 13 {
		t.Fatalf("initial snapshot: %d records, %v", len(initial.events), err)
	}
	prepared := bus.PrepareLaunchPreamble(inboxTestReader)
	if err := prepared.Err(); err != nil {
		t.Fatal(err)
	}
	if len(prepared.Text) > bus.DefaultTurnAdmissionBytes {
		t.Fatalf("turn input exceeds PTY budget: %d bytes", len(prepared.Text))
	}
	if n := prepared.AdmissionReport().AdmittedItems; n == 0 || n >= 13 {
		t.Fatalf("want a bounded partial batch, admitted %d: %s", n, prepared.Text)
	}
	// Preparation is a peek until the destination accepts the text.
	peek, err := snapshotUnifiedInbox(inboxTestReader, 0, true)
	if err != nil || len(peek.events) != 13 {
		t.Fatalf("preparation consumed mail: %d records, %v", len(peek.events), err)
	}
	if err := bus.Publish(bus.Notification{Principal: "human", To: roleTopic, Body: "newer unshown input"}); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Commit(); err != nil {
		t.Fatal(err)
	}
	again, err := snapshotUnifiedInbox(inboxTestReader, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	remaining := map[string]bool{}
	for _, event := range again.events {
		remaining[turnInboxID(event)] = true
	}
	for _, event := range initial.events {
		prefix := event.Source + ":" + strconv.FormatInt(event.Seq, 10)
		shown := false
		for _, line := range strings.Split(prepared.Text, "\n") {
			if strings.HasPrefix(line, "- [P") && strings.Contains(line, prefix+" ") { // ":1 " must not match ":10"
				shown = true
			}
		}
		if remaining[turnInboxID(event)] == shown {
			t.Fatalf("ack mismatch for %s (shown=%v remaining=%v)", turnInboxID(event), shown, remaining[turnInboxID(event)])
		}
	}
	if !strings.Contains(fmt.Sprint(again.events), "newer unshown input") {
		t.Fatalf("new input vanished: %+v", again.events)
	}
}

func TestUnifiedTurnItemAckIsPrefixMonotonicAndKeepsLateMail(t *testing.T) {
	isolateUnifiedInbox(t)
	wireMessageBoard()
	for i := 1; i <= 3; i++ {
		if err := bus.PostMessage(bus.Post{From: "human", To: inboxTestReader, Body: fmt.Sprintf("post %d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	// --limit keeps the tail unread; its acks are per rendered record and a
	// snapshot (peek) never applies them.
	capped, err := snapshotInbox(inboxTestReader, 2, false, false)
	if err != nil || len(capped.events) != 2 || len(capped.acks) != 2 {
		t.Fatalf("capped snapshot: events=%d acks=%d err=%v", len(capped.events), len(capped.acks), err)
	}
	if got := bus.SeenSeq(inboxTestReader); got != 0 {
		t.Fatalf("capped peek consumed mail: cursor %d", got)
	}
	items, err := unifiedTurnItems(inboxTestReader)
	if err != nil || len(items) != 3 {
		t.Fatalf("items: %d, %v", len(items), err)
	}
	if err := bus.PostMessage(bus.Post{From: "human", To: inboxTestReader, Body: "late"}); err != nil {
		t.Fatal(err)
	}
	// A later record acknowledged first must not move the cursor past the gap.
	if err := items[2].Acknowledge(); err != nil {
		t.Fatal(err)
	}
	if got := bus.SeenSeq(inboxTestReader); got != 0 {
		t.Fatalf("cursor jumped past unacknowledged records: %d", got)
	}
	if err := items[0].Acknowledge(); err != nil {
		t.Fatal(err)
	}
	if got := bus.SeenSeq(inboxTestReader); got != 1 {
		t.Fatalf("cursor after first ack = %d, want 1", got)
	}
	if err := items[1].Acknowledge(); err != nil {
		t.Fatal(err)
	}
	if got := bus.SeenSeq(inboxTestReader); got != 3 {
		t.Fatalf("cursor after contiguous prefix = %d, want 3", got)
	}
	// A stale acknowledgement replays harmlessly and never regresses or overreaches.
	if err := items[0].Acknowledge(); err != nil {
		t.Fatal(err)
	}
	if got := bus.SeenSeq(inboxTestReader); got != 3 {
		t.Fatalf("stale ack moved cursor to %d", got)
	}
	again, err := snapshotInbox(inboxTestReader, 0, false, false)
	if err != nil || len(again.events) != 1 || again.events[0].Body != "late" {
		t.Fatalf("late mail was not preserved: %+v, %v", again.events, err)
	}
}

func cappedBodies(events []unifiedInboxEvent) string {
	bodies := make([]string, 0, len(events))
	for _, e := range events {
		bodies = append(bodies, e.Body)
	}
	return strings.Join(bodies, ",")
}

// watchOneBatch runs the real watch loop (render, then acknowledge) for exactly
// the first snapshot; every later poll sees an empty inbox so the test controls
// which batches are consumed.
func watchOneBatch(t *testing.T, limit int, out *bytes.Buffer) {
	t.Helper()
	calls := 0
	poll := sprintWatchTestPoll(func(reader string, _ int, includeBus bool) (inboxBatch, error) {
		calls++
		if calls > 1 {
			return inboxBatch{}, nil
		}
		return snapshotInbox(reader, limit, includeBus, false)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if err := runUnifiedInboxWithPoll(ctx, out, &bytes.Buffer{}, inboxTestReader, limit, false, false, true, 0, poll); err != nil {
		t.Fatal(err)
	}
}

func TestCappedWatchAcknowledgesExactlyDisplayedMBRecords(t *testing.T) {
	isolateUnifiedInbox(t)
	wireMessageBoard()
	for i := 1; i <= 12; i++ {
		if err := bus.PostMessage(bus.Post{From: "human", To: inboxTestReader, Body: fmt.Sprintf("post-%02d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	// A peek, however often repeated, never consumes the capped window.
	for i := 0; i < 2; i++ {
		peek, err := snapshotInbox(inboxTestReader, 2, false, false)
		if err != nil || cappedBodies(peek.events) != "post-01,post-02" || bus.SeenSeq(inboxTestReader) != 0 {
			t.Fatalf("peek %d: %q cursor=%d err=%v", i, cappedBodies(peek.events), bus.SeenSeq(inboxTestReader), err)
		}
	}
	var out bytes.Buffer
	watchOneBatch(t, 2, &out)
	if !strings.Contains(out.String(), "post-01") || !strings.Contains(out.String(), "post-02") || strings.Contains(out.String(), "post-03") {
		t.Fatalf("watch rendered the wrong window: %s", out.String())
	}
	if got := bus.SeenSeq(inboxTestReader); got != 2 {
		t.Fatalf("cursor = %d, want exactly the displayed prefix 2 (not the full-source watermark)", got)
	}
	if err := bus.PostMessage(bus.Post{From: "human", To: inboxTestReader, Body: "post-late"}); err != nil {
		t.Fatal(err)
	}
	next, err := snapshotInbox(inboxTestReader, 2, false, false)
	if err != nil || cappedBodies(next.events) != "post-03,post-04" {
		t.Fatalf("next batch repeated the first two or skipped the tail: %q, %v", cappedBodies(next.events), err)
	}
	if err := acknowledgeInboxBatch(next); err != nil {
		t.Fatal(err)
	}
	if got := bus.SeenSeq(inboxTestReader); got != 4 {
		t.Fatalf("cursor after second batch = %d, want 4", got)
	}
	rest, err := snapshotInbox(inboxTestReader, 0, false, false)
	if err != nil || len(rest.events) != 9 || rest.events[0].Body != "post-05" || rest.events[8].Body != "post-late" {
		t.Fatalf("tail or late mail lost: %q, %v", cappedBodies(rest.events), err)
	}
}
