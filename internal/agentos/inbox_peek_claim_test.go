package agentos

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/room"
)

func foreignLivePID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start foreign holder: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	return cmd.Process.Pid
}

func TestInboxPeekClaimSatisfiesMbSendInSameSession(t *testing.T) {
	isolateUnifiedInbox(t)
	const name = "peek-claim-agent"
	const target = "peek-target-agent"
	if err := fleet.New().SaveTool(fleet.Tool{Name: "claude", Kind: "cli", CLI: fleet.ToolCLI{Launch: fleet.ToolLaunch{SessionEnv: []string{"CLAUDE_CODE_SESSION_ID"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "claude", Model: "opus5"}); err != nil {
		t.Fatal(err)
	}
	if err := fleet.New().SaveAgent(fleet.Agent{Name: target, Tool: "claude", Model: "opus5"}); err != nil {
		t.Fatal(err)
	}
	wireMessageBoard()
	const fakeSession = "test-session-12345-fake-id"

	t.Setenv("CLAUDE_CODE_SESSION_ID", fakeSession)
	origDetect := bus.DetectHarness
	bus.DetectHarness = func() (string, bool) { return "claude", true }
	t.Cleanup(func() { bus.DetectHarness = origDetect })
	wireMessageBoard()

	// No incumbent — empty seat. Peek should claim it.
	_, _, _ = room.RemoveMember(room.AgentClaimID(name))
	if _, live, _ := room.Find(room.AgentClaimID(name)); live {
		t.Fatal("precondition: no live claim")
	}
	out, _, err := runInboxCmd(t, "--as", name, "--peek", "--json")
	if err != nil {
		t.Fatalf("peek failed: %v out=%q", err, out)
	}
	card, live, ferr := room.Find(room.AgentClaimID(name))
	if ferr != nil || !live {
		t.Fatalf("peek claim not live: %v live=%v", ferr, live)
	}
	wantHash := bus.HashSessionClaim(fakeSession)
	if card.SessionClaim != wantHash {
		t.Fatalf("peek claim hash = %q, want %q", card.SessionClaim, wantHash)
	}
	if card.Mode != "peek" {
		t.Fatalf("peek claim mode = %q, want peek", card.Mode)
	}

	// After peek, mb send --as SEAT should succeed without BASHY_AGENT
	t.Setenv("BASHY_AGENT", "")
	cmd2 := bus.NewMessageBoardCmd()
	var out2 bytes.Buffer
	cmd2.SetOut(&out2)
	cmd2.SetErr(&bytes.Buffer{})
	cmd2.SetArgs([]string{"send", "--as", name, target, "hello after peek"})
	if err := cmd2.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("mb send after peek failed: %v", err)
	}
	posts, err := bus.Posts()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range posts {
		if p.Body == "hello after peek" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("post not found after peek claim")
	}

	// Different session should still be refused (do not weaken rule)
	t.Setenv("CLAUDE_CODE_SESSION_ID", "different-session-999")
	wireMessageBoard()
	cmd3 := bus.NewMessageBoardCmd()
	cmd3.SetOut(&bytes.Buffer{})
	cmd3.SetErr(&bytes.Buffer{})
	cmd3.SetArgs([]string{"send", "--as", name, target, "should be refused"})
	if err := cmd3.ExecuteContext(context.Background()); err == nil || !strings.Contains(err.Error(), "no matching live session claim") {
		t.Fatalf("different session should be refused, got err=%v", err)
	}
}

func TestInboxPeekDoesNotEvictLiveForeignHolder(t *testing.T) {
	isolateUnifiedInbox(t)
	const name = "peek-foreign-agent"
	const target = "peek-foreign-target"
	if err := fleet.New().SaveTool(fleet.Tool{Name: "claude", Kind: "cli", CLI: fleet.ToolCLI{Launch: fleet.ToolLaunch{SessionEnv: []string{"CLAUDE_CODE_SESSION_ID"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "claude", Model: "opus5"}); err != nil {
		t.Fatal(err)
	}
	if err := fleet.New().SaveAgent(fleet.Agent{Name: target, Tool: "claude", Model: "opus5"}); err != nil {
		t.Fatal(err)
	}
	wireMessageBoard()

	// Create a LIVE foreman holder with a different session digest.
	livePID := foreignLivePID(t)
	foremanSession := "foreman-session-different-xyz"
	foremanHash := bus.HashSessionClaim(foremanSession)
	if err := room.Join(room.Card{
		ID: room.AgentClaimID(name), Nick: name, Tool: "claude", Binding: "claude:test",
		Mode: "foreman", PID: livePID, OwnerPID: livePID, SessionClaim: foremanHash, Principal: "operator",
	}); err != nil {
		// Join may use session claim path; if foreman mode uses Join without digest handling, force via ClaimSession
		// Fallback: try ClaimSession
		if err2 := room.ClaimSession(room.Card{
			ID: room.AgentClaimID(name), Nick: name, Tool: "claude", Binding: "claude:test",
			Mode: "foreman", PID: livePID, OwnerPID: livePID, SessionClaim: foremanHash, Principal: "operator",
		}); err2 != nil {
			t.Fatalf("foreman claim: join=%v claim=%v", err, err2)
		}
	}
	t.Cleanup(func() { room.LeavePID(room.AgentClaimID(name), livePID) })
	// Verify live
	if card, live, _ := room.Find(room.AgentClaimID(name)); !live || card.SessionClaim != foremanHash {
		t.Fatalf("foreman holder not live as expected")
	}

	// Attended session with different CLAUDE_CODE_SESSION_ID tries to peek.
	const fakeSession = "attended-session-aaa-111"
	t.Setenv("CLAUDE_CODE_SESSION_ID", fakeSession)
	origDetect := bus.DetectHarness
	bus.DetectHarness = func() (string, bool) { return "claude", true }
	t.Cleanup(func() { bus.DetectHarness = origDetect })
	wireMessageBoard()

	// Peek should still read but MUST NOT evict the live foreign holder.
	out, _, err := runInboxCmd(t, "--as", name, "--peek", "--json")
	if err != nil {
		t.Fatalf("peek with live foreign holder failed to read: %v out=%q", err, out)
	}
	// Holder must remain the foreman, not the peek session.
	card, live, ferr := room.Find(room.AgentClaimID(name))
	if ferr != nil || !live {
		t.Fatalf("card missing after peek: %v live=%v", ferr, live)
	}
	if card.SessionClaim != foremanHash {
		t.Fatalf("live foreign holder was evicted! hash now %q want foreman %q (mode %q)", card.SessionClaim, foremanHash, card.Mode)
	}
	if card.Mode != "foreman" {
		t.Fatalf("mode changed after peek: got %q want foreman", card.Mode)
	}

	// mb send --as SEAT must be refused and error must name holder and mode
	// and guide to take.
	t.Setenv("BASHY_AGENT", "")
	// Use the wrapped command (via agentos wiring) to get holder-detail error.
	// newBusFrontDoorCmd wraps errors; NewMessageBoardCmd directly does not fully wrap.
	// So we use bus.NewMessageBoardCmd but also check room directly for holder detail in wrapped path.
	// To exercise the wrapping, use newBusFrontDoorCmd("mb").
	mbCmd, _, ok := newBusFrontDoorCmd("mb")
	if !ok {
		t.Fatal("mb not mounted")
	}
	mbCmd.SetOut(&bytes.Buffer{})
	mbCmd.SetErr(&bytes.Buffer{})
	mbCmd.SetArgs([]string{"send", "--as", name, target, "should be refused - live foreign"})
	err = mbCmd.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no matching live session claim") {
		t.Fatalf("expected refusal with live foreign holder, got err=%v", err)
	}
	// Must name holder mode and pid, and mention sprint take
	msg := err.Error()
	if !strings.Contains(strings.ToLower(msg), "foreman") {
		t.Fatalf("refusal should name holder mode foreman, got: %q", msg)
	}
	if !strings.Contains(msg, "held live by") {
		t.Fatalf("refusal should say 'held live by', got: %q", msg)
	}
	if !strings.Contains(msg, "bashy sprint take") {
		t.Fatalf("refusal should guide to `bashy sprint take`, got: %q", msg)
	}
}

func TestInboxPeekAdoptsStaleHolder(t *testing.T) {
	isolateUnifiedInbox(t)
	const name = "peek-stale-agent"
	const target = "peek-stale-target"
	if err := fleet.New().SaveTool(fleet.Tool{Name: "claude", Kind: "cli", CLI: fleet.ToolCLI{Launch: fleet.ToolLaunch{SessionEnv: []string{"CLAUDE_CODE_SESSION_ID"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "claude", Model: "opus5"}); err != nil {
		t.Fatal(err)
	}
	if err := fleet.New().SaveAgent(fleet.Agent{Name: target, Tool: "claude", Model: "opus5"}); err != nil {
		t.Fatal(err)
	}
	wireMessageBoard()

	// Create a STALE holder (dead pid). Use a pid that definitely does not exist.
	// room.Join will write card but cardAlive will be false because pid dead.
	stalePID := 999999
	staleHash := bus.HashSessionClaim("stale-session-old")
	// Ensure stale pid is not alive
	if err := room.Join(room.Card{
		ID: room.AgentClaimID(name), Nick: name, Tool: "claude", Binding: "claude:test",
		Mode: "foreman", PID: stalePID, OwnerPID: stalePID, SessionClaim: staleHash, Principal: "operator",
	}); err != nil {
		// Join may succeed even for dead pid; if it fails due to digest, try ClaimSession
		_ = room.ClaimSession(room.Card{
			ID: room.AgentClaimID(name), Nick: name, Tool: "claude", Binding: "claude:test",
			Mode: "foreman", PID: stalePID, OwnerPID: stalePID, SessionClaim: staleHash, Principal: "operator",
		})
	}
	// Verify not live (stale)
	if _, live, _ := room.Find(room.AgentClaimID(name)); live {
		t.Fatalf("stale holder unexpectedly live (pid %d may be alive), pick different pid", stalePID)
	}

	const fakeSession = "new-session-adopts-stale"
	t.Setenv("CLAUDE_CODE_SESSION_ID", fakeSession)
	origDetect := bus.DetectHarness
	bus.DetectHarness = func() (string, bool) { return "claude", true }
	t.Cleanup(func() { bus.DetectHarness = origDetect })
	wireMessageBoard()

	out, _, err := runInboxCmd(t, "--as", name, "--peek", "--json")
	if err != nil {
		t.Fatalf("peek with stale holder failed: %v out=%q", err, out)
	}
	card, live, ferr := room.Find(room.AgentClaimID(name))
	if ferr != nil || !live {
		t.Fatalf("peek did not adopt stale holder: %v live=%v", ferr, live)
	}
	wantHash := bus.HashSessionClaim(fakeSession)
	if card.SessionClaim != wantHash {
		t.Fatalf("stale adoption hash = %q want %q", card.SessionClaim, wantHash)
	}

	// mb send should now succeed (peek claimed stale seat)
	t.Setenv("BASHY_AGENT", "")
	cmd2 := bus.NewMessageBoardCmd()
	var out2 bytes.Buffer
	cmd2.SetOut(&out2)
	cmd2.SetErr(&bytes.Buffer{})
	cmd2.SetArgs([]string{"send", "--as", name, target, "hello after stale adopt"})
	if err := cmd2.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("mb send after stale adoption failed: %v", err)
	}
}

func TestInboxPeekWithoutSessionDoesNotCreateClaim(t *testing.T) {
	isolateUnifiedInbox(t)
	const name = "peek-no-session-agent"
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "claude", Model: "opus5"}); err != nil {
		t.Fatal(err)
	}
	wireMessageBoard()
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("BASHY_AGENT", "")
	// Ensure no existing claim
	_, _, _ = room.RemoveMember(room.AgentClaimID(name))
	if _, live, _ := room.Find(room.AgentClaimID(name)); live {
		t.Fatal("precondition: no live claim")
	}
	if _, _, err := runInboxCmd(t, "--as", name, "--peek"); err != nil {
		t.Fatal(err)
	}
	if _, live, _ := room.Find(room.AgentClaimID(name)); live {
		t.Fatal("peek without session created a claim unexpectedly")
	}
}
