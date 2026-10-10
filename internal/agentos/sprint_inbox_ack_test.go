package agentos

import (
	"os"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/room"
)

// Sprint 413, todo 491a03436cbb. Sprint 329's manager had 14 unread messages,
// ran `bashy sprint inbox-ack 329 --as s329-manager`, got exit 1 and NOTHING on
// either stream, and `sprint tick` still reported 14 unread. The silence was the
// host's (see sprint_dispatch_test.go); the message underneath it collapsed four
// different causes into one sentence and never named the command that actually
// consumed the mail — `bashy inbox --as s329-manager --watch`, which the manager
// found by guessing.
//
// requireAttachedSprintWatch is that precondition. Its contract: name the cause,
// name the consuming read, and mutate NOTHING.

func TestSprintInboxAckRefusalWithNoWatchIsActionable(t *testing.T) {
	sprintWatchIsolate(t)
	err := requireAttachedSprintWatch(329, "s329-manager")
	if err == nil {
		t.Fatal("inbox-ack must refuse when the owner has no attached watch")
	}
	msg := err.Error()
	for _, want := range []string{
		"no live attached sprint watch", // the cause, specifically
		"bashy inbox --as s329-manager", // THE CONSUMING READ, by name
		"--watch",                       // ...and how to start the seat's stream
		"take 329 --owner s329-manager", // how to get a batch worth acking
		"--peek",                        // how to look without consuming
		"unread mail is untouched",      // what the refusal did NOT do
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal does not mention %q:\n%s", want, msg)
		}
	}
	// A refusal of an invocation, not a runtime failure: exit 2, like every
	// other sprint guard yoke reports (runerr.go).
	if code := sprintExitCode(err); code != 2 {
		t.Errorf("refusal exit code = %d, want 2 (invalid arg)", code)
	}
}

// A live card in the WRONG mode is a different fact and must not be reported as
// "no watch": the owner is running an ordinary `bashy inbox --watch`, which
// consumes by itself and needs no ack at all. Telling that session to start a
// sprint watch instead would be the opposite of the right advice.
func TestSprintInboxAckRefusalNamesAWrongModeWatcher(t *testing.T) {
	sprintWatchIsolate(t)
	const name = "plain-watcher-manager"
	card := room.Card{
		ID: room.AgentClaimID(name), Nick: name, Mode: inboxWatcherMode,
		Task: "watching Bashy inbox", PID: os.Getpid(),
	}
	if err := room.Join(card); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { room.LeavePID(card.ID, card.PID) })

	err := requireAttachedSprintWatch(413, name)
	if err == nil {
		t.Fatal("a plain inbox watcher has no sprint batch to acknowledge")
	}
	msg := err.Error()
	if !strings.Contains(msg, `"inbox"`) {
		t.Errorf("refusal does not report the mode it actually found:\n%s", msg)
	}
	if !strings.Contains(msg, "needs no ack") {
		t.Errorf("refusal does not say a plain watch consumes by itself:\n%s", msg)
	}
	if strings.Contains(msg, "has no live attached sprint watch") {
		t.Errorf("a live wrong-mode watcher was reported as no watcher at all:\n%s", msg)
	}
}

// The attached watch is accepted. Without this the refusal above is untestable
// as a refusal — anything would "fail correctly".
func TestSprintInboxAckAcceptsAnAttachedSprintWatch(t *testing.T) {
	sprintWatchIsolate(t)
	const name = "attached-manager"
	card := room.Card{
		ID: room.AgentClaimID(name), Nick: name, Mode: "sprint-inbox",
		Task: "managing sprint with attached inbox stream",
		Caps: []string{room.CapInboxStream}, PID: os.Getpid(),
	}
	if err := room.Join(card); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { room.LeavePID(card.ID, card.PID) })

	if err := requireAttachedSprintWatch(413, name); err != nil {
		t.Fatalf("a live attached sprint watch was refused: %v", err)
	}
	// Case-insensitively, too: the mode check used == and a card written by a
	// peer that cased it differently would have been refused.
	card.Mode = "SPRINT-INBOX"
	if err := room.Join(card); err != nil {
		t.Fatal(err)
	}
	if err := requireAttachedSprintWatch(413, name); err != nil {
		t.Fatalf("mode casing decided whether the watch existed: %v", err)
	}
}

// PRESERVATION. The whole reason inbox-ack exists is that unread input is never
// consumed without proof; a refusal that drained anything would defeat it.
func TestSprintInboxAckRefusalPreservesUnreadRecords(t *testing.T) {
	isolateUnifiedInbox(t)
	for _, body := range []string{"first unacked", "second unacked"} {
		if err := bus.PostMessage(bus.Post{From: "human", To: inboxTestReader, Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	before, err := snapshotUnifiedInbox(inboxTestReader, 0, true)
	if err != nil || len(before.events) != 2 {
		t.Fatalf("seed snapshot = %+v, %v", before.events, err)
	}
	if err := requireAttachedSprintWatch(413, inboxTestReader); err == nil {
		t.Fatal("no attached watch exists; the precondition must refuse")
	}
	after, err := snapshotUnifiedInbox(inboxTestReader, 0, true)
	if err != nil || len(after.events) != 2 {
		t.Fatalf("a refused inbox-ack consumed unread records: %+v, %v", after.events, err)
	}
	if bus.SeenSeq(inboxTestReader) != 0 {
		t.Fatal("a refused inbox-ack advanced a source cursor")
	}
}

// The help is the other half of todo 491a03436cbb: "the help should name the
// consuming read". An agent that reads `--help` after a refusal must not have to
// guess which command moves the cursor.
func TestSprintInboxAckHelpNamesTheConsumingRead(t *testing.T) {
	cmd := newSprintInboxAckCmd()
	for _, want := range []string{
		"bashy inbox --as NAME",
		"--watch",
		"--peek",
		"consumes nothing on its own",
	} {
		if !strings.Contains(cmd.Long, want) {
			t.Errorf("inbox-ack --help does not name %q", want)
		}
	}
}
