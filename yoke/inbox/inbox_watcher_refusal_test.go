package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/room"
)

func TestDuplicateInboxWatcherRefusalNamesTheHolder(t *testing.T) {
	const name = "duplicate-watcher-diagnosis"
	sessionedWatcherAgent(t, name)

	claim, err := registerSprintInboxWatcher(name)
	if err != nil {
		t.Fatal(err)
	}
	defer claim.leave()

	_, err = registerSprintInboxWatcher(name)
	if err == nil {
		t.Fatal("a second watcher for one identity must be refused")
	}
	msg := err.Error()
	// The rule every existing caller matches on, unchanged.
	if !strings.Contains(msg, "already has a live inbox watcher") {
		t.Errorf("refusal lost the rule it states:\n%s", msg)
	}
	for _, want := range []string{
		fmt.Sprintf("pid %d", os.Getpid()), // WHO holds it
		`mode "sprint-inbox"`,              // and as what
		"bashy agent list --all",           // how to look
		"nothing was consumed",             // what the refusal did not do
		"--peek",                           // the read that needs no watcher
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal does not mention %q:\n%s", want, msg)
		}
	}
}

// The diagnostic is best-effort: a refusal must never depend on being able to
// read the holder's card, or an unreadable room turns a clear conflict into a
// confusing one.
func TestDuplicateInboxWatcherRefusalSurvivesAnUnreadableHolderCard(t *testing.T) {
	msg := errInboxWatcherLive(inboxWatcherIdentity{
		kind: "registered agent", name: "no-card-here",
		claimID: room.AgentClaimID("no-card-here"),
	}).Error()
	if !strings.Contains(msg, "already has a live inbox watcher") {
		t.Fatalf("refusal without a readable holder card lost its message:\n%s", msg)
	}
	if strings.Contains(msg, "holder:") {
		t.Fatalf("refusal invented a holder it could not read:\n%s", msg)
	}
}
