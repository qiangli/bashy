package agentos

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/meet"
	"github.com/qiangli/yoke/pkg/room"
)

// Sprint 467, story a8c2a2aeb094 (seq 2381). A managed manager session holds
// its singleton room identity as a foreman-mode card, so every drain path
// refused: `inbox --watch` hit the duplicate-owner Join refusal, a bounded
// `inbox` read always peeks by design, and `sprint inbox-ack` needs an
// attached sprint-inbox watch the foreman is not. The overflow kept
// re-delivering with no supported command able to drain it.
//
// The fixed path is `inbox --watch` adopted onto the caller's own managed
// session: the live foreman card stays exactly as it was (no takeover, no
// rewrite, no retire-on-exit), and the watch renders before it acknowledges,
// so only displayed records advance their source watermarks.
func seedManagedOwnerInbox(t *testing.T, reader string) (meetRoom string) {
	t.Helper()
	st, err := meet.Create(meet.CreateOptions{Topic: "manager channel", Board: true, Participants: []string{reader}, Human: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if err := meet.AppendEvent(st.ID, meet.Event{Speaker: "operator", To: reader, Kind: "status", Text: "managed overflow meet record"}); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"managed overflow board one", "managed overflow board two"} {
		if err := bus.PostMessage(bus.Post{From: "human", To: reader, Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	return st.ID
}

// joinForemanHolder publishes the managed-session card the production foreman
// holds: this identity, mode foreman, held by holderPID. The caller owns
// cleanup through the room leave.
func joinForemanHolder(t *testing.T, name string, holderPID int) room.Card {
	t.Helper()
	card := room.Card{
		ID: room.AgentClaimID(name), Nick: name, Mode: "foreman",
		Task: "sprint-9-manager", Caps: []string{room.CapInboxDelivery},
		PID: holderPID,
	}
	if err := room.Join(card); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { room.LeavePID(card.ID, holderPID) })
	return card
}

func TestManagedOwnerWatchDrainsOverflowAndPreservesOwnership(t *testing.T) {
	const name = "managed-drain-manager"
	sessionedWatcherAgent(t, name)
	roomID := seedManagedOwnerInbox(t, name)

	// The managed session itself holds the identity; this test process stands
	// in for a command running inside that session (a descendant of the
	// holder), which is the only shape allowed to share it.
	holder := joinForemanHolder(t, name, os.Getppid())

	claim, err := registerInboxWatcher(name)
	if err != nil {
		t.Fatalf("watch inside the owning managed session refused: %v", err)
	}
	defer claim.leave()

	var out, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	poll := defaultInboxPollRuntime(true)
	poll.ownerLive = claim.ownerLive
	if err := runUnifiedInboxWithPoll(ctx, &out, &errOut, name, 0, false, false, true, time.Second, poll); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"managed overflow meet record", "managed overflow board one", "managed overflow board two"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("adopted watch did not render %q:\n%s\nstderr:\n%s", want, out.String(), errOut.String())
		}
	}
	// Real emitted source watermarks advanced over exactly the displayed batch.
	if got := bus.SeenSeq(name); got == 0 {
		t.Fatal("adopted watch rendered board mail without advancing its watermark")
	}
	if got := meet.SeenSeq(roomID, name); got == 0 {
		t.Fatal("adopted watch rendered meet mail without advancing its watermark")
	}
	after, err := snapshotUnifiedInbox(name, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.events) != 0 {
		t.Fatalf("drained overflow re-delivered %d record(s): %+v", len(after.events), after.events)
	}
	// Ownership preserved: the foreman card is untouched and still live.
	cur, live, err := room.Find(room.AgentClaimID(name))
	if err != nil || !live {
		t.Fatalf("managed owner card lost by the drain: live=%v err=%v", live, err)
	}
	if cur.PID != holder.PID || cur.Mode != "foreman" || cur.Task != holder.Task {
		t.Fatalf("managed owner card rewritten by the drain: %#v", cur)
	}
}

// Leaving an adopted watch must hand back only the kernel lease: the foreman
// card it never wrote stays live for the session that still owns it.
func TestManagedOwnerWatchLeaveKeepsForemanCard(t *testing.T) {
	const name = "managed-leave-manager"
	sessionedWatcherAgent(t, name)
	joinForemanHolder(t, name, os.Getppid())

	claim, err := registerInboxWatcher(name)
	if err != nil {
		t.Fatalf("watch inside the owning managed session refused: %v", err)
	}
	claim.leave()
	if _, live, err := room.Find(room.AgentClaimID(name)); err != nil || !live {
		t.Fatalf("watch exit retired the managed owner's card: live=%v err=%v", live, err)
	}
}

// A live foreman card held OUTSIDE this session is still a refusal: sharing an
// identity requires proving the same session, and nothing was consumed.
func TestManagedOwnerWatchRefusesForeignSession(t *testing.T) {
	const name = "managed-foreign-manager"
	sessionedWatcherAgent(t, name)
	seedManagedOwnerInbox(t, name)

	// A live holder that is not an ancestor of this process: the relationship
	// check, not liveness, is what refuses it.
	other := exec.Command("sleep", "30")
	if err := other.Start(); err != nil {
		t.Skipf("cannot start a holder process: %v", err)
	}
	defer func() {
		_ = other.Process.Kill()
		_, _ = other.Process.Wait()
	}()
	joinForemanHolder(t, name, other.Process.Pid)

	if _, err := registerInboxWatcher(name); err == nil {
		t.Fatal("a foreign session adopted the managed owner's identity")
	} else if !strings.Contains(err.Error(), "already live") {
		t.Fatalf("foreign refusal lost the duplicate-owner rule:\n%v", err)
	}
	after, err := snapshotUnifiedInbox(name, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.events) != 3 {
		t.Fatalf("refused foreign watch consumed records: %d left", len(after.events))
	}
	if bus.SeenSeq(name) != 0 {
		t.Fatal("refused foreign watch advanced a source cursor")
	}
}

// One identity keeps exactly one consuming reader: a second watch from inside
// the same managed session is refused by the held kernel lease.
func TestManagedOwnerSecondWatchStillRefused(t *testing.T) {
	const name = "managed-second-manager"
	sessionedWatcherAgent(t, name)
	joinForemanHolder(t, name, os.Getppid())

	first, err := registerInboxWatcher(name)
	if err != nil {
		t.Fatalf("first watch inside the owning managed session refused: %v", err)
	}
	defer first.leave()
	if _, err := registerInboxWatcher(name); err == nil {
		t.Fatal("a second concurrent watcher for one identity was allowed")
	} else if !strings.Contains(err.Error(), "already has a live inbox watcher") {
		t.Fatalf("second-watcher refusal lost the single-consumer rule:\n%v", err)
	}
}
