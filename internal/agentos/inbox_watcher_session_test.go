package agentos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/room"
)

// sessionedWatcherAgent registers an agent whose tool reports a session id, so
// the watcher's card carries a SessionClaim digest and an OwnerPID — the shape
// a watcher has under a real agent harness, where room ownership resolves to
// the session (OwnerPID) and not to the watcher process.
func sessionedWatcherAgent(t *testing.T, name string) {
	t.Helper()
	isolateUnifiedInbox(t)
	const sessionEnv = "BASHY_TEST_WATCHER_SESSION"
	if err := fleet.New().SaveTool(fleet.Tool{Name: "watcher-session-tool", Kind: "cli", CLI: fleet.ToolCLI{Launch: fleet.ToolLaunch{SessionEnv: []string{sessionEnv}}}}); err != nil {
		t.Fatal(err)
	}
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "watcher-session-tool", Model: "test"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(sessionEnv, "watcher-session-value")
}

func TestInboxWatcherWithSessionDigestRetiresItsCardAndRestarts(t *testing.T) {
	const name = "session-digest-watcher"
	sessionedWatcherAgent(t, name)

	claim, err := registerInboxWatcher(name)
	if err != nil {
		t.Fatal(err)
	}
	card, live, err := room.Find(room.AgentClaimID(name))
	if err != nil || !live {
		t.Fatalf("watcher card: live=%v err=%v", live, err)
	}
	if card.SessionClaim == "" || card.OwnerPID != os.Getppid() || card.PID != os.Getpid() {
		t.Fatalf("test needs a session-owned watcher card, got %#v", card)
	}

	claim.leave()
	if members, err := room.Members(); err != nil || len(members) != 0 {
		t.Fatalf("watcher card survived its exit while the session lives: members=%#v err=%v", members, err)
	}

	next, err := registerInboxWatcher(name)
	if err != nil {
		t.Fatalf("restart in the same session refused: %v", err)
	}
	next.leave()
}

func TestInboxWatcherLeaveDoesNotRetireAnotherHoldersCard(t *testing.T) {
	const name = "session-digest-foreign"
	sessionedWatcherAgent(t, name)

	claim, err := registerInboxWatcher(name)
	if err != nil {
		t.Fatal(err)
	}
	claim.leave()

	// A different process now holds the id under the same session digest.
	other := os.Getppid()
	foreign := room.Card{
		ID: room.AgentClaimID(name), Nick: name, Mode: inboxWatcherMode,
		PID: other, OwnerPID: other, SessionClaim: "sha256:foreign",
	}
	if err := room.Join(foreign); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { room.LeavePID(foreign.ID, other) })

	claim.leave()
	if _, live, err := room.Find(foreign.ID); err != nil || !live {
		t.Fatalf("a stale watcher exit retired another holder's card: live=%v err=%v", live, err)
	}
}

func TestInboxWatcherClaimDisabledBypassesLedgerKeepsLocalExclusivity(t *testing.T) {
	const name = "claim-off-watcher"
	isolateUnifiedInbox(t)
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "codex", Model: "gpt5.6-sol"}); err != nil {
		t.Fatal(err)
	}
	for _, off := range []string{"0", "off"} {
		t.Run(off, func(t *testing.T) {
			t.Setenv("BASHY_CLAIM", off)
			notDir := filepath.Join(t.TempDir(), "file")
			if err := os.WriteFile(notDir, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("BASHY_COORD_DIR", filepath.Join(notDir, "ledger"))
			claim, err := registerInboxWatcher(name)
			if err != nil {
				t.Fatalf("watcher needed the coord ledger with claims disabled: %v", err)
			}
			if _, err := registerInboxWatcher(name); err == nil || !strings.Contains(err.Error(), "already has a live inbox watcher") {
				t.Fatalf("second watcher in one process error = %v, want local exclusivity", err)
			}
			claim.leave()
			again, err := registerInboxWatcher(name)
			if err != nil {
				t.Fatalf("restart after leave: %v", err)
			}
			again.leave()
		})
	}
}

func TestInboxWatcherClaimDisabledRefusesLiveCardHeldElsewhere(t *testing.T) {
	const name = "claim-off-foreign"
	sessionedWatcherAgent(t, name)
	t.Setenv("BASHY_CLAIM", "0")

	other := os.Getppid()
	foreign := room.Card{
		ID: room.AgentClaimID(name), Nick: name, Mode: inboxWatcherMode,
		PID: other, OwnerPID: other, SessionClaim: "sha256:foreign",
	}
	if err := room.Join(foreign); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { room.LeavePID(foreign.ID, other) })

	if _, err := registerInboxWatcher(name); err == nil || !strings.Contains(err.Error(), "already has a live inbox watcher") {
		t.Fatalf("watcher over a live foreign card error = %v, want exclusivity", err)
	}
}
