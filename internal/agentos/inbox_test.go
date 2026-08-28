package agentos

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/meet"
)

type failingInboxWriter struct{}

func (failingInboxWriter) Write([]byte) (int, error) { return 0, errors.New("render failed") }

type shortInboxWriter struct{}

func (shortInboxWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestInboxAggregatesBoardAndBusWithoutConsumingOnPeek(t *testing.T) {
	isolateUnifiedInbox(t)

	if err := bus.PostMessage(bus.Post{From: "human", To: "alice", Topic: "harness", Body: "board message"}); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(bus.Notification{Principal: "scheduler", To: "alice", Body: "bus notification"}); err != nil {
		t.Fatal(err)
	}

	cmd, _, ok := newBusFrontDoorCmd("inbox")
	if !ok {
		t.Fatal("inbox is not mounted")
	}
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"--as", "alice", "--peek"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"board message", "bus notification"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("unified inbox omitted %q:\nstdout:\n%s\nstderr:\n%s", want, out.String(), errOut.String())
		}
	}
	if bus.SeenSeq("alice") != 0 {
		t.Fatal("--peek advanced the message-board cursor")
	}
	if got, _, err := bus.UnreadNotifications("alice"); err != nil || len(got) != 1 {
		t.Fatalf("--peek consumed bus pending: len=%d err=%v", len(got), err)
	}
}

func isolateUnifiedInbox(t *testing.T) {
	t.Helper()
	t.Setenv("BASHY_MB_DIR", t.TempDir())
	t.Setenv("BASHY_ROOM_DIR", t.TempDir())
	t.Setenv("BASHY_FLEET_DIR", t.TempDir())
	t.Setenv("BASHY_MEET_DIR", t.TempDir())
	t.Setenv("BASHY_PRINCIPAL", "")
	t.Setenv("USER", "tester")
}

func TestInboxAggregatesMeetAndAcknowledgesEachRenderedSource(t *testing.T) {
	isolateUnifiedInbox(t)
	reader := "codex-gpt5.6-sol"
	st, err := meet.Create(meet.CreateOptions{Topic: "sprint channel", Board: true, Participants: []string{reader}, Human: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := meet.PostAs(st.ID, reader, reader, "meet message"); err != nil {
		t.Fatal(err)
	}
	if err := bus.PostMessage(bus.Post{From: "human", To: reader, Body: "board message"}); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := runUnifiedInbox(context.Background(), &out, &errOut, reader, 0, false, false, false, 0); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"meet message", "board message"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("inbox omitted %q:\n%s", want, out.String())
		}
	}
	if bus.SeenSeq(reader) == 0 || meet.SeenSeq(st.ID, reader) == 0 {
		t.Fatalf("rendered source was not acknowledged: mb=%d meet=%d", bus.SeenSeq(reader), meet.SeenSeq(st.ID, reader))
	}

	out.Reset()
	if err := runUnifiedInbox(context.Background(), &out, &errOut, reader, 0, false, false, false, 0); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("acknowledged records were delivered twice: %q", out.String())
	}
}

func TestInboxWatchDeliversANewBoardPostWithoutHumanRelay(t *testing.T) {
	isolateUnifiedInbox(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	posted := make(chan error, 1)
	go func() {
		time.Sleep(30 * time.Millisecond)
		posted <- bus.PostMessage(bus.Post{From: "bob", To: "alice", Body: "arrived while watching"})
	}()
	var out, errOut bytes.Buffer
	if err := runUnifiedInbox(ctx, &out, &errOut, "alice", 0, false, false, false, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := <-posted; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "arrived while watching") {
		t.Fatalf("bounded watch did not deliver new input: %q", out.String())
	}
}

func TestUnifiedTurnPreambleDeliversBoardAndMeetOnce(t *testing.T) {
	isolateUnifiedInbox(t)
	reader := "codex-gpt5.6-sol"
	st, err := meet.Create(meet.CreateOptions{Topic: "sprint channel", Board: true, Participants: []string{reader}, Human: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := meet.PostAs(st.ID, reader, reader, "turn meet"); err != nil {
		t.Fatal(err)
	}
	if err := bus.PostMessage(bus.Post{From: "human", To: reader, Body: "turn board"}); err != nil {
		t.Fatal(err)
	}
	prepared := unifiedTurnPreamble(reader)
	got := prepared.Text
	for _, want := range []string{"turn meet", "turn board"} {
		if !strings.Contains(got, want) {
			t.Fatalf("turn preamble omitted %q: %q", want, got)
		}
	}
	if beforeAck := unifiedTurnPreamble(reader).Text; beforeAck == "" {
		t.Fatal("preparing a turn consumed input before successful injection")
	}
	if err := prepared.Commit(); err != nil {
		t.Fatal(err)
	}
	if again := unifiedTurnPreamble(reader).Text; again != "" {
		t.Fatalf("turn preamble redelivered acknowledged input: %q", again)
	}
}

func TestInboxRenderFailureLeavesEverySourceUnread(t *testing.T) {
	isolateUnifiedInbox(t)
	reader := "codex-gpt5.6-sol"
	st, err := meet.Create(meet.CreateOptions{Topic: "sprint channel", Board: true, Participants: []string{reader}, Human: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := meet.PostAs(st.ID, reader, reader, "meet survives"); err != nil {
		t.Fatal(err)
	}
	if err := bus.PostMessage(bus.Post{From: "human", To: reader, Body: "mb survives"}); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(bus.Notification{Principal: "scheduler", To: reader, Body: "bus survives"}); err != nil {
		t.Fatal(err)
	}
	if err := runUnifiedInbox(context.Background(), failingInboxWriter{}, &bytes.Buffer{}, reader, 0, false, false, false, 0); err == nil {
		t.Fatal("failing output was reported successful")
	}
	if bus.SeenSeq(reader) != 0 || meet.SeenSeq(st.ID, reader) != 0 {
		t.Fatalf("render failure advanced cursor: mb=%d meet=%d", bus.SeenSeq(reader), meet.SeenSeq(st.ID, reader))
	}
	if items, _, err := bus.UnreadNotifications(reader); err != nil || len(items) != 1 {
		t.Fatalf("render failure consumed bus input: len=%d err=%v", len(items), err)
	}
}

func TestInboxShortWriteLeavesSourceUnread(t *testing.T) {
	isolateUnifiedInbox(t)
	if err := bus.PostMessage(bus.Post{From: "human", To: "alice", Body: "do not consume"}); err != nil {
		t.Fatal(err)
	}
	if err := runUnifiedInbox(context.Background(), shortInboxWriter{}, &bytes.Buffer{}, "alice", 0, false, false, false, 0); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write error = %v", err)
	}
	if bus.SeenSeq("alice") != 0 {
		t.Fatal("short write advanced message-board cursor")
	}
}

func TestInboxExcludesMeetBoardsReaderNeverJoined(t *testing.T) {
	isolateUnifiedInbox(t)
	other := "claude-opus5"
	st, err := meet.Create(meet.CreateOptions{Topic: "other channel", Board: true, Participants: []string{other}, Human: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := meet.PostAs(st.ID, other, other, "not alice's channel"); err != nil {
		t.Fatal(err)
	}
	// A stale cursor is not membership. This models a removed participant or a
	// one-time open reader: future traffic must stop reaching it.
	if err := meet.MarkSeen(st.ID, "codex-gpt5.6-sol"); err != nil {
		t.Fatal(err)
	}
	if _, err := meet.PostAs(st.ID, other, other, "future private traffic"); err != nil {
		t.Fatal(err)
	}
	batch, err := snapshotUnifiedInbox("codex-gpt5.6-sol", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range batch.events {
		if event.Room == st.ID || strings.Contains(event.Body, "not alice's channel") {
			t.Fatalf("unrelated Meet board leaked into inbox: %+v", event)
		}
	}
	if got := meet.SeenSeq(st.ID, "codex-gpt5.6-sol"); got != 1 {
		t.Fatalf("unrelated Meet board cursor advanced from stale watermark: %d", got)
	}
}

func TestUnifiedTurnPreambleSurfacesSourceFailureWithoutAcknowledging(t *testing.T) {
	isolateUnifiedInbox(t)
	reader := "codex-gpt5.6-sol"
	if err := bus.PostMessage(bus.Post{From: "human", To: reader, Body: "must survive source error"}); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_MEET_DIR", blocked)
	prepared := unifiedTurnPreamble(reader)
	if !strings.Contains(prepared.Text, "unified inbox warning") || !strings.Contains(prepared.Text, "No source cursor was advanced") {
		t.Fatalf("source failure was converted to an empty inbox: %q", prepared.Text)
	}
	if bus.SeenSeq(reader) != 0 {
		t.Fatal("source failure acknowledged an earlier source")
	}
}

func TestInboxRefusesWatchWithLimit(t *testing.T) {
	isolateUnifiedInbox(t)
	cmd := newUnifiedInboxCmd()
	cmd.SetArgs([]string{"--as", "alice", "--watch", "--limit", "1"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("watch+limit error = %v", err)
	}
}
