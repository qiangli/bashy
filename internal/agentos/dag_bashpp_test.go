// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coreskills "github.com/qiangli/yoke/pkg/skills"
)

// Sprint 216, Story 541: a ```bashpp dag body runs through the same runner
// wiring as `bashy --bashsharp FILE`. Weave run 36 found the gap: `@guard`
// on an agentic function inside a dag body printed BASHPP-EDECO-UNDEF, the
// call failed with status 1, and the target still exited 0. These tests
// drive the exact dispatch shape bashy ships (runDagDispatch) and derive
// their expectations from the runtime features' own established fixtures:
// the Sprint 203 contract fixture (test/contracts/agentic-boundary.bpp and
// its pinned transcript) and the B18 attestation ledger read path.

// dagEnvelope is the slice of the dag-v1 envelope these tests read.
type dagEnvelope struct {
	Status string `json:"status"`
	Result struct {
		Tasks []struct {
			Name     string `json:"name"`
			Status   string `json:"status"`
			ExitCode int    `json:"exit_code"`
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
		} `json:"tasks"`
	} `json:"result"`
}

// runDagBody writes a one-target dag file whose body is `lang`-fenced and
// runs it through the shipped dispatcher in --json mode, returning the
// process-level exit code and the target's captured result.
func runDagBody(t *testing.T, lang, effects, body string, extra ...string) (int, dagEnvelope) {
	t.Helper()
	t.Setenv("BASHY_HINTS", "off")
	t.Setenv("BASHY_AUDIT", "0")
	t.Setenv("BASHY_ADVISOR", "0")
	dir := t.TempDir()
	var md strings.Builder
	md.WriteString("## Tasks\n\n### smoke\nSprint 216 story 541 body.\n")
	if effects != "" {
		md.WriteString("Effects: " + effects + "\n")
	}
	md.WriteString("\n```" + lang + "\n" + body + "\n```\n")
	path := filepath.Join(dir, "dag.md")
	if err := os.WriteFile(path, []byte(md.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	args := append([]string{"--file", path, "--json", "--no-journal", "--cache-dir", filepath.Join(dir, "cache")}, extra...)
	args = append(args, "smoke")
	var out, errOut bytes.Buffer
	code := runDagDispatch(args, &out, &errOut)
	var env dagEnvelope
	raw := out.Bytes()
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = errOut.Bytes() // the error envelope goes to stderr
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("no dag envelope (exit %d): %v\nstdout=%s\nstderr=%s", code, err, out.String(), errOut.String())
	}
	if len(env.Result.Tasks) != 1 || env.Result.Tasks[0].Name != "smoke" {
		t.Fatalf("envelope did not report the smoke target: %s", raw)
	}
	return code, env
}

// The Sprint 203 fixture, byte-for-byte, as a ```bashpp dag body. Its pinned
// transcript is the expectation, with exactly one dag-specific difference: a
// guard-denied write is refused by dag's OUTERMOST effect-cap handler, which
// reports the denial and exits 126 (the B17 contract), where the cold CLI's
// audit handler denies silently with 1. Everything else — require → body →
// ensure order, exit 3 naming the clause, the yield's untouched 6 with no
// ensure run, the typed-function RESULT binding — must read identically.
func TestDagBashPPBodyRunsContractFixture(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "test", "contracts", "agentic-boundary.bpp"))
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(filepath.Join("..", "..", "test", "contracts", "agentic-boundary.expected"))
	if err != nil {
		t.Fatal(err)
	}
	store := t.TempDir()
	t.Setenv("BASHY_SKILLS_DIR", store)

	code, env := runDagBody(t, "bashpp", "", string(src))
	task := env.Result.Tasks[0]
	if code != 0 || task.Status != "done" || task.ExitCode != 0 {
		t.Fatalf("fixture body: exit %d task %s/%d\nstdout=%s\nstderr=%s", code, task.Status, task.ExitCode, task.Stdout, task.Stderr)
	}
	if strings.Contains(task.Stderr, "EDECO") {
		t.Fatalf("a decorator did not resolve inside the dag body:\n%s", task.Stderr)
	}

	// The transcript interleaves stdout and stderr; split it by which
	// stream each line is known to go to (clause failures are stderr).
	var wantOut, wantErr []string
	for _, line := range strings.Split(strings.TrimRight(string(expected), "\n"), "\n") {
		switch {
		case strings.Contains(line, "precondition failed"), strings.Contains(line, "postcondition failed"):
			wantErr = append(wantErr, line)
		case line == "write   -> 1":
			wantOut = append(wantOut, "write   -> 126")
		default:
			wantOut = append(wantOut, line)
		}
	}
	if got := strings.TrimRight(task.Stdout, "\n"); got != strings.Join(wantOut, "\n") {
		t.Fatalf("stdout diverged from the pinned transcript:\n--- got\n%s\n--- want\n%s", got, strings.Join(wantOut, "\n"))
	}
	gotErr := strings.Split(strings.TrimRight(task.Stderr, "\n"), "\n")
	var clauseLines []string
	denied := 0
	for _, line := range gotErr {
		switch {
		case strings.Contains(line, "condition failed"):
			clauseLines = append(clauseLines, line)
		case strings.Contains(line, `effect cap denied "touch"`):
			denied++
		default:
			t.Errorf("unexpected stderr line: %q", line)
		}
	}
	if strings.Join(clauseLines, "\n") != strings.Join(wantErr, "\n") {
		t.Fatalf("clause diagnostics diverged:\n--- got\n%s\n--- want\n%s", strings.Join(clauseLines, "\n"), strings.Join(wantErr, "\n"))
	}
	if denied != 1 {
		t.Fatalf("the guard denial was reported %d times, want 1:\n%s", denied, task.Stderr)
	}

	// B18: every call attested into the existing ledger, read back through
	// the existing read path — same statuses TestAttestFixtureLedger pins,
	// with the guard denial carrying dag's 126.
	got := readAttest(t, store, "summarize")
	wantStatus := []int{0, 3, 3, 126, 1, coreskills.AttestYield}
	if len(got) != len(wantStatus) {
		t.Fatalf("summarize receipts = %d, want %d: %+v", len(got), len(wantStatus), got)
	}
	for i, w := range wantStatus {
		if s := statusOf(t, got[i]); s != w {
			t.Errorf("summarize call %d: status %d, want %d", i, s, w)
		}
	}
	if !got[5].Yielded() || got[5].Valid {
		t.Fatalf("the yield read back as a completion: %+v", got[5])
	}
	if r := readAttest(t, store, "twice"); len(r) != 2 || !r[0].Valid || r[1].Valid {
		t.Fatalf("typed-function receipts = %+v", r)
	}
}

// The exit-6 handoff at the target boundary: an agentic function that
// yields for missing input propagates 6 through the body and out of the
// target (the envelope carries it, the target is not "done"); the same
// graph re-run with the answer supplied explicitly as a make-style override
// completes, and @ensure judges the completed call.
func TestDagBashPPYieldPropagatesThenResumes(t *testing.T) {
	store := t.TempDir()
	t.Setenv("BASHY_SKILLS_DIR", store)
	body := `@require('test -n "$1"')
@ensure('test -n "$answer_seen"')
agentic function need() {
	[ -n "${ANSWER-}" ] || return 6
	answer_seen=$ANSWER
	echo "resumed with $ANSWER"
}
agentic { need q; }`

	code, env := runDagBody(t, "bashpp", "read", body)
	task := env.Result.Tasks[0]
	if code == 0 || task.Status == "done" || task.ExitCode != 6 {
		t.Fatalf("yield did not propagate: exit %d task %s/%d\nstdout=%s\nstderr=%s", code, task.Status, task.ExitCode, task.Stdout, task.Stderr)
	}
	if strings.Contains(task.Stderr, "postcondition") {
		t.Fatalf("a yield must not be judged by @ensure:\n%s", task.Stderr)
	}

	code, env = runDagBody(t, "bashpp", "read", body, "ANSWER=42")
	task = env.Result.Tasks[0]
	if code != 0 || task.Status != "done" || task.ExitCode != 0 || !strings.Contains(task.Stdout, "resumed with 42") {
		t.Fatalf("resume with the supplied answer failed: exit %d task %s/%d\nstdout=%s\nstderr=%s", code, task.Status, task.ExitCode, task.Stdout, task.Stderr)
	}

	got := readAttest(t, store, "need")
	if len(got) != 2 || statusOf(t, got[0]) != coreskills.AttestYield || !got[0].Yielded() || statusOf(t, got[1]) != 0 || !got[1].Valid {
		t.Fatalf("ledger should read one handoff then one completion: %+v", got)
	}
}

// B17 is not displaced by the re-wire: a target's declared Effects: cap is
// still dag's outermost handler in a ```bashpp body. Since Sprint 230 the cap
// is advisory — the undeclared effect is reported once on stderr and the
// command runs — while a @guard inside the body keeps denying (126).
func TestDagBashPPEffectsCapStillOutermost(t *testing.T) {
	leak := filepath.Join(t.TempDir(), "leak")
	code, env := runDagBody(t, "bashpp", "read", "touch "+leak+"; echo \"touch -> $?\"")
	task := env.Result.Tasks[0]
	if code != 0 || !strings.Contains(task.Stdout, "touch -> 0") {
		t.Fatalf("an advisory Effects: cap must not fail the command: exit %d\nstdout=%s\nstderr=%s", code, task.Stdout, task.Stderr)
	}
	if !strings.Contains(task.Stderr, `effect cap: target "smoke": "touch" needs write not in Effects: read`) {
		t.Fatalf("dag's report is missing:\n%s", task.Stderr)
	}
	if _, err := os.Stat(leak); err != nil {
		t.Fatal("the reported write did not happen")
	}
}

// Bash OFF is unchanged: a Classic (```bash) body still runs through yoke's
// own interpreter — decorator syntax is not Bash and does not run, and no
// function in it ever attests.
func TestDagClassicBodyUnchanged(t *testing.T) {
	store := t.TempDir()
	t.Setenv("BASHY_SKILLS_DIR", store)

	code, env := runDagBody(t, "bash", "read", "@guard(effects: \"read\")\nfunction f() { echo in-f; }\nf")
	task := env.Result.Tasks[0]
	if code == 0 || task.Status == "done" {
		t.Fatalf("decorator syntax must not run under Bash OFF: exit %d\nstdout=%s\nstderr=%s", code, task.Stdout, task.Stderr)
	}
	if strings.Contains(task.Stdout, "in-f") {
		t.Fatalf("a Classic body ran a decorated function:\n%s", task.Stdout)
	}

	code, env = runDagBody(t, "bash", "read", "agentic() { echo plain-$1; }\nagentic function\nf() { echo in-f; }\nf")
	task = env.Result.Tasks[0]
	if code != 0 || !strings.Contains(task.Stdout, "plain-function") || !strings.Contains(task.Stdout, "in-f") {
		t.Fatalf("Classic body changed: exit %d\nstdout=%s\nstderr=%s", code, task.Stdout, task.Stderr)
	}
	if _, err := os.Stat(filepath.Join(store, "attest")); !os.IsNotExist(err) {
		t.Fatalf("a Classic body left a ledger: %v", err)
	}
}
