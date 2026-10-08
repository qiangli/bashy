package agentos

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/weave"
)

func runInboxCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := newUnifiedInboxCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetContext(context.Background())
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestInboxBareReadNeverConsumes(t *testing.T) {
	isolateUnifiedInbox(t)
	t.Setenv("BASHY_MAILBOX_DIR", t.TempDir())
	if err := bus.PostMessage(bus.Post{From: "human", To: inboxTestReader, Body: "board message"}); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(bus.Notification{Principal: "scheduler", To: inboxTestReader, Body: "bus notification"}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--as", inboxTestReader},
		{"--as", inboxTestReader, "--json"},
		{"--as", inboxTestReader, "--wait", "1s"},
		{"--as", inboxTestReader, "--wait", "1s", "--json"},
	} {
		out, _, err := runInboxCmd(t, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		for _, want := range []string{"board message", "bus notification"} {
			if !strings.Contains(out, want) {
				t.Fatalf("%v omitted %q: %q", args, want, out)
			}
		}
		if bus.SeenSeq(inboxTestReader) != 0 {
			t.Fatalf("%v advanced the message-board cursor", args)
		}
		if got, _, err := bus.UnreadNotifications(inboxTestReader); err != nil || len(got) != 1 {
			t.Fatalf("%v consumed bus pending: len=%d err=%v", args, len(got), err)
		}
		_, state, err := snapshotMailbox(mailboxSpec{Key: "agent:" + inboxTestReader, Address: inboxTestReader, Kind: "agent"})
		if err != nil || len(state.Marks) != 0 {
			t.Fatalf("%v changed mailbox state: marks=%v err=%v", args, state.Marks, err)
		}
	}
}

func TestInboxWatchStillAcknowledgesAfterDelivery(t *testing.T) {
	isolateUnifiedInbox(t)
	if err := bus.PostMessage(bus.Post{From: "human", To: inboxTestReader, Body: "watch me"}); err != nil {
		t.Fatal(err)
	}
	out, _, err := runInboxCmd(t, "--as", inboxTestReader, "--watch", "--wait", "1s")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "watch me") {
		t.Fatalf("watch delivered nothing: %q", out)
	}
	if bus.SeenSeq(inboxTestReader) == 0 {
		t.Fatal("managed watch delivery did not acknowledge the delivered record")
	}
}

func TestInboxStreamIsChronologicalByParsedTime(t *testing.T) {
	isolateUnifiedInbox(t)
	// String order would put the +02:00 record last; by instant it is first.
	for _, p := range []bus.Post{
		{From: "carol", To: inboxTestReader, Body: "third", At: "2026-10-08T09:30:00Z"},
		{From: "alice", To: inboxTestReader, Body: "first", At: "2026-10-08T10:00:00.5+02:00"},
		{From: "bob", To: inboxTestReader, Body: "second", At: "2026-10-08T09:00:00Z"},
	} {
		if err := bus.PostMessage(p); err != nil {
			t.Fatal(err)
		}
	}
	out, _, err := runInboxCmd(t, "--as", inboxTestReader)
	if err != nil {
		t.Fatal(err)
	}
	i1, i2, i3 := strings.Index(out, "first"), strings.Index(out, "second"), strings.Index(out, "third")
	if i1 < 0 || !(i1 < i2 && i2 < i3) {
		t.Fatalf("stream not chronological, newest last:\n%s", out)
	}
	if !strings.Contains(out, "[mb:") || !strings.Contains(out, "alice") {
		t.Fatalf("source/sender columns missing:\n%s", out)
	}
}

func TestSortInboxEventsMixedSources(t *testing.T) {
	events := []unifiedInboxEvent{
		{Source: "bus", Seq: 1, At: "2026-10-08T10:00:00Z"},
		{Source: "meet", Seq: 4, At: "2026-10-08T09:59:59.999999999Z"},
		{Source: "mb", Seq: 9, At: ""},
		{Source: "mb", Seq: 2, At: "2026-10-08T10:00:00Z"},
	}
	sortInboxEvents(events)
	got := []string{}
	for _, e := range events {
		got = append(got, e.Source)
	}
	if strings.Join(got, ",") != "mb,meet,bus,mb" {
		// untimed first, then by instant, ties by source ("bus" < "mb")
		t.Fatalf("order = %v", got)
	}
}

func TestInboxFromSearchSinceFilters(t *testing.T) {
	isolateUnifiedInbox(t)
	t.Setenv("BASHY_MAILBOX_DIR", t.TempDir())
	recent := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	old := time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339)
	for _, p := range []bus.Post{
		{From: "alice-bot", To: inboxTestReader, Body: "profile ready", At: recent},
		{From: "bob", To: inboxTestReader, Body: "unrelated note", At: recent},
		{From: "alice-bot", To: inboxTestReader, Body: "profile stale", At: old},
	} {
		if err := bus.PostMessage(p); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		args    []string
		want    []string
		notWant []string
	}{
		{[]string{"--from", "ALICE"}, []string{"profile ready", "profile stale"}, []string{"unrelated"}},
		{[]string{"--search", "unrelated"}, []string{"unrelated note"}, []string{"profile"}},
		{[]string{"--since", "2h"}, []string{"profile ready", "unrelated note"}, []string{"stale"}},
		{[]string{"--since", "1d", "--from", "alice"}, []string{"profile ready"}, []string{"stale", "unrelated"}},
	}
	for _, c := range cases {
		out, _, err := runInboxCmd(t, append([]string{"--as", inboxTestReader}, c.args...)...)
		if err != nil {
			t.Fatalf("%v: %v", c.args, err)
		}
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Fatalf("%v omitted %q:\n%s", c.args, w, out)
			}
		}
		for _, w := range c.notWant {
			if strings.Contains(out, w) {
				t.Fatalf("%v leaked %q:\n%s", c.args, w, out)
			}
		}
	}
	if bus.SeenSeq(inboxTestReader) != 0 {
		t.Fatal("filtered read advanced the cursor")
	}
	if _, _, err := runInboxCmd(t, "--as", inboxTestReader, "--since", "nonsense"); err == nil {
		t.Fatal("bad --since accepted")
	}
	if _, _, err := runInboxCmd(t, "--as", inboxTestReader, "--watch", "--from", "alice"); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("watch+filter error = %v", err)
	}
}

func TestInboxSyncStampOmitsDeliveredZero(t *testing.T) {
	isolateUnifiedInbox(t)
	deliverSessionMail = func(context.Context, string, string) (weave.DeliveryReport, error) {
		return weave.DeliveryReport{Session: "0192f3a4-7c1e-7000-8000-000000000001", Cursor: "c9"}, nil
	}
	batch, err := snapshotUnifiedInbox(inboxTestReader, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range batch.warns {
		if strings.Contains(w, "delivered") {
			t.Fatalf("delivered-0 footer still printed: %q", w)
		}
	}
	if len(batch.warns) != 1 || !strings.Contains(batch.warns[0], "synced as of") {
		t.Fatalf("sync stamp lost: %v", batch.warns)
	}
}

func TestParseInboxSince(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	for raw, want := range map[string]time.Time{
		"2h":                   now.Add(-2 * time.Hour),
		"3d":                   now.Add(-72 * time.Hour),
		"2026-10-01":           time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		"2026-10-07T01:02:03Z": time.Date(2026, 10, 7, 1, 2, 3, 0, time.UTC),
	} {
		got, err := parseInboxSince(raw, now)
		if err != nil || !got.Equal(want) {
			t.Fatalf("parseInboxSince(%q) = %v, %v; want %v", raw, got, err, want)
		}
	}
	for _, raw := range []string{"soon", "-1h", "xd", "1.5d"} {
		if _, err := parseInboxSince(raw, now); err == nil {
			t.Fatalf("parseInboxSince(%q) accepted", raw)
		}
	}
}
