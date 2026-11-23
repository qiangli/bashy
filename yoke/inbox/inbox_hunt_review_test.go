package main

import (
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/room"
)

func TestReviewConcurrentTurnExposesOnlyClaimedBody(t *testing.T) {
	store := isolateExtensionTestEnv(t)
	first := openTestLineInstance(t, store, "claude+2", "turn-session-two", 0)
	second := openTestLineInstance(t, store, "claude+3", "turn-session-three", 0)
	const body = "one hunt-group body"
	post, err := bus.Send(bus.SendRequest{From: "operator", To: "claude", Body: body})
	if err != nil {
		t.Fatal(err)
	}

	// Both consumers prepare before either writer has finished handing its
	// text to the PTY. This orders the competing steps deterministically.
	a := bus.PrepareLaunchPreamble(first.MailAddress())
	b := bus.PrepareLaunchPreamble(second.MailAddress())
	if a.Err() != nil || b.Err() != nil {
		t.Fatalf("turn preparation failed: %v / %v", a.Err(), b.Err())
	}
	copies := 0
	for _, text := range []string{a.Text, b.Text} {
		if strings.Contains(text, body) {
			copies++
		}
	}
	if copies != 1 {
		t.Fatalf("%d turns exposed the hunt-group body before Commit; claim holder = %q", copies, bus.ClaimHolder(post.Seq))
	}
	if holder := bus.ClaimHolder(post.Seq); !bus.ClaimMatchesReader(holder, first.MailAddress()) {
		t.Fatalf("expected claim holder %q, got %q", first.MailAddress(), holder)
	}
	if err := a.Commit(); err != nil {
		t.Fatalf("winner Commit failed: %v", err)
	}
	if err := b.Commit(); err != nil {
		t.Fatalf("loser Commit failed: %v", err)
	}

	// Parallel race across PrepareLaunchPreamble before any Commit also exposes the body once.
	post2, err := bus.Send(bus.SendRequest{From: "operator", To: "claude", Body: "concurrent race hunt body"})
	if err != nil {
		t.Fatal(err)
	}
	var raceCopies atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	preps := make([]bus.PreparedPreamble, 2)
	for idx, inst := range []fleet.Instance{first, second} {
		wg.Add(1)
		go func(i int, target fleet.Instance) {
			defer wg.Done()
			<-start
			p := bus.PrepareLaunchPreamble(target.MailAddress())
			preps[i] = p
			if strings.Contains(p.Text, "concurrent race hunt body") {
				raceCopies.Add(1)
			}
		}(idx, inst)
	}
	close(start)
	wg.Wait()
	for i, p := range preps {
		if p.Err() != nil {
			t.Fatalf("race preparation %d failed: %v", i, p.Err())
		}
	}
	if got := raceCopies.Load(); got != 1 {
		t.Fatalf("concurrent PrepareLaunchPreamble exposed body %d times before Commit, want 1", got)
	}
	if holder := bus.ClaimHolder(post2.Seq); holder == "" {
		t.Fatalf("expected ClaimHolder set before Commit on seq %d", post2.Seq)
	}
}

func TestReviewWithheldHuntItemStaysUnclaimedAndUnread(t *testing.T) {
	store := isolateExtensionTestEnv(t)
	first := openTestLineInstance(t, store, "claude+2", "turn-withheld-two", 0)
	second := openTestLineInstance(t, store, "claude+3", "turn-withheld-three", 0)

	// One direct item addressed to first (wins the single turn slot) and one
	// hunt-group item on claude (withheld as "1 of 2 unread").
	if _, err := bus.Send(bus.SendRequest{From: "operator", To: "claude+2", Body: "direct item for plus2"}); err != nil {
		t.Fatal(err)
	}
	huntPost, err := bus.Send(bus.SendRequest{From: "operator", To: "claude", Body: "withheld hunt item"})
	if err != nil {
		t.Fatal(err)
	}

	// Observational peek leaves hunt item unclaimed.
	peekOut, _, err := runInboxCmd(t, "--as", "claude+2", "--peek")
	if err != nil || !strings.Contains(peekOut, "withheld hunt item") {
		t.Fatalf("peek missed hunt item (err=%v):\n%s", err, peekOut)
	}
	if holder := bus.ClaimHolder(huntPost.Seq); holder != "" {
		t.Fatalf("--peek claimed hunt item: %q", holder)
	}

	// Prepare and commit first's bounded turn: direct item is admitted, hunt item is withheld.
	prep := bus.PrepareLaunchPreamble(first.MailAddress())
	if err := prep.Err(); err != nil {
		t.Fatalf("PrepareLaunchPreamble: %v", err)
	}
	if !strings.Contains(prep.Text, "direct item for plus2") ||
		!strings.Contains(prep.Text, "1 of 2 unread") ||
		strings.Contains(prep.Text, "withheld hunt item") {
		t.Fatalf("unexpected bounded preamble:\n%s", prep.Text)
	}
	if holder := bus.ClaimHolder(huntPost.Seq); holder != "" {
		t.Fatalf("withheld hunt item was claimed during preparation: holder=%q", holder)
	}
	if err := prep.Commit(); err != nil {
		t.Fatalf("prep.Commit: %v", err)
	}
	if holder := bus.ClaimHolder(huntPost.Seq); holder != "" {
		t.Fatalf("withheld hunt item was claimed on Commit: holder=%q", holder)
	}

	// Withheld hunt item stays [unread] in first's mailbox and can still be claimed by second.
	listOut, _, err := runTestMailboxListCmd(t, "--as", "claude+2")
	if err != nil {
		t.Fatalf("inbox list failed: %v", err)
	}
	if !strings.Contains(listOut, "withheld hunt item") || !strings.Contains(listOut, "[unread]") {
		t.Fatalf("expected withheld hunt item to remain [unread], got:\n%s", listOut)
	}

	secondPrep := bus.PrepareLaunchPreamble(second.MailAddress())
	if err := secondPrep.Err(); err != nil {
		t.Fatalf("second PrepareLaunchPreamble: %v", err)
	}
	if !strings.Contains(secondPrep.Text, "withheld hunt item") {
		t.Fatalf("second live instance did not receive unclaimed withheld hunt item:\n%s", secondPrep.Text)
	}
	if holder := bus.ClaimHolder(huntPost.Seq); !bus.ClaimMatchesReader(holder, second.MailAddress()) {
		t.Fatalf("expected second to claim withheld hunt item, holder=%q", holder)
	}
}

func TestReviewWithheldBusAndHostItemsStayUnreadAcrossSeparateDeliveryCommit(t *testing.T) {
	store := isolateExtensionTestEnv(t)
	i2 := openTestLineInstance(t, store, "claude+2", "turn-sep-plus2", 0)

	if _, err := bus.Send(bus.SendRequest{From: "operator", To: "claude+2", Body: "direct mb item"}); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(bus.Notification{Principal: "operator", To: i2.MailAddress(), Body: "withheld bus notification"}); err != nil {
		t.Fatal(err)
	}

	prep := bus.PrepareLaunchPreamble(i2.MailAddress())
	if err := prep.Err(); err != nil {
		t.Fatalf("PrepareLaunchPreamble: %v", err)
	}
	if !strings.Contains(prep.Text, "1 of 2 unread") {
		t.Fatalf("expected bounded 1-of-2 turn preamble, got:\n%s", prep.Text)
	}
	if err := prep.Commit(); err != nil {
		t.Fatalf("prep.Commit: %v", err)
	}

	withheld := "withheld bus notification"
	if strings.Contains(prep.Text, withheld) {
		withheld = "direct mb item"
	}
	inboxOut, _, err := runInboxCmd(t, "--as", "claude+2")
	if err != nil {
		t.Fatalf("inbox read failed: %v", err)
	}
	if !strings.Contains(inboxOut, withheld) {
		t.Fatalf("withheld item %q was consumed on separate-delivery Commit:\n%s", withheld, inboxOut)
	}

	second := bus.PrepareLaunchPreamble(i2.MailAddress())
	if err := second.Err(); err != nil {
		t.Fatalf("second PrepareLaunchPreamble: %v", err)
	}
	if !strings.Contains(second.Text, withheld) {
		t.Fatalf("withheld item %q was not delivered on second turn:\n%s", withheld, second.Text)
	}
}

func TestReviewNamedSeatPreservesDeliveryWithoutHuntClaim(t *testing.T) {
	isolateExtensionTestEnv(t)
	if err := fleet.New().SaveAgent(fleet.Agent{Name: "esme", Tool: "claude", Model: "sonnet4.6"}); err != nil {
		t.Fatal(err)
	}
	if err := room.Join(room.Card{
		ID:        room.AgentClaimID("esme"),
		Nick:      "esme",
		Tool:      "claude",
		Model:     "sonnet4.6",
		Binding:   "claude:sonnet4.6",
		Mode:      "interactive",
		PID:       os.Getpid(),
		OwnerPID:  os.Getpid(),
		Principal: "operator",
	}); err != nil {
		t.Fatal(err)
	}
	post, err := bus.Send(bus.SendRequest{From: "operator", To: "esme", Body: "named seat assignment"})
	if err != nil {
		t.Fatal(err)
	}
	prep := bus.PrepareLaunchPreamble("esme")
	if err := prep.Err(); err != nil {
		t.Fatalf("PrepareLaunchPreamble(esme): %v", err)
	}
	if !strings.Contains(prep.Text, "named seat assignment") {
		t.Fatalf("named seat missed its message:\n%s", prep.Text)
	}
	if holder := bus.ClaimHolder(post.Seq); holder != "" {
		t.Fatalf("named seat message unexpectedly created a hunt claim: %q", holder)
	}
}
