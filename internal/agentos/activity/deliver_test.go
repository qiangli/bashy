package activity

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// harness isolates a test completely: its own state directory AND its own
// transport stubs. Both halves matter. The directory keeps the outbox out of
// the operator's home; the stubs keep bus.Publish and bus.SteerLive from ever
// being called, so a test can never append to the real room timeline or steer
// a real agent's session. A test that only did the first would look hermetic
// and would not be.
type harness struct {
	mu         sync.Mutex
	published  []string // "to|subject|priority"
	inboxes    []string
	woken      []string
	live       map[string]bool
	publishErr map[string]error
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BASHY_ACTIVITY_DIR", dir)
	// BASHY_HOME too: anything that resolves the home must land in the temp
	// tree, not in ~/.config/bashy.
	t.Setenv("BASHY_HOME", dir)

	h := &harness{live: map[string]bool{}, publishErr: map[string]error{}}

	oldEnsure, oldPublish, oldWake, oldNow := EnsureInbox, PublishDurable, WakeLive, Now
	t.Cleanup(func() {
		EnsureInbox, PublishDurable, WakeLive, Now = oldEnsure, oldPublish, oldWake, oldNow
	})

	EnsureInbox = func(sub string) error {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.inboxes = append(h.inboxes, sub)
		return nil
	}
	PublishDurable = func(principal, to, subject, topic, priority string) error {
		h.mu.Lock()
		defer h.mu.Unlock()
		if err := h.publishErr[to]; err != nil {
			return err
		}
		h.published = append(h.published, fmt.Sprintf("%s|%s|%s", to, subject, priority))
		return nil
	}
	WakeLive = func(sub, subject string) (bool, string) {
		h.mu.Lock()
		defer h.mu.Unlock()
		if !h.live[sub] {
			return false, "not running"
		}
		h.woken = append(h.woken, sub)
		return true, ""
	}
	return h
}

func (h *harness) publishedTo(sub string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, p := range h.published {
		if strings.HasPrefix(p, sub+"|") {
			n++
		}
	}
	return n
}

func (h *harness) wakeCount(sub string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, w := range h.woken {
		if w == sub {
			n++
		}
	}
	return n
}

func failEvent(token string) Event {
	return Event{
		Source: SourceWeave, Actor: "conductor", Action: ActionFail, Noun: "run",
		Object: "weave:run/42", Status: StatusFailed, Token: token,
		Scope: Scope{Repo: "bashy"}, Owner: "steward",
	}
}

func TestEmitPublishesDurablyThenWakes(t *testing.T) {
	h := newHarness(t)
	h.live["steward"] = true

	res, err := Emit(failEvent("failed"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Recipients) != 1 || res.Recipients[0].Subscriber != "steward" {
		t.Fatalf("recipients = %v", res.Recipients)
	}
	if h.publishedTo("steward") != 1 {
		t.Fatalf("durable publish did not happen exactly once")
	}
	if res.Wakes["steward"] != WakeSteered {
		t.Fatalf("wake outcome = %q", res.Wakes["steward"])
	}
	// The offline-recipient inbox is ensured before anything is addressed.
	if len(h.inboxes) != 1 || h.inboxes[0] != "steward" {
		t.Fatalf("inbox was not ensured: %v", h.inboxes)
	}
	// The subject carries the id, so the recipient can fetch the full envelope.
	if !strings.Contains(h.published[0], "id="+res.Event.ID) {
		t.Fatalf("published subject lacks the event id: %q", h.published[0])
	}
}

func TestEmitIsIdempotentOnTheEventID(t *testing.T) {
	h := newHarness(t)
	first, err := Emit(failEvent("failed"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Emit(failEvent("failed"))
	if err != nil {
		t.Fatal(err)
	}
	if !second.Duplicate {
		t.Fatalf("a replayed emit was not reported as a duplicate")
	}
	if second.Event.ID != first.Event.ID || second.Event.Seq != first.Event.Seq {
		t.Fatalf("a duplicate emit changed the record: %+v vs %+v", second.Event, first.Event)
	}
	if h.publishedTo("steward") != 1 {
		t.Fatalf("a replayed emit published %d times, want 1", h.publishedTo("steward"))
	}
	// A genuinely new transaction boundary is a new event.
	third, err := Emit(failEvent("retried"))
	if err != nil {
		t.Fatal(err)
	}
	if third.Duplicate || third.Event.ID == first.Event.ID {
		t.Fatalf("a new token did not produce a new event")
	}
}

func TestPerSourceOrderingIsMonotonic(t *testing.T) {
	newHarness(t)
	var weave, meet []int64
	for i := 0; i < 4; i++ {
		w, err := Emit(Event{Source: SourceWeave, Actor: "c", Action: ActionUpdate, Noun: "run",
			Object: fmt.Sprintf("weave:run/%d", i), Owner: "steward", Token: "t"})
		if err != nil {
			t.Fatal(err)
		}
		weave = append(weave, w.Event.Seq)
		m, err := Emit(Event{Source: SourceMeet, Actor: "c", Action: ActionCreate, Noun: "message",
			Object: fmt.Sprintf("meet:room/1#%d", i), Owner: "steward", Token: "t"})
		if err != nil {
			t.Fatal(err)
		}
		meet = append(meet, m.Event.Seq)
	}
	// Each source counts independently: a global counter would imply a total
	// order across subsystems that no single lock actually establishes.
	for i := range weave {
		if weave[i] != int64(i+1) || meet[i] != int64(i+1) {
			t.Fatalf("sequences are not per-source monotonic: weave=%v meet=%v", weave, meet)
		}
	}
}

func TestPartialDeliveryIsResumedNotRestarted(t *testing.T) {
	h := newHarness(t)
	h.publishErr["atlas"] = fmt.Errorf("transport down")

	e := Event{Source: SourceSprint, Actor: "conductor", Action: ActionUpdate, Noun: "story",
		Object: "sprint:88/story/7", Token: "assigned", Owner: "steward", Assignees: []string{"atlas"}}
	res, err := Emit(e)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) == 0 {
		t.Fatalf("a failed recipient was not reported")
	}
	if h.publishedTo("steward") != 1 {
		t.Fatalf("one unreachable recipient silenced the rest of the fleet")
	}

	// Recovery re-drives only what is still owed.
	delete(h.publishErr, "atlas")
	out, err := Recover()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("recover re-drove %d records, want 1", len(out))
	}
	if h.publishedTo("atlas") != 1 {
		t.Fatalf("atlas received %d copies, want 1", h.publishedTo("atlas"))
	}
	if h.publishedTo("steward") != 1 {
		t.Fatalf("recovery re-published to an already-delivered recipient (%d copies)", h.publishedTo("steward"))
	}
	// Nothing is owed any more.
	if again, _ := Recover(); len(again) != 0 {
		t.Fatalf("recover still reports owed deliveries: %v", again)
	}
}

func TestWakeFailureNeverLosesTheEvent(t *testing.T) {
	h := newHarness(t)
	h.live["steward"] = false // offline: no control socket

	res, err := Emit(failEvent("failed"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Wakes["steward"] != WakeUnreachable {
		t.Fatalf("wake outcome = %q, want %q", res.Wakes["steward"], WakeUnreachable)
	}
	// The durable append happened anyway — that is the whole guarantee.
	if h.publishedTo("steward") != 1 {
		t.Fatalf("a failed wake cost the durable delivery")
	}
	rec, ok, err := Show(res.Event.ID)
	if err != nil || !ok {
		t.Fatalf("record not journaled: ok=%v err=%v", ok, err)
	}
	if !rec.Published || !rec.Delivered["steward"] {
		t.Fatalf("record does not show the durable delivery: %+v", rec)
	}
}

func TestCoalescingAndRateLimitingDemoteButNeverDrop(t *testing.T) {
	h := newHarness(t)
	h.live["steward"] = true

	base := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	clock := base
	Now = func() time.Time { return clock }

	// Two updates to the SAME object inside the coalesce window.
	for i, token := range []string{"a", "b"} {
		clock = base.Add(time.Duration(i) * time.Second)
		res, err := Emit(Event{Source: SourceWeave, Actor: "c", Action: ActionUpdate, Noun: "run",
			Object: "weave:run/42", Owner: "steward", Token: token})
		if err != nil {
			t.Fatal(err)
		}
		want := WakeSteered
		if i == 1 {
			want = WakeCoalesced
		}
		if res.Wakes["steward"] != want {
			t.Fatalf("emit %d wake = %q, want %q", i, res.Wakes["steward"], want)
		}
	}
	// DEMOTE, NEVER DROP: both were still durably published.
	if h.publishedTo("steward") != 2 {
		t.Fatalf("coalescing dropped a durable delivery (%d published, want 2)", h.publishedTo("steward"))
	}
	if h.wakeCount("steward") != 1 {
		t.Fatalf("coalescing did not suppress the second wake (%d wakes)", h.wakeCount("steward"))
	}

	// Rate limiting: distinct objects, so coalescing does not apply, but the
	// per-minute cap does.
	limited := 0
	for i := 0; i < 5; i++ {
		clock = base.Add(time.Duration(10+i) * time.Second)
		res, err := Emit(Event{Source: SourceWeave, Actor: "c", Action: ActionUpdate, Noun: "run",
			Object: fmt.Sprintf("weave:run/%d", 100+i), Owner: "steward", Token: "t"})
		if err != nil {
			t.Fatal(err)
		}
		if res.Wakes["steward"] == WakeRateLimited {
			limited++
		}
	}
	if got := h.wakeCount("steward"); got > DefaultMaxWakePerMin {
		t.Fatalf("%d wakes in a minute exceeds the cap of %d", got, DefaultMaxWakePerMin)
	}
	// Guard against a vacuous pass: the cap must actually have bound here,
	// otherwise this test would still be green with wakes disabled entirely.
	if limited == 0 {
		t.Fatalf("the per-minute cap never bound; the test proves nothing")
	}
	// Every one of them is still durably in the inbox.
	if h.publishedTo("steward") != 7 {
		t.Fatalf("rate limiting dropped a durable delivery (%d published, want 7)", h.publishedTo("steward"))
	}
}

func TestRoutedToNobodyIsRecordedNotFailed(t *testing.T) {
	h := newHarness(t)
	res, err := Emit(Event{Source: SourceTodo, Actor: "c", Action: ActionCreate, Noun: "task",
		Object: "todo:task/1", Token: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Recipients) != 0 {
		t.Fatalf("an event that interests nobody routed to %v", res.Recipients)
	}
	if len(h.published) != 0 {
		t.Fatalf("an event that interests nobody was still published")
	}
	// It IS visible, so an operator can see that it interested nobody.
	if rec, ok, _ := Show(res.Event.ID); !ok || !rec.Published {
		t.Fatalf("an unrouted event was not journaled")
	}
}

func TestReservedSourceIsRefusedAtEmit(t *testing.T) {
	newHarness(t)
	_, err := Emit(Event{Source: SourceActivity, Actor: "c", Action: ActionCreate, Noun: "n", Object: "o"})
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("the delivery path was allowed to announce itself: %v", err)
	}
}

func TestSubscribeReplacesRatherThanWidens(t *testing.T) {
	newHarness(t)
	if err := Subscribe(Interest{Subscriber: "atlas", Sources: []string{SourceWeave}, Wake: true}); err != nil {
		t.Fatal(err)
	}
	if err := Subscribe(Interest{Subscriber: "atlas", Sources: []string{SourceMeet}, Wake: true}); err != nil {
		t.Fatal(err)
	}
	in, err := LoadInterests()
	if err != nil {
		t.Fatal(err)
	}
	if len(in) != 1 {
		t.Fatalf("subscribe created a duplicate interest: %v", in)
	}
	// The failure mode of this design is an interest that quietly grew into a
	// firehose, so a second subscribe must REPLACE.
	if len(in[0].Sources) != 1 || in[0].Sources[0] != SourceMeet {
		t.Fatalf("subscribe merged instead of replacing: %v", in[0].Sources)
	}
	if err := Unsubscribe("atlas"); err != nil {
		t.Fatal(err)
	}
	if err := Unsubscribe("atlas"); err == nil {
		t.Fatalf("unsubscribing an absent interest reported success")
	}
}

func TestSinceIsTheReconnectCatchUpQuery(t *testing.T) {
	newHarness(t)
	var seqs []int64
	for i := 0; i < 3; i++ {
		res, err := Emit(Event{Source: SourceMB, Actor: "c", Action: ActionCreate, Noun: "message",
			Object: fmt.Sprintf("mb:post/%d", i), Owner: "steward", Token: "t"})
		if err != nil {
			t.Fatal(err)
		}
		seqs = append(seqs, res.Event.Seq)
	}
	got, err := Since(SourceMB, seqs[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Event.Seq != seqs[1] || got[1].Event.Seq != seqs[2] {
		t.Fatalf("catch-up returned %d records: %v", len(got), got)
	}
	// A different source's stream is untouched by this cursor.
	if other, _ := Since(SourceWeave, 0); len(other) != 0 {
		t.Fatalf("catch-up crossed sources: %v", other)
	}
}

func TestAdapterAnnouncesAtTheTransactionBoundary(t *testing.T) {
	h := newHarness(t)
	h.live["steward"] = true

	a, err := For(SourceWeave)
	if err != nil {
		t.Fatal(err)
	}
	a = a.As("conductor").In(Scope{Repo: "bashy", Sprint: "88"})

	res, err := a.Lifecycle(ActionFail, "run", "weave:run/42", StatusFailed, "failed",
		Interested{Owner: "steward", Members: []string{"atlas"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Event.Scope.Repo != "bashy" || res.Event.Source != SourceWeave || res.Event.Actor != "conductor" {
		t.Fatalf("adapter did not bind its constants: %+v", res.Event)
	}
	if len(res.Recipients) != 2 {
		t.Fatalf("recipients = %v", res.Recipients)
	}
	// Calling again on a recovery path is a no-op.
	again, err := a.Lifecycle(ActionFail, "run", "weave:run/42", StatusFailed, "failed",
		Interested{Owner: "steward", Members: []string{"atlas"}})
	if err != nil {
		t.Fatal(err)
	}
	if !again.Duplicate {
		t.Fatalf("a replayed adapter call was not deduped")
	}

	if _, err := For("bogus"); err == nil {
		t.Fatalf("an unregistered source was accepted")
	}
	if _, err := For(SourceActivity); err == nil {
		t.Fatalf("the reserved source was accepted by the adapter")
	}
}

func TestReactingCarriesTheLoopChainAndTerminates(t *testing.T) {
	newHarness(t)
	a, err := For(SourceMB)
	if err != nil {
		t.Fatal(err)
	}
	a = a.As("relay")

	cause := Event{ID: "seed", Hop: 0}
	var last Result
	for hop := 0; hop < MaxHop; hop++ {
		last, err = a.Reacting(cause, ActionCreate, "message", fmt.Sprintf("mb:post/%d", hop), StatusOK, "t",
			Interested{Owner: "steward"})
		if hop == MaxHop-1 {
			// The chain terminates in a reported error at a named call site,
			// not in an unbounded fan-out.
			if err == nil {
				t.Fatalf("hop %d was accepted; the loop does not terminate", cause.Hop+1)
			}
			return
		}
		if err != nil {
			t.Fatalf("hop %d refused too early: %v", hop, err)
		}
		cause = last.Event
	}
}

func TestStateIsIsolatedToTheConfiguredDirectory(t *testing.T) {
	newHarness(t)
	dir := StateDir()
	if dir == "" || !strings.Contains(dir, t.TempDir()[:len(t.TempDir())-2]) {
		// TempDir() returns a fresh subdirectory per call, so compare prefixes
		// loosely; the point is that it is not the operator's home.
		t.Logf("state dir = %s", dir)
	}
	if strings.Contains(dir, ".config/bashy") {
		t.Fatalf("the test resolved the operator's real state directory: %s", dir)
	}
}

// TestRecoverWhenEveryPublishFailed is the regression for a panic this
// package's own partial-delivery test could not see.
//
// Record.Wakes and Record.Delivered are both `omitempty`, so a record whose
// recipients ALL failed to publish serializes with neither map and comes back
// from the journal nil. Recover then wrote into a nil map. The existing
// partial-delivery test missed it because one of its two recipients succeeded,
// which left a non-empty Wakes map that survived the round trip — the bug was
// reachable only when nothing at all got through, which is precisely the
// situation recovery exists for.
func TestRecoverWhenEveryPublishFailed(t *testing.T) {
	h := newHarness(t)
	h.publishErr["steward"] = fmt.Errorf("transport down")

	res, err := Emit(failEvent("failed"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) == 0 {
		t.Fatalf("expected the failed publish to be reported")
	}
	rec, ok, err := Show(res.Event.ID)
	if err != nil || !ok {
		t.Fatalf("record not journaled: ok=%v err=%v", ok, err)
	}
	if len(rec.Wakes) != 0 || len(rec.Delivered) != 0 {
		t.Fatalf("precondition lost: this test needs a record with both maps empty (%+v)", rec)
	}

	delete(h.publishErr, "steward")
	out, err := Recover()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("recover re-drove %d records, want 1", len(out))
	}
	if h.publishedTo("steward") != 1 {
		t.Fatalf("steward received %d copies, want 1", h.publishedTo("steward"))
	}
	if again, _ := Recover(); len(again) != 0 {
		t.Fatalf("recover still reports owed deliveries")
	}
}
