package agentos

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/meet"
	"github.com/qiangli/coreutils/pkg/room"
)

func isolateMailbox(t *testing.T) mailboxSpec {
	t.Helper()
	isolateUnifiedInbox(t)
	t.Setenv("BASHY_MAILBOX_DIR", t.TempDir())
	spec, err := currentHumanMailbox()
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestHumanMailboxAggregatesEverySourceAndBroadcastWithoutAgentConsumption(t *testing.T) {
	human := isolateMailbox(t)
	for _, p := range []bus.Post{
		{From: "agent-a", To: human.Address, Topic: "posix-cert", Body: "direct board"},
		{From: "agent-b", Topic: "announce", Body: "broadcast board"},
		{From: "agent-c", To: "some-agent", Body: "agent only"},
	} {
		if err := bus.PostMessage(p); err != nil {
			t.Fatal(err)
		}
	}
	st, err := meet.Create(meet.CreateOptions{Topic: "dhnt", Board: true, Human: human.Address})
	if err != nil {
		t.Fatal(err)
	}
	if err = meet.AppendEvent(st.ID, meet.Event{Speaker: "agent-a", To: human.Address, Kind: "status", Text: "direct meet"}); err != nil {
		t.Fatal(err)
	}
	if err = meet.AppendEvent(st.ID, meet.Event{Speaker: "agent-a", Kind: "status", Text: "broadcast meet"}); err != nil {
		t.Fatal(err)
	}
	if err = bus.Publish(bus.Notification{Principal: "scheduler", To: human.Address, Topic: "posix-cert", Body: "direct bus"}); err != nil {
		t.Fatal(err)
	}
	if err = bus.Publish(bus.Notification{Principal: "scheduler", Topic: "announce", Body: "broadcast bus"}); err != nil {
		t.Fatal(err)
	}

	items, _, err := snapshotMailbox(human)
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, i := range items {
		joined += "\n" + i.Body
	}
	for _, want := range []string{"direct board", "broadcast board", "direct meet", "broadcast meet", "direct bus", "broadcast bus"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q from %s", want, joined)
		}
	}
	if strings.Contains(joined, "agent only") {
		t.Fatal("human mailbox included another agent's direct post")
	}
	// The mailbox overlay never advances any native source cursor, especially
	// the agent's cursor used by turn-boundary console injection.
	if bus.SeenSeq("some-agent") != 0 || bus.SeenSeq(human.Address) != 0 {
		t.Fatal("mailbox list advanced a native MB cursor")
	}
}

func TestMailboxFiltersUnreadFirstAndExplicitReadAckPreserve(t *testing.T) {
	human := isolateMailbox(t)
	if err := bus.PostMessage(bus.Post{From: "agent", To: human.Address, Topic: "posix-cert", Body: "Profile D status ref docs/status.md"}); err != nil {
		t.Fatal(err)
	}
	items, state, err := snapshotMailbox(human)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%v err=%v", items, err)
	}
	id := items[0].ID
	state.Marks[id] = mailboxMark{Project: "dhnt", Status: "blocked"}
	if err = saveMailboxState(human, state); err != nil {
		t.Fatal(err)
	}
	items, _, _ = snapshotMailbox(human)
	got := mailboxFilter{topic: "posix-cert", project: "dhnt", status: "blocked", search: "profile d"}.apply(items)
	if len(got) != 1 {
		t.Fatalf("filtered=%+v", got)
	}
	if got := (mailboxFilter{search: "agent"}).apply(items); len(got) != 1 {
		t.Fatalf("sender search=%+v", got)
	}
	organize := newMailboxOrganizeCmd(true)
	organize.SetOut(&bytes.Buffer{})
	organize.SetErr(&bytes.Buffer{})
	organize.SetArgs([]string{"--project", "certification", "--status", "active", id})
	if err := organize.Execute(); err != nil {
		t.Fatal(err)
	}
	items, _, _ = snapshotMailbox(human)
	if items[0].Project != "certification" || items[0].Status != "active" {
		t.Fatalf("organize=%+v", items[0])
	}

	run := func(action string) {
		t.Helper()
		c := newMailboxStateCmd(action, true)
		c.SetOut(&bytes.Buffer{})
		c.SetErr(&bytes.Buffer{})
		c.SetArgs([]string{id})
		if err := c.Execute(); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
	run("read")
	items, _, _ = snapshotMailbox(human)
	if !items[0].Read || items[0].Acknowledged {
		t.Fatalf("read consumed item: %+v", items[0])
	}
	run("ack")
	items, _, _ = snapshotMailbox(human)
	if len(mailboxFilter{}.apply(items)) != 0 || !items[0].Acknowledged {
		t.Fatalf("ack did not remove from pending: %+v", items)
	}
	run("preserve")
	items, _, _ = snapshotMailbox(human)
	if len(mailboxFilter{}.apply(items)) != 1 || !items[0].Preserved {
		t.Fatalf("preserve did not reopen: %+v", items)
	}

	// Windows does not implement Unix permission bits; Stat reports the
	// writable state as 0666 even when WriteFile was given 0600.
	if runtime.GOOS != "windows" {
		p, _ := mailboxStatePath(human)
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("state mode=%o", fi.Mode().Perm())
		}
		di, _ := os.Stat(os.Getenv("BASHY_MAILBOX_DIR"))
		if di.Mode().Perm() != 0o700 {
			t.Fatalf("dir mode=%o", di.Mode().Perm())
		}
	}
}

func TestMailboxConcurrentOrganizationPreservesIndependentFields(t *testing.T) {
	human := isolateMailbox(t)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, change := range []func(*mailboxState){func(s *mailboxState) { m := s.Marks["mb:1"]; m.Project = "dhnt"; s.Marks["mb:1"] = m }, func(s *mailboxState) { m := s.Marks["mb:2"]; m.Status = "blocked"; s.Marks["mb:2"] = m }} {
		wg.Add(1)
		go func(fn func(*mailboxState)) {
			defer wg.Done()
			errs <- updateMailboxState(human, func(s *mailboxState) error { fn(s); return nil })
		}(change)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	state, err := loadMailboxState(human)
	if err != nil {
		t.Fatal(err)
	}
	if state.Marks["mb:1"].Project != "dhnt" || state.Marks["mb:2"].Status != "blocked" {
		t.Fatalf("lost concurrent state: %+v", state.Marks)
	}
}

func TestAgentMailboxUsesSameJSONQueryModel(t *testing.T) {
	isolateUnifiedInbox(t)
	t.Setenv("BASHY_MAILBOX_DIR", t.TempDir())
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/alice")
	if err := bus.PostMessage(bus.Post{From: "bob", To: "alice", Topic: "harness", Body: "agent record"}); err != nil {
		t.Fatal(err)
	}
	c := newMailboxListCmd(false)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"--as", "alice", "--source", "mb", "--topic", "harness", "--json"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	var item mailboxItem
	if err := json.Unmarshal(out.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.Schema != mailboxSchema || item.Body != "agent record" || item.Source != "mb" {
		t.Fatalf("item=%+v", item)
	}
}

func TestAgentMailboxIncludesOnlyRolesItCurrentlyHolds(t *testing.T) {
	isolateUnifiedInbox(t)
	t.Setenv("BASHY_MAILBOX_DIR", t.TempDir())
	prior := bus.HostRoles
	bus.HostRoles = func() []bus.HostRole {
		return []bus.HostRole{{Label: "steward", Topic: "steward.host", Holder: "alice"}, {Label: "conductor", Topic: "conductor.host", Holder: "bob"}}
	}
	t.Cleanup(func() { bus.HostRoles = prior })
	if err := bus.AppendPending("steward.host", bus.Pending{Seq: 7, Principal: "human", To: "steward.host", Body: "held role"}); err != nil {
		t.Fatal(err)
	}
	if err := bus.AppendPending("conductor.host", bus.Pending{Seq: 8, Principal: "human", To: "conductor.host", Body: "other role"}); err != nil {
		t.Fatal(err)
	}
	items, _, err := snapshotMailbox(mailboxSpec{Key: "agent:alice", Address: "alice", Kind: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, i := range items {
		joined += i.Body
	}
	if !strings.Contains(joined, "held role") || strings.Contains(joined, "other role") {
		t.Fatalf("role selection=%q", joined)
	}
}

func TestHumanSendEnforcesBoundAndRecordsOrganizingMetadata(t *testing.T) {
	human := isolateMailbox(t)
	const sessionEnv = "BASHY_TEST_HUMAN_SEND_SESSION"
	if err := fleet.New().SaveTool(fleet.Tool{Name: "human-send-tool", Kind: "cli", CLI: fleet.ToolCLI{Launch: fleet.ToolLaunch{SessionEnv: []string{sessionEnv}}}}); err != nil {
		t.Fatal(err)
	}
	if err := fleet.New().SaveAgent(fleet.Agent{Name: "alice", Tool: "human-send-tool", Model: "test-model"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(sessionEnv, "alice-session")
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/alice")
	wireMessageBoard()
	watcher, err := registerInboxWatcher("alice")
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.leave()
	c := newHumanSendCmd()
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"--as", "alice", "--topic", "posix-cert", "--project", "dhnt", "--status", "blocked", "--ref", "docs/status.md", "Profile D status"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	items, _, err := snapshotMailbox(human)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if items[0].Project != "dhnt" || items[0].Status != "blocked" || !strings.Contains(items[0].Body, "ref: docs/status.md") {
		t.Fatalf("metadata=%+v", items[0])
	}
	c = newHumanSendCmd()
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"--as", "alice", strings.Repeat("x", 1025)})
	if err := c.Execute(); err == nil {
		t.Fatal("oversize human status was accepted")
	}
}

func TestHumanSendForgedPrincipalCannotUseAForeignLiveClaim(t *testing.T) {
	if os.Getenv("BASHY_TEST_FOREIGN_CLAIM_HOLDER") != "" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		return
	}
	human := isolateMailbox(t)
	const (
		name       = "foreign-claim-sentinel"
		sessionEnv = "BASHY_TEST_FOREIGN_SESSION"
		rejected   = "REJECTED-HUMAN-INBOX-BODY"
	)
	if err := fleet.New().SaveTool(fleet.Tool{Name: "foreign-claim-tool", Kind: "cli", CLI: fleet.ToolCLI{Launch: fleet.ToolLaunch{SessionEnv: []string{sessionEnv}}}}); err != nil {
		t.Fatal(err)
	}
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "foreign-claim-tool", Model: "test-model"}); err != nil {
		t.Fatal(err)
	}

	holder := exec.Command(os.Args[0], "-test.run=^TestHumanSendForgedPrincipalCannotUseAForeignLiveClaim$")
	stdin, err := holder.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	holder.Env = append(os.Environ(), "BASHY_TEST_FOREIGN_CLAIM_HOLDER=1")
	if err = holder.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = holder.Wait()
	})
	if err = room.Join(room.Card{
		ID: name, Nick: name, Tool: "foreign-claim-tool", Binding: "foreign-claim-tool:test-model",
		Mode: "inbox", PID: holder.Process.Pid, OwnerPID: holder.Process.Pid,
		SessionClaim: bus.HashSessionClaim("rightful-session"), Principal: "dhnt:agent/" + name,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { room.LeavePID(name, holder.Process.Pid) })

	t.Setenv(sessionEnv, "forged-session")
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/"+name)
	wireMessageBoard()
	c := newHumanSendCmd()
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"--as", name, rejected})
	if err = c.Execute(); err == nil || !strings.Contains(err.Error(), "no matching live session claim") {
		t.Fatalf("forged human send error = %v", err)
	}
	items, _, err := snapshotMailbox(human)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if strings.Contains(item.Body, rejected) {
			t.Fatalf("refused body entered human mailbox: %+v", item)
		}
	}
	warnings, _, err := bus.UnreadNotifications(name)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || warnings[0].Topic != "identity.refusal" || warnings[0].To != name || strings.Contains(warnings[0].Body, rejected) {
		t.Fatalf("rightful-owner warning = %+v", warnings)
	}
}

func TestHumanSendAcceptsNestedWrapperFromClaimedWatcherSession(t *testing.T) {
	const (
		name = "nested-wrapper-sentinel"
		body = "nested wrapper status"
	)
	if os.Getenv("BASHY_TEST_NESTED_AUTHOR") != "" {
		wireMessageBoard()
		c := newHumanSendCmd()
		c.SetOut(io.Discard)
		c.SetErr(io.Discard)
		c.SetArgs([]string{"--as", name, body})
		if err := c.Execute(); err != nil {
			t.Fatal(err)
		}
		return
	}
	if os.Getenv("BASHY_TEST_NESTED_WRAPPER") != "" {
		author := exec.Command(os.Args[0], "-test.run=^TestHumanSendAcceptsNestedWrapperFromClaimedWatcherSession$")
		author.Env = append(os.Environ(), "BASHY_TEST_NESTED_AUTHOR=1")
		if out, err := author.CombinedOutput(); err != nil {
			t.Fatalf("nested author: %v\n%s", err, out)
		}
		return
	}
	if os.Getenv("BASHY_TEST_NESTED_WATCHER") != "" {
		watcher, err := registerInboxWatcher(name)
		if err != nil {
			t.Fatal(err)
		}
		defer watcher.leave()
		fmt.Fprintln(os.Stdout, "WATCHER-READY")
		_, _ = io.Copy(io.Discard, os.Stdin)
		return
	}

	human := isolateMailbox(t)
	if err := fleet.New().SaveTool(fleet.Tool{Name: "nested-wrapper-tool", Kind: "cli"}); err != nil {
		t.Fatal(err)
	}
	if err := fleet.New().SaveAgent(fleet.Agent{Name: name, Tool: "nested-wrapper-tool", Model: "test-model"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/"+name)

	watcher := exec.Command(os.Args[0], "-test.run=^TestHumanSendAcceptsNestedWrapperFromClaimedWatcherSession$")
	watcher.Env = append(os.Environ(), "BASHY_TEST_NESTED_WATCHER=1")
	watcherIn, err := watcher.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	watcherOut, err := watcher.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = watcher.Start(); err != nil {
		t.Fatal(err)
	}
	if line, readErr := bufio.NewReader(watcherOut).ReadString('\n'); readErr != nil || strings.TrimSpace(line) != "WATCHER-READY" {
		_ = watcherIn.Close()
		_ = watcher.Wait()
		t.Fatalf("watcher readiness = %q, %v", line, readErr)
	}

	wrapper := exec.Command(os.Args[0], "-test.run=^TestHumanSendAcceptsNestedWrapperFromClaimedWatcherSession$")
	wrapper.Env = append(os.Environ(), "BASHY_TEST_NESTED_WRAPPER=1")
	if out, runErr := wrapper.CombinedOutput(); runErr != nil {
		_ = watcherIn.Close()
		_ = watcher.Wait()
		t.Fatalf("nested wrapper: %v\n%s", runErr, out)
	}
	if err = watcherIn.Close(); err != nil {
		t.Fatal(err)
	}
	if err = watcher.Wait(); err != nil {
		t.Fatal(err)
	}

	items, _, err := snapshotMailbox(human)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].From != name || items[0].Body != body {
		t.Fatalf("nested wrapper human message = %+v", items)
	}
	if _, live, err := room.Find(name); err != nil || live {
		t.Fatalf("watcher claim after exit: live=%v err=%v", live, err)
	}
}

func TestInboxHelpTeachesHumanMailboxDiscoveryAndStableReferences(t *testing.T) {
	c := newUnifiedInboxCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"human", "--help"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"list", "read", "ack", "preserve", "posix-cert", "stable shared reference", "another authorized local agent"} {
		if !strings.Contains(s, want) {
			t.Errorf("help omitted %q:\n%s", want, s)
		}
	}
}
