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
	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/meet"
	"github.com/qiangli/coreutils/pkg/room"
)

type failingInboxWriter struct{}

func (failingInboxWriter) Write([]byte) (int, error) { return 0, errors.New("render failed") }

type shortInboxWriter struct{}

func (shortInboxWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestInboxAggregatesBoardAndBusWithoutConsumingOnPeek(t *testing.T) {
	isolateUnifiedInbox(t)
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/alice")

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
	fleetDir := t.TempDir()
	t.Setenv("BASHY_FLEET_DIR", fleetDir)
	t.Setenv("BASHY_MEET_DIR", t.TempDir())
	// THE SPRINT STORE IS PART OF THE INBOX PATH, AND FORGETTING IT WAS NOT
	// HARMLESS. runUnifiedInbox opens by recording that this reader checked
	// its mail (weave.RefreshSprintOwnerActivity), which WRITES a sprint lease
	// held by that name. Unisolated, the suite reached into the operator's own
	// board and renewed a live conductor lease for another full TTL on every
	// run -- so a seat nobody was sitting in kept reporting healthy, and the
	// only thing propping it up was `go test`.
	//
	// BASHY_HOME as well as BASHY_SPRINT_DIR on purpose: the store resolves
	// BASHY_SPRINT_DIR, then BASHY_HOME, then the real home directory. Setting
	// only the specific variable isolates the store we happen to know about
	// today and leaves the next one reachable; setting the root closes the
	// path itself. Four named stores above were not enough exactly once, and
	// once was enough.
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("BASHY_SPRINT_DIR", filepath.Join(home, "sprint"))
	t.Setenv("BASHY_PRINCIPAL", "")
	t.Setenv("USER", "tester")
	registerTestInboxAgent(t, fleetDir, inboxTestReader)
}

// inboxTestReader is the name these tests read as. It is registered into the
// test's own empty fleet ring rather than borrowed from the shipped baseline.
//
// The tests used to read as a REAL baseline agent, because meet.Create refuses
// an unregistered participant and the baseline was the only thing that
// resolved. That made a live fleet identity -- somebody's actual conductor --
// the subject of every run, and the inbox path writes to whatever sprint lease
// that name holds. Owning the name is the fix: the ring is already isolated,
// so the test can simply put its own agent in it.
const inboxTestReader = "inbox-test-reader"

func registerTestInboxAgent(t *testing.T, dir, name string) {
	t.Helper()
	agents := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "name: " + name + "\nkind: agent\ntool: claude\nmodel: opus5\n"
	if err := os.WriteFile(filepath.Join(agents, name+".yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := fleet.New().Agent(name); !ok {
		t.Fatalf("test agent %q did not register in the isolated fleet ring %s", name, dir)
	}
}

func TestInboxAggregatesMeetAndAcknowledgesEachRenderedSource(t *testing.T) {
	isolateUnifiedInbox(t)
	reader := inboxTestReader
	st, err := meet.Create(meet.CreateOptions{Topic: "sprint channel", Board: true, Participants: []string{reader}, Human: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if err := meet.AppendEvent(st.ID, meet.Event{Speaker: "claude-opus5", To: reader, Kind: "status", Text: "meet message"}); err != nil {
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

func TestInboxSilentlyAdvancesPastOwnMeetPostThenDeliversPeerReply(t *testing.T) {
	isolateUnifiedInbox(t)
	reader, peer := inboxTestReader, "claude-opus5"
	st, err := meet.Create(meet.CreateOptions{Topic: "sprint channel", Board: true, Participants: []string{reader, peer}, Human: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if err := meet.AppendEvent(st.ID, meet.Event{Speaker: reader, Kind: "message", Text: "A status"}); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := runUnifiedInbox(context.Background(), &out, &errOut, reader, 0, false, false, false, 0); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "A status") || out.Len() != 0 {
		t.Fatalf("reader received its own Meet post: %q", out.String())
	}
	if got := meet.SeenSeq(st.ID, reader); got != 1 {
		t.Fatalf("self-only Meet watermark=%d, want 1", got)
	}
	if got := meet.SeenSeq(st.ID, peer); got != 0 {
		t.Fatalf("reader advanced peer cursor to %d", got)
	}

	if err := meet.AppendEvent(st.ID, meet.Event{Speaker: peer, To: reader, Kind: "message", Text: "B reply"}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if err := runUnifiedInbox(context.Background(), &out, &errOut, reader, 0, false, false, false, 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "B reply") || strings.Contains(out.String(), "A status") {
		t.Fatalf("peer delivery stdout=%q, want only B reply", out.String())
	}
	if got := meet.SeenSeq(st.ID, reader); got != 2 {
		t.Fatalf("peer Meet watermark=%d, want 2", got)
	}
}

func TestInboxBoundedWaitDoesNotFinishOnOwnMeetPost(t *testing.T) {
	isolateUnifiedInbox(t)
	reader, peer := inboxTestReader, "claude-opus5"
	st, err := meet.Create(meet.CreateOptions{Topic: "sprint channel", Board: true, Participants: []string{reader, peer}, Human: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	posted := make(chan error, 2)
	go func() {
		time.Sleep(30 * time.Millisecond)
		posted <- meet.AppendEvent(st.ID, meet.Event{Speaker: reader, Kind: "message", Text: "A status"})
		time.Sleep(120 * time.Millisecond)
		posted <- meet.AppendEvent(st.ID, meet.Event{Speaker: peer, To: reader, Kind: "message", Text: "B reply"})
	}()

	var out, errOut bytes.Buffer
	started := time.Now()
	if err := runUnifiedInbox(context.Background(), &out, &errOut, reader, 0, false, false, false, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := <-posted; err != nil {
		t.Fatal(err)
	}
	if err := <-posted; err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 100*time.Millisecond {
		t.Fatalf("bounded wait finished on reader's own post after %s", elapsed)
	}
	if !strings.Contains(out.String(), "B reply") || strings.Contains(out.String(), "A status") {
		t.Fatalf("bounded wait stdout=%q, want only B reply", out.String())
	}
}

func TestUnifiedTurnPreambleDeliversBoardAndMeetOnce(t *testing.T) {
	isolateUnifiedInbox(t)
	reader := inboxTestReader
	st, err := meet.Create(meet.CreateOptions{Topic: "sprint channel", Board: true, Participants: []string{reader}, Human: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if err := meet.AppendEvent(st.ID, meet.Event{Speaker: "claude-opus5", To: reader, Kind: "status", Text: "turn meet"}); err != nil {
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
	reader := inboxTestReader
	st, err := meet.Create(meet.CreateOptions{Topic: "sprint channel", Board: true, Participants: []string{reader}, Human: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if err := meet.AppendEvent(st.ID, meet.Event{Speaker: "claude-opus5", To: reader, Kind: "status", Text: "meet survives"}); err != nil {
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
	if err := meet.AppendEvent(st.ID, meet.Event{Speaker: other, To: other, Kind: "status", Text: "not alice's channel"}); err != nil {
		t.Fatal(err)
	}
	// A stale cursor is not membership. This models a removed participant or a
	// one-time open reader: future traffic must stop reaching it.
	if err := meet.MarkSeen(st.ID, inboxTestReader); err != nil {
		t.Fatal(err)
	}
	if err := meet.AppendEvent(st.ID, meet.Event{Speaker: other, To: other, Kind: "status", Text: "future private traffic"}); err != nil {
		t.Fatal(err)
	}
	batch, err := snapshotUnifiedInbox(inboxTestReader, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range batch.events {
		if event.Room == st.ID || strings.Contains(event.Body, "not alice's channel") {
			t.Fatalf("unrelated Meet board leaked into inbox: %+v", event)
		}
	}
	if got := meet.SeenSeq(st.ID, inboxTestReader); got != 1 {
		t.Fatalf("unrelated Meet board cursor advanced from stale watermark: %d", got)
	}
}

// A sprint's conductor room is an ordinary chaired meeting, so this is the
// exact case that made the room a sprint advertises the one channel its owner
// could not receive: the seat existed, the transcript held the questions, and
// the inbox reported nothing. Deliverability keys on the seat (P0-a).
func TestInboxDeliversChairedRoomMailToSeatedParticipant(t *testing.T) {
	isolateUnifiedInbox(t)
	reader := inboxTestReader
	peer := "claude-opus5"
	st, err := meet.Create(meet.CreateOptions{Topic: "conductor 99", Participants: []string{reader, peer}, NoSecretary: true, Human: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Board {
		t.Fatal("fixture opened a board; this case is about a CHAIRED room")
	}
	if err := meet.AppendEvent(st.ID, meet.Event{Speaker: "operator", Kind: "human", Text: "what is the plan?"}); err != nil {
		t.Fatal(err)
	}
	batch, err := snapshotUnifiedInbox(reader, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, event := range batch.events {
		if event.Room == st.ID && strings.Contains(event.Body, "what is the plan?") {
			found = true
		}
	}
	if !found {
		t.Fatalf("chaired-room mail never reached a seated participant: %+v", batch.events)
	}
}

// The seat is the whole gate, so removing the room-type condition must not
// widen delivery to rooms the reader holds no seat in.
func TestInboxExcludesChairedRoomsReaderHoldsNoSeatIn(t *testing.T) {
	isolateUnifiedInbox(t)
	other := "claude-opus5"
	st, err := meet.Create(meet.CreateOptions{Topic: "someone else's meeting", Participants: []string{other}, NoSecretary: true, Human: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if err := meet.AppendEvent(st.ID, meet.Event{Speaker: other, Kind: "status", Text: "not the reader's meeting"}); err != nil {
		t.Fatal(err)
	}
	batch, err := snapshotUnifiedInbox(inboxTestReader, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range batch.events {
		if event.Room == st.ID || strings.Contains(event.Body, "not the reader's meeting") {
			t.Fatalf("unseated chaired room leaked into inbox: %+v", event)
		}
	}
}

func TestUnifiedTurnPreambleSurfacesSourceFailureWithoutAcknowledging(t *testing.T) {
	isolateUnifiedInbox(t)
	reader := inboxTestReader
	if err := bus.PostMessage(bus.Post{From: "human", To: reader, Body: "must survive source error"}); err != nil {
		t.Fatal(err)
	}
	oldMeetRooms := inboxMeetRooms
	inboxMeetRooms = func() ([]meet.RoomSummary, error) {
		return nil, errors.New("forced source failure")
	}
	t.Cleanup(func() { inboxMeetRooms = oldMeetRooms })
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

func TestInboxWatcherPublishesActiveRosterPresenceForItsLifetime(t *testing.T) {
	isolateUnifiedInbox(t)
	const name = "inbox-sentinel-test"
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "codex", Model: "gpt5.6-sol"}); err != nil {
		t.Fatal(err)
	}

	claim, err := registerInboxWatcher(name)
	if err != nil {
		t.Fatal(err)
	}
	members, err := room.Members()
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 {
		t.Fatalf("watcher room members = %#v, want one", members)
	}
	card := members[0]
	if card.ID != name || card.Nick != name || card.Mode != inboxWatcherMode || card.PID != os.Getpid() || card.OwnerPID != os.Getppid() || card.Task != "watching Bashy inbox" {
		t.Fatalf("watcher card = %#v", card)
	}

	assignments, err := reconciledAgentRoster()
	if err != nil {
		t.Fatal(err)
	}
	var watcher *agentAssignment
	for i := range assignments {
		if assignments[i].Agent == name {
			watcher = &assignments[i]
			break
		}
	}
	if watcher == nil || watcher.Mode != inboxWatcherMode || watcher.Health != "healthy" || watcher.Title != "watching Bashy inbox" {
		t.Fatalf("watcher assignment = %#v; full roster = %#v", watcher, assignments)
	}

	claim.leave()
	members, err = room.Members()
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 0 {
		t.Fatalf("watcher remained after exit: %#v", members)
	}
}

func TestSprintInboxWatcherAdvertisesAttachedStream(t *testing.T) {
	isolateUnifiedInbox(t)
	const name = "external-sprint-stream"
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "agy", Model: "test"}); err != nil {
		t.Fatal(err)
	}
	claim, err := registerSprintInboxWatcher(name)
	if err != nil {
		t.Fatal(err)
	}
	defer claim.leave()
	card, live, err := room.Find(room.AgentClaimID(name))
	if err != nil || !live {
		t.Fatalf("sprint watcher card: live=%v err=%v", live, err)
	}
	if card.Mode != "sprint-inbox" || !room.HasCapability(card, room.CapInboxStream) || room.HasCapability(card, room.CapInboxDelivery) {
		t.Fatalf("sprint watcher advertised the wrong delivery contract: %#v", card)
	}
}

func TestInboxWatcherRefusesASecondLiveClaimOfTheSameIdentity(t *testing.T) {
	isolateUnifiedInbox(t)
	const name = "singleton-inbox-sentinel"
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "codex", Model: "gpt5.6-sol"}); err != nil {
		t.Fatal(err)
	}
	watcher, err := registerInboxWatcher(name)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.leave()

	if _, err := registerInboxWatcher(name); err == nil || !strings.Contains(err.Error(), "already has a live inbox watcher") {
		t.Fatalf("second watcher claim error = %v", err)
	}
}

func TestInboxWatcherRequiresARegisteredFleetIdentity(t *testing.T) {
	isolateUnifiedInbox(t)
	if _, err := registerInboxWatcher("observed-but-unregistered"); err == nil || !strings.Contains(err.Error(), "not a registered Bashy agent") {
		t.Fatalf("unregistered watcher error = %v", err)
	}
}

func TestInboxWatchRejectsAnObservedButUnregisteredAgentSeat(t *testing.T) {
	isolateUnifiedInbox(t)
	const name = "observed-seat-only"
	if err := bus.SaveSubscription(bus.Subscription{Subscriber: name, To: name}); err != nil {
		t.Fatal(err)
	}
	cmd := newUnifiedInboxCmd()
	cmd.SetArgs([]string{"--as", name, "--watch", "--wait", "1ms"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "not a registered Bashy agent") {
		t.Fatalf("observed-only watcher error = %v", err)
	}
}

func TestInboxWatcherCannotOverwriteAnotherLiveUseOfTheAgentIdentity(t *testing.T) {
	isolateUnifiedInbox(t)
	const name = "claimed-agent"
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "codex", Model: "gpt5.6-sol"}); err != nil {
		t.Fatal(err)
	}
	if err := room.Join(room.Card{ID: name, Nick: name, Mode: "interactive", PID: os.Getppid()}); err != nil {
		t.Fatal(err)
	}
	defer room.LeavePID(name, os.Getppid())
	if _, err := registerInboxWatcher(name); err == nil || !strings.Contains(err.Error(), "already live") {
		t.Fatalf("global identity collision error = %v", err)
	}
}

func TestDottedInteractiveClaimRefusesInboxWatcher(t *testing.T) {
	isolateUnifiedInbox(t)
	const name = "codex.gpt5.6.sol"
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "codex", Model: "gpt5.6-sol"}); err != nil {
		t.Fatal(err)
	}
	claimID := room.AgentClaimID(name)
	if claimID == name {
		t.Fatalf("test requires a sanitized dotted claim id, got %q", claimID)
	}
	if err := room.Join(room.Card{ID: claimID, Nick: name, Mode: "interactive", PID: os.Getppid()}); err != nil {
		t.Fatal(err)
	}
	defer room.LeavePID(claimID, os.Getppid())
	if _, err := registerInboxWatcher(name); err == nil || !strings.Contains(err.Error(), "already live") {
		t.Fatalf("watcher against dotted interactive claim error = %v", err)
	}
}

func TestDottedInboxWatcherRefusesInteractiveClaim(t *testing.T) {
	isolateUnifiedInbox(t)
	const name = "codex.gpt5.6.sol"
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "codex", Model: "gpt5.6-sol"}); err != nil {
		t.Fatal(err)
	}
	watcher, err := registerInboxWatcher(name)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.leave()
	claimID := room.AgentClaimID(name)
	if err := room.Join(room.Card{ID: claimID, Nick: name, Mode: "interactive", PID: os.Getppid()}); err == nil || !strings.Contains(err.Error(), "already live") {
		t.Fatalf("interactive against dotted watcher claim error = %v", err)
	}
}

func TestBoundedWatchHoldsAndReleasesTheAgentClaim(t *testing.T) {
	isolateUnifiedInbox(t)
	const name = "bounded-watch-sentinel"
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "codex", Model: "gpt5.6-sol"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := newUnifiedInboxCmd()
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--as", name, "--watch", "--wait", "300ms", "--json"})
	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()

	claimID := room.AgentClaimID(name)
	deadline := time.Now().Add(time.Second)
	for {
		card, live, err := room.Find(claimID)
		if err != nil {
			t.Fatal(err)
		}
		if live {
			if card.Mode != inboxWatcherMode || card.Nick != name {
				t.Fatalf("bounded watcher card = %#v", card)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bounded --watch never published its identity claim")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, live, err := room.Find(claimID); err != nil || live {
		t.Fatalf("bounded watcher claim after return: live=%v err=%v", live, err)
	}
}

func TestInboxWatcherRecordsSessionProofSeparatelyFromAttribution(t *testing.T) {
	isolateUnifiedInbox(t)
	const name = "governed-sentinel"
	const sessionEnv = "BASHY_TEST_TOOL_SESSION"
	if err := fleet.New().SaveTool(fleet.Tool{Name: "sentinel-tool", Kind: "cli", CLI: fleet.ToolCLI{Launch: fleet.ToolLaunch{SessionEnv: []string{sessionEnv}}}}); err != nil {
		t.Fatal(err)
	}
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "sentinel-tool", Model: "gpt5.6-sol"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/"+name)
	t.Setenv(sessionEnv, "private-session-value")
	watcher, err := registerInboxWatcher(name)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.leave()
	members, err := room.Members()
	if err != nil {
		t.Fatal(err)
	}
	wantClaim := bus.HashSessionClaim("private-session-value")
	if len(members) != 1 || members[0].Principal != "dhnt:agent/"+name || members[0].SessionClaim != wantClaim || members[0].OwnerPID != os.Getppid() {
		t.Fatalf("governed watcher claim = %#v", members)
	}
	if strings.Contains(members[0].SessionClaim, "private-session-value") {
		t.Fatalf("watcher persisted raw tool session: %#v", members[0])
	}
}

func TestInboxWatcherCanonicalizesAliasToTheGlobalAgentClaim(t *testing.T) {
	isolateUnifiedInbox(t)
	if err := fleet.New().SaveAgent(fleet.Agent{Name: "canonical-sentinel", Aliases: []string{"topic-sentinel"}, Tool: "codex", Model: "gpt5.6-sol"}); err != nil {
		t.Fatal(err)
	}
	watcher, err := registerInboxWatcher("topic-sentinel")
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.leave()
	members, err := room.Members()
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].ID != "canonical-sentinel" || members[0].Nick != "canonical-sentinel" {
		t.Fatalf("canonical watcher claim = %#v", members)
	}
}

func TestInboxHelpTeachesBoundedSentinelResponseAndIdentitySafety(t *testing.T) {
	cmd := newUnifiedInboxCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, behavior := range []string{
		"--as NAME --watch --json",
		"poll its output at",
		"appears as active in",
		"second watcher cannot claim",
		"--as NAME --watch --wait 60s",
		"every bounded run hold NAME's",
		"one empty timeout does not end",
		"distinct registered Bashy",
		"agents show NAME",
		"stable repo-relative",
		"Separate concurrent topic",
		"1024 UTF-8",
		"never truncate or auto-split",
		"acknowledge receipt with owner, action, and ETA",
		"Never read as another identity",
		"cooperative authored-message claim",
		"different live agent session",
		"whois agent:NAME",
		"monitoring ENDED",
		"continued monitoring after",
		"skills show inbox",
	} {
		if !strings.Contains(help, behavior) {
			t.Fatalf("inbox help does not teach %q:\n%s", behavior, help)
		}
	}
}

func TestSentinelReadAdvancesOnlySentinelCursor(t *testing.T) {
	isolateUnifiedInbox(t)
	if err := bus.PostMessage(bus.Post{From: "human", To: "sentinel-agent", Body: "route this request"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runUnifiedInbox(context.Background(), &out, io.Discard, "sentinel-agent", 0, false, false, false, 0); err != nil {
		t.Fatal(err)
	}
	if bus.SeenSeq("sentinel-agent") == 0 {
		t.Fatal("sentinel did not advance its own cursor")
	}
	if got := bus.SeenSeq("supervisor-agent"); got != 0 {
		t.Fatalf("sentinel advanced supervisor cursor to %d", got)
	}
	if directed, _, _, err := bus.Unseen("supervisor-agent", 0); err != nil {
		t.Fatal(err)
	} else if len(directed) != 0 {
		t.Fatalf("sentinel-directed post became supervisor-private input: %+v", directed)
	}
}

func TestInboxRejectsAuthenticatedAgentBorrowingAnotherCursor(t *testing.T) {
	isolateUnifiedInbox(t)
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/supervisor-agent")
	cmd := newUnifiedInboxCmd()
	cmd.SetArgs([]string{"--as", "sentinel-agent", "--peek"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "cannot read as") {
		t.Fatalf("authenticated cross-identity read error = %v", err)
	}
	if bus.SeenSeq("sentinel-agent") != 0 || bus.SeenSeq("supervisor-agent") != 0 {
		t.Fatal("rejected cross-identity read advanced a cursor")
	}
}

func TestInboxRejectsUnregisteredExplicitIdentity(t *testing.T) {
	isolateUnifiedInbox(t)
	cmd := newUnifiedInboxCmd()
	cmd.SetArgs([]string{"--as", "not-a-registered-agent-zz", "--peek"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "not a registered Bashy agent") {
		t.Fatalf("unregistered identity error = %v", err)
	}
}

func TestInboxExternalRoleAliasCannotDrainRolePending(t *testing.T) {
	isolateUnifiedInbox(t)
	const topic = "steward.host-test"
	prior := bus.HostRoles
	bus.HostRoles = func() []bus.HostRole {
		return []bus.HostRole{{Label: "steward", Topic: topic, Holder: "holder-agent"}}
	}
	t.Cleanup(func() { bus.HostRoles = prior })
	if _, err := bus.EnsureRoleInbox(topic); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(bus.Notification{Principal: "human", To: topic, Body: "held-role-private"}); err != nil {
		t.Fatal(err)
	}
	cmd := newUnifiedInboxCmd()
	cmd.SetArgs([]string{"--as", "steward", "--peek"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "not a registered Bashy agent") {
		t.Fatalf("external role read error = %v", err)
	}
	snapshot, err := bus.SnapshotInbox(topic)
	if err != nil || len(snapshot.Items) != 1 {
		t.Fatalf("rejected role alias drained pending: items=%d err=%v", len(snapshot.Items), err)
	}
}

func TestInboxCollapsesMeetSeedOnlyByStructuredMBOriginAndAcknowledgesBoth(t *testing.T) {
	isolateUnifiedInbox(t)
	reader := inboxTestReader
	wireMeet()
	if err := bus.PostMessage(bus.Post{From: "human", To: reader, Topic: "mb", Body: "INBOX_E2E_DEDUP"}); err != nil {
		t.Fatal(err)
	}
	st, err := meet.Create(meet.CreateOptions{Topic: "seeded", Board: true, Participants: []string{reader}, Human: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := meet.SeedBoardFromMB(st, []int64{1}); err != nil {
		t.Fatal(err)
	}
	// Same prose without provenance is an independent message and must survive.
	if err := meet.AppendEvent(st.ID, meet.Event{Speaker: "human", Kind: "message", Text: "INBOX_E2E_DEDUP"}); err != nil {
		t.Fatal(err)
	}
	batch, err := snapshotUnifiedInbox(reader, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	var original, independent, copied int
	for _, event := range batch.events {
		if event.Source == "mb" && event.Seq == 1 {
			original++
		}
		if event.Source == "meet" && event.Body == "INBOX_E2E_DEDUP" && event.Origin == nil {
			independent++
		}
		if event.Source == "meet" && event.Origin != nil && event.Origin.Source == "mb" && event.Origin.Seq == 1 {
			copied++
		}
	}
	if original != 1 || independent != 1 || copied != 0 {
		t.Fatalf("provenance collapse = original %d independent %d copied %d; events=%+v", original, independent, copied, batch.events)
	}
	for _, ack := range batch.acks {
		if err := ack(); err != nil {
			t.Fatal(err)
		}
	}
	if bus.SeenSeq(reader) == 0 || meet.SeenSeq(st.ID, reader) != 2 {
		t.Fatalf("both source watermarks not acknowledged: mb=%d meet=%d", bus.SeenSeq(reader), meet.SeenSeq(st.ID, reader))
	}
}

func TestInboxLegacyRolePendingIsVisibleOnlyToCurrentHolder(t *testing.T) {
	isolateUnifiedInbox(t)
	const roleTopic = "conductor.83"
	prior := bus.HostRoles
	bus.HostRoles = func() []bus.HostRole {
		return []bus.HostRole{{Label: "conductor:83", Topic: roleTopic, Holder: "holder-agent"}}
	}
	t.Cleanup(func() { bus.HostRoles = prior })
	if _, err := bus.EnsureRoleInbox(roleTopic); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(bus.Notification{Principal: "human", To: roleTopic, Body: "seat-only backlog"}); err != nil {
		t.Fatal(err)
	}
	if _, err := bus.ResolveFor(roleTopic); err != nil {
		t.Fatal(err)
	}
	batch, err := snapshotUnifiedInbox("unrelated-agent", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range batch.events {
		if strings.Contains(event.Body, "seat-only backlog") {
			t.Fatalf("unrelated agent saw role backlog: %+v", event)
		}
	}
	before, err := bus.SnapshotInbox(roleTopic)
	if err != nil || len(before.Items) != 1 {
		t.Fatalf("unrelated read advanced role backlog: items=%d err=%v", len(before.Items), err)
	}
	holder, err := snapshotUnifiedInbox("holder-agent", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range holder.events {
		found = found || strings.Contains(event.Body, "seat-only backlog")
	}
	if !found {
		t.Fatalf("current holder did not receive role backlog: %+v", holder.events)
	}
	for _, ack := range holder.acks {
		if err := ack(); err != nil {
			t.Fatal(err)
		}
	}
	after, err := bus.SnapshotInbox(roleTopic)
	if err != nil || len(after.Items) != 0 {
		t.Fatalf("holder did not advance role backlog: items=%d err=%v", len(after.Items), err)
	}
}

// TestIsolatedInboxNeverTouchesTheRealSprintStore is a regression for a bug
// that took an hour to find and that no unit assertion would ever have caught,
// because the damage landed OUTSIDE the test.
//
// runUnifiedInbox opens by recording that the reader checked its mail, and
// that WRITES a sprint lease held by that name. The isolation helper redirected
// four stores and not the fifth, and one test read as a real fleet agent — so
// every `go test ./internal/agentos/` renewed a live conductor lease on the
// operator's own board. A seat nobody occupied kept reporting healthy for
// half an hour at a time, and the thing propping it up was the test suite.
//
// The property is stated where it broke: run the isolated path, then look at
// the REAL store and require that nothing moved. It cannot rot the way an
// "every store is isolated" checklist would, because it asserts the outcome
// rather than the mechanism.
func TestIsolatedInboxNeverTouchesTheRealSprintStore(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory to protect: %v", err)
	}
	real := filepath.Join(home, ".bashy", "sprint", "queue.json")
	before, err := os.Stat(real)
	if err != nil {
		// Nothing to clobber on this host; the assertion has no subject.
		t.Skipf("no sprint store at %s", real)
	}

	isolateUnifiedInbox(t)
	// Deliberately the shape that did the damage: a plain, non-peek read under
	// a name that could be somebody's live conductor.
	var out, errOut bytes.Buffer
	if err := runUnifiedInbox(context.Background(), &out, &errOut, inboxTestReader, 0, false, false, false, 0); err != nil {
		t.Fatal(err)
	}

	after, err := os.Stat(real)
	if err != nil {
		t.Fatalf("the real sprint store went missing during an isolated test: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		t.Fatalf("an isolated inbox test wrote the REAL sprint store %s\n"+
			"  before: %s (%d bytes)\n   after: %s (%d bytes)\n"+
			"isolateUnifiedInbox must redirect every store this path can reach",
			real, before.ModTime(), before.Size(), after.ModTime(), after.Size())
	}
}
