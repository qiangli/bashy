// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build e2e

package agentos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type kbE2EEnv struct {
	vars  []string
	kbDir string
}

func scratchKBE2EEnv(t *testing.T) kbE2EEnv {
	t.Helper()
	root := t.TempDir()
	kbDir := filepath.Join(root, "kb")
	return kbE2EEnv{
		kbDir: kbDir,
		vars: []string{
			"BASHY_KB_DIR=" + kbDir,
			"BASHY_HOME=" + filepath.Join(root, "home"),
			"BASHY_SKILLS_DIR=" + filepath.Join(root, "skills"),
			"YCODE_DATA_DIR=" + filepath.Join(root, "agent-data"),
			"BASHY_HINTS=off",
		},
	}
}

func addKBNoteE2E(t *testing.T, bin string, env kbE2EEnv, title, body string) {
	t.Helper()
	stdout, stderr, code := runBashyStdEnv(bin, env.vars, "kb", "note", "add",
		"--candidate", "--ring", "host", "--title", title, "--body", body)
	if code != 0 {
		t.Fatalf("kb note add exited %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
}

func TestE2EKBContextHonorsBudgetAndGoldenShape(t *testing.T) {
	env := scratchKBE2EEnv(t)
	bin := bashyBinary(t)
	addKBNoteE2E(t, bin, env, "context budget envelope", "context budget envelope remains bounded and parseable")

	stdout, stderr, code := runBashyStdEnv(bin, env.vars, "kb", "context",
		"--for", "context budget envelope", "--rings", "host", "--forms", "note", "--budget", "40", "--json")
	if code != 0 {
		t.Fatalf("kb context exited %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode kb context: %v\n%s", err, stdout)
	}
	assertKBContextShape(t, got)
	budget := got["budget"].(map[string]any)
	if used, limit := budget["used"].(float64), budget["limit"].(float64); limit != 40 || used > limit {
		t.Fatalf("budget = used %.0f / limit %.0f, want limit 40 and no overflow", used, limit)
	}
	if len(got["blocks"].([]any)) == 0 {
		t.Fatalf("context did not return the seeded note: %s", stdout)
	}

	root, ok := findBashySourceRoot(mustGetwd())
	if !ok {
		t.Fatal("cannot locate bashy source root")
	}
	golden, err := os.ReadFile(filepath.Join(root, "..", "coreutils", "pkg", "recall", "testdata", "kb-context-envelope.json"))
	if err != nil {
		t.Fatal(err)
	}
	var frozen map[string]any
	if err := json.Unmarshal(golden, &frozen); err != nil {
		t.Fatal(err)
	}
	assertKBContextShape(t, frozen)
	if got["context_version"] != frozen["context_version"] {
		t.Fatalf("context version %v differs from golden %v", got["context_version"], frozen["context_version"])
	}
}

func assertKBContextShape(t *testing.T, envelope map[string]any) {
	t.Helper()
	for _, key := range []string{"context_version", "for", "budget", "abstained", "rings", "blocks"} {
		if _, ok := envelope[key]; !ok {
			t.Fatalf("context envelope missing %q: %#v", key, envelope)
		}
	}
	budget, ok := envelope["budget"].(map[string]any)
	if !ok {
		t.Fatalf("context budget has wrong shape: %#v", envelope["budget"])
	}
	for _, key := range []string{"limit", "used"} {
		if _, ok := budget[key]; !ok {
			t.Fatalf("context budget missing %q: %#v", key, budget)
		}
	}
	rings, ok := envelope["rings"].([]any)
	if !ok || len(rings) == 0 {
		t.Fatalf("context rings have wrong shape: %#v", envelope["rings"])
	}
	for _, key := range []string{"name", "ok"} {
		if _, ok := rings[0].(map[string]any)[key]; !ok {
			t.Fatalf("context ring missing %q: %#v", key, rings[0])
		}
	}
	blocks, ok := envelope["blocks"].([]any)
	if !ok || len(blocks) == 0 {
		t.Fatalf("context blocks have wrong shape: %#v", envelope["blocks"])
	}
	for _, key := range []string{"ring", "form", "ref", "tokens", "text"} {
		if _, ok := blocks[0].(map[string]any)[key]; !ok {
			t.Fatalf("context block missing %q: %#v", key, blocks[0])
		}
	}
}

func TestE2EKBDoctorFlagsAndNeverFixes(t *testing.T) {
	env := scratchKBE2EEnv(t)
	bin := bashyBinary(t)
	addKBNoteE2E(t, bin, env, "doctor leaves bytes", "an orphan note for the doctor")
	page := filepath.Join(env.kbDir, "pages", "doctor-leaves-bytes.md")
	before, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}

	help, _, code := runBashyStdEnv(bin, env.vars, "kb", "doctor", "--help")
	if code != 0 || strings.Contains(help, "--fix") || !strings.Contains(help, "never 'kb fix'") {
		t.Fatalf("doctor help must promise flag-only behavior (exit %d): %s", code, help)
	}
	stdout, stderr, code := runBashyStdEnv(bin, env.vars, "kb", "doctor", "--ring", "host", "--json")
	if code != 0 || !strings.Contains(stdout, `"orphans"`) {
		t.Fatalf("kb doctor exited %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	after, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("kb doctor rewrote a page instead of only flagging it")
	}
}

func TestE2EKBNoteAddCreatesCandidate(t *testing.T) {
	env := scratchKBE2EEnv(t)
	bin := bashyBinary(t)
	addKBNoteE2E(t, bin, env, "candidate stage note", "candidate persistence evidence")
	b, err := os.ReadFile(filepath.Join(env.kbDir, "pages", "candidate-stage-note.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{"form: note", "status: candidate"} {
		if !strings.Contains(text, want) {
			t.Fatalf("candidate note missing %q:\n%s", want, text)
		}
	}
}

func TestE2EKBValidateFromGateRefusesMissingEvent(t *testing.T) {
	env := scratchKBE2EEnv(t)
	bin := bashyBinary(t)
	addKBNoteE2E(t, bin, env, "gate refusal note", "must stay a candidate")
	page := filepath.Join(env.kbDir, "pages", "gate-refusal-note.md")
	before, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runBashyStdEnv(bin, env.vars, "kb", "validate", "gate-refusal-note",
		"--ring", "host", "--from-gate", "missing-event")
	if code == 0 || !strings.Contains(stderr, "no such observe event") {
		t.Fatalf("validate --from-gate missing event = exit %d, stderr %q", code, stderr)
	}
	after, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || !strings.Contains(string(after), "status: candidate") {
		t.Fatal("refused gate validation mutated or promoted the candidate")
	}
}
