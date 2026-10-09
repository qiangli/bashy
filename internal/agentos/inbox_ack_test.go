package agentos

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/meet"
)

func TestInboxExplicitAckRemovesMBFromPeekJSON(t *testing.T) {
	isolateUnifiedInbox(t)
	for _, body := range []string{"first directed message", "second directed message"} {
		if err := bus.PostMessage(bus.Post{From: "human", To: inboxTestReader, Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	out, _, err := runInboxCmd(t, "ack", "--as", inboxTestReader, "mb:1")
	if err != nil || !strings.Contains(out, "[mb:1]") || !strings.Contains(out, "[acked]") {
		t.Fatalf("ack = %q, %v", out, err)
	}
	out, _, err = runInboxCmd(t, "--as", inboxTestReader, "--peek", "--json")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Fatalf("want only mb:2 after ack mb:1; got %s", out)
	}
	var event unifiedInboxEvent
	if err := json.Unmarshal([]byte(lines[0]), &event); err != nil || event.Source != "mb" || event.Seq != 2 {
		t.Fatalf("pending event = %+v, %v", event, err)
	}
}

func TestInboxExplicitAckKeepsOtherRecordsPending(t *testing.T) {
	for _, source := range []string{"mb", "bus", "meet"} {
		t.Run(source, func(t *testing.T) {
			isolateUnifiedInbox(t)
			roomID := ""
			if source == "meet" {
				st, err := meet.Create(meet.CreateOptions{Topic: "ack test", Board: true, Participants: []string{inboxTestReader}, Human: "human"})
				if err != nil {
					t.Fatal(err)
				}
				roomID = st.ID
			}
			for _, body := range []string{"before ack", "ack target", "after ack"} {
				var err error
				switch source {
				case "mb":
					err = bus.PostMessage(bus.Post{From: "human", To: inboxTestReader, Body: body})
				case "bus":
					err = bus.Publish(bus.Notification{Principal: "human", To: inboxTestReader, Body: body})
				case "meet":
					err = meet.AppendEvent(roomID, meet.Event{Speaker: "human", To: inboxTestReader, Kind: "message", Text: body})
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			batch, err := snapshotUnifiedInbox(inboxTestReader, 0, true)
			if err != nil || len(batch.events) != 3 {
				t.Fatalf("initial snapshot = %+v, %v", batch.events, err)
			}
			ids := make([]string, 0, 3)
			for _, e := range batch.events {
				prefix := e.Source
				if source == "meet" {
					prefix += ":" + e.Room
				}
				ids = append(ids, fmt.Sprintf("%s:%d", prefix, e.Seq))
			}
			if _, _, err := runInboxCmd(t, "ack", "--as", inboxTestReader, ids[1]); err != nil {
				t.Fatal(err)
			}
			for _, flags := range [][]string{nil, {"--json"}, {"--peek", "--json"}, {"--limit", "1"}} {
				out, _, err := runInboxCmd(t, append([]string{"--as", inboxTestReader}, flags...)...)
				if err != nil || strings.Contains(out, "ack target") || !strings.Contains(out, "before ack") {
					t.Fatalf("%v: pending view = %q, %v", flags, out, err)
				}
				if len(flags) == 0 && !strings.Contains(out, "after ack") {
					t.Fatalf("lost later record: %s", out)
				}
			}
			batch, err = snapshotUnifiedInbox(inboxTestReader, 0, true)
			if err != nil {
				t.Fatal(err)
			}
			if sum, ok := summarizeUnreadHint(batch); !ok || sum.count != 2 {
				t.Fatalf("unread summary after ack = %+v, %v", sum, ok)
			}
			if bus.SeenSeq(inboxTestReader) != 0 || (roomID != "" && meet.SeenSeq(roomID, inboxTestReader) != 0) {
				t.Fatal("explicit ack or ordinary reads advanced a source cursor")
			}
			if _, _, err := runInboxCmd(t, "ack", "--as", inboxTestReader, ids[0], ids[2]); err != nil {
				t.Fatal(err)
			}
			batch, err = snapshotUnifiedInbox(inboxTestReader, 0, true)
			if err != nil || len(batch.events) != 0 {
				t.Fatalf("all acked snapshot = %+v, %v", batch.events, err)
			}
			if sum, ok := summarizeUnreadHint(batch); ok {
				t.Fatalf("acked inbox still hints: %s", formatUnreadHint(sum))
			}
			// Reopening just one record must also be reflected in the stream.
			if _, _, err := runInboxCmd(t, "preserve", "--as", inboxTestReader, ids[1]); err != nil {
				t.Fatal(err)
			}
			out, _, err := runInboxCmd(t, "--as", inboxTestReader, "--watch", "--wait", "1s")
			if err != nil || !strings.Contains(out, "ack target") || strings.Contains(out, "before ack") || strings.Contains(out, "after ack") {
				t.Fatalf("watch = %q, %v", out, err)
			}
			batch, err = snapshotUnifiedInbox(inboxTestReader, 0, true)
			if err != nil || len(batch.events) != 0 {
				t.Fatalf("watch did not acknowledge delivery: %+v, %v", batch.events, err)
			}
		})
	}
}

func TestInboxExplicitAckIsReaderScopedAndHidesMeetCopy(t *testing.T) {
	isolateUnifiedInbox(t)
	if err := bus.PostMessage(bus.Post{From: "human", Body: "original broadcast"}); err != nil {
		t.Fatal(err)
	}
	st, err := meet.Create(meet.CreateOptions{Topic: "seeded ack", Board: true, Participants: []string{inboxTestReader}, Human: "human"})
	if err != nil {
		t.Fatal(err)
	}
	if err := meet.AppendEvent(st.ID, meet.Event{Speaker: "human", Kind: "message", Text: "original broadcast", Origin: &meet.EventOrigin{Source: "mb", Seq: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := bus.PostMessage(bus.Post{From: "human", Body: "still pending"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runInboxCmd(t, "ack", "--as", inboxTestReader, "mb:1"); err != nil {
		t.Fatal(err)
	}
	batch, err := snapshotUnifiedInbox(inboxTestReader, 1, true)
	if err != nil || len(batch.events) != 1 || batch.events[0].Body != "still pending" || len(batch.warns) != 0 {
		t.Fatalf("acked original/copy occupied limit: %+v, %v", batch, err)
	}
	for _, snapshot := range []func(string) (inboxBatch, error){localUnreadSnapshot, liveUnreadSnapshot} {
		batch, err := snapshot(inboxTestReader)
		if err != nil {
			t.Fatal(err)
		}
		if sum, ok := summarizeUnreadHint(batch); !ok || sum.count != 1 {
			t.Fatalf("actual unread hint snapshot = %+v, %v", sum, ok)
		}
	}
	other, err := snapshotUnifiedInbox("other-reader", 0, true)
	if err != nil || len(other.events) != 2 {
		t.Fatalf("another reader lost the acknowledged broadcast: %+v, %v", other.events, err)
	}
	if _, _, err := runInboxCmd(t, "ack", "--as", inboxTestReader, "mb:2"); err != nil {
		t.Fatal(err)
	}
	for _, snapshot := range []func(string) (inboxBatch, error){localUnreadSnapshot, liveUnreadSnapshot} {
		hint, _, ran := checkUnreadArrivals(inboxTestReader, 0, false, inboxHintArrivals{}, snapshot, nil)
		if !ran || hint != "" {
			t.Fatalf("all acked hint = %q, ran=%v", hint, ran)
		}
	}
}
