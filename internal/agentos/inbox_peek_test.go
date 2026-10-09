package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/principal"
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

func TestInboxInstanceReaderSeparatesReusedLabel(t *testing.T) {
	isolateUnifiedInbox(t)
	t.Setenv("BASHY_MAILBOX_DIR", t.TempDir())
	store := fleet.NewInstanceStore("")
	family := fleet.Family{Name: "test-family", Bindings: []string{"claude:test"}}
	old, err := store.Open(family, fleet.OpenOptions{Label: "Iris"})
	if err != nil {
		t.Fatal(err)
	}
	if err := bus.PostMessage(bus.Post{From: "human", To: old.MailAddress(), Body: "old private mail"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Retire(old.UUID, func(fleet.Instance) ([]string, error) { return []string{"old private mail"}, nil }); err != nil {
		t.Fatal(err)
	}
	fresh, err := store.Open(family, fleet.OpenOptions{Label: "Iris"})
	if err != nil {
		t.Fatal(err)
	}
	if err := bus.PostMessage(bus.Post{From: "human", To: fresh.MailAddress(), Body: "new private mail"}); err != nil {
		t.Fatal(err)
	}
	other, err := store.Open(family, fleet.OpenOptions{Label: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	for _, stamp := range []string{"principal", "instance"} {
		t.Run(stamp, func(t *testing.T) {
			t.Setenv("BASHY_PRINCIPAL", "")
			t.Setenv("BASHY_INSTANCE", "")
			if stamp == "principal" {
				t.Setenv("BASHY_PRINCIPAL", principal.InstanceURN(fresh.UUID))
			} else {
				t.Setenv("BASHY_INSTANCE", fresh.UUID)
			}
			got, err := resolveInboxReader("")
			if err != nil || got != fresh.MailAddress() {
				t.Fatalf("reader = %q, %v", got, err)
			}
			for _, args := range [][]string{nil, {"--json"}, {"--search", "private"}, {"list", "--json"}} {
				out, _, err := runInboxCmd(t, args...)
				if err != nil || !strings.Contains(out, "new private mail") || strings.Contains(out, "old private mail") {
					t.Fatalf("%v: %s; %v", args, out, err)
				}
			}
			if _, err := resolveInboxReader(other.MailAddress()); err == nil {
				t.Error("stamped instance could borrow another live UUID mailbox")
			}
			if _, err := resolveInboxReader(old.MailAddress()); err == nil {
				t.Error("stamped instance could read the old UUID with --as")
			}
			if got, err := resolveInboxReader(fresh.MailAddress()); err != nil || got != fresh.MailAddress() {
				t.Errorf("own explicit address = %q, %v", got, err)
			}
			if bus.SeenSeq(old.MailAddress()) != 0 || bus.SeenSeq(fresh.MailAddress()) != 0 {
				t.Error("peek advanced instance cursor")
			}
		})
	}
	archived, err := store.ArchivedMail(old.UUID)
	if err != nil || len(archived) != 1 {
		t.Fatalf("archive changed: %v, %v", archived, err)
	}
}

func TestInboxTUIInstanceSenderAttribution(t *testing.T) {
	isolateUnifiedInbox(t)
	t.Setenv("BASHY_MAILBOX_DIR", t.TempDir())
	store := fleet.NewInstanceStore("")
	sender, err := store.Open(fleet.Family{Name: "test-family", Bindings: []string{"claude:test"}}, fleet.OpenOptions{Label: "Iris", Handle: "iris-handle"})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_INSTANCE", sender.UUID)
	t.Setenv(bus.SelectedBindingEnv, "claude:test")
	// The TUI's authored-send path passes its display label; the stamped
	// session makes Send persist the canonical sender snapshot.
	if _, err := bus.Send(bus.SendRequest{From: sender.Label, To: inboxTestReader, Body: "TUI authored message"}); err != nil {
		t.Fatal(err)
	}
	// Reusing the display label cannot rewrite the TUI post's provenance.
	if _, err := store.Retire(sender.UUID, func(fleet.Instance) ([]string, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(fleet.Family{Name: "replacement-family", Bindings: []string{"codex:other"}}, fleet.OpenOptions{Label: sender.Label, Handle: "replacement-handle"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_INSTANCE", "")
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/"+inboxTestReader)
	for _, command := range [][]string{nil, {"list"}} {
		for _, filter := range []string{"", "IRIS-HANDLE", "test-family", "claude:test", sender.UUID} {
			args := append([]string{}, command...)
			if filter != "" {
				args = append(args, "--from", filter)
			}
			out, _, err := runInboxCmd(t, args...)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"TUI authored message", "iris-handle", "test-family", "claude:test", sender.UUID} {
				if !strings.Contains(out, want) {
					t.Errorf("%v omitted %q: %s", args, want, out)
				}
			}
		}
		out, _, err := runInboxCmd(t, append(command, "--json")...)
		if err != nil {
			t.Fatal(err)
		}
		// Both views retain their JSON-lines schema.
		var event map[string]json.RawMessage
		err = json.Unmarshal([]byte(out), &event)
		if err != nil {
			t.Fatal(err)
		}
		var party bus.Party
		if err := json.Unmarshal(event["from_party"], &party); err != nil || party.UUID != sender.UUID || party.FamilyID != sender.FamilyID || party.Selected != "claude:test" {
			t.Errorf("sender snapshot missing: %s, %v", out, err)
		}
		if string(event["from"]) != `"Iris"` || string(event["from_handle"]) != `"iris-handle"` {
			t.Errorf("compatibility/handle: %s", out)
		}
	}
	if bus.SeenSeq(inboxTestReader) != 0 {
		t.Error("sender search consumed mail")
	}
}

func TestInboxRoleMailRequiresAuthorization(t *testing.T) {
	isolateUnifiedInbox(t)
	t.Setenv("BASHY_MAILBOX_DIR", t.TempDir())
	const topic = "conductor.83"
	priorRoles, priorAuth := bus.HostRoles, bus.RoleReaderAuthorizer
	t.Cleanup(func() { bus.HostRoles, bus.RoleReaderAuthorizer = priorRoles, priorAuth })
	bus.HostRoles = func() []bus.HostRole {
		return []bus.HostRole{{Label: "conductor:83", Topic: topic, Holder: inboxTestReader}}
	}
	if err := bus.PostMessage(bus.Post{From: "human", To: topic, Body: "role board mail"}); err != nil {
		t.Fatal(err)
	}
	if _, err := bus.EnsureRoleInbox(topic); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(bus.Notification{Principal: "human", To: topic, Body: "role legacy mail"}); err != nil {
		t.Fatal(err)
	}
	if _, err := bus.ResolveFor(topic); err != nil {
		t.Fatal(err)
	}
	for _, allowed := range []bool{false, true} {
		bus.RoleReaderAuthorizer = func(to, reader string) bool { return allowed && to == topic && reader == inboxTestReader }
		for _, args := range [][]string{{"--as", inboxTestReader}, {"list", "--as", inboxTestReader}} {
			out, _, err := runInboxCmd(t, args...)
			if err != nil {
				t.Fatal(err)
			}
			for _, body := range []string{"role board mail", "role legacy mail"} {
				if strings.Contains(out, body) != allowed {
					t.Errorf("allowed=%v args=%v: %s", allowed, args, out)
				}
			}
		}
	}
	pending, err := bus.SnapshotInbox(topic)
	if err != nil || len(pending.Items) != 1 || bus.SeenSeq(inboxTestReader) != 0 {
		t.Fatalf("role peek changed state: %v, %v", pending, err)
	}
}
