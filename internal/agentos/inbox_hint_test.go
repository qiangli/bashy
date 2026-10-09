// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Sprint 321 unread-hint suite: seeded mail becomes visible within one turn
// on every surface (dispatch hint, inbox-hook envelope), without consuming
// mail, without leaking across instances, with turn+arrival suppression, and
// with structured stdout kept clean.
package agentos

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/meet"
)

func isolateHintState(t *testing.T) {
	t.Helper()
	t.Setenv("BASHY_HINTS_STATE", filepath.Join(t.TempDir(), "inbox-hints.json"))
	// Hermetic: hints are gated on the ambient agent environment, which a
	// plain CI shell does not have and an agent session does.
	t.Setenv("BASHY_HINTS", "on")
	t.Setenv("BASHY_AGENTIC", "")
	t.Setenv("BASHY_ADVISOR_NOMEM", "")
}

func seedHintMail(t *testing.T, reader string) {
	t.Helper()
	if err := bus.PostMessage(bus.Post{From: "conductor", To: reader, Topic: "harness", Body: "seeded board mail"}); err != nil {
		t.Fatal(err)
	}
	st, err := meet.Create(meet.CreateOptions{Topic: "sprint channel", Board: true, Participants: []string{reader}, Human: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if err := meet.AppendEvent(st.ID, meet.Event{Speaker: "conductor", To: reader, Kind: "status", Text: "seeded meet mail"}); err != nil {
		t.Fatal(err)
	}
}

func TestSummarizeUnreadHintAttribution(t *testing.T) {
	batch := inboxBatch{events: []unifiedInboxEvent{
		{Source: "mb", Seq: 1, From: "conductor", Body: "a"},
		{Source: "mb", Seq: 2, From: "conductor", Body: "b"},
		{Source: "meet", Seq: 3, Room: "room1", From: "scheduler", Body: "c"},
		{Source: "role:conductor:1", Seq: 4, From: "", Body: "d"},
	}}
	sum, ok := summarizeUnreadHint(batch)
	if !ok {
		t.Fatal("expected a summary for a non-empty batch")
	}
	if sum.count != 4 {
		t.Errorf("count=%d, want 4", sum.count)
	}
	text := formatUnreadHint(sum)
	for _, want := range []string{"4 unread", "mb:2", "meet/room1:1", "role:conductor:1:1", "conductor", "scheduler", "bashy inbox"} {
		if !strings.Contains(text, want) {
			t.Errorf("hint %q missing %q", text, want)
		}
	}
	if strings.Contains(text, "seeded") {
		t.Errorf("hint must carry attribution, never bodies: %q", text)
	}
	if _, ok := summarizeUnreadHint(inboxBatch{}); ok {
		t.Error("empty batch must report silence, not a hint")
	}
}

func TestCheckUnreadArrivalsSuppressesDuplicatesAndRehints(t *testing.T) {
	calls := 0
	snapshot := func(string) (inboxBatch, error) {
		calls++
		return inboxBatch{events: []unifiedInboxEvent{{Source: "mb", Seq: 1, From: "a", Body: "x"}}}, nil
	}
	st := inboxHintArrivals{}
	hint, next, ran := checkUnreadArrivals("r", 7, true, st, snapshot, nil)
	if !ran || hint == "" || calls != 1 {
		t.Fatalf("first arrival must hint: hint=%q ran=%v calls=%d", hint, ran, calls)
	}
	if _, _, ran := checkUnreadArrivals("r", 7, true, next, snapshot, nil); ran || calls != 1 {
		t.Fatalf("same turn must suppress without re-reading: ran=%v calls=%d", ran, calls)
	}
	hint, _, ran = checkUnreadArrivals("r", 8, true, next, snapshot, nil)
	if !ran || hint == "" || calls != 2 {
		t.Fatalf("new arrival must re-hint: hint=%q ran=%v calls=%d", hint, ran, calls)
	}
}

func TestCheckUnreadArrivalsQuietStoreSkipsReads(t *testing.T) {
	calls := 0
	snapshot := func(string) (inboxBatch, error) {
		calls++
		return inboxBatch{}, nil
	}
	st := inboxHintArrivals{}
	if _, next, ran := checkUnreadArrivals("r", 7, true, st, snapshot, nil); !ran || calls != 1 {
		t.Fatalf("first probe must read: ran=%v calls=%d", ran, calls)
	} else {
		st = next
	}
	if _, _, ran := checkUnreadArrivals("r", 7, true, st, snapshot, nil); ran || calls != 1 {
		t.Fatalf("quiet store must not re-read: ran=%v calls=%d", ran, calls)
	}
}

func TestCheckUnreadArrivalsResamplesAfterRead(t *testing.T) {
	// The first read lazily materializes cursor files, moving the fingerprint
	// without mail arriving. The gate must record the post-read generation,
	// or the very next turn re-hints the same arrival.
	calls := 0
	snapshot := func(string) (inboxBatch, error) {
		calls++
		return inboxBatch{events: []unifiedInboxEvent{{Source: "mb", Seq: 1, From: "a", Body: "x"}}}, nil
	}
	fps := []uint64{8, 8}
	refingerprint := func(string) (uint64, bool) {
		fp := fps[0]
		if len(fps) > 1 {
			fps = fps[1:]
		}
		return fp, true
	}
	st := inboxHintArrivals{}
	hint, next, ran := checkUnreadArrivals("r", 7, true, st, snapshot, refingerprint)
	if !ran || hint == "" {
		t.Fatalf("arrival must hint: hint=%q ran=%v", hint, ran)
	}
	if next.Hinted["r"] != 8 || next.Probed["r"] != 8 {
		t.Fatalf("must record the post-read generation: %+v", next)
	}
	if _, _, ran := checkUnreadArrivals("r", 8, true, next, snapshot, refingerprint); ran {
		t.Fatalf("post-read generation must suppress: calls=%d", calls)
	}
	if calls != 2 {
		t.Fatalf("expected one re-read while the store moved, got %d snapshots", calls)
	}
}

func TestCheckUnreadArrivalsSnapshotErrorRetries(t *testing.T) {
	calls := 0
	snapshot := func(string) (inboxBatch, error) {
		calls++
		return inboxBatch{}, errTestHint
	}
	st := inboxHintArrivals{}
	if hint, next, ran := checkUnreadArrivals("r", 7, true, st, snapshot, nil); hint != "" || ran {
		t.Fatalf("snapshot error must stay silent and unsaved: hint=%q ran=%v", hint, ran)
	} else if next.Probed["r"] != 0 {
		t.Fatalf("failed snapshot must not advance the probe mark: %+v", next)
	}
	_ = calls
}

type hintTestError string

func (e hintTestError) Error() string { return string(e) }

const errTestHint = hintTestError("test snapshot failure")

func TestUnreadHintNeverConsumesMail(t *testing.T) {
	isolateUnifiedInbox(t)
	isolateHintState(t)
	reader := inboxTestReader
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/"+reader)
	seedHintMail(t, reader)

	var buf bytes.Buffer
	emitUnreadHint([]string{"status"}, &buf, true)
	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("seeded mail produced no hint")
	}
	var nl nudgeLine
	if err := json.Unmarshal([]byte(line), &nl); err != nil {
		t.Fatalf("dispatch hint must be one JSON line on stderr: %v (%q)", err, line)
	}
	if nl.Kind != "hint" || nl.Tool != "inbox" || !strings.Contains(nl.Suggest, "bashy inbox") {
		t.Errorf("unexpected hint shape: %+v", nl)
	}
	if !strings.Contains(nl.Suggest, "conductor") || !strings.Contains(nl.Suggest, "mb:") {
		t.Errorf("hint must carry sender/source attribution: %q", nl.Suggest)
	}

	if got := bus.SeenSeq(reader); got != 0 {
		t.Fatalf("hint advanced the board cursor to %d", got)
	}
	if again, _, _ := checkUnreadArrivals(reader, 999, true, inboxHintArrivals{}, liveUnreadSnapshot, nil); again == "" {
		t.Fatal("mail waited after the hint: second view must still see it")
	}
}

func TestUnreadHintHumanModeProse(t *testing.T) {
	isolateUnifiedInbox(t)
	isolateHintState(t)
	reader := inboxTestReader
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/"+reader)
	seedHintMail(t, reader)

	var buf bytes.Buffer
	emitUnreadHint([]string{"status"}, &buf, false)
	out := buf.String()
	if !strings.Contains(out, "bashy inbox") {
		t.Fatalf("human hint must point at bashy inbox: %q", out)
	}
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("human mode must not emit JSON: %q", out)
	}
}

func TestUnreadHintIsolatesInstances(t *testing.T) {
	isolateUnifiedInbox(t)
	isolateHintState(t)
	other := "hint-other-reader"
	registerTestInboxAgent(t, os.Getenv("BASHY_FLEET_DIR"), other)
	if err := bus.PostMessage(bus.Post{From: "conductor", To: other, Body: "other's mail"}); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/"+inboxTestReader)
	var buf bytes.Buffer
	emitUnreadHint([]string{"status"}, &buf, true)
	if buf.String() != "" {
		t.Fatalf("cross-instance leakage: hint for %q on %q's mail: %q", inboxTestReader, other, buf.String())
	}

	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/"+other)
	buf.Reset()
	emitUnreadHint([]string{"status"}, &buf, true)
	if !strings.Contains(buf.String(), "1 unread") {
		t.Fatalf("owner saw no hint for its own mail: %q", buf.String())
	}
}

func TestUnreadHintRetiredPrincipalStaysSilent(t *testing.T) {
	isolateUnifiedInbox(t)
	isolateHintState(t)
	reader := inboxTestReader
	seedHintMail(t, reader)
	// A stale session whose agent no longer has a live fleet definition must
	// not surface archived mail: silence, not another instance's hint.
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/retired-gone")
	if _, retired, err := resolveHintReader(""); err != nil || !retired {
		t.Fatalf("retired principal must resolve retired=true: retired=%v err=%v", retired, err)
	}
	var buf bytes.Buffer
	emitUnreadHint([]string{"status"}, &buf, true)
	if buf.String() != "" {
		t.Fatalf("retired instance hinted: %q", buf.String())
	}
}

func TestUnreadHintSkipsReadersAndSwitches(t *testing.T) {
	isolateUnifiedInbox(t)
	isolateHintState(t)
	reader := inboxTestReader
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/"+reader)
	seedHintMail(t, reader)
	var buf bytes.Buffer
	for _, argv := range [][]string{{"inbox"}, {"mb"}, {"notify"}, {"serve"}, {"mcp"}, {"inbox-hook"}, {"-c"}, {"./script.sh"}} {
		buf.Reset()
		emitUnreadHint(argv, &buf, true)
		if buf.String() != "" {
			t.Errorf("verb %q must not probe: %q", argv, buf.String())
		}
	}
	t.Setenv("BASHY_HINTS", "off")
	buf.Reset()
	emitUnreadHint([]string{"status"}, &buf, true)
	if buf.String() != "" {
		t.Errorf("BASHY_HINTS=off must silence the hint: %q", buf.String())
	}
}

func TestSniffHookEventBothCasings(t *testing.T) {
	if got := sniffHookEvent([]byte(`{"hook_event_name":"SessionStart","session_id":"s"}`)); got != "SessionStart" {
		t.Errorf("snake SessionStart: %q", got)
	}
	if got := sniffHookEvent([]byte(`{"hookEventName":"UserPromptSubmit","prompt":"hi"}`)); got != "UserPromptSubmit" {
		t.Errorf("camel UserPromptSubmit: %q", got)
	}
	if got := sniffHookEvent([]byte(`{"hookEventName":"Stop"}`)); got != "Stop" {
		t.Errorf("passthrough event: %q", got)
	}
	if got := sniffHookEvent([]byte(`not json`)); got != "" {
		t.Errorf("garbage must yield no event: %q", got)
	}
	if got := sniffHookEvent(nil); got != "" {
		t.Errorf("empty input must yield no event: %q", got)
	}
}

func TestInboxHookSeededDeliveryEnvelope(t *testing.T) {
	isolateUnifiedInbox(t)
	isolateHintState(t)
	reader := inboxTestReader
	seedHintMail(t, reader)

	stdin := strings.NewReader(`{"hook_event_name":"SessionStart","session_id":"s1"}`)
	var stdout, stderr bytes.Buffer
	if rc := dispatchInboxHookTo([]string{"--as", reader}, stdin, &stdout, &stderr); rc != 0 {
		t.Fatalf("hook exit=%d stderr=%q", rc, stderr.String())
	}
	var env struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	line := strings.TrimSpace(stdout.String())
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		t.Fatalf("hook stdout must be exactly one JSON envelope: %v (%q)", err, line)
	}
	if env.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Errorf("event mismatch: %+v", env.HookSpecificOutput)
	}
	for _, want := range []string{"2 unread", "mb:", "conductor", "bashy inbox"} {
		if !strings.Contains(env.HookSpecificOutput.AdditionalContext, want) {
			t.Errorf("envelope context missing %q: %q", want, env.HookSpecificOutput.AdditionalContext)
		}
	}

	// Same turn, same arrival: the hook stays silent (duplicate suppression).
	stdout.Reset()
	if rc := dispatchInboxHookTo([]string{"--as", reader}, strings.NewReader(`{"hookEventName":"SessionStart"}`), &stdout, &stderr); rc != 0 {
		t.Fatalf("second hook exit=%d", rc)
	}
	if stdout.String() != "" {
		t.Fatalf("duplicate notice within the turn: %q", stdout.String())
	}

	// Newly arrived mail re-hints on the next turn boundary.
	if err := bus.PostMessage(bus.Post{From: "conductor", To: reader, Body: "late mail"}); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if rc := dispatchInboxHookTo([]string{"--as", reader, "--for", "UserPromptSubmit"}, nil, &stdout, &stderr); rc != 0 {
		t.Fatalf("third hook exit=%d", rc)
	}
	if !strings.Contains(stdout.String(), `"hookEventName":"UserPromptSubmit"`) || !strings.Contains(stdout.String(), "bashy inbox") {
		t.Fatalf("new arrival did not re-hint: %q", stdout.String())
	}
}

func TestInboxHookEmptyStaysSilent(t *testing.T) {
	isolateUnifiedInbox(t)
	isolateHintState(t)
	reader := inboxTestReader
	var stdout, stderr bytes.Buffer
	if rc := dispatchInboxHookTo([]string{"--as", reader}, strings.NewReader(`{"hook_event_name":"UserPromptSubmit"}`), &stdout, &stderr); rc != 0 {
		t.Fatalf("hook exit=%d stderr=%q", rc, stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("empty inbox must stay silent on stdout: %q", stdout.String())
	}
}

func TestInboxHookDoesNotConsume(t *testing.T) {
	isolateUnifiedInbox(t)
	isolateHintState(t)
	reader := inboxTestReader
	seedHintMail(t, reader)
	var stdout, stderr bytes.Buffer
	dispatchInboxHookTo([]string{"--as", reader}, strings.NewReader(`{}`), &stdout, &stderr)
	if got := bus.SeenSeq(reader); got != 0 {
		t.Fatalf("hook advanced the board cursor to %d", got)
	}
	prepared := unifiedTurnPreamble(reader)
	if !strings.Contains(prepared.Text, "seeded board mail") {
		t.Fatalf("mail missing from managed delivery after hook: %q", prepared.Text)
	}
}

func TestClaudeHooksMergePreservesAndIdempotent(t *testing.T) {
	m := map[string]any{
		"env":   map[string]any{"CLAUDE_CODE_SHELL": "/bin/bashy"},
		"model": "opus",
		"hooks": map[string]any{
			"PreToolUse": []any{"keep-me"},
		},
	}
	if !mergeClaudeInboxHooks(m, "/bin/bashy", "alice") {
		t.Fatal("first merge must report a change")
	}
	hooks := m["hooks"].(map[string]any)
	for _, event := range []string{"SessionStart", "UserPromptSubmit"} {
		if !claudeHookHasCommand(m, event) {
			t.Errorf("missing hook group for %s: %v", event, hooks[event])
		}
	}
	if m["env"].(map[string]any)["CLAUDE_CODE_SHELL"] != "/bin/bashy" || m["model"] != "opus" {
		t.Errorf("unrelated settings not preserved: %v", m)
	}
	if hooks["PreToolUse"].([]any)[0] != "keep-me" {
		t.Errorf("unrelated hook group not preserved: %v", hooks)
	}
	before, _ := json.Marshal(m)
	if mergeClaudeInboxHooks(m, "/bin/bashy", "alice") {
		t.Error("second merge must be a no-op")
	}
	after, _ := json.Marshal(m)
	if string(before) != string(after) {
		t.Errorf("merge not idempotent:\n%s\n%s", before, after)
	}
}

func TestClaudeHooksStripOnlyOurs(t *testing.T) {
	m := map[string]any{}
	mergeClaudeInboxHooks(m, "/bin/bashy", "alice")
	hooks := m["hooks"].(map[string]any)
	hooks["PreToolUse"] = []any{"keep-me"}
	if !stripClaudeInboxHooks(m) {
		t.Fatal("strip must report a change")
	}
	if claudeHookHasCommand(m, "SessionStart") || claudeHookHasCommand(m, "UserPromptSubmit") {
		t.Errorf("inbox hooks remain: %v", m["hooks"])
	}
	if m["hooks"].(map[string]any)["PreToolUse"].([]any)[0] != "keep-me" {
		t.Errorf("unrelated hook group damaged: %v", m["hooks"])
	}
	m2 := map[string]any{}
	mergeClaudeInboxHooks(m2, "/bin/bashy", "")
	if !stripClaudeInboxHooks(m2) || len(m2) != 0 {
		t.Errorf("strip must prune emptied hooks key: %v", m2)
	}
}

func TestCodexHookBlockRoundTrip(t *testing.T) {
	orig := "check_for_update_on_startup = false\nmodel = \"gpt-6.1-sol\"\n"
	block := codexInboxHookBlock("/bin/bashy", "alice")
	for _, want := range []string{"[[hooks.SessionStart]]", "[[hooks.UserPromptSubmit]]", "inbox-hook --for SessionStart --as alice", "inbox-hook --for UserPromptSubmit --as alice"} {
		if !strings.Contains(block, want) {
			t.Errorf("codex block missing %q:\n%s", want, block)
		}
	}
	merged := replaceCodexHookBlock(orig, block)
	if !strings.HasPrefix(merged, orig) || !strings.Contains(merged, "[[hooks.SessionStart.hooks]]") {
		t.Errorf("merge damaged unrelated TOML:\n%s", merged)
	}
	again := replaceCodexHookBlock(merged, codexInboxHookBlock("/bin/bashy", "alice"))
	if again != merged {
		t.Errorf("re-install must be stable:\n%s\n%s", merged, again)
	}
	updated := replaceCodexHookBlock(merged, codexInboxHookBlock("/other/bashy", "bob"))
	if !strings.Contains(updated, "/other/bashy") || strings.Contains(updated, "/bin/bashy") {
		t.Errorf("shell/identity change must update in place:\n%s", updated)
	}
	restored, ok := removeCodexHookBlock(updated)
	if !ok || restored != orig {
		t.Errorf("uninstall must restore the file byte-for-byte: ok=%v got:\n%q want:\n%q", ok, restored, orig)
	}
	if _, ok := removeCodexHookBlock(orig); ok {
		t.Error("remove on a file without the block must report absent")
	}
}

func TestHookWriterRejectsNonHookAgents(t *testing.T) {
	if rc := dispatchInstallAgentHooks("opencode", "/bin/bashy", "", false, false, false, false); rc != 1 {
		t.Errorf("opencode hooks must be an explicit unsupported error, got %d", rc)
	}
	if rc := dispatchInstallAgentHooks("claude", "/bin/bashy", "alice", false, true, false, false); rc != 0 {
		t.Errorf("dry-run must succeed, got %d", rc)
	}
}
