package agentos

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/meet"
)

func TestUnifiedTurnAdmissionAcknowledgesOnlyRenderedRoleAndBusRecords(t *testing.T) {
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
			if strings.HasPrefix(line, "- [P") && strings.Contains(line, prefix) {
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

func TestSprintInboxAckCommitsRenderedRoleBatchOnly(t *testing.T) {
	isolateUnifiedInbox(t)
	const topic = "conductor.408"
	prior := bus.HostRoles
	bus.HostRoles = func() []bus.HostRole {
		return []bus.HostRole{{Label: "conductor:408", Topic: topic, Holder: inboxTestReader}}
	}
	t.Cleanup(func() { bus.HostRoles = prior })
	for i := 0; i < 12; i++ {
		if err := bus.Publish(bus.Notification{Principal: "human", To: topic, Body: fmt.Sprintf("role %d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	reads := 0
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sawNew := false
	rt := sprintWatchRuntime{ackEvery: time.Second}
	rt.poll = sprintWatchTestPoll(func(reader string, limit int, includeBus bool) (inboxBatch, error) {
		batch, err := snapshotUnifiedInbox(reader, limit, includeBus)
		if err == nil && len(batch.events) == 1 && batch.events[0].Body == "new after rendered batch" {
			sawNew = true
			cancel()
		}
		return batch, err
	})
	rt.ackSeq = func(int64, string) (int64, error) {
		reads++
		if reads == 2 {
			if err := bus.Publish(bus.Notification{Principal: "human", To: topic, Body: "new after rendered batch"}); err != nil {
				return 0, err
			}
			return 1, nil
		}
		if reads > 2 {
			return 1, nil
		}
		return 0, nil
	}
	var out bytes.Buffer
	if err := runSprintInboxWatch(ctx, &out, &bytes.Buffer{}, 408, inboxTestReader, rt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "role 0") || !strings.Contains(out.String(), "role 11") {
		t.Fatalf("watch did not render the batch: %s", out.String())
	}
	if !sawNew {
		t.Fatalf("watch did not finish acknowledging the rendered batch: %s", out.String())
	}
	again, err := snapshotUnifiedInbox(inboxTestReader, 0, true)
	if err != nil || len(again.events) != 1 || again.events[0].Body != "new after rendered batch" {
		t.Fatalf("sprint ack repeated old role input or consumed new input: %+v, %v", again.events, err)
	}
}

func TestUnifiedTurnAdmissionAcknowledgesMeetCopyWithItsMBOrigin(t *testing.T) {
	isolateUnifiedInbox(t)
	wireMessageBoard()
	wireMeet()
	if err := bus.PostMessage(bus.Post{From: "human", To: inboxTestReader, Body: "original"}); err != nil {
		t.Fatal(err)
	}
	st, err := meet.Create(meet.CreateOptions{Topic: "seeded", Board: true, Participants: []string{inboxTestReader}, Human: "human"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := meet.SeedBoardFromMB(st, []int64{1}); err != nil {
		t.Fatal(err)
	}
	if err := meet.AppendEvent(st.ID, meet.Event{Speaker: "human", Kind: "message", Text: "independent"}); err != nil {
		t.Fatal(err)
	}
	prepared := bus.PrepareLaunchPreamble(inboxTestReader)
	if err := prepared.Err(); err != nil {
		t.Fatal(err)
	}
	if strings.Count(prepared.Text, "original") != 1 || !strings.Contains(prepared.Text, "independent") {
		t.Fatalf("turn did not preserve provenance: %s", prepared.Text)
	}
	if err := prepared.Commit(); err != nil {
		t.Fatal(err)
	}
	again, err := snapshotUnifiedInbox(inboxTestReader, 0, true)
	if err != nil || len(again.events) != 0 {
		t.Fatalf("copied or independent message repeated: %+v, %v", again.events, err)
	}
	if bus.SeenSeq(inboxTestReader) == 0 || meet.SeenSeq(st.ID, inboxTestReader) != 2 {
		t.Fatalf("source cursors did not advance: mb=%d meet=%d", bus.SeenSeq(inboxTestReader), meet.SeenSeq(st.ID, inboxTestReader))
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

func TestCappedWatchAcknowledgesExactlyDisplayedRoleAndBusRecords(t *testing.T) {
	isolateUnifiedInbox(t)
	wireMessageBoard()
	const topic = "conductor.408"
	prior := bus.HostRoles
	bus.HostRoles = func() []bus.HostRole {
		return []bus.HostRole{{Label: "conductor:408", Topic: topic, Holder: inboxTestReader}}
	}
	t.Cleanup(func() { bus.HostRoles = prior })
	for i := 0; i < 12; i++ {
		if err := bus.Publish(bus.Notification{Principal: "human", To: topic, Body: fmt.Sprintf("role-%02d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 5; i++ {
		if err := bus.Publish(bus.Notification{Principal: "human", To: inboxTestReader, Body: fmt.Sprintf("direct-%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	peek, err := snapshotInbox(inboxTestReader, 2, true, false)
	if err != nil || len(peek.events) != 4 {
		t.Fatalf("capped snapshot: %q, %v", cappedBodies(peek.events), err)
	}
	again, err := snapshotInbox(inboxTestReader, 2, true, false)
	if err != nil || cappedBodies(again.events) != cappedBodies(peek.events) {
		t.Fatalf("peek consumed mail: %q then %q, %v", cappedBodies(peek.events), cappedBodies(again.events), err)
	}

	// Sprint inbox watch: render the capped batch, acknowledge on the manager's
	// acknowledgement, then a record lands before the next read.
	reads := 0
	rt := sprintWatchRuntime{ackEvery: time.Second}
	calls := 0
	rt.poll = sprintWatchTestPoll(func(reader string, _ int, includeBus bool) (inboxBatch, error) {
		calls++
		if calls > 1 {
			return inboxBatch{}, nil
		}
		return snapshotInbox(reader, 2, includeBus, false)
	})
	rt.ackSeq = func(int64, string) (int64, error) {
		reads++
		if reads == 2 {
			if err := bus.Publish(bus.Notification{Principal: "human", To: topic, Body: "role-late"}); err != nil {
				return 0, err
			}
			return 1, nil
		}
		if reads > 2 {
			return 1, nil
		}
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	var out bytes.Buffer
	if err := runSprintInboxWatch(ctx, &out, &bytes.Buffer{}, 408, inboxTestReader, rt); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"role-00", "role-01"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("watch did not render %s: %s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "role-02") {
		t.Fatalf("watch rendered beyond the cap: %s", out.String())
	}
	next, err := snapshotInbox(inboxTestReader, 0, true, false)
	if err != nil {
		t.Fatal(err)
	}
	var roles, directs []string
	for _, e := range next.events {
		if strings.HasPrefix(e.Body, "role-") {
			roles = append(roles, e.Body)
		} else {
			directs = append(directs, e.Body)
		}
	}
	if len(roles) != 11 || roles[0] != "role-02" || roles[10] != "role-late" {
		t.Fatalf("role tail/late mail wrong after capped ack: %v", roles)
	}
	if len(directs) != 3 {
		t.Fatalf("bus tail after capped ack = %v, want only the 3 undisplayed records", directs)
	}
	batch2, err := snapshotInbox(inboxTestReader, 2, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cappedBodies(batch2.events), "role-00") || strings.Contains(cappedBodies(batch2.events), "role-01") {
		t.Fatalf("next batch repeated displayed records: %q", cappedBodies(batch2.events))
	}
}

func TestCappedWatchAcknowledgesExactlyDisplayedMeetRecords(t *testing.T) {
	isolateUnifiedInbox(t)
	wireMessageBoard()
	wireMeet()
	st, err := meet.Create(meet.CreateOptions{Topic: "capped", Board: true, Participants: []string{inboxTestReader}, Human: "human"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		if err := meet.AppendEvent(st.ID, meet.Event{Speaker: "human", Kind: "message", Text: fmt.Sprintf("meet-%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	watchOneBatch(t, 2, &out)
	if !strings.Contains(out.String(), "meet-1") || !strings.Contains(out.String(), "meet-2") || strings.Contains(out.String(), "meet-3") {
		t.Fatalf("watch rendered the wrong window: %s", out.String())
	}
	if err := meet.AppendEvent(st.ID, meet.Event{Speaker: "human", Kind: "message", Text: "meet-late"}); err != nil {
		t.Fatal(err)
	}
	rest, err := snapshotInbox(inboxTestReader, 0, false, false)
	if err != nil || cappedBodies(rest.events) != "meet-3,meet-4,meet-late" {
		t.Fatalf("meet tail or late mail wrong: %q, %v", cappedBodies(rest.events), err)
	}
}
