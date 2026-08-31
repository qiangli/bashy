package agentos

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/room"
)

// stubInboxAncestry installs a deterministic process tree. Real pids are
// unusable here: a test cannot kill its own parent, and a pid it does kill may
// be recycled before the assertion runs — which is the exact confusion this
// feature exists to remove.
func stubInboxAncestry(t *testing.T, parents map[int]int) {
	t.Helper()
	priorParent, priorKnown := inboxParentPID, inboxAncestryKnown
	inboxParentPID = func(pid int) (int, bool) {
		parent, ok := parents[pid]
		return parent, ok
	}
	inboxAncestryKnown = func() bool { return true }
	t.Cleanup(func() { inboxParentPID, inboxAncestryKnown = priorParent, priorKnown })
}

func stubInboxAncestryUnsupported(t *testing.T) {
	t.Helper()
	priorParent, priorKnown := inboxParentPID, inboxAncestryKnown
	inboxParentPID = func(int) (int, bool) { return 0, false }
	inboxAncestryKnown = func() bool { return false }
	t.Cleanup(func() { inboxParentPID, inboxAncestryKnown = priorParent, priorKnown })
}

func TestInboxOwnerRelationAnswersRelationshipNotLiveness(t *testing.T) {
	// 10 -> 20 -> 30 -> 1
	tree := map[int]int{10: 20, 20: 30, 30: 1, 1: 0, 40: 1}
	cases := []struct {
		name        string
		pid, anchor int
		want        inboxOwnerState
	}{
		{"direct parent", 10, 20, inboxOwnerLive},
		{"grandparent", 10, 30, inboxOwnerLive},
		{"self", 10, 10, inboxOwnerLive},
		// 40 is a perfectly live pid — it is simply not in this chain, which is
		// what a recycled owner pid looks like.
		{"live but unrelated", 10, 40, inboxOwnerGone},
		{"reparented to init", 10, 99, inboxOwnerGone},
		{"unreadable pid", 77, 20, inboxOwnerGone},
		{"no anchor", 10, 0, inboxOwnerGone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubInboxAncestry(t, tree)
			if got := inboxOwnerRelation(tc.pid, tc.anchor); got != tc.want {
				t.Fatalf("inboxOwnerRelation(%d, %d) = %v, want %v", tc.pid, tc.anchor, got, tc.want)
			}
		})
	}

	t.Run("cycle", func(t *testing.T) {
		stubInboxAncestry(t, map[int]int{10: 20, 20: 10})
		if got := inboxOwnerRelation(10, 30); got != inboxOwnerGone {
			t.Fatalf("cyclic process table = %v, want gone", got)
		}
	})

	t.Run("unsupported platform fails open", func(t *testing.T) {
		stubInboxAncestryUnsupported(t)
		if got := inboxOwnerRelation(10, 40); got != inboxOwnerUnknown {
			t.Fatalf("unsupported ancestry = %v, want unknown", got)
		}
	})
}

// orphanedInboxWatcher registers NAME, then makes the recorded owner
// unreachable from this process so the watcher looks exactly like one whose
// agent session exited and left it reparented.
func orphanedInboxWatcher(t *testing.T, name string) {
	t.Helper()
	isolateUnifiedInbox(t)
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "codex", Model: "gpt5.6-sol"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/"+name)
	// This process's chain dead-ends immediately, so the real ppid recorded at
	// registration is no longer an ancestor.
	stubInboxAncestry(t, map[int]int{})
}

func runInboxWatchCommand(t *testing.T, args ...string) (string, string, error) {
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

func TestInboxWatchStopsBeforeConsumingWhenTheOwningSessionIsGone(t *testing.T) {
	const name = "orphan-sentinel"
	orphanedInboxWatcher(t, name)
	if err := bus.PostMessage(bus.Post{From: "human", To: name, Body: "must survive the orphaned watcher"}); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(bus.Notification{Principal: "scheduler", To: name, Body: "still pending"}); err != nil {
		t.Fatal(err)
	}

	out, _, err := runInboxWatchCommand(t, "--as", name, "--watch", "--wait", "2s")
	if err == nil {
		t.Fatal("orphaned watcher kept monitoring for a session that had exited")
	}
	for _, want := range []string{"monitoring ENDED", name, "resume coverage"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("orphan exit %q omitted %q", err.Error(), want)
		}
	}
	if out != "" {
		t.Fatalf("orphaned watcher rendered records into a dead session: %q", out)
	}
	if got := bus.SeenSeq(name); got != 0 {
		t.Fatalf("orphaned watcher advanced the message-board cursor to %d", got)
	}
	pending, _, err := bus.UnreadNotifications(name)
	if err != nil || len(pending) != 1 {
		t.Fatalf("orphaned watcher consumed bus pending: len=%d err=%v", len(pending), err)
	}
}

func TestOrphanedInboxWatchReleasesItsRoomCardAndKernelClaim(t *testing.T) {
	const name = "orphan-release-sentinel"
	orphanedInboxWatcher(t, name)

	if _, _, err := runInboxWatchCommand(t, "--as", name, "--watch", "--wait", "2s"); err == nil {
		t.Fatal("orphaned watcher returned success")
	}

	members, err := room.Members()
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 0 {
		t.Fatalf("orphaned watcher left its room card behind: %#v", members)
	}
	// The kernel claim is the half a surviving corpse would hold: the whole
	// point of stopping is that the replacement watcher can start.
	next, err := registerInboxWatcher(name)
	if err != nil {
		t.Fatalf("identity stayed claimed after the orphaned watcher exited: %v", err)
	}
	next.leave()
}

func TestInboxWatchKeepsDeliveringWhileTheOwningSessionIsLive(t *testing.T) {
	const name = "live-sentinel"
	isolateUnifiedInbox(t)
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "codex", Model: "gpt5.6-sol"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/"+name)
	stubInboxAncestry(t, map[int]int{os.Getpid(): os.Getppid(), os.Getppid(): 1})
	if err := bus.PostMessage(bus.Post{From: "human", To: name, Body: "delivered to a live session"}); err != nil {
		t.Fatal(err)
	}

	out, _, err := runInboxWatchCommand(t, "--as", name, "--watch", "--wait", "300ms")
	if err != nil {
		t.Fatalf("live watcher stopped: %v", err)
	}
	if !strings.Contains(out, "delivered to a live session") {
		t.Fatalf("live watcher delivered nothing: %q", out)
	}
	if bus.SeenSeq(name) == 0 {
		t.Fatal("live watcher rendered a record without acknowledging it")
	}
}

func TestInboxWatchKeepsDeliveringWhereAncestryCannotBeRead(t *testing.T) {
	const name = "unproved-sentinel"
	isolateUnifiedInbox(t)
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "codex", Model: "gpt5.6-sol"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/"+name)
	stubInboxAncestryUnsupported(t)
	if err := bus.PostMessage(bus.Post{From: "human", To: name, Body: "unsupported platforms still deliver"}); err != nil {
		t.Fatal(err)
	}

	out, _, err := runInboxWatchCommand(t, "--as", name, "--watch", "--wait", "300ms")
	if err != nil {
		t.Fatalf("watch failed closed on a platform without a process tree: %v", err)
	}
	if !strings.Contains(out, "unsupported platforms still deliver") {
		t.Fatalf("unproved-ancestry watcher delivered nothing: %q", out)
	}
}

func joinInboxWatcherCard(t *testing.T, name string, ownerPID int) {
	t.Helper()
	card := room.Card{
		ID: room.AgentClaimID(name), Nick: name, Tool: "codex", Model: "gpt5.6-sol",
		Binding: "codex:gpt5.6-sol", Mode: inboxWatcherMode, Task: "watching Bashy inbox",
		PID: os.Getpid(), OwnerPID: ownerPID,
	}
	if err := room.Join(card); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { room.Leave(card.ID) })
}

func rosterAssignment(t *testing.T, name string) agentAssignment {
	t.Helper()
	assignments, err := reconciledAgentRoster()
	if err != nil {
		t.Fatal(err)
	}
	for _, assignment := range assignments {
		if assignment.Agent == name {
			return assignment
		}
	}
	t.Fatalf("no roster assignment for %q: %#v", name, assignments)
	return agentAssignment{}
}

func TestAgentRosterClassifiesAnOrphanedInboxWatcherNonLive(t *testing.T) {
	const name = "roster-orphan-sentinel"
	isolateUnifiedInbox(t)
	t.Setenv("HOME", t.TempDir())
	const owner = 424242
	joinInboxWatcherCard(t, name, owner)

	// The watcher process is alive and the owner pid is even in use — by an
	// unrelated process. Only the relationship distinguishes the two cases.
	stubInboxAncestry(t, map[int]int{os.Getpid(): 7, 7: 1, owner: 1})
	orphan := rosterAssignment(t, name)
	if orphan.Health != "orphaned" || !strings.Contains(orphan.HealthReason, "outlived the agent session") {
		t.Fatalf("orphaned watcher assignment = %#v", orphan)
	}
	if assignmentLive(orphan) {
		t.Fatalf("orphaned inbox watcher counted as live: %#v", orphan)
	}

	var view bytes.Buffer
	if err := renderAgentRosterView(&view, false, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(view.String(), name) {
		t.Fatalf("default roster still shows the orphaned watcher:\n%s", view.String())
	}
	if !strings.Contains(view.String(), "ORPHANED 1") {
		t.Fatalf("orphaned watcher was not counted:\n%s", view.String())
	}

	var all bytes.Buffer
	if err := renderAgentRosterView(&all, false, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(all.String(), name) {
		t.Fatalf("--all hid the orphaned watcher:\n%s", all.String())
	}

	// Same card, owner still an ancestor: unchanged behavior.
	stubInboxAncestry(t, map[int]int{os.Getpid(): owner, owner: 1})
	live := rosterAssignment(t, name)
	if live.Health != "healthy" || !assignmentLive(live) {
		t.Fatalf("watcher with a live owning session = %#v", live)
	}
}
