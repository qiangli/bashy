// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/principal"
	"github.com/qiangli/yoke/pkg/room"
)

func isolateExtensionTestEnv(t *testing.T) *fleet.InstanceStore {
	t.Helper()
	isolateUnifiedInbox(t)
	wireMessageBoard()
	t.Setenv("BASHY_MAILBOX_DIR", t.TempDir())
	t.Setenv("BASHY_PRINCIPAL", "")
	t.Setenv("BASHY_INSTANCE", "")
	t.Setenv("BASHY_INSTANCE_UUID", "")
	t.Setenv("BASHY_SESSION_CLAIM", "")
	return fleet.NewInstanceStore("")
}

func openTestLineInstance(t *testing.T, store *fleet.InstanceStore, label, session string, mailFrom int64) fleet.Instance {
	t.Helper()
	inst, err := store.Open(fleet.Family{
		Name: "claude", Display: bus.MainLineBase(label), Policy: fleet.PolicySingle, Bindings: []string{"claude:sonnet4.6"},
	}, fleet.OpenOptions{Label: label, MailFrom: mailFrom})
	if err != nil {
		t.Fatalf("open %q: %v", label, err)
	}
	if session != "" {
		if err := room.Join(room.Card{
			ID:           inst.ClaimID(),
			Nick:         label,
			Tool:         "claude",
			Binding:      "claude:sonnet4.6",
			Mode:         "interactive",
			PID:          os.Getpid(),
			OwnerPID:     os.Getpid(),
			SessionClaim: bus.HashSessionClaim(session),
			Principal:    principal.InstanceURN(inst.UUID),
		}); err != nil {
			t.Fatalf("room.Join %q: %v", label, err)
		}
	}
	return inst
}

func runTestMBCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := bus.NewMessageBoardCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetContext(context.Background())
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func runTestMailboxListCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := newUnifiedInboxCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetContext(context.Background())
	cmd.SetArgs(append([]string{"list"}, args...))
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestExtensions_DirectMailIsolationAndReusedExtensionStartsEmpty(t *testing.T) {
	store := isolateExtensionTestEnv(t)
	openTestLineInstance(t, store, "claude", "sess-main", 0)
	i2 := openTestLineInstance(t, store, "claude+2", "sess-plus2", 0)
	openTestLineInstance(t, store, "claude+3", "sess-plus3", 0)

	if _, err := bus.Send(bus.SendRequest{From: "operator", To: "claude+2", Body: "direct to plus2"}); err != nil {
		t.Fatalf("send claude+2: %v", err)
	}
	if _, err := bus.Send(bus.SendRequest{From: "operator", To: "claude+3", Body: "direct to plus3"}); err != nil {
		t.Fatalf("send claude+3: %v", err)
	}

	outMain, _, err := runInboxCmd(t, "--as", "claude", "--peek")
	if err != nil {
		t.Fatalf("inbox peek claude: %v", err)
	}
	if strings.Contains(outMain, "direct to plus2") || strings.Contains(outMain, "direct to plus3") {
		t.Fatalf("claude main line saw extension direct mail:\n%s", outMain)
	}

	out2, _, err := runInboxCmd(t, "--as", "claude+2", "--peek")
	if err != nil || !strings.Contains(out2, "direct to plus2") || strings.Contains(out2, "direct to plus3") {
		t.Fatalf("claude+2 inbox mismatch (err=%v):\n%s", err, out2)
	}
	out3, _, err := runInboxCmd(t, "--as", "claude+3", "--peek")
	if err != nil || !strings.Contains(out3, "direct to plus3") || strings.Contains(out3, "direct to plus2") {
		t.Fatalf("claude+3 inbox mismatch (err=%v):\n%s", err, out3)
	}

	// Retire old claude+2 with unread mail still pending; new claude+2 starts empty.
	if _, err := store.Retire(i2.UUID, bus.InstanceMail); err != nil {
		t.Fatalf("retire old claude+2: %v", err)
	}
	room.Leave(i2.ClaimID())

	openTestLineInstance(t, store, "claude+2", "sess-plus2-reused", bus.BoardHeadSeq()+1)
	out2Reused, _, err := runInboxCmd(t, "--as", "claude+2")
	if err != nil {
		t.Fatalf("reused claude+2 inbox: %v", err)
	}
	if strings.Contains(out2Reused, "direct to plus2") {
		t.Fatalf("reused claude+2 saw old generation's mail:\n%s", out2Reused)
	}
}

func TestExtensions_HuntGroupWaitsWithoutLiveSessionAndClaimsOnceConcurrently(t *testing.T) {
	store := isolateExtensionTestEnv(t)

	res, err := bus.Send(bus.SendRequest{From: "operator", To: "claude", Body: "hunt-group job A"})
	if err != nil {
		t.Fatalf("send to claude main line: %v", err)
	}

	// An instance with no live room card must not claim a hunt-group message on a consuming read.
	openTestLineInstance(t, store, "claude+8", "", bus.BoardHeadSeq()+1)
	ghostOut, _, err := runTestMBCmd(t, "--as", "claude+8")
	if err != nil {
		t.Fatalf("ghost mb read: %v", err)
	}
	if strings.Contains(ghostOut, "hunt-group job A") {
		t.Fatalf("non-live instance claimed waiting hunt-group post:\n%s", ghostOut)
	}
	if holder := bus.ClaimHolder(res.Seq); holder != "" {
		t.Fatalf("ClaimHolder = %q after non-live mb read; want empty", holder)
	}

	var live []fleet.Instance
	for idx, label := range []string{"claude", "claude+2", "claude+3"} {
		live = append(live, openTestLineInstance(t, store, label, fmt.Sprintf("sess-hg-%d", idx), bus.BoardHeadSeq()+1))
	}

	// Observational inbox/mb --peek does not claim the hunt-group post.
	peekOut, _, err := runInboxCmd(t, "--as", "claude+3", "--peek")
	if err != nil || !strings.Contains(peekOut, "hunt-group job A") {
		t.Fatalf("live --peek missed waiting hunt-group post (err=%v):\n%s", err, peekOut)
	}
	if holder := bus.ClaimHolder(res.Seq); holder != "" {
		t.Fatalf("--peek claimed hunt-group post: %q", holder)
	}

	// First verify interactive live instances claim job A atomically via consuming mb reads.
	var mbWinners atomic.Int64
	var mbWG sync.WaitGroup
	mbStart := make(chan struct{})
	for _, inst := range live {
		mbWG.Add(1)
		go func(target fleet.Instance) {
			defer mbWG.Done()
			<-mbStart
			out, _, err := runTestMBCmd(t, "--as", target.MailAddress())
			if err == nil && strings.Contains(out, "hunt-group job A") {
				mbWinners.Add(1)
			}
		}(inst)
	}
	close(mbStart)
	mbWG.Wait()
	if got := mbWinners.Load(); got != 1 {
		t.Fatalf("concurrent mb hunt-group winners = %d, want 1", got)
	}
	if holder := bus.ClaimHolder(res.Seq); holder == "" {
		t.Fatalf("expected atomic ClaimHolder on seq %d, got empty", res.Seq)
	}

	// Concurrent consuming inbox watches across live instances claim job B atomically once.
	// Use peek-claimed session cards (as real `--peek` -> `--watch` sessions hold)
	// so interactive cards keep their exclusive collision semantics.
	res2, err := bus.Send(bus.SendRequest{From: "operator", To: "claude", Body: "hunt-group job B"})
	if err != nil {
		t.Fatalf("second send to claude main line: %v", err)
	}
	var peekLive []fleet.Instance

	for _, label := range []string{"claude+4", "claude+5", "claude+6"} {
		inst := openTestLineInstance(t, store, label, "", bus.BoardHeadSeq()+1)
		if err := room.Join(room.Card{
			ID:        inst.ClaimID(),
			Nick:      label,
			Tool:      "claude",
			Binding:   "claude:sonnet4.6",
			Mode:      "peek",
			PID:       os.Getpid(),
			OwnerPID:  os.Getpid(),
			Principal: principal.InstanceURN(inst.UUID),
		}); err != nil {
			t.Fatalf("join peek card %s: %v", label, err)
		}
		peekLive = append(peekLive, inst)
	}
	var winners atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	outputs := make([]string, len(peekLive))
	errs := make([]error, len(peekLive))
	for idx, inst := range peekLive {
		wg.Add(1)
		go func(i int, target fleet.Instance) {
			defer wg.Done()
			<-start
			out, _, err := runInboxCmd(t, "--as", target.MailAddress(), "--watch", "--wait", "250ms", "--until-count", "1")
			outputs[i] = out
			errs[i] = err
			if strings.Contains(out, "hunt-group job B") {
				winners.Add(1)
			}
		}(idx, inst)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil && !strings.Contains(err.Error(), "timeout waiting for 1 matching message(s)") {
			t.Fatalf("concurrent reader %d unexpected error: %v", i, err)
		}
	}
	if got := winners.Load(); got != 1 {
		t.Fatalf("concurrent inbox hunt-group winners = %d, want 1; outputs=%q", got, outputs)
	}
	if holder := bus.ClaimHolder(res2.Seq); holder == "" {
		t.Fatalf("expected atomic ClaimHolder on seq %d, got empty", res2.Seq)
	}
}

func TestExtensions_SendAttributionSelfAndAsWithRefusals(t *testing.T) {
	store := isolateExtensionTestEnv(t)
	priorDetect := bus.DetectHarness
	bus.DetectHarness = func() (string, bool) { return "claude", true }
	t.Cleanup(func() { bus.DetectHarness = priorDetect })

	i2 := openTestLineInstance(t, store, "claude+2", "sess-send-plus2", 0)
	i3 := openTestLineInstance(t, store, "claude+3", "sess-send-plus3", 0)

	t.Setenv("BASHY_PRINCIPAL", principal.InstanceURN(i2.UUID))
	t.Setenv("BASHY_INSTANCE", i2.UUID)
	t.Setenv("BASHY_SESSION_CLAIM", bus.HashSessionClaim("sess-send-plus2"))

	if _, _, err := runTestMBCmd(t, "send", "claude+3", "self-send from plus2"); err != nil {
		t.Fatalf("self mb send: %v", err)
	}
	if _, _, err := runTestMBCmd(t, "send", "--as", "claude+2", "claude+3", "explicit --as plus2"); err != nil {
		t.Fatalf("--as claude+2 mb send: %v", err)
	}

	posts, err := bus.Posts()
	if err != nil || len(posts) != 2 {
		t.Fatalf("posts = %+v (err=%v), want 2", posts, err)
	}
	for _, p := range posts {
		if p.From != "claude+2" || p.FromParty == nil || p.FromParty.UUID != i2.UUID {
			t.Fatalf("unexpected attribution %+v, want From=claude+2 UUID=%s", p, i2.UUID)
		}
	}

	// Refuse cross-extension and cross-UUID authoring from i2's session.
	if _, _, err := runTestMBCmd(t, "send", "--as", "claude+3", "claude+3", "spoof"); err == nil {
		t.Fatal("expected refusal authoring as claude+3 from claude+2 session")
	}
	if _, _, err := runTestMBCmd(t, "send", "--as", i3.UUID, "claude+3", "spoof uuid"); err == nil {
		t.Fatal("expected refusal authoring as i3 UUID from claude+2 session")
	}

	// Stale generation cannot read or send as reused claude+2.
	if _, err := store.Retire(i2.UUID, bus.InstanceMail); err != nil {
		t.Fatal(err)
	}
	openTestLineInstance(t, store, "claude+2", "sess-send-plus2-gen2", bus.BoardHeadSeq()+1)
	if _, _, err := runInboxCmd(t, "--as", "claude+2"); err == nil {
		t.Fatal("expected retired i2 session to be refused reading reused claude+2")
	}
}

func TestExtensions_WithheldTurnItemStaysUnreadAndRetrievable(t *testing.T) {
	store := isolateExtensionTestEnv(t)
	i2 := openTestLineInstance(t, store, "claude+2", "sess-turn-plus2", 0)

	if _, err := bus.Send(bus.SendRequest{From: "operator", To: "claude+2", Body: "first unread item"}); err != nil {
		t.Fatalf("send 1: %v", err)
	}
	if _, err := bus.Send(bus.SendRequest{From: "operator", To: "claude+2", Body: "second unread item withheld"}); err != nil {
		t.Fatalf("send 2: %v", err)
	}

	// Default BASHY_CHAT_INBOX_MAX admits 1 item and withholds the other ("1 of 2 unread").
	prepared := bus.PrepareLaunchPreamble(i2.MailAddress())
	if err := prepared.Err(); err != nil {
		t.Fatalf("PrepareLaunchPreamble: %v", err)
	}
	shownBody, withheldBody := "second unread item withheld", "first unread item"
	if strings.Contains(prepared.Text, "first unread item") {
		shownBody, withheldBody = "first unread item", "second unread item withheld"
	}
	if !strings.Contains(prepared.Text, shownBody) ||
		!strings.Contains(prepared.Text, "1 of 2 unread") ||
		strings.Contains(prepared.Text, withheldBody) {
		t.Fatalf("unexpected bounded preamble:\n%s", prepared.Text)
	}
	if err := prepared.Commit(); err != nil {
		t.Fatalf("prepared.Commit: %v", err)
	}

	// Withheld item MUST remain readable from `bashy inbox` AND `bashy inbox list`.
	inboxOut, _, err := runInboxCmd(t, "--as", "claude+2")
	if err != nil {
		t.Fatalf("inbox read after withheld turn commit: %v", err)
	}
	if !strings.Contains(inboxOut, withheldBody) {
		t.Fatalf("withheld item was lost/consumed after bounded turn admission:\n%s", inboxOut)
	}

	listOut, _, err := runTestMailboxListCmd(t, "--as", "claude+2")
	if err != nil {
		t.Fatalf("inbox list after withheld turn commit: %v", err)
	}
	if !strings.Contains(listOut, shownBody) || !strings.Contains(listOut, "[delivered]") {
		t.Fatalf("expected shown item (%q) in [delivered] state (not [acked]), got:\n%s", shownBody, listOut)
	}
	if !strings.Contains(listOut, withheldBody) || !strings.Contains(listOut, "[unread]") {
		t.Fatalf("expected withheld item (%q) still [unread], got:\n%s", withheldBody, listOut)
	}

	// Next prompt turn should NOT re-inject the already-delivered item, and
	// should now deliver the withheld item.
	secondTurn := bus.PrepareLaunchPreamble(i2.MailAddress())
	if err := secondTurn.Err(); err != nil {
		t.Fatalf("second PrepareLaunchPreamble: %v", err)
	}
	if strings.Contains(secondTurn.Text, shownBody) {
		t.Fatalf("already-delivered item re-injected on second turn:\n%s", secondTurn.Text)
	}
	if !strings.Contains(secondTurn.Text, withheldBody) {
		t.Fatalf("withheld item was not queued for second turn:\n%s", secondTurn.Text)
	}
}
