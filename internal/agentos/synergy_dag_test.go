// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The S12 synergy fixture (examples/dag/synergy/dag.md) is driven here through
// the real bashy binary: every control-plane command — meet dm, app pair,
// kb, todo, chat, approve, notify, chat timeline — is the shipped CLI against
// an isolated HOME. Only the two external agent turns are stand-ins, and they
// are stubbed where an operator would stub them: the documented
// BASHY_TOOL_BINARY_<TOOL> launcher override. Nothing inside bashy is mocked.
//
// What this proves: the DAG composes those surfaces end to end, every ID in
// the ledger was parsed from a receipt the CLI actually printed, and a step
// that cannot produce its receipt fails the run instead of passing silently.
// What it does NOT prove: the Linux full-DAG run with live agents, phone
// pairing, or the two-host approval — those stay manager-owned gates.

// synergyLedgerLine is one record of $STATE_DIR/event-ids.ndjson.
type synergyLedgerLine struct {
	Step     string  `json:"step"`
	Command  string  `json:"command"`
	Exit     int     `json:"exit"`
	IDSource string  `json:"id_source"`
	EventID  *string `json:"event_id"`
	Absence  string  `json:"absence"`
	Receipt  string  `json:"receipt"`
}

// synergyExpectedLedger is the receipt sequence the fixture must produce, in
// order. nilID marks a command whose current receipt carries no ID: the line
// must say so (absence) rather than carry an invented one.
var synergyExpectedLedger = []struct {
	step, command string
	nilID         bool
}{
	{"unpaired-refusal", "meet dm", false},
	{"pair-approve", "app pair approve", false},
	{"pair-approve", "app devices", false},
	{"paired-input", "meet dm", true},
	{"paired-input", "meet list", false},
	{"claude-plan", "kb note add", false},
	{"claude-plan", "kb list", false},
	{"claude-plan", "todo add", false},
	{"codex-implementation", "todo start", false},
	{"codex-implementation", "chat", true},
	{"approval-request", "approve request", false},
	{"approve", "meet dm /approve", false},
	{"approve", "approve show", false},
	{"done-notification", "todo done", false},
	{"done-notification", "notify", true},
	{"done-notification", "chat timeline", true},
	{"done-notification", "activity tail", true},
}

var synergyTargets = []string{
	"init", "unpaired-refusal", "pair-approve", "paired-input", "claude-plan",
	"codex-implementation", "approval-request", "approve", "done-notification",
}

// synergyBashyBinary resolves the bashy under test without building the
// 100 MB AgentOS binary inside a unit test: BASHY_E2E_BIN, then the
// checkout's bin/bashy (what `make build-bashy` leaves), then — only outside
// -short — a plain `go build` into the test's temp dir.
func synergyBashyBinary(t *testing.T) (bin, root string) {
	t.Helper()
	root, ok := findBashySourceRoot(mustGetwd())
	if !ok {
		t.Skip("not inside the bashy checkout; the fixture lives at examples/dag/synergy/dag.md")
	}
	if p := strings.TrimSpace(os.Getenv("BASHY_E2E_BIN")); p != "" {
		return p, root
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	built := filepath.Join(root, "bin", "bashy"+suffix)
	if st, err := os.Stat(built); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
		return built, root
	}
	if testing.Short() {
		t.Skip("needs a built bashy: run `make build-bashy` or set BASHY_E2E_BIN (not built under -short)")
	}
	out := filepath.Join(t.TempDir(), "bashy"+suffix)
	cmd := exec.Command("go", "build", "-o", out, "./cmd/bashy")
	cmd.Dir = root
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build bashy: %v\n%s", err, b)
	}
	return out, root
}

// synergyEnv is the isolated environment the fixture runs in: a throwaway
// HOME (pair store, approval queue, DM rooms, host kb/todo, room timeline),
// stubbed planner/implementer binaries, and none of the invoking agent's own
// BASHY_* identity, which would otherwise leak into principals and stores.
type synergyEnv struct {
	home, work, state, stubLog string
	vars                       []string
}

func newSynergyEnv(t *testing.T) synergyEnv {
	t.Helper()
	home := t.TempDir()
	work := t.TempDir()
	stubs := filepath.Join(home, "stubs")
	if err := os.MkdirAll(stubs, 0o755); err != nil {
		t.Fatal(err)
	}
	stubLog := filepath.Join(home, "stub")
	write := func(name, reply, action string) string {
		p := filepath.Join(stubs, name)
		body := "#!/bin/sh\n" +
			"printf '%s\\n' \"$*\" >> \"$SYNERGY_STUB_LOG." + name + "\"\n" +
			action +
			"printf '%s\\n' '" + reply + "'\n"
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	claude := write("claude", "PLAN: create synergy_result.txt with SYNERGY_OK and verify it", "")
	codex := write("codex", "IMPLEMENTED: deterministic fixture outcome", "printf 'SYNERGY_OK\\n' > synergy_result.txt\n")
	tmp := filepath.Join(home, "tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	return synergyEnv{
		home: home, work: work, state: filepath.Join(home, "state"), stubLog: stubLog,
		vars: []string{
			"PATH=" + os.Getenv("PATH"),
			"HOME=" + home,
			"USERPROFILE=" + home,
			"TMPDIR=" + tmp,
			"BASHY_HOME=" + filepath.Join(home, "bashy-home"),
			"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
			"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
			"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
			"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
			"DAG_CACHE_DIR=" + filepath.Join(home, "dagcache"),
			"BASHY_HINTS=off",
			"SYNERGY_STUB_LOG=" + stubLog,
			"BASHY_TOOL_BINARY_CLAUDE=" + claude,
			"BASHY_TOOL_BINARY_CODEX=" + codex,
		},
	}
}

func (e synergyEnv) run(t *testing.T, bin string, timeout time.Duration, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = e.work
	cmd.Env = e.vars
	cmd.Stdin = strings.NewReader("")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("%s %s did not finish within %s (the fixture must stay bounded)\nstdout=%s\nstderr=%s",
			filepath.Base(bin), strings.Join(args, " "), timeout, out.String(), errb.String())
	}
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("%s %s: %v", bin, strings.Join(args, " "), err)
	}
	return out.String(), errb.String(), code
}

type synergyDagEnvelope struct {
	Status string `json:"status"`
	Result struct {
		Tasks []struct {
			Name     string `json:"name"`
			Status   string `json:"status"`
			ExitCode int    `json:"exit_code"`
		} `json:"tasks"`
		Failed bool `json:"failed"`
	} `json:"result"`
}

func readSynergyLedger(t *testing.T, path string) []synergyLedgerLine {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	var lines []synergyLedgerLine
	for i, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var rec synergyLedgerLine
		if err := json.Unmarshal([]byte(l), &rec); err != nil {
			t.Fatalf("ledger line %d is not JSON: %v\n%s", i+1, err, l)
		}
		lines = append(lines, rec)
	}
	return lines
}

// TestSynergyDAGCheckAndList is the syntax/graph gate: the fixture parses,
// its dependency chain is acyclic, and it exposes exactly the nine targets of
// the S12 sequence.
func TestSynergyDAGCheckAndList(t *testing.T) {
	bin, root := synergyBashyBinary(t)
	dag := filepath.Join(root, "examples", "dag", "synergy", "dag.md")
	env := newSynergyEnv(t)

	out, errOut, code := env.run(t, bin, time.Minute, "dag", "-f", dag, "--check")
	if code != 0 || !strings.Contains(out, "9 targets") {
		t.Fatalf("dag --check: exit=%d\nstdout=%s\nstderr=%s", code, out, errOut)
	}
	out, errOut, code = env.run(t, bin, time.Minute, "dag", "-f", dag, "--list")
	if code != 0 {
		t.Fatalf("dag --list: exit=%d\nstdout=%s\nstderr=%s", code, out, errOut)
	}
	for _, name := range synergyTargets {
		if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `\b`).MatchString(out) {
			t.Fatalf("dag --list lacks target %q:\n%s", name, out)
		}
	}
	plan, errOut, code := env.run(t, bin, time.Minute, "dag", "-f", dag, "--dry-run", "done-notification")
	if code != 0 {
		t.Fatalf("dag --dry-run: exit=%d\nstdout=%s\nstderr=%s", code, plan, errOut)
	}
	last := -1
	for _, name := range synergyTargets {
		idx := strings.Index(plan, " "+name+" ")
		if idx < 0 || idx < last {
			t.Fatalf("dry-run plan does not order %q after its dependency:\n%s", name, plan)
		}
		last = idx
	}
}

// TestSynergyDAGBoundedFixture runs the whole sequence once against a fresh
// HOME and checks the ledger against the receipts, then runs it again against
// the SAME home to prove the first step refuses (the sender is already
// paired) instead of passing through.
func TestSynergyDAGBoundedFixture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the planner/implementer stand-ins are POSIX sh scripts")
	}
	bin, root := synergyBashyBinary(t)
	dag := filepath.Join(root, "examples", "dag", "synergy", "dag.md")
	source, err := os.ReadFile(dag)
	if err != nil {
		t.Fatal(err)
	}
	env := newSynergyEnv(t)

	// --- source-level guards the manager asked for -------------------------
	if strings.Contains(string(source), "|| true") {
		t.Fatalf("dag.md must stay fail-closed: found `|| true`")
	}
	if strings.Contains(string(source), "todo list") {
		t.Fatalf("dag.md must never select a todo from a listing")
	}
	bodies := strings.Count(string(source), "\n```bash\n")
	strict := strings.Count(string(source), "\nset -euo pipefail\n")
	if bodies != len(synergyTargets) || strict != bodies {
		t.Fatalf("every body must open with `set -euo pipefail`: %d bodies, %d strict", bodies, strict)
	}

	// --- the run --------------------------------------------------------------
	out, errOut, code := env.run(t, bin, 3*time.Minute,
		"dag", "-f", dag, "-B", "--json", "STATE_DIR="+env.state, "done-notification")
	var envelope synergyDagEnvelope
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("dag --json envelope: %v\nexit=%d\nstdout=%s\nstderr=%s", err, code, out, errOut)
	}
	if code != 0 || envelope.Status != "ok" || envelope.Result.Failed {
		t.Fatalf("synergy DAG failed: exit=%d status=%s\nstdout=%s\nstderr=%s", code, envelope.Status, out, errOut)
	}
	if len(envelope.Result.Tasks) != len(synergyTargets) {
		t.Fatalf("ran %d targets, want %d:\n%s", len(envelope.Result.Tasks), len(synergyTargets), out)
	}
	for i, task := range envelope.Result.Tasks {
		if task.Name != synergyTargets[i] || task.Status != "done" || task.ExitCode != 0 {
			t.Fatalf("task %d = %+v, want %s done exit 0", i, task, synergyTargets[i])
		}
	}
	artifact, err := os.ReadFile(filepath.Join(env.work, "synergy_result.txt"))
	if err != nil || string(artifact) != "SYNERGY_OK\n" {
		t.Fatalf("implementer artifact = %q, want SYNERGY_OK with one newline (%v)", artifact, err)
	}

	// --- every raw outcome exists and exited 0 -------------------------------
	rawDir := filepath.Join(env.state, "raw")
	statuses, err := filepath.Glob(filepath.Join(rawDir, "*.status"))
	if err != nil || len(statuses) == 0 {
		t.Fatalf("no raw receipts under %s: %v", rawDir, err)
	}
	for _, st := range statuses {
		b, err := os.ReadFile(st)
		if err != nil || strings.TrimSpace(string(b)) != "0" {
			t.Fatalf("%s = %q, want 0 (%v)", st, b, err)
		}
		base := strings.TrimSuffix(st, ".status")
		for _, ext := range []string{".out", ".err"} {
			if _, err := os.Stat(base + ext); err != nil {
				t.Fatalf("raw outcome %s%s missing: %v", base, ext, err)
			}
		}
	}

	// --- the ledger: shape, order, and provenance of every ID ----------------
	ledger := readSynergyLedger(t, filepath.Join(env.state, "event-ids.ndjson"))
	if len(ledger) != len(synergyExpectedLedger) {
		t.Fatalf("ledger has %d lines, want %d:\n%+v", len(ledger), len(synergyExpectedLedger), ledger)
	}
	ids := map[string]string{} // "step/command" -> event_id
	for i, want := range synergyExpectedLedger {
		got := ledger[i]
		key := want.step + "/" + want.command
		if got.Step != want.step || got.Command != want.command {
			t.Fatalf("ledger[%d] = %s/%s, want %s", i, got.Step, got.Command, key)
		}
		if got.Exit != 0 {
			t.Fatalf("ledger[%d] %s exit=%d, want 0", i, key, got.Exit)
		}
		if got.IDSource == "" || got.Receipt == "" {
			t.Fatalf("ledger[%d] %s lacks id_source or receipt: %+v", i, key, got)
		}
		receiptPath := filepath.Join(env.state, filepath.FromSlash(got.Receipt))
		receipt, err := os.ReadFile(receiptPath)
		if err != nil {
			t.Fatalf("ledger[%d] %s receipt %s: %v", i, key, got.Receipt, err)
		}
		if want.nilID {
			if got.EventID != nil {
				t.Fatalf("ledger[%d] %s carries event_id %q but the current CLI prints no id there", i, key, *got.EventID)
			}
			if got.Absence == "" {
				t.Fatalf("ledger[%d] %s has a null event_id with no absence text", i, key)
			}
			// The two JSON receipts without IDs must really lack one; a CLI
			// that starts emitting an id makes this fail so the ledger is updated.
			if want.command == "chat" || want.command == "notify" {
				if strings.Contains(string(receipt), `"id"`) {
					t.Fatalf("ledger[%d] %s: receipt now carries an \"id\" field; the ledger must parse it:\n%s", i, key, receipt)
				}
			}
			continue
		}
		if got.EventID == nil || *got.EventID == "" {
			t.Fatalf("ledger[%d] %s has no event_id", i, key)
		}
		id := *got.EventID
		if got.Absence != "" {
			t.Fatalf("ledger[%d] %s has both an event_id and an absence", i, key)
		}
		// Provenance: the ID is a substring of the receipt the CLI printed...
		if !strings.Contains(string(receipt), id) {
			t.Fatalf("ledger[%d] %s event_id %q does not appear in its receipt %s:\n%s", i, key, id, got.Receipt, receipt)
		}
		// ...and is not a label that was sitting in the DAG source.
		if strings.Contains(string(source), id) {
			t.Fatalf("ledger[%d] %s event_id %q is hard-coded in dag.md", i, key, id)
		}
		ids[key] = id
	}

	// --- per-step shape of the IDs ------------------------------------------
	if code := ids["unpaired-refusal/meet dm"]; !regexp.MustCompile(`^[A-Z0-9]{4}-[A-Z0-9]{4}$`).MatchString(code) {
		t.Fatalf("pair code %q is not the app-pair single-use shape", code)
	}
	if ids["pair-approve/app pair approve"] != ids["pair-approve/app devices"] {
		t.Fatalf("device id differs between approve receipt %q and app devices %q",
			ids["pair-approve/app pair approve"], ids["pair-approve/app devices"])
	}
	if room := ids["paired-input/meet list"]; !strings.Contains(room, "-dm-") {
		t.Fatalf("room id %q is not a DM room id", room)
	}
	todoID := ids["claude-plan/todo add"]
	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(todoID) {
		t.Fatalf("todo id %q is not a todo id", todoID)
	}
	// todo start/done print the id prefix, so the ledger carries exactly that
	// prefix (parsed from the receipt) and it must belong to the created todo.
	for _, key := range []string{"codex-implementation/todo start", "done-notification/todo done"} {
		prefix := ids[key]
		if len(prefix) < 8 || !strings.HasPrefix(todoID, prefix) {
			t.Fatalf("%s acted on todo %q, want a prefix of the created todo %q", key, prefix, todoID)
		}
	}
	approvalID := ids["approval-request/approve request"]
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(approvalID) {
		t.Fatalf("approval id %q is not an approval id", approvalID)
	}
	for _, key := range []string{"approve/meet dm /approve", "approve/approve show"} {
		if ids[key] != approvalID {
			t.Fatalf("%s settled %q, want the requested approval %q", key, ids[key], approvalID)
		}
	}
	shown, err := os.ReadFile(filepath.Join(rawDir, "approve.approve-show.out"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"state":"allow"`, `"by":"fixture:alice"`, `"via":"human"`} {
		if !strings.Contains(string(shown), want) {
			t.Fatalf("approve show lacks %s:\n%s", want, shown)
		}
	}
	notify, err := os.ReadFile(filepath.Join(rawDir, "done.notify.out"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(notify), `"schema_version":"bashy-bus-v1"`) || strings.Contains(string(notify), `"state":"failed"`) {
		t.Fatalf("notify receipt is not an accepted bashy-bus-v1 receipt:\n%s", notify)
	}
	timeline, err := os.ReadFile(filepath.Join(rawDir, "done.chat-timeline.out"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"inbound", "approval:" + approvalID[:8], "#pending", "#allow", "todo:" + todoID[:8], "#done", "notify",
	} {
		if !strings.Contains(string(timeline), want) {
			t.Fatalf("host event log lacks %q:\n%s", want, timeline)
		}
	}

	// --- the stand-ins: invoked exactly where an agent turn belongs ---------
	claudeLog, err := os.ReadFile(env.stubLog + ".claude")
	if err != nil {
		t.Fatalf("the planner was never launched: %v", err)
	}
	if n := strings.Count(string(claudeLog), "\n"); n != 1 {
		t.Fatalf("planner launched %d times, want exactly 1 (the unpaired refusal and /approve must invoke no agent):\n%s", n, claudeLog)
	}
	if !strings.Contains(string(claudeLog), "Plan bounded-synergy-fixture") {
		t.Fatalf("planner did not receive the sender's message:\n%s", claudeLog)
	}
	codexLog, err := os.ReadFile(env.stubLog + ".codex")
	if err != nil {
		t.Fatalf("the implementer was never launched: %v", err)
	}
	if n := strings.Count(string(codexLog), "\n"); n != 1 {
		t.Fatalf("implementer launched %d times, want exactly 1:\n%s", n, codexLog)
	}
	if !strings.Contains(string(codexLog), "todo:"+todoID) || !strings.Contains(string(codexLog), "kb:"+ids["claude-plan/kb note add"]) {
		t.Fatalf("implementer was not addressed with the created todo and kb page:\n%s", codexLog)
	}

	// --- fail-closed: a second pass on the same host must refuse early ------
	out, errOut, code = env.run(t, bin, 3*time.Minute,
		"dag", "-f", dag, "-B", "--json", "STATE_DIR="+env.state, "done-notification")
	// A failed run's dag-v1 envelope is reported on stderr.
	payload := out
	if strings.TrimSpace(payload) == "" {
		payload = errOut
	}
	var second synergyDagEnvelope
	if err := json.Unmarshal([]byte(payload), &second); err != nil {
		t.Fatalf("second run envelope: %v\nstdout=%s\nstderr=%s", err, out, errOut)
	}
	if code == 0 || !second.Result.Failed {
		t.Fatalf("second run against a paired sender passed; the refusal step must fail closed\nstdout=%s\nstderr=%s", out, errOut)
	}
	if len(second.Result.Tasks) != 2 || second.Result.Tasks[1].Name != "unpaired-refusal" || second.Result.Tasks[1].Status != "failed" {
		t.Fatalf("second run should stop at unpaired-refusal: %+v", second.Result.Tasks)
	}
	if again := readSynergyLedger(t, filepath.Join(env.state, "event-ids.ndjson")); len(again) != 0 {
		t.Fatalf("a refused run must leave an empty ledger, got %d lines", len(again))
	}
	if b, _ := os.ReadFile(env.stubLog + ".claude"); strings.Count(string(b), "\n") != 2 {
		// The paired message reached the planner (second launch) before the
		// refusal check rejected the run — that is the real front door
		// behaviour for an already-paired sender, and exactly why the step
		// checks the receipt instead of trusting exit 0.
		t.Fatalf("planner launches after the second run = %d, want 2:\n%s", strings.Count(string(b), "\n"), b)
	}
}

// TestSynergyDAGRejectsEmptyImplementation proves that a successful agent
// process and nonempty reply cannot advance a todo without the promised file.
func TestSynergyDAGRejectsEmptyImplementation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the planner/implementer stand-ins are POSIX sh scripts")
	}
	bin, root := synergyBashyBinary(t)
	dag := filepath.Join(root, "examples", "dag", "synergy", "dag.md")
	env := newSynergyEnv(t)
	refusal := "#!/bin/sh\nprintf '%s\\n' 'cannot implement: no source material'\n"
	if err := os.WriteFile(filepath.Join(env.home, "stubs", "codex"), []byte(refusal), 0o755); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := env.run(t, bin, 3*time.Minute,
		"dag", "-f", dag, "-B", "--json", "STATE_DIR="+env.state, "done-notification")
	if code == 0 || strings.TrimSpace(out) != "" {
		t.Fatalf("nonempty refusal advanced the DAG: exit=%d stdout=%s stderr=%s", code, out, errOut)
	}
	var result synergyDagEnvelope
	if err := json.Unmarshal([]byte(errOut), &result); err != nil {
		t.Fatalf("failed DAG envelope: %v\nstderr=%s", err, errOut)
	}
	if !result.Result.Failed || len(result.Result.Tasks) != 6 {
		t.Fatalf("refusal should stop at the sixth target: %+v", result.Result.Tasks)
	}
	last := result.Result.Tasks[5]
	if last.Name != "codex-implementation" || last.Status != "failed" || last.ExitCode == 0 {
		t.Fatalf("refusal should fail codex-implementation: %+v", last)
	}
	if _, err := os.Stat(filepath.Join(env.work, "synergy_result.txt")); !os.IsNotExist(err) {
		t.Fatalf("refusal unexpectedly created the artifact: %v", err)
	}
	for _, name := range []string{"approval.approve-request.status", "done.todo-done.status", "done.notify.status"} {
		if _, err := os.Stat(filepath.Join(env.state, "raw", name)); !os.IsNotExist(err) {
			t.Fatalf("%s exists after refusal: %v", name, err)
		}
	}
	todoID, err := os.ReadFile(filepath.Join(env.state, "todo-id"))
	if err != nil {
		t.Fatal(err)
	}
	todo, todoErr, todoCode := env.run(t, bin, time.Minute, "todo", "show", strings.TrimSpace(string(todoID)), "--user", "--json")
	// todo show --json is a bashy-graph-v1 frame (Sprint 446 S19); the item's
	// status is a compact node attr.
	if todoCode != 0 || !strings.Contains(todo, `"status":"doing"`) {
		t.Fatalf("refused implementation should leave the todo doing: exit=%d stdout=%s stderr=%s", todoCode, todo, todoErr)
	}
	chat, err := os.ReadFile(filepath.Join(env.state, "raw", "impl.chat.out"))
	if err != nil || !strings.Contains(string(chat), "cannot implement") {
		t.Fatalf("nonempty implementer refusal was not recorded: %q (%v)", chat, err)
	}
}
