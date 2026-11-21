package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/meet"
	"github.com/qiangli/yoke/pkg/weave"
)

func parseNDJSONInboxEvents(t *testing.T, raw string) []unifiedInboxEvent {
	t.Helper()
	var out []unifiedInboxEvent
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev unifiedInboxEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("unmarshal NDJSON line %q: %v", line, err)
		}
		out = append(out, ev)
	}
	return out
}

// TestInboxS678SequenceRoleDoneAfterLeaseTTLExpiry reproduces the Sprint 678
// incident:
//  1. Conductor holds the lease on Sprint 678 ("doing").
//  2. Worker sends `DONE fa9fcc8de90d` via `mb send --role conductor` (Audience
//     {Role: "conductor"}), another report to `conductor:678` (`conductor.678`),
//     and a report inside Sprint 678's Meet contact room (even when the room's
//     participants list still names an earlier holder alias).
//  3. The 30m lease TTL elapses before the next `bashy inbox --as <conductor>`
//     poll.
//
// Before the fix:
//   - newUnifiedInboxCmd bypassed refreshSprintOwnerActivity on entry, so
//     polling inbox never renewed the sprint lease;
//   - once the 30m TTL elapsed, LiveSprintManagers and conductorRoles dropped
//     Sprint 678, hiding all role/seat mail from `bashy inbox --as <conductor>`.
func TestInboxS678SequenceRoleDoneAfterLeaseTTLExpiry(t *testing.T) {
	isolateUnifiedInbox(t)
	wireMessageBoard()

	conductor := inboxTestReader
	now := time.Now().UTC()

	// Create Sprint 678's Meet room under a prior session's agent (as observed on
	// Sprint 576's contact.ref room) to verify conducted-room inclusion even when
	// the current conductor is not listed in room.Members.
	roomState, err := meet.Create(meet.CreateOptions{
		Topic:        "conductor 678: make dag bridge",
		Participants: []string{"claude-opus5"},
		Human:        "operator",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Step 1: Sprint 678 lease is live when worker sends DONE.
	liveStory := seededStory(678, conductor, now)
	liveStory["contact"] = map[string]any{
		"kind":   "meet",
		"ref":    roomState.ID,
		"topic":  "conductor.678",
		"name":   "sprint 678",
		"holder": "claude-opus5",
	}
	seedSprintBoard(t, []map[string]any{liveStory})

	// Step 2: Worker posts role-addressed DONE while lease is live.
	if _, err := bus.Send(bus.SendRequest{
		From:     "muse-wonder-full-1.0",
		Audience: &bus.Audience{Role: "conductor"},
		Topic:    "mb",
		Body:     "[ref:s678s1 1/3] DONE fa9fcc8de90d: branch s678/fa9fcc8d ready for review",
	}); err != nil {
		t.Fatal(err)
	}
	if err := bus.PostMessage(bus.Post{
		From:  "worker-two",
		To:    "conductor.678",
		Topic: "mb",
		Body:  "DESIGN 9ac774dd8009: files and test plan ready",
	}); err != nil {
		t.Fatal(err)
	}
	if err := meet.AppendEvent(roomState.ID, meet.Event{
		Speaker: "worker-three",
		To:      "conductor:678",
		Kind:    "message",
		Text:    "BLOCKED 1ee0c4e59988: waiting on clarification",
	}); err != nil {
		t.Fatal(err)
	}

	// Step 3: 45 minutes pass (> weave.SprintLeaseTTL = 30m) before the conductor
	// polls `bashy inbox --as <conductor>`.
	staleAt := now.Add(-weave.SprintLeaseTTL - 15*time.Minute)
	staleStory := seededStory(678, conductor, staleAt)
	staleStory["contact"] = liveStory["contact"]
	seedSprintBoard(t, []map[string]any{staleStory})

	// Verify Part A: even if refreshSprintOwnerActivity were a no-op (lease
	// remains stale on disk during the snapshot), conducted-sprint scoping still
	// surfaces all three seat/role messages and types them accurately.
	prevRefresh := refreshSprintOwnerActivity
	var refreshedFor []string
	refreshSprintOwnerActivity = func(name string) {
		refreshedFor = append(refreshedFor, name)
	}
	t.Cleanup(func() { refreshSprintOwnerActivity = prevRefresh })

	out, _, err := runInboxCmd(t, "--as", conductor, "--peek", "--json")
	if err != nil {
		t.Fatalf("inbox --peek after TTL expiry: %v", err)
	}
	if len(refreshedFor) != 1 || refreshedFor[0] != conductor {
		t.Fatalf("newUnifiedInboxCmd entry refresh calls = %v, want [%s]", refreshedFor, conductor)
	}

	events := parseNDJSONInboxEvents(t, out)
	if len(events) != 3 {
		t.Fatalf("events after TTL expiry = %d, want 3:\n%s", len(events), out)
	}

	byType := map[string]unifiedInboxEvent{}
	for _, ev := range events {
		byType[ev.EventType] = ev
	}
	doneEv, ok := byType["worker_done"]
	if !ok || doneEv.StoryID != "fa9fcc8de90d" || doneEv.SuggestedVerb != "review" || doneEv.To != "role conductor" {
		t.Fatalf("worker_done event = %+v, want story_id=fa9fcc8de90d suggested_verb=review to='role conductor'", doneEv)
	}
	designEv, ok := byType["worker_design"]
	if !ok || designEv.StoryID != "9ac774dd8009" || designEv.SuggestedVerb != "reply" || designEv.To != "conductor:678" {
		t.Fatalf("worker_design event = %+v, want story_id=9ac774dd8009 suggested_verb=reply to='conductor:678'", designEv)
	}
	blockedEv, ok := byType["worker_blocked"]
	if !ok || blockedEv.StoryID != "1ee0c4e59988" || blockedEv.SuggestedVerb != "reply" || blockedEv.Source != "meet" {
		t.Fatalf("worker_blocked event = %+v, want story_id=1ee0c4e59988 suggested_verb=reply source='meet'", blockedEv)
	}

	// Verify Part B: with the real weave.RefreshSprintOwnerActivity restored, a
	// CLI `bashy inbox --as <conductor>` poll renews the stale lease on disk so
	// subsequent `FleetSelect(role=conductor)` sees the conductor as live again.
	refreshSprintOwnerActivity = prevRefresh
	if _, _, err := runInboxCmd(t, "--as", conductor, "--peek"); err != nil {
		t.Fatalf("inbox --peek with real refresher: %v", err)
	}
	liveManagers, err := weave.LiveSprintManagers()
	if err != nil {
		t.Fatal(err)
	}
	if !stringMember(liveManagers, conductor) {
		t.Fatalf("LiveSprintManagers after inbox poll = %v, want %s renewed", liveManagers, conductor)
	}
}

func TestInboxUntilCountExitsZeroOnNthMatch(t *testing.T) {
	isolateUnifiedInbox(t)
	wireMessageBoard()
	reader := inboxTestReader

	// Seed 1 matching message and 1 non-matching message before starting wait.
	if err := bus.PostMessage(bus.Post{From: "worker-a", To: reader, Body: "DONE aabbcc112233: first story ready"}); err != nil {
		t.Fatal(err)
	}
	if err := bus.PostMessage(bus.Post{From: "worker-b", To: reader, Body: "progress heartbeat"}); err != nil {
		t.Fatal(err)
	}

	posted := make(chan error, 1)
	go func() {
		time.Sleep(40 * time.Millisecond)
		_ = bus.PostMessage(bus.Post{From: "worker-c", To: reader, Body: "still running"})
		time.Sleep(40 * time.Millisecond)
		posted <- bus.PostMessage(bus.Post{From: "worker-b", To: reader, Body: "DONE ddeeff445566: second story ready"})
	}()

	started := time.Now()
	out, _, err := runInboxCmd(t, "--as", reader, "--wait", "2s", "--until-count", "2", "--match", `^DONE\s+[0-9a-f]+`, "--json")
	if err != nil {
		t.Fatalf("until-count=2 returned error: %v (exit=%d)", err, inboxCommandExitCode(err))
	}
	if err := <-posted; err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 60*time.Millisecond {
		t.Fatalf("until-count=2 returned before second match arrived (elapsed=%s)", elapsed)
	}

	events := parseNDJSONInboxEvents(t, out)
	if len(events) != 2 {
		t.Fatalf("matched events = %d, want 2:\n%s", len(events), out)
	}
	if events[0].StoryID != "aabbcc112233" || events[1].StoryID != "ddeeff445566" {
		t.Fatalf("unexpected story IDs: %+v", events)
	}
	// Non-consuming --wait --until-count leaves cursor untouched.
	if got := bus.SeenSeq(reader); got != 0 {
		t.Fatalf("--wait --until-count advanced MB cursor to %d, want 0", got)
	}
}

func TestInboxUntilCountTimeoutExitsThreeWhilePlainWaitExitsZero(t *testing.T) {
	isolateUnifiedInbox(t)
	wireMessageBoard()
	reader := inboxTestReader

	// Non-matching message must not satisfy --until-count 1 --match '^DONE\b'.
	if err := bus.PostMessage(bus.Post{From: "worker-a", To: reader, Body: "progress check-in"}); err != nil {
		t.Fatal(err)
	}

	cmd, _, ok := newBusFrontDoorCmd("inbox")
	if !ok {
		t.Fatal("inbox front-door command not mounted")
	}
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--as", reader, "--wait", "25ms", "--until-count", "1", "--match", `^DONE\b`, "--json"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected timeout error when --until-count 1 is unmet")
	}
	if got := inboxCommandExitCode(err); got != 3 {
		t.Fatalf("timeout exit code = %d, want 3 (err=%v)", got, err)
	}
	if !inboxSilentExit(err) {
		t.Fatal("timeout error must be marked silent so front-door dispatch does not double-print")
	}
	var env map[string]any
	if uerr := json.Unmarshal(out.Bytes(), &env); uerr != nil || env["schema"] != unifiedInboxSchema {
		t.Fatalf("timeout stdout = %q, want valid %s empty envelope (err=%v)", out.String(), unifiedInboxSchema, uerr)
	}
	if !strings.Contains(errOut.String(), "timeout") {
		t.Fatalf("timeout stderr = %q, want timeout message", errOut.String())
	}

	// Plain --wait (without --until-count) must continue exiting 0 on timeout.
	plainOut, _, plainErr := runInboxCmd(t, "--as", reader, "--wait", "15ms", "--match", `^DONE\b`, "--json")
	if plainErr != nil {
		t.Fatalf("plain --wait without --until-count returned error: %v", plainErr)
	}
	if uerr := json.Unmarshal([]byte(plainOut), &env); uerr != nil || env["schema"] != unifiedInboxSchema {
		t.Fatalf("plain --wait stdout = %q, want valid %s envelope", plainOut, unifiedInboxSchema)
	}
}

func TestInboxSearchRemainsSubstringWhileMatchUsesRegex(t *testing.T) {
	isolateUnifiedInbox(t)
	wireMessageBoard()
	reader := inboxTestReader

	for _, body := range []string{
		"[ref:s678s1 1/3] status.ok (v1)",
		"statusXok plain note",
	} {
		if err := bus.PostMessage(bus.Post{From: "worker-a", To: reader, Body: body}); err != nil {
			t.Fatal(err)
		}
	}

	// --search treats brackets and dots as literal substring characters (never regex).
	searchOut, _, err := runInboxCmd(t, "--as", reader, "--search", "[ref:s678s1 1/3] status.ok", "--peek")
	if err != nil {
		t.Fatalf("--search with brackets/dots failed: %v", err)
	}
	if !strings.Contains(searchOut, "[ref:s678s1 1/3] status.ok (v1)") || strings.Contains(searchOut, "statusXok") {
		t.Fatalf("--search output = %q, want only literal bracket+dot match", searchOut)
	}

	// Unclosed bracket in --search still works as literal substring.
	unclosedOut, _, err := runInboxCmd(t, "--as", reader, "--search", "[ref:s678s1", "--peek")
	if err != nil || !strings.Contains(unclosedOut, "[ref:s678s1 1/3]") {
		t.Fatalf("--search unclosed bracket = %q, err=%v; want literal match", unclosedOut, err)
	}

	// --match compiles a Go regular expression and rejects invalid syntax upfront.
	matchOut, _, err := runInboxCmd(t, "--as", reader, "--match", `status[X.]ok`, "--peek")
	if err != nil {
		t.Fatalf("--match regex failed: %v", err)
	}
	if !strings.Contains(matchOut, "status.ok") || !strings.Contains(matchOut, "statusXok") {
		t.Fatalf("--match output = %q, want both regex matches", matchOut)
	}

	if _, _, err := runInboxCmd(t, "--as", reader, "--match", `[unclosed`, "--peek"); err == nil || !strings.Contains(err.Error(), "invalid --match regex") {
		t.Fatalf("invalid --match regex err = %v, want invalid --match regex refusal", err)
	}
}
