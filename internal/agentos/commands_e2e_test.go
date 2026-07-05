// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build e2e

// End-to-end: build the real bashy binary and confirm every command that
// `bashy commands` advertises actually dispatches — no "No such command" / "not in
// this build" / "No such file". Runs identically on macOS and Windows via
//
//	go test -tags e2e ./internal/agentos/
package agentos

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// unsupportedSignals are substrings that mean a command did NOT dispatch to a real
// handler (the failure modes this goal forbids). Checked on verb `--help` output
// (cobra help is clean, so any of these means a broken/absent handler) and on the
// coreutils execution smoke.
var unsupportedSignals = []string{
	"no such command",           // bashy tool dispatch, unregistered
	"not in this build",         // engine stub, old behavior
	"rebuild with -tags",        // engine stub, old behavior
	"no such file or directory", // verb fell through to exec its own name (the docker bug)
	"command not found",         // verb/tool fell through to PATH and wasn't found
}

var (
	buildOnce sync.Once
	builtBin  string
	buildErr  error
)

func bashyBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		// Escape hatch: a prebuilt binary (e.g. cross-compiled bashy.exe shipped
		// to a Windows host that has no Go toolchain/source tree). Lets the same
		// dispatch e2e run on macOS and Windows identically.
		if p := strings.TrimSpace(os.Getenv("BASHY_E2E_BIN")); p != "" {
			builtBin = p
			return
		}
		root, ok := findBashySourceRoot(mustGetwd())
		if !ok {
			buildErr = os.ErrNotExist
			return
		}
		out := filepath.Join(os.TempDir(), "bashy-e2e-cmds")
		if runtime.GOOS == "windows" {
			out += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", out, "./cmd/bashy")
		cmd.Dir = root
		if b, err := cmd.CombinedOutput(); err != nil {
			buildErr = err
			t.Logf("build output:\n%s", b)
			return
		}
		builtBin = out
	})
	if buildErr != nil {
		t.Fatalf("build bashy: %v", buildErr)
	}
	return builtBin
}

func runBashy(bin string, args ...string) (string, int) {
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "BASHY_AGENTIC=1")
	cmd.Stdin = strings.NewReader("") // empty stdin so stdin-reading tools never hang
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	}
	return string(out), code
}

func unsupportedSignal(out string) string {
	lo := strings.ToLower(out)
	for _, s := range unsupportedSignals {
		if strings.Contains(lo, s) {
			return s
		}
	}
	return ""
}

func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func featureAvailable(bin, name string) (bool, string) {
	o, _ := runBashy(bin, "commands", name, "--features", "--json")
	var info map[string]any
	if err := json.Unmarshal([]byte(o), &info); err != nil {
		return false, "no feature report"
	}
	class, _ := info["class"].(string)
	return info["available"] == true && class != "not-found", class
}

// TestE2EDoctor is the first e2e: build the real binary and run `bashy doctor`,
// the environment self-diagnostic. It must emit its JSON envelope, sweep the
// coreutils userland + every managed external, and exit 0 (warnings are allowed —
// doctor is advisory). This is the fast canary before the fuller dispatch sweep.
func TestE2EDoctor(t *testing.T) {
	bin := bashyBinary(t)

	// JSON envelope: schema + a non-empty check list including the tool-surface sweep.
	out, code := runBashy(bin, "doctor", "--json")
	if code != 0 {
		t.Fatalf("`bashy doctor --json` exited %d:\n%s", code, out)
	}
	var env struct {
		Schema string `json:"schema_version"`
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode doctor json: %v\n%s", err, out)
	}
	if env.Schema != "bashy-doctor-v1" {
		t.Errorf("doctor schema_version = %q, want bashy-doctor-v1", env.Schema)
	}
	if len(env.Checks) == 0 {
		t.Fatal("doctor emitted no checks")
	}

	byName := map[string]string{} // name -> status
	for _, c := range env.Checks {
		byName[c.Name] = c.Status
		if c.Status != "ok" && c.Status != "warn" && c.Status != "info" {
			t.Errorf("doctor check %q has invalid status %q", c.Name, c.Status)
		}
	}
	// The command-surface sweep must be present and healthy.
	if s, ok := byName["coreutils userland"]; !ok {
		t.Error("doctor is missing the 'coreutils userland' check")
	} else if s != "ok" {
		t.Errorf("'coreutils userland' status = %q, want ok", s)
	}
	if _, ok := byName["front-door verbs"]; !ok {
		t.Error("doctor is missing the 'front-door verbs' check")
	}
	// Every managed external must be reported (each 'ext: <name>').
	for _, ext := range []string{"podman", "ollama", "gh", "loom", "go"} {
		if _, ok := byName["ext: "+ext]; !ok {
			t.Errorf("doctor is missing the 'ext: %s' check", ext)
		}
	}

	// Plain (human) form must also render and exit 0.
	plain, code := runBashy(bin, "doctor")
	if code != 0 {
		t.Fatalf("`bashy doctor` exited %d:\n%s", code, plain)
	}
	if !strings.Contains(plain, "coreutils userland") || !strings.Contains(plain, "ext: ollama") {
		t.Errorf("plain doctor output missing tool-surface sweep:\n%s", plain)
	}
}

// TestE2EAllListedCommandsDispatch runs the real binary and checks every command
// `bashy commands` prints:
//   - coreutils tools: recognized+available per the binary (all of them), plus a
//     live execution smoke proves the in-process userland actually runs;
//   - native + engine front-door verbs: really invoked (`--help`), asserting no
//     unsupported signal (the docker/podman regression class);
//   - download/passthrough verbs: dispatch recognition (so we don't pull hundreds
//     of MB of upstream tools).
func TestE2EAllListedCommandsDispatch(t *testing.T) {
	bin := bashyBinary(t)

	out, code := runBashy(bin, "commands", "--json")
	if code != 0 {
		t.Fatalf("`bashy commands --json` exited %d:\n%s", code, out)
	}
	var cat struct {
		Builtins  []string `json:"builtins"`
		Coreutils []string `json:"coreutils"`
		Verbs     []string `json:"verbs"`
	}
	if err := json.Unmarshal([]byte(out), &cat); err != nil {
		t.Fatalf("decode commands json: %v\n%s", err, out)
	}
	if len(cat.Coreutils) == 0 || len(cat.Verbs) == 0 {
		t.Fatalf("empty catalog: %d coreutils, %d verbs", len(cat.Coreutils), len(cat.Verbs))
	}

	// Builtins: a sample resolves as a builtin.
	for _, b := range []string{"cd", "export", "echo", "read"} {
		if !contains(cat.Builtins, b) {
			continue
		}
		if o, _ := runBashy(bin, "-c", "type -t "+b); strings.TrimSpace(o) != "builtin" {
			t.Errorf("builtin %q did not resolve as a builtin: %q", b, strings.TrimSpace(o))
		}
	}

	// coreutils: every listed tool is recognized+available per the binary. (--help
	// is not universal across tools, and stdin-readers would hang, so per-tool we
	// check dispatch recognition, not execution.)
	for _, tool := range cat.Coreutils {
		if ok, class := featureAvailable(bin, tool); !ok {
			t.Errorf("coreutils %q is listed but not available per dispatch (class=%s)", tool, class)
		}
	}
	// ...and a live smoke proves the in-process userland actually runs.
	smoke, code := runBashy(bin, "-c", `printf 'a\nb\n' | grep b | tr a-z A-Z | wc -l`)
	if s := unsupportedSignal(smoke); s != "" || code != 0 {
		t.Errorf("coreutils execution smoke failed (%q, exit %d): %s", s, code, firstLineOf(smoke))
	}

	// Native front-door verbs (safe, side-effect-free `--help`) + engine verbs
	// (docker/podman/ollama — the regression class) are really invoked.
	native := set("weave", "sprint", "chat", "agent", "sdlc", "web", "dag",
		"schedule", "secrets", "skills", "run", "commands", "context", "doctor",
		"self", "check", "verify", "git")
	engine := set("podman", "ollama", "docker")

	for _, v := range cat.Verbs {
		switch {
		case engine[v], native[v]:
			o, _ := runBashy(bin, v, "--help")
			if s := unsupportedSignal(o); s != "" {
				t.Errorf("verb %q is listed but unsupported (%q): %s", v, s, firstLineOf(o))
			}
		default:
			// Download/passthrough verb (gh/act/rclone/loom/zot/seaweedfs/kopia/
			// mirror/go/cmake/clang): confirm dispatch recognition without pulling
			// the upstream tool.
			if ok, class := featureAvailable(bin, v); !ok {
				t.Errorf("verb %q is listed but dispatch does not recognize it (class=%s)", v, class)
			}
		}
	}
}

func set(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// runBashyStd is runBashy with stdout and stderr separated — needed by the
// skills show byte-compat contract (content on stdout, verdict on stderr).
func runBashyStd(bin string, args ...string) (stdout, stderr string, code int) {
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "BASHY_AGENTIC=1")
	cmd.Stdin = strings.NewReader("")
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	}
	return out.String(), errb.String(), code
}

// TestSkillsE2E drives the env-gated skills catalog end to end: list shows
// the embedded conductor (ungated → applicable everywhere), probe emits a
// parseable coordinate, and show keeps skill content on stdout with the
// verdict annotation on stderr.
func TestSkillsE2E(t *testing.T) {
	bin := bashyBinary(t)

	stdout, _, code := runBashyStd(bin, "skills", "list")
	if code != 0 || !contains(strings.Fields(stdout), "conductor") {
		t.Fatalf("skills list (exit %d):\n%s", code, stdout)
	}

	stdout, _, code = runBashyStd(bin, "skills", "probe", "--json")
	if code != 0 {
		t.Fatalf("skills probe --json exit %d:\n%s", code, stdout)
	}
	var probe struct {
		Probes     map[string]string `json:"probes"`
		ContextKey string            `json:"context_key"`
	}
	if err := json.Unmarshal([]byte(stdout), &probe); err != nil {
		t.Fatalf("probe json: %v\n%s", err, stdout)
	}
	if probe.Probes["os"] == "" || probe.Probes["arch"] == "" || !strings.HasPrefix(probe.ContextKey, "c") {
		t.Fatalf("probe = %+v", probe)
	}

	stdout, stderr, code := runBashyStd(bin, "skills", "show", "conductor")
	if code != 0 || !strings.HasPrefix(stdout, "---\n") || strings.Contains(stdout, "ring=") {
		t.Fatalf("skills show stdout not byte-clean (exit %d): %.120q", code, stdout)
	}
	if !strings.Contains(stderr, "ring=embedded") {
		t.Fatalf("skills show stderr missing verdict: %q", stderr)
	}

	if _, _, code := runBashyStd(bin, "skills", "show", "no-such-skill"); code == 0 {
		t.Fatal("skills show no-such-skill exited 0")
	}
}

// TestSkillsAddVerifyE2E drives the P1 verified-admission loop end to end
// against an isolated store ($BASHY_SKILLS_DIR): author a dual-bundle
// skill, add it, see it env-gated in list, verify it, and confirm the
// admission gate refuses a broken canonical face.
func TestSkillsAddVerifyE2E(t *testing.T) {
	bin := bashyBinary(t)
	store := t.TempDir()
	env := "BASHY_SKILLS_DIR=" + store

	run := func(args ...string) (string, string, int) {
		cmd := exec.Command(bin, args...)
		cmd.Env = append(os.Environ(), "BASHY_AGENTIC=1", env)
		cmd.Stdin = strings.NewReader("")
		var out, errb strings.Builder
		cmd.Stdout, cmd.Stderr = &out, &errb
		err := cmd.Run()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		return out.String(), errb.String(), code
	}

	src := t.TempDir()
	dir := filepath.Join(src, "port-check")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	frontmatter := "---\nname: port-check\ndescription: example dual-bundle skill\nmetadata:\n  requires: \"os=linux,darwin,windows\"\n  check-tests: \"true\"\n---\n# port-check\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(frontmatter), 0o644); err != nil {
		t.Fatal(err)
	}
	canon := "sokilili demo efefecato reada wurite fini enisure gereeni fini fini\n"
	if err := os.WriteFile(filepath.Join(dir, "skill.dhnt"), []byte(canon), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, code := run("skills", "add", dir)
	if code != 0 || !strings.Contains(stdout, "identity: h") {
		t.Fatalf("add (exit %d):\n%s", code, stdout)
	}

	stdout, _, code = run("skills", "list")
	if code != 0 || !contains(strings.Fields(stdout), "port-check") {
		t.Fatalf("list after add (exit %d):\n%s", code, stdout)
	}

	stdout, _, code = run("skills", "verify", "port-check")
	if code != 0 || !strings.Contains(stdout, "valid: true") {
		t.Fatalf("verify (exit %d):\n%s", code, stdout)
	}

	// P2: run the skill — contract binds to metadata check-tests, the
	// attestation is stored in the isolated store, exit 0 iff Valid.
	stdout, _, code = run("skills", "run", "port-check")
	if code != 0 || !strings.Contains(stdout, "valid: true") || !strings.Contains(stdout, "passed: gereeni") {
		t.Fatalf("run (exit %d):\n%s", code, stdout)
	}
	if _, err := os.Stat(filepath.Join(store, "attest", "port-check.jsonl")); err != nil {
		t.Fatalf("attestation not stored: %v", err)
	}

	// Broken canonical face: loud refusal, nothing installed.
	bad := filepath.Join(src, "bad-face")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "SKILL.md"), []byte("---\nname: bad-face\ndescription: x\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "skill.dhnt"), []byte("NOT canonical\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, code := run("skills", "add", bad); code == 0 {
		t.Fatal("bad-face admitted")
	}
	if _, err := os.Stat(filepath.Join(store, "bad-face")); err == nil {
		t.Fatal("bad-face installed despite gate failure")
	}
}

// TestSkillsAdaptE2E drives the P3 contribution loop through the real
// binary with a fake script repair agent: baseline fails, the agent
// proposes a fixing step, the fix is verified/folded/saved, and a plain
// re-run heals via the overlay after drift.
func TestSkillsAdaptE2E(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake repair agent is a shell script")
	}
	bin := bashyBinary(t)
	store, work, src := t.TempDir(), t.TempDir(), t.TempDir()
	flag := filepath.Join(work, "flag.txt")

	dir := filepath.Join(src, "demo-heal")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fm := "---\nname: demo-heal\ndescription: self-healing example\nmetadata:\n  check-tests: \"test -f " + flag + "\"\n---\n# demo-heal\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(fm), 0o644); err != nil {
		t.Fatal(err)
	}
	canon := "sokilili demo efefecato reada wurite fini enisure gereeni fini fini\n"
	if err := os.WriteFile(filepath.Join(dir, "skill.dhnt"), []byte(canon), 0o644); err != nil {
		t.Fatal(err)
	}

	agent := filepath.Join(src, "fake-agent.sh")
	reply := "<dhnt>sokilili demo efefecato reada wurite fini sotepo one faceto fini enisure gereeni fini fini</dhnt>\n" +
		"<meta>\nstep-faceto: echo healed > " + flag + "\n</meta>\n"
	if err := os.WriteFile(agent, []byte("#!/bin/sh\nprintf '%s' '"+strings.ReplaceAll(reply, "'", "'\\''")+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) (string, string, int) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(), "BASHY_AGENTIC=1", "BASHY_SKILLS_DIR="+store)
		cmd.Stdin = strings.NewReader("")
		var out, errb strings.Builder
		cmd.Stdout, cmd.Stderr = &out, &errb
		err := cmd.Run()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		return out.String(), errb.String(), code
	}

	if _, _, code := run("skills", "add", dir); code != 0 {
		t.Fatal("add failed")
	}
	stdout, stderr, code := run("skills", "run", "demo-heal", "--adapt", "--repair-agent", agent)
	if code != 0 || !strings.Contains(stdout, "outcome: repaired") {
		t.Fatalf("adapt (exit %d):\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	// Drift: remove the artifact; a PLAIN run heals via the overlay.
	if err := os.Remove(flag); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = run("skills", "run", "demo-heal")
	if code != 0 || !strings.Contains(stdout, "valid: true") || !strings.Contains(stderr, "learned version") {
		t.Fatalf("overlay rerun (exit %d):\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	// Promote renders the review bundle from the learned version.
	out := filepath.Join(work, "bundle")
	if stdout, _, code = run("skills", "promote", "demo-heal", "--out", out); code != 0 {
		t.Fatalf("promote (exit %d): %s", code, stdout)
	}
	promo, err := os.ReadFile(filepath.Join(out, "PROMOTION.md"))
	if err != nil || !strings.Contains(string(promo), "preserved from base: **true**") {
		t.Fatalf("PROMOTION.md: %v", err)
	}
}

// TestSkillsStandaloneSurfacesE2E: the standalone-first surfaces — the
// embedded reference dual-bundle skill is env-gated + runnable, a shared
// catalog dir ($BASHY_SKILLS_PATH — a cloned skills repo) is a read-only
// ring, and `bashy context` advertises applicable skills first-hop.
func TestSkillsStandaloneSurfacesE2E(t *testing.T) {
	bin := bashyBinary(t)
	store, shared := t.TempDir(), t.TempDir()

	sdir := filepath.Join(shared, "team-tip")
	if err := os.MkdirAll(sdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdir, "SKILL.md"),
		[]byte("---\nname: team-tip\ndescription: from the shared catalog\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) (string, int) {
		cmd := exec.Command(bin, args...)
		cmd.Env = append(os.Environ(), "BASHY_AGENTIC=1",
			"BASHY_SKILLS_DIR="+store, "BASHY_SKILLS_PATH="+shared)
		cmd.Stdin = strings.NewReader("")
		out, err := cmd.CombinedOutput()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		return string(out), code
	}

	// go-repo-health: embedded, gated on has=go (present here — the e2e
	// builds with go), dual bundle.
	out, code := run("skills", "list", "--json", "--all")
	if code != 0 {
		t.Fatalf("list: %s", out)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	found := map[string]map[string]any{}
	for _, r := range rows {
		found[r["name"].(string)] = r
	}
	if r := found["go-repo-health"]; r == nil || r["applicable"] != true || r["identity"] == nil {
		t.Fatalf("go-repo-health row: %+v", found["go-repo-health"])
	}
	if r := found["team-tip"]; r == nil || r["ring"] != "shared" {
		t.Fatalf("team-tip row: %+v", found["team-tip"])
	}

	// context advertises applicable skills (L1 progressive disclosure).
	out, code = run("context", "--json")
	if code != 0 {
		t.Fatalf("context: %s", out)
	}
	var report struct {
		Skills []struct {
			Name     string `json:"name"`
			Verified bool   `json:"verified"`
			Ring     string `json:"ring"`
		} `json:"skills"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("context json: %v", err)
	}
	names := map[string]bool{}
	verified := map[string]bool{}
	for _, s := range report.Skills {
		names[s.Name] = true
		verified[s.Name] = s.Verified
	}
	if !names["conductor"] || !names["go-repo-health"] || !names["team-tip"] || !names["bashy"] {
		t.Fatalf("context skills: %+v", report.Skills)
	}
	if !verified["go-repo-health"] || verified["team-tip"] {
		t.Fatalf("verified flags: %+v", report.Skills)
	}
}

// TestSkillsAdvertisementLadderE2E drives the ladder through the real
// binary: the L1 once-per-repo hint (agent detected + agent-naive repo),
// its suppression in configured repos, the L2 export verb with the
// ownership marker, and the shell→agent manifest env from the preamble.
func TestSkillsAdvertisementLadderE2E(t *testing.T) {
	bin := bashyBinary(t)
	store := t.TempDir()

	// Neutralize every agent marker inherited from THIS harness (the e2e
	// itself may run under an agentic tool), then add back per case.
	neutral := []string{
		"CLAUDECODE=", "CLAUDE_CODE_ENTRYPOINT=", "CODEX_SANDBOX=", "CODEX_THREAD_ID=",
		"GEMINI_CLI=", "CURSOR_AGENT=", "CURSOR_TRACE_ID=", "GOOSE_TERMINAL=",
		"OPENCODE_CLIENT=", "CLINE_ACTIVE=", "AGENT=", "AI_AGENT=",
	}
	run := func(dir string, extraEnv []string, args ...string) (string, string, int) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		env := append(os.Environ(), neutral...)
		cmd.Env = append(append(env, "BASHY_SKILLS_DIR="+store), extraEnv...)
		cmd.Stdin = strings.NewReader("")
		var out, errb strings.Builder
		cmd.Stdout, cmd.Stderr = &out, &errb
		err := cmd.Run()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		return out.String(), errb.String(), code
	}

	// L1: agent-naive repo → one hint, then silence.
	naive := t.TempDir()
	if err := exec.Command("git", "-C", naive, "init", "-q").Run(); err != nil {
		t.Skip("git unavailable")
	}
	agentEnv := []string{"CLAUDECODE=1"}
	_, stderr, _ := run(naive, agentEnv, "skills", "list")
	if !strings.Contains(stderr, "bashy skills show bashy") {
		t.Fatalf("no L1 hint:\n%s", stderr)
	}
	_, stderr, _ = run(naive, agentEnv, "skills", "list")
	if strings.Contains(stderr, "bashy skills show bashy") {
		t.Fatalf("hint repeated:\n%s", stderr)
	}

	// Configured repo → never hints.
	configured := t.TempDir()
	if err := exec.Command("git", "-C", configured, "init", "-q").Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configured, "AGENTS.md"), []byte("# agents\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, _ = run(configured, agentEnv, "skills", "list")
	if strings.Contains(stderr, "bashy skills show bashy") {
		t.Fatalf("hinted in a configured repo:\n%s", stderr)
	}

	// No agent driving → never hints.
	quiet := t.TempDir()
	if err := exec.Command("git", "-C", quiet, "init", "-q").Run(); err != nil {
		t.Fatal(err)
	}
	_, stderr, _ = run(quiet, nil, "skills", "list")
	if strings.Contains(stderr, "bashy skills show bashy") {
		t.Fatalf("hinted without an agent:\n%s", stderr)
	}

	// L2: export the bashy skill to a directory, with ownership marker.
	out := filepath.Join(t.TempDir(), "skills")
	stdout, _, code := run(t.TempDir(), nil, "skills", "export", "bashy", "--to", out)
	if code != 0 || !strings.Contains(stdout, "exported: ") {
		t.Fatalf("export (exit %d): %s", code, stdout)
	}
	for _, f := range []string{"SKILL.md", ".bashy-export.json"} {
		if _, err := os.Stat(filepath.Join(out, "bashy", f)); err != nil {
			t.Fatalf("export missing %s", f)
		}
	}

	// L3 seed: the preamble exports the shell→agent manifest.
	stdout, _, code = run(t.TempDir(), nil, "-c", `printf %s "$BASHY_AGENT_MANIFEST"`)
	if code != 0 || !strings.Contains(stdout, "first-hop") {
		t.Fatalf("manifest (exit %d): %q", code, stdout)
	}
}
